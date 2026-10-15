<script>
  // USB passthrough section extracted from routes/VmDetail.svelte —
  // markup, texts and behaviour unchanged (admin only).
  import BlockCard from '$lib/components/BlockCard.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { t } from '$lib/i18n.svelte.js';
  import { api, auth } from '$lib/stores/auth.svelte.js';
  import { toast } from '$lib/components/ui/toast';

  let { vmId, vm, actionLoading = $bindable(''), onLoad } = $props();

  let hostUSBDevices = $state([]);
  let hostUSBLoaded = false;
  async function loadHostUSBDevices(force = false) {
    if (hostUSBLoaded && !force) return;
    hostUSBLoaded = true;
    try {
      hostUSBDevices = await api.listHostUSBDevices();
    } catch (e) {
      // Allow a later retry (the button calls with force=true) but do
      // not let the auto-effect spam an error toast on every re-run.
      hostUSBLoaded = false;
      toast.error(e.message);
    }
  }
  $effect(() => {
    if (auth.isAdmin()) loadHostUSBDevices();
  });
  async function attachUSB(vendorId, productId) {
    actionLoading = 'usb';
    try {
      await api.attachUSBDevice(vmId, vendorId, productId);
      toast.success(t('vmDetail.usbDeviceAttached'));
      await onLoad();
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }
  async function detachUSB(vendorId, productId) {
    actionLoading = 'usb';
    try {
      await api.detachUSBDevice(vmId, vendorId, productId);
      toast.success(t('vmDetail.usbDeviceDetached'));
      await onLoad();
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }
</script>

<BlockCard bid="usb" title={t('vmDetail.usbDevices')}>
  <div class="space-y-3">
    <div>
      <div class="text-xs font-medium text-muted-foreground mb-1.5">
        {t('vmDetail.usbAttached')}
      </div>
      {#if !vm.usb_devices || vm.usb_devices.length === 0}
        <EmptyState compact icon="usb" title={t('vmDetail.usbNoneAttached')} />
      {:else}
        <div class="space-y-1.5">
          <!-- Keyed on position, not on the device identity.
               parseUSBHostdevs (libvirt/domain.go) only reads
               vendor and product out of the hostdev XML, so Name,
               Bus and Device arrive empty and the old key
               collapsed to "vendor:product:". Two identical USB
               devices — two of the same stick, mouse or webcam —
               then produced the same key, and Svelte 5 aborts the
               whole render on each_key_duplicate, leaving the
               section empty. The list is a static set of attached
               devices, so position is a sound identity here. -->
          {#each vm.usb_devices as dev, i (i)}
            <div
              class="flex items-center justify-between px-3 py-2 rounded-md border border-border bg-background"
            >
              <span class="text-xs font-mono text-muted-foreground"
                >{dev.vendor_id}:{dev.product_id}</span
              >
              <button
                onclick={() => detachUSB(dev.vendor_id, dev.product_id)}
                disabled={actionLoading === 'usb'}
                class="text-xs text-muted-foreground hover:text-destructive px-2 py-1 rounded hover:bg-destructive/10 disabled:opacity-50 disabled:cursor-not-allowed"
                >{t('vmDetail.usbDetach')}</button
              >
            </div>
          {/each}
        </div>
      {/if}
    </div>
    <div>
      <div class="flex items-center justify-between mb-1.5">
        <div class="text-xs font-medium text-muted-foreground">
          {t('vmDetail.usbAvailable')}
        </div>
        <button
          onclick={() => loadHostUSBDevices(true)}
          class="text-xs text-accent hover:text-accent-hover"
          >{t('common.refresh')}</button
        >
      </div>
      {#if hostUSBDevices.length === 0}
        <EmptyState compact icon="usb" title={t('vmDetail.usbNoneFound')} />
      {:else}
        <div class="space-y-1.5">
          {#each hostUSBDevices as dev (`${dev.bus}:${dev.device}:${dev.vendor_id}:${dev.product_id}`)}
            <div
              class="flex items-center justify-between px-3 py-2 rounded-md border border-border bg-background"
            >
              <div class="flex items-center gap-2 min-w-0">
                <span class="text-sm truncate">{dev.name}</span>
                <span class="text-xs font-mono text-muted-foreground"
                  >{dev.vendor_id}:{dev.product_id} · bus {dev.bus} dev {dev.device}</span
                >
              </div>
              <button
                onclick={() => attachUSB(dev.vendor_id, dev.product_id)}
                disabled={actionLoading === 'usb'}
                class="text-xs text-accent hover:text-accent-hover px-2 py-1 rounded hover:bg-muted disabled:opacity-50 disabled:cursor-not-allowed"
                >{t('vmDetail.usbAttach')}</button
              >
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</BlockCard>
