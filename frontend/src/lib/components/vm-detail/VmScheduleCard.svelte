<script>
  // Schedule section extracted from routes/VmDetail.svelte — markup,
  // texts and behaviour unchanged. The schedule state lives in the
  // parent (load() seeds it and the snapshots card shares schedSnap).
  import BlockCard from '$lib/components/BlockCard.svelte';
  import Spinner from '$lib/components/Spinner.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import Switch from '$lib/components/Switch.svelte';
  import CronPicker from '$lib/components/CronPicker.svelte';
  import { Button } from '$lib/components/ui/button';
  import { t } from '$lib/i18n.svelte.js';

  let {
    schedStart = $bindable(''),
    schedStop = $bindable(''),
    schedSnap = $bindable(''),
    schedSaving,
    onSaveSchedule,
    retention, // snippet: shared retention row (also used by Snapshots)
  } = $props();
</script>

<BlockCard bid="schedule" title={t('vmDetail.scheduleTitle')} anchor="schedule">
  <div class="space-y-4 mb-4">
    <!-- Encendido Programado -->
    <div class="p-4 rounded-xl border border-border bg-card/40 space-y-3">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <Icon name="play" size={14} class="text-success" />
          <span class="text-xs font-semibold uppercase tracking-wider text-foreground">
            {t('vmDetail.scheduleStart')}
          </span>
        </div>
        <Switch
          size="sm"
          checked={schedStart !== ''}
          onchange={(v) => (schedStart = v ? '0 8 * * 1-5' : '')}
        />
      </div>
      {#if !schedStart}
        <p class="text-xs text-muted-foreground py-1">
          {t('vmDetail.noActiveSchedule')}
        </p>
      {:else}
        <CronPicker bind:expression={schedStart} />
      {/if}
    </div>

    <!-- Apagado Programado -->
    <div class="p-4 rounded-xl border border-border bg-card/40 space-y-3">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <Icon name="power" size={14} class="text-destructive" />
          <span class="text-xs font-semibold uppercase tracking-wider text-foreground">
            {t('vmDetail.scheduleStop')}
          </span>
        </div>
        <Switch
          size="sm"
          checked={schedStop !== ''}
          onchange={(v) => (schedStop = v ? '0 20 * * 1-5' : '')}
        />
      </div>
      {#if !schedStop}
        <p class="text-xs text-muted-foreground py-1">
          {t('vmDetail.noActiveSchedule')}
        </p>
      {:else}
        <CronPicker bind:expression={schedStop} />
      {/if}
    </div>

    <!-- Instantáneas Automáticas -->
    <div class="p-4 rounded-xl border border-border bg-card/40 space-y-3">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <Icon name="camera" size={14} class="text-accent" />
          <span class="text-xs font-semibold uppercase tracking-wider text-foreground">
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
    </div>
  </div>

  <p class="text-xs text-muted-foreground mb-3">{t('vmDetail.scheduleHint')}</p>
  <Button size="sm" onclick={onSaveSchedule} disabled={schedSaving}>
    {#if schedSaving}<Spinner size="xs" color="text-white" />{:else}{t('common.save')}{/if}
  </Button>
</BlockCard>
