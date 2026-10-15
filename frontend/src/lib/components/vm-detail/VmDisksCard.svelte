<script>
  // Disks section extracted from routes/VmDetail.svelte — markup, texts
  // and behaviour unchanged. Owns the Add Disk / Resize / Change Bus
  // dialogs (only reachable from this card); Change ISO and the remove
  // confirmation stay in the parent (also reachable from Overview).
  import BlockCard from '$lib/components/BlockCard.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import Checkbox from '$lib/components/Checkbox.svelte';
  import Spinner from '$lib/components/Spinner.svelte';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import * as Dialog from '$lib/components/ui/dialog';
  import { t } from '$lib/i18n.svelte.js';
  import { api, auth } from '$lib/stores/auth.svelte.js';
  import { toast } from '$lib/components/ui/toast';
  import { vmDiskPools } from '$lib/purpose.js';
  import { bytesToStr, diskLabel } from './utils.js';

  let {
    vmId,
    vm,
    isContainerVm,
    pools,
    isos,
    actionLoading = $bindable(''),
    requireShutoff,
    onLoad,
    onSetISO,
    onRemove,
  } = $props();

  // Add Disk state
  let showAddDisk = $state(false);
  let aDiskDevice = $state('disk');
  let aDiskBus = $state('virtio');
  let aDiskSize = $state(10);
  let aDiskPool = $state('webkvm-disks');
  let aDiskISO = $state('');
  let aDiskFormat = $state('qcow2');
  let aDiskVolumes = $state([]);
  let aDiskExistingVol = $state('');
  let aDiskZVol = $state('');
  let aDiskZVols = $state([]);
  let aDiskZVolsLoading = $state(false);
  let aDiskZVolCustom = $state(false);
  // Result of probing the selected existing disk (null = not probed yet).
  // Used to warn that the image already contains data before attaching.
  let aDiskProbe = $state(null);
  let aDiskProbing = $state(false);
  let aDiskForce = $state(false);
  let aDiskWWN = $state('');
  let aDiskSerial = $state('');
  let aDiskAlias = $state('');

  // Resize Disk state
  let showResizeDisk = $state(false);
  let resizeDiskTarget = $state('');
  let resizeDiskSize = $state(10);
  let resizeDiskCurrent = $state(0);

  // Change Disk Bus state
  let showChangeBus = $state(false);
  let changeBusTarget = $state('');
  let changeBusCurrent = $state('');
  let changeBusNew = $state('virtio');

  async function loadDiskVolumesForPool() {
    try {
      aDiskVolumes = (await api.listVolumes(aDiskPool)) || [];
    } catch (_e) {
      aDiskVolumes = [];
    }
  }

  async function loadHostZVols() {
    if (!auth.isAdmin() || isContainerVm) return;
    aDiskZVolsLoading = true;
    try {
      aDiskZVols = (await api.listHostZVols()) || [];
    } catch (_e) {
      aDiskZVols = [];
    } finally {
      aDiskZVolsLoading = false;
    }
  }

  async function probeZVolDisk() {
    aDiskProbe = null;
    aDiskForce = false;
    const name = aDiskZVol?.trim();
    if (!name) return;
    const devPath = name.startsWith('/dev/zvol/') ? name : `/dev/zvol/${name}`;
    aDiskProbing = true;
    try {
      aDiskProbe = await api.probeDisk(devPath, false);
    } catch (_e) {
      aDiskProbe = null;
    } finally {
      aDiskProbing = false;
    }
  }

  // Inspect the selected existing disk so we can warn if it already has
  // data before it is attached. Basic probe (qemu-img) is fast and
  // always available; never blocks the dialog on failure.
  async function probeExistingDisk() {
    aDiskProbe = null;
    aDiskForce = false;
    const path = aDiskExistingVol;
    if (!path) return;
    aDiskProbing = true;
    try {
      aDiskProbe = await api.probeDisk(path, false);
    } catch (_e) {
      aDiskProbe = null;
    } finally {
      aDiskProbing = false;
    }
  }

  async function addDisk() {
    actionLoading = 'adddisk';
    try {
      const data = {
        device: aDiskDevice === 'existing' || aDiskDevice === 'zvol' ? 'disk' : aDiskDevice,
        bus: aDiskBus,
      };
      if (aDiskDevice === 'cdrom') {
        data.source = aDiskISO;
      } else if (aDiskDevice === 'zvol') {
        const zvolName = aDiskZVol?.trim();
        if (!zvolName) {
          toast.error(t('vmDetail.zvolSelectPlaceholder'));
          actionLoading = '';
          return;
        }
        data.zvol = zvolName;
        data.format = 'raw';
        if (aDiskProbe?.has_data && !aDiskForce) {
          toast.warning(t('vmDetail.diskHasDataWarning'));
          aDiskProbing = false;
          actionLoading = '';
          return;
        }
        data.force = aDiskForce;
      } else if (aDiskDevice === 'existing') {
        const vol = aDiskVolumes.find((v) => v.path === aDiskExistingVol);
        data.source = aDiskExistingVol;
        data.format = vol?.format || 'qcow2';
        // If the disk holds data and the operator has not confirmed,
        // surface the same guard the backend enforces instead of a raw
        // 409 round-trip.
        if (aDiskProbe?.has_data && !aDiskForce) {
          toast.warning(t('vmDetail.diskHasDataWarning'));
          aDiskProbing = false;
          actionLoading = '';
          return;
        }
        data.force = aDiskForce;
      } else {
        data.format = aDiskFormat;
        data.size_gb = aDiskSize;
        data.pool = aDiskPool;
      }
      if (aDiskDevice !== 'cdrom') {
        if (aDiskWWN.trim()) data.wwn = aDiskWWN.trim();
        if (aDiskSerial.trim()) data.serial = aDiskSerial.trim();
        if (aDiskAlias.trim()) data.alias = aDiskAlias.trim();
      }
      await api.createDisk(vmId, data);
      showAddDisk = false;
      toast.success(t('vmDetail.diskAdded'));
      await onLoad();
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }

  async function resizeDisk() {
    if (!resizeDiskTarget) return;
    if (resizeDiskSize < resizeDiskCurrent) {
      if (!requireShutoff(t('vmDetail.requireShutoffShrinkingDisk'))) return;
    }
    actionLoading = 'resizedisk';
    try {
      await api.resizeVmDisk(vm.id, resizeDiskTarget, resizeDiskSize);
      showResizeDisk = false;
      toast.success(t('vmDetail.diskResized'));
      await onLoad();
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }

  async function changeDiskBus() {
    if (!changeBusTarget || !changeBusNew) return;
    if (!requireShutoff(t('vmDetail.requireShutoffChangeBus'))) return;
    actionLoading = 'changebus';
    try {
      await api.changeDiskBus(vm.id, changeBusTarget, changeBusNew);
      showChangeBus = false;
      toast.success(t('vmDetail.diskBusChanged'));
      await onLoad();
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }
</script>

<BlockCard bid="disks" title={t('vmDetail.disks')}>
  <div class="flex items-center justify-between mb-3">
    <Button
      size="xs"
      variant="outline"
      onclick={() => {
        aDiskDevice = 'disk';
        aDiskBus = isContainerVm ? 'incus' : 'virtio';
        aDiskSize = 10;
        aDiskPool = vmDiskPools(pools)[0]?.name || (isContainerVm ? 'default' : 'webkvm-disks');
        aDiskExistingVol = '';
        aDiskVolumes = [];
        aDiskWWN = '';
        aDiskSerial = '';
        aDiskAlias = '';
        showAddDisk = true;
      }}>+ Add Disk</Button
    >
  </div>
  {#if !vm.disks || vm.disks.length === 0}
    <EmptyState compact icon="hardDrive" title={t('vmDetail.noDisks')} />
  {:else}
    <div class="space-y-1.5">
      {#each vm.disks as disk (disk.target || disk.source || disk.name)}
        <div
          class="flex items-center justify-between px-3 py-2 rounded-md border border-border bg-background"
        >
          <div class="flex items-center gap-2 min-w-0">
            <span class="text-xs font-mono text-muted-foreground w-8">{disk.target}</span>
            <span
              class="text-xs px-1.5 py-0.5 rounded border {disk.device === 'cdrom'
                ? 'border-accent/30 bg-accent/10 text-accent'
                : disk.zvol
                  ? 'border-primary/30 bg-primary/10 text-primary'
                  : 'border-border bg-muted text-muted-foreground'}"
              >{disk.device === 'cdrom' ? 'CDROM' : disk.zvol ? 'ZVOL' : 'DISK'}</span
            >
            <span class="text-xs text-muted-foreground">{disk.bus}</span>
            <span class="text-sm truncate">{diskLabel(disk)}</span>
            {#if disk.serial}
              <span
                class="text-[11px] font-mono px-1.5 py-0.5 rounded bg-muted text-muted-foreground"
                title={t('vmDetail.diskSerialLabel')}>SN: {disk.serial}</span
              >
            {/if}
            {#if disk.wwn}
              <span
                class="text-[11px] font-mono px-1.5 py-0.5 rounded bg-muted text-muted-foreground"
                title={t('vmDetail.diskWwnLabel')}>WWN: {disk.wwn}</span
              >
            {/if}
            {#if disk.alias}
              <span
                class="text-[11px] font-mono px-1.5 py-0.5 rounded bg-muted text-muted-foreground"
                title={t('vmDetail.diskAliasLabel')}>{disk.alias}</span
              >
            {/if}
          </div>
          <div class="flex items-center gap-1 shrink-0">
            {#if disk.device === 'cdrom'}
              <button
                onclick={() => onSetISO(disk)}
                class="text-xs text-accent hover:text-accent-hover px-2 py-1 rounded hover:bg-muted"
                >{t('vmDetail.changeIso')}</button
              >
            {:else if disk.pool}
              <span class="text-xs text-muted-foreground tnum"
                >{disk.size_gb ? disk.size_gb + ' GB' : ''}</span
              >
              <button
                onclick={() => {
                  resizeDiskTarget = disk.target;
                  resizeDiskSize = disk.size_gb || 10;
                  resizeDiskCurrent = disk.size_gb || 0;
                  showResizeDisk = true;
                }}
                class="text-xs text-accent hover:text-accent-hover px-2 py-1 rounded hover:bg-muted"
                >{t('vmDetail.resize')}</button
              >
            {/if}
            {#if !isContainerVm}
              <button
                onclick={() => {
                  changeBusTarget = disk.target;
                  changeBusCurrent = disk.bus || '';
                  changeBusNew = disk.bus || 'virtio';
                  showChangeBus = true;
                }}
                class="text-xs text-accent hover:text-accent-hover px-2 py-1 rounded hover:bg-muted"
                >{t('vmDetail.changeBus')}</button
              >
            {/if}
            {#if disk.target !== 'root'}
              <button
                onclick={() => onRemove(disk)}
                class="text-xs text-muted-foreground hover:text-destructive px-2 py-1 rounded hover:bg-destructive/10"
                >{t('vmDetail.remove')}</button
              >
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</BlockCard>

<!-- Add Disk Dialog -->
<Dialog.Root bind:open={showAddDisk}>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title
        >{aDiskDevice === 'cdrom'
          ? t('vmDetail.attachIso')
          : aDiskDevice === 'existing'
            ? t('vmDetail.attachExistingDisk')
            : aDiskDevice === 'zvol'
              ? t('vmDetail.attachZVol')
              : t('vmDetail.addDisk')}</Dialog.Title
      >
    </Dialog.Header>
    <div class="space-y-3">
      <div>
        <label for="adisk-type" class="block text-sm font-medium mb-1.5">{t('common.type')}</label>
        <select
          id="adisk-type"
          bind:value={aDiskDevice}
          onchange={() => {
            aDiskBus = aDiskDevice === 'cdrom' ? 'scsi' : 'virtio';
            if (aDiskDevice === 'existing') loadDiskVolumesForPool();
            if (aDiskDevice === 'zvol') loadHostZVols();
          }}
          class="input"
        >
          <option value="disk">{t('vmDetail.diskType')}</option>
          <option value="existing">{t('vmDetail.existingDisk')}</option>
          {#if auth.isAdmin() && !isContainerVm}
            <option value="zvol">{t('vmDetail.zvolType')}</option>
          {/if}
          <option value="cdrom">{t('vmDetail.cdromIso')}</option>
        </select>
      </div>
      <div>
        <label for="adisk-bus" class="block text-sm font-medium mb-1.5"
          >{t('vmDetail.busLabel')}</label
        >
        <select id="adisk-bus" bind:value={aDiskBus} class="input">
          {#if aDiskDevice === 'cdrom'}
            <option value="scsi">{t('vmDetail.scsiRecommended')}</option>
            <option value="sata">SATA</option>
          {:else}
            <option value="virtio">{t('vmDetail.virtioRecommended')}</option>
            <option value="sata">SATA</option>
            <option value="scsi">SCSI</option>
            <!-- IDE has no controller at all on Q35 (every VM here
                 unless created with the legacy i440fx chipset) —
                 offering it there just guarantees a failed attach. -->
            {#if vm?.chipset === 'i440fx'}
              <option value="ide">IDE</option>
            {/if}
          {/if}
        </select>
      </div>
      {#if aDiskDevice === 'disk' || aDiskDevice === 'existing'}
        <div>
          <label for="adisk-pool" class="block text-sm font-medium mb-1.5"
            >{t('vmDetail.storagePool')}</label
          >
          <select
            id="adisk-pool"
            bind:value={aDiskPool}
            onchange={() => {
              if (aDiskDevice === 'existing') loadDiskVolumesForPool();
            }}
            class="input"
          >
            {#each vmDiskPools(pools) as p (p.name)}<option value={p.name}>{p.name}</option>{/each}
          </select>
        </div>
      {/if}
      {#if aDiskDevice === 'disk'}
        <div>
          <label for="adisk-size" class="block text-sm font-medium mb-1.5"
            >{t('vmDetail.sizeGb')}</label
          >
          <Input id="adisk-size" type="number" min="1" bind:value={aDiskSize} class="tnum" />
        </div>
        <div>
          <label for="adisk-fmt" class="block text-sm font-medium mb-1.5"
            >{t('vmDetail.format')}</label
          >
          <select id="adisk-fmt" bind:value={aDiskFormat} class="input">
            <option value="qcow2">qcow2</option>
            <option value="raw">raw</option>
          </select>
        </div>
      {:else if aDiskDevice === 'existing'}
        <div>
          <label for="adisk-existing" class="block text-sm font-medium mb-1.5"
            >{t('vmDetail.existingDisk')}</label
          >
          <select
            id="adisk-existing"
            bind:value={aDiskExistingVol}
            onchange={probeExistingDisk}
            class="input"
          >
            <option value="">{t('vmDetail.empty')}</option>
            {#each aDiskVolumes.filter((v) => !v.is_snapshot) as v (v.id || v.name)}
              <option value={v.path}>{v.name} ({bytesToStr(v.capacity)})</option>
            {/each}
          </select>
        </div>
        {#if aDiskProbing}
          <p class="text-xs text-muted-foreground">{t('vmDetail.diskProbing')}</p>
        {:else if aDiskProbe?.has_data}
          <div class="rounded-lg border border-warning/40 bg-warning/5 p-2.5 space-y-2">
            <p class="text-[11px] text-warning flex items-start gap-1.5">
              <Icon name="alertTriangle" size={12} class="mt-0.5 shrink-0" />
              <span>{t('vmDetail.diskHasDataDetail', { format: aDiskProbe.format })}</span>
            </p>
            <label class="flex items-center gap-2 text-xs cursor-pointer">
              <Checkbox bind:checked={aDiskForce} />
              <span>{t('vmDetail.diskForceAttach')}</span>
            </label>
          </div>
        {/if}
      {:else if aDiskDevice === 'zvol'}
        <div>
          <label for="adisk-zvol" class="block text-sm font-medium mb-1.5"
            >{t('vmDetail.zvolLabel')}</label
          >
          {#if aDiskZVolCustom}
            <div class="space-y-2">
              <Input
                id="adisk-zvol-input"
                type="text"
                placeholder={t('vmDetail.zvolPlaceholder')}
                bind:value={aDiskZVol}
                onblur={probeZVolDisk}
              />
              <button
                type="button"
                class="text-xs text-accent hover:underline"
                onclick={() => {
                  aDiskZVolCustom = false;
                  aDiskZVol = '';
                  aDiskProbe = null;
                }}
              >
                ← {t('vmDetail.zvolSelectPlaceholder')}
              </button>
            </div>
          {:else}
            <select
              id="adisk-zvol"
              bind:value={aDiskZVol}
              disabled={aDiskZVolsLoading}
              onchange={(e) => {
                if (e.target.value === '__custom__') {
                  aDiskZVolCustom = true;
                  aDiskZVol = '';
                  aDiskProbe = null;
                } else {
                  probeZVolDisk();
                }
              }}
              class="input"
            >
              <option value="">{t('vmDetail.zvolSelectPlaceholder')}</option>
              {#each aDiskZVols as z (z.name)}
                <option value={z.name} disabled={!!z.used_by && z.used_by.vm_id !== vmId}>
                  {z.name} ({bytesToStr(z.volsize)})
                  {#if z.used_by}
                    - {t('vmDetail.zvolInUseWarning', {
                      vm: z.used_by.vm_name,
                      target: z.used_by.target,
                    })}
                  {/if}
                </option>
              {/each}
              <option value="__custom__">➕ {t('vmDetail.zvolCustomOption')}</option>
            </select>
          {/if}
          <p class="text-[11px] text-muted-foreground mt-1">{t('vmDetail.zvolHint')}</p>
        </div>
        {#if aDiskProbing}
          <p class="text-xs text-muted-foreground">{t('vmDetail.diskProbing')}</p>
        {:else if aDiskProbe?.has_data}
          <div class="rounded-lg border border-warning/40 bg-warning/5 p-2.5 space-y-2">
            <p class="text-[11px] text-warning flex items-start gap-1.5">
              <Icon name="alertTriangle" size={12} class="mt-0.5 shrink-0" />
              <span>{t('vmDetail.diskHasDataDetail', { format: aDiskProbe.format })}</span>
            </p>
            <label class="flex items-center gap-2 text-xs cursor-pointer">
              <Checkbox bind:checked={aDiskForce} />
              <span>{t('vmDetail.diskForceAttach')}</span>
            </label>
          </div>
        {/if}
      {:else}
        <div>
          <label for="adisk-iso" class="block text-sm font-medium mb-1.5">{t('vmDetail.iso')}</label
          >
          <select id="adisk-iso" bind:value={aDiskISO} class="input">
            <option value="">{t('vmDetail.empty')}</option>
            {#each isos as iso (iso.path || iso.name)}<option value={iso.path}>{iso.name}</option
              >{/each}
          </select>
        </div>
      {/if}
      {#if aDiskDevice === 'disk' || aDiskDevice === 'existing' || aDiskDevice === 'zvol'}
        <div class="border-t border-border pt-3 space-y-3">
          <div>
            <label for="adisk-serial" class="block text-xs font-medium mb-1"
              >{t('vmDetail.diskSerialLabel')}</label
            >
            <Input
              id="adisk-serial"
              type="text"
              placeholder={t('vmDetail.diskSerialPlaceholder')}
              bind:value={aDiskSerial}
            />
          </div>
          <div>
            <label for="adisk-wwn" class="block text-xs font-medium mb-1"
              >{t('vmDetail.diskWwnLabel')}</label
            >
            <Input
              id="adisk-wwn"
              type="text"
              placeholder={t('vmDetail.diskWwnPlaceholder')}
              bind:value={aDiskWWN}
            />
          </div>
          <div>
            <label for="adisk-alias" class="block text-xs font-medium mb-1"
              >{t('vmDetail.diskAliasLabel')}</label
            >
            <Input
              id="adisk-alias"
              type="text"
              placeholder={t('vmDetail.diskAliasPlaceholder')}
              bind:value={aDiskAlias}
            />
          </div>
        </div>
      {/if}
    </div>
    <Dialog.Footer class="gap-2">
      <Button
        variant="outline"
        onclick={() => (showAddDisk = false)}
        disabled={actionLoading === 'adddisk'}>{t('common.cancel')}</Button
      >
      <Button onclick={addDisk} disabled={actionLoading === 'adddisk'}>
        {#if actionLoading === 'adddisk'}<Spinner size="sm" color="text-white" />{:else}{t(
            'common.add'
          )}{/if}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Resize Disk Dialog -->
<Dialog.Root bind:open={showResizeDisk}>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>{t('vmDetail.resizeDiskTitle', { target: resizeDiskTarget })}</Dialog.Title>
      <Dialog.Description
        >{t('vmDetail.resizeDiskDesc', { current: resizeDiskCurrent })}</Dialog.Description
      >
      {#if vm && vm.state !== 'shutoff'}
        <p class="text-xs text-muted-foreground mt-1">{t('vmDetail.resizeDiskLiveNotice')}</p>
      {/if}
    </Dialog.Header>
    <div>
      <label for="rdisk-size" class="block text-sm font-medium mb-1.5"
        >{t('vmDetail.newSizeGb')}</label
      >
      <Input
        id="rdisk-size"
        type="number"
        min={resizeDiskCurrent}
        bind:value={resizeDiskSize}
        class="tnum"
      />
    </div>
    <Dialog.Footer class="gap-2">
      <Button
        variant="outline"
        onclick={() => (showResizeDisk = false)}
        disabled={actionLoading === 'resizedisk'}>{t('common.cancel')}</Button
      >
      <Button onclick={resizeDisk} disabled={actionLoading === 'resizedisk' || !resizeDiskSize}>
        {#if actionLoading === 'resizedisk'}<Spinner size="sm" color="text-white" />{:else}{t(
            'vmDetail.resize'
          )}{/if}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Change Disk Bus Dialog -->
<Dialog.Root bind:open={showChangeBus}>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>{t('vmDetail.changeBusTitle', { target: changeBusTarget })}</Dialog.Title>
      <Dialog.Description
        >{t('vmDetail.changeBusDesc', { current: changeBusCurrent })}</Dialog.Description
      >
    </Dialog.Header>
    <div>
      <label for="cbus-new" class="block text-sm font-medium mb-1.5">{t('vmDetail.busLabel')}</label
      >
      <select id="cbus-new" bind:value={changeBusNew} class="input">
        <option value="virtio">{t('vmDetail.virtioRecommended')}</option>
        <option value="sata">SATA</option>
        <option value="scsi">SCSI</option>
        <!-- IDE has no controller at all on Q35 (every VM here unless
             created with the legacy i440fx chipset) — offering it
             there just guarantees a failed attach. -->
        {#if vm?.chipset === 'i440fx'}
          <option value="ide">IDE</option>
        {/if}
      </select>
    </div>
    <Dialog.Footer class="gap-2">
      <Button
        variant="outline"
        onclick={() => (showChangeBus = false)}
        disabled={actionLoading === 'changebus'}>{t('common.cancel')}</Button
      >
      <Button
        onclick={changeDiskBus}
        disabled={actionLoading === 'changebus' || changeBusNew === changeBusCurrent}
      >
        {#if actionLoading === 'changebus'}<Spinner size="sm" color="text-white" />{:else}{t(
            'vmDetail.changeBus'
          )}{/if}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
