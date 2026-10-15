<script>
  // Snapshots section extracted from routes/VmDetail.svelte — markup,
  // texts and behaviour unchanged. The parent keeps snapshot data,
  // the create/revert/delete actions and the shared confirm dialog;
  // this component renders the card and owns the create-form state.
  import BlockCard from '$lib/components/BlockCard.svelte';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import Switch from '$lib/components/Switch.svelte';
  import CronPicker from '$lib/components/CronPicker.svelte';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { t } from '$lib/i18n.svelte.js';
  import { buildSnapshotTree, formatSnapshotDate, bytesToStr } from './utils.js';

  let {
    vmId,
    snapshots,
    actionLoading = $bindable(''),
    schedSnap = $bindable(''),
    schedSnapMax = $bindable(3),
    schedSaving,
    onCreate,
    onRevert,
    onDelete,
    onSaveSchedule,
    retention, // snippet: shared retention row (also used by Schedule)
  } = $props();

  let snapName = $state('');
  let snapDesc = $state('');
  let snapMemory = $state(false);

  const snapshotTree = $derived.by(() => buildSnapshotTree(snapshots));

  // The form is only cleared when the parent reports success, matching
  // the original inline behaviour (values survive a failed create).
  async function submitCreate() {
    if (!snapName) return;
    const ok = await onCreate({ name: snapName, description: snapDesc, memory: snapMemory });
    if (ok) {
      snapName = '';
      snapDesc = '';
      snapMemory = false;
    }
  }
</script>

<BlockCard bid="snaps" title={t('vmDetail.snapshots')} anchor="snapshots">
  <div class="flex gap-2 mb-3">
    <Input bind:value={snapName} placeholder="Snapshot name" class="flex-1" />
    <Button onclick={submitCreate} disabled={!snapName || actionLoading === 'snapshot'}>
      {#if actionLoading === 'snapshot'}<Spinner size="xs" color="text-white" />{:else}{t(
          'vmDetail.createSnapshot'
        )}{/if}
    </Button>
  </div>
  <label
    class="flex items-center gap-2 text-sm text-muted-foreground mb-3 cursor-pointer select-none"
  >
    <input type="checkbox" bind:checked={snapMemory} class="w-4 h-4 rounded border-border" />
    {t('vmDetail.snapshotWithMemory')}
  </label>
  {#if snapshots.length === 0}
    <EmptyState compact icon="camera" title={t('vmDetail.noSnapshots')} />
  {:else}
    <div class="space-y-0.5">
      {#each snapshotTree.roots as root (root.name)}
        {@render snapshotNode(root, 0)}
      {/each}
    </div>
  {/if}

  <!-- Automated Snapshot Policy -->
  <div class="mt-4 pt-3 border-t border-border">
    <div class="flex items-center justify-between mb-2">
      <div class="flex items-center gap-2">
        <Icon name="clock" size={13} class="text-accent" />
        <span class="text-xs font-semibold text-foreground">
          {t('vmDetail.snapScheduleTitle')}
        </span>
      </div>
      <Switch
        size="sm"
        checked={schedSnap !== ''}
        onchange={(v) => (schedSnap = v ? '0 2 * * *' : '')}
      />
    </div>
    {#if !schedSnap}
      <p class="text-xs text-muted-foreground py-1">
        {t('vmDetail.noSnapshotsScheduled')}
      </p>
    {:else}
      <CronPicker bind:expression={schedSnap} />
      {@render retention()}
    {/if}
    <div class="mt-3 flex justify-end">
      <Button size="xs" onclick={onSaveSchedule} disabled={schedSaving}>
        {#if schedSaving}<Spinner size="xs" color="text-white" />{:else}{t('common.save')}{/if}
      </Button>
    </div>
  </div>
</BlockCard>

{#snippet snapshotNode(node, depth)}
  <div
    class="flex items-center justify-between py-1.5 border-b border-border last:border-0"
    style="padding-left: {depth * 20}px"
  >
    <div class="flex items-center gap-2 min-w-0">
      {#if depth > 0}
        <svg
          class="w-3 h-3 text-muted-foreground/40 shrink-0"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          viewBox="0 0 24 24"><polyline points="9 6 15 12 9 18" /></svg
        >
      {/if}
      <div class="min-w-0">
        <p class="text-sm font-medium truncate">
          {node.name}
          {#if node.current}
            <span
              class="text-[10px] text-success ml-1.5 px-1.5 py-0.5 rounded border border-success/30 bg-success/10 uppercase tracking-wider"
              >{t('vmDetail.current')}</span
            >
          {/if}
        </p>
        <p class="text-xs text-muted-foreground tnum">
          {formatSnapshotDate(node.creation_time)}
          {#if node.size_at_snap_bytes}
            <span class="mx-1.5 text-border">·</span>
            <span class="text-accent">{bytesToStr(node.size_at_snap_bytes)}</span>
            <span class="text-muted-foreground/70">{t('vmDetail.atCreation')}</span>
          {/if}
        </p>
      </div>
    </div>
    <div class="flex gap-1 shrink-0">
      <!-- Revert works for the current snapshot too (backend supports
           it): it rolls the VM back to the latest snapshot, so the
           confirm dialog carries an extra warning. -->
      <button
        onclick={() => onRevert(node.id, !!node.current)}
          class="text-xs text-warning hover:text-warning px-2 py-1 rounded hover:bg-warning/10"
          >{t('vmDetail.revert')}</button
        >
      <button
        onclick={() => onDelete(node.id)}
        class="text-xs text-muted-foreground hover:text-destructive px-2 py-1 rounded hover:bg-destructive/10"
        >{t('common.delete')}</button
      >
    </div>
  </div>
  {#each node.children as child (child.name)}
    {@render snapshotNode(child, depth + 1)}
  {/each}
{/snippet}
