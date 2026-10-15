<script>
  // Network interfaces section extracted from routes/VmDetail.svelte —
  // markup, texts and behaviour unchanged. Actions (add/remove
  // interface, model change, link toggle) stay in the parent.
  import BlockCard from '$lib/components/BlockCard.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { Button } from '$lib/components/ui/button';
  import { t } from '$lib/i18n.svelte.js';
  import { api } from '$lib/stores/auth.svelte.js';
  import { toast } from '$lib/components/ui/toast';
  import * as Dialog from '$lib/components/ui/dialog';
  import { supports } from '$lib/stores/capabilities.svelte.js';
  import { networkLabel, networkLabelFor } from '$lib/utils/networkLabel.js';
  import { networkModels } from './utils.js';

  let {
    vmId,
    vm,
    isContainerVm,
    hostNetworks,
    netOptions, // networks offered by the Add dialog (allowlist-filtered)
    actionLoading = $bindable(''),
    canMutate,
    onModelChange,
    onToggleLink,
    onRemove,
    onLoad,
  } = $props();

  // Add Net state
  let showAddNet = $state(false);
  let aNetNetwork = $state('default');
  let aNetModel = $state('virtio');

  function openAddNet() {
    aNetNetwork = netOptions[0]?.name || 'default';
    aNetModel = isContainerVm ? 'incus' : 'virtio';
    showAddNet = true;
  }

  async function addNet() {
    actionLoading = 'addnet';
    try {
      await api.createNetIface(vmId, { network: aNetNetwork, model: aNetModel });
      showAddNet = false;
      toast.success(t('vmDetail.netAdded'));
      await onLoad();
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }
</script>

<BlockCard bid="net" title={t('vmDetail.networkInterfaces')}>
  <div class="flex items-center justify-between mb-3">
    <Button size="xs" variant="outline" onclick={openAddNet}>+ {t('vmDetail.addInterface')}</Button>
  </div>
  {#if !vm.networks || vm.networks.length === 0}
    <EmptyState compact icon="network" title={t('vmDetail.noNetworkInterfaces')} />
  {:else}
    <div class="space-y-1.5">
      {#each vm.networks as iface (iface.mac || iface.target || iface.name)}
        <div
          class="flex items-center justify-between px-3 py-2 rounded-md border border-border bg-background"
        >
          <div class="flex items-center gap-2 flex-wrap min-w-0">
            <span class="text-xs font-mono text-muted-foreground">{iface.mac}</span>
            {#if !isContainerVm && canMutate}
              <!-- Model change needs a full device redefine, so
                   the select is only enabled while the VM is
                   shut off (same rule the backend enforces). -->
              <select
                bind:value={iface.model}
                disabled={vm.state !== 'shutoff' || actionLoading === 'netmodel'}
                onchange={() => onModelChange(iface)}
                title={vm.state !== 'shutoff'
                  ? t('vmDetail.netModelChangeRequiresShutoff')
                  : t('vmDetail.model')}
                class="input !py-0.5 !text-xs !px-1.5 w-auto"
              >
                {#each networkModels as m (m.value)}
                  <option value={m.value} disabled={!supports('network', m.value)}
                    >{m.value}</option
                  >
                {/each}
              </select>
            {:else}
              <span
                class="text-xs px-1.5 py-0.5 rounded border border-border bg-muted text-muted-foreground"
                >{iface.model}</span
              >
            {/if}
            <span class="text-sm">{networkLabelFor(iface.network, hostNetworks)}</span>
            {#if iface.link_state === 'down'}
              <span
                class="text-xs px-1.5 py-0.5 rounded bg-destructive/10 border border-destructive/20 text-destructive font-mono"
              >
                Cable desconectado
              </span>
            {:else}
              <span
                class="text-xs px-1.5 py-0.5 rounded bg-emerald-500/10 border border-emerald-500/20 text-emerald-500 font-mono"
              >
                Enlace activo
              </span>
            {/if}
            {#if vm.state === 'running' && iface.ips?.length}
              <span
                class="text-xs px-1.5 py-0.5 rounded bg-accent/10 border border-accent/20 text-accent font-mono"
                >{iface.ips.join(', ')}</span
              >
            {/if}
          </div>
          <div class="flex items-center gap-1.5 shrink-0">
            {#if vm.state === 'running'}
              <button
                onclick={() => onToggleLink(iface)}
                disabled={actionLoading === 'netlink'}
                class="text-xs px-2 py-1 rounded border border-border hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
                title="Conectar o desconectar cable virtual en caliente"
              >
                {iface.link_state === 'down' ? '🔌 Conectar cable' : '🔌 Desconectar cable'}
              </button>
            {/if}
            <button
              onclick={() => onRemove(iface.mac)}
              class="text-xs text-muted-foreground hover:text-destructive px-2 py-1 rounded hover:bg-destructive/10 disabled:opacity-50 disabled:cursor-not-allowed"
              >{t('vmDetail.remove')}</button
            >
          </div>
        </div>
      {/each}
    </div>
  {/if}
</BlockCard>

<!-- Add Net Dialog -->
<Dialog.Root bind:open={showAddNet}>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>{t('vmDetail.addNetTitle')}</Dialog.Title>
    </Dialog.Header>
    <div class="space-y-3">
      <div>
        <label for="anet-net" class="block text-sm font-medium mb-1.5"
          >{t('vmDetail.networkLabel')}</label
        >
        <select id="anet-net" bind:value={aNetNetwork} class="input">
          {#each netOptions as net (net.name)}<option value={net.name}>{networkLabel(net)}</option
            >{/each}
        </select>
      </div>
      {#if !isContainerVm}
        <div>
          <label for="anet-model" class="block text-sm font-medium mb-1.5"
            >{t('vmDetail.model')}</label
          >
          <select id="anet-model" bind:value={aNetModel} class="input">
            {#each networkModels as m (m)}
              <option value={m.value} disabled={!supports('network', m.value)}>{m.label}</option>
            {/each}
          </select>
        </div>
      {/if}
    </div>
    <Dialog.Footer class="gap-2">
      <Button
        variant="outline"
        onclick={() => (showAddNet = false)}
        disabled={actionLoading === 'addnet'}>{t('common.cancel')}</Button
      >
      <Button onclick={addNet} disabled={actionLoading === 'addnet'}>
        {#if actionLoading === 'addnet'}<Spinner size="sm" color="text-white" />{:else}{t(
            'common.add'
          )}{/if}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
