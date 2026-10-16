<script>
  // Workstation view — a VMware Workstation / Proxmox style single screen:
  // VM library on the left, live VNC console for the selected machine on
  // the right. Console tickets, realtime state updates and the snapshot /
  // ISO action menu follow the same patterns as MultiView.svelte.
  import { onMount, onDestroy } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { api, auth } from '$lib/stores/auth.svelte.js';
  import { t } from '$lib/i18n.svelte.js';
  import { isContainer } from '$lib/utils/computeType.js';
  import Alert from '$lib/components/Alert.svelte';
  import { Button } from '$lib/components/ui/button';
  import Icon from '$lib/components/Icon.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Spinner from '$lib/components/Spinner.svelte';
  import { toast } from '$lib/components/ui/toast';
  import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
  import { events } from '$lib/stores/events.svelte.js';

  let vms = $state([]);
  let loading = $state(true);
  let error = $state('');
  let search = $state('');
  let selectedVmId = $state(null);
  let isFullscreen = $state(false);
  let pollInterval = null;

  // VNC tickets per VM, kept in a cache: switching back and forth between
  // machines reuses the existing ticket instead of refetching (tickets are
  // dropped when a VM leaves the running state, see onVmState below).
  let tickets = $state({});
  let loadingTickets = $state({});

  // --- Header action state ---
  let busy = $state(''); // `${vmId}:${action}` in-flight guard
  let shutdownConfirmOpen = $state(false);
  let shutdownConfirmVm = $state(null);
  let forceOffConfirmOpen = $state(false);
  let forceOffConfirmVm = $state(null);

  // --- "⋯" menu state (snapshot / revert / ISO), mirrors MultiView ---
  let menuOpen = $state(false);
  let submenu = $state(null); // 'snapshots' | 'iso' | null
  let snapshotList = $state([]);
  let snapshotsLoading = $state(false);
  let isoList = $state(null); // null until first open, then cached
  let isosLoading = $state(false);
  let disks = $state({}); // vmId -> disks (lazy, for the ISO submenu)
  let revertConfirmOpen = $state(false);
  let revertConfirmSnapshot = $state(null);
  let revertLoading = $state(false);

  const selectedVm = $derived(vms.find((v) => v.id === selectedVmId) || null);

  function formatRAM(vm) {
    const mb = vm.ram_mb || vm.memory || vm.memory_mb || 0;
    if (!mb) return '—';
    if (mb >= 1024) {
      const gb = mb / 1024;
      return `${gb % 1 === 0 ? gb.toFixed(0) : gb.toFixed(1)} GB`;
    }
    return `${mb} MB`;
  }

  function stateDotClass(state) {
    switch (state) {
      case 'running':
        return 'bg-success shadow-[0_0_6px_rgba(34,197,94,0.8)]';
      case 'paused':
        return 'bg-warning';
      case 'crashed':
        return 'bg-destructive';
      default:
        return 'bg-muted-foreground';
    }
  }

  function stateLabel(state) {
    switch (state) {
      case 'running':
        return t('common.running');
      case 'paused':
        return t('common.paused');
      case 'crashed':
        return t('common.crashed');
      case 'shutoff':
        return t('common.shutoff');
      default:
        return state || t('common.unknown');
    }
  }

  // Running machines first, then alphabetical. Cached tickets are kept for
  // machines that are still running even when they are filtered away.
  let sortedVms = $derived.by(() => {
    const rank = (v) => (v.state === 'running' ? 0 : 1);
    return [...vms].sort(
      (a, b) => rank(a) - rank(b) || (a.alias || a.name || '').localeCompare(b.alias || b.name || '')
    );
  });

  let filteredVms = $derived.by(() => {
    const q = search.toLowerCase().trim();
    if (!q) return sortedVms;
    return sortedVms.filter(
      (v) =>
        (v.name || '').toLowerCase().includes(q) ||
        (v.alias && v.alias.toLowerCase().includes(q)) ||
        stateLabel(v.state).toLowerCase().includes(q)
    );
  });

  // Tag chips: groups when present, plus any free-form tags on the VM.
  function vmTags(vm) {
    const tags = [];
    if (Array.isArray(vm.groups)) tags.push(...vm.groups);
    if (Array.isArray(vm.tags)) tags.push(...vm.tags);
    return tags;
  }

  async function loadVMs() {
    try {
      const data = await api.listVMs();
      vms = (data || []).filter((v) => !isContainer(v));
      if (!selectedVmId && vms.length > 0) {
        const firstRunning = sortedVms.find((v) => v.state === 'running') || sortedVms[0];
        selectVm(firstRunning.id);
      }
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function selectVm(id) {
    selectedVmId = id;
    closeMenu();
    const vm = vms.find((v) => v.id === id);
    if (vm && vm.state === 'running' && !tickets[id] && !loadingTickets[id]) {
      fetchTicketFor(id);
    }
  }

  async function fetchTicketFor(vmId) {
    if (loadingTickets[vmId] || tickets[vmId]) return;
    loadingTickets = { ...loadingTickets, [vmId]: true };
    try {
      const res = await (api.getVNCTicket ? api.getVNCTicket(vmId) : api.createVNCTicket(vmId));
      if (res && res.vnc_ticket) {
        tickets = { ...tickets, [vmId]: res.vnc_ticket };
      }
    } catch (e) {
      console.warn(`[Workstation] Failed ticket for ${vmId}:`, e.message);
    } finally {
      loadingTickets = { ...loadingTickets, [vmId]: false };
    }
  }

  function dropTicket(vmId) {
    if (!tickets[vmId]) return;
    const next = { ...tickets };
    delete next[vmId];
    tickets = next;
  }

  // --- Power actions ---
  async function powerOn(vm) {
    const key = `${vm.id}:start`;
    if (busy === key) return;
    busy = key;
    try {
      toast.info(t('vms.starting'));
      await api.startVM(vm.id);
      later(() => fetchTicketFor(vm.id), 2500);
      await loadVMs();
    } catch (e) {
      toast.error(e.message);
    } finally {
      busy = '';
    }
  }

  function askShutdown(vm) {
    shutdownConfirmVm = vm;
    shutdownConfirmOpen = true;
    closeMenu();
  }

  async function doShutdown() {
    const vm = shutdownConfirmVm;
    if (!vm) return;
    const key = `${vm.id}:shutdown`;
    busy = key;
    try {
      await api.shutdownVM(vm.id);
      toast.success(t('vms.shutdownSent', { name: vm.alias || vm.name }));
      dropTicket(vm.id);
      shutdownConfirmOpen = false;
      await loadVMs();
    } catch (e) {
      toast.error(e.message);
      shutdownConfirmOpen = false;
    } finally {
      busy = '';
    }
  }

  function askForceOff(vm) {
    forceOffConfirmVm = vm;
    forceOffConfirmOpen = true;
    closeMenu();
  }

  async function doForceOff() {
    const vm = forceOffConfirmVm;
    if (!vm) return;
    const key = `${vm.id}:forceoff`;
    busy = key;
    try {
      await api.forceOffVM(vm.id);
      toast.success(t('vms.forceoffDone', { name: vm.alias || vm.name }));
      dropTicket(vm.id);
      forceOffConfirmOpen = false;
      await loadVMs();
    } catch (e) {
      toast.error(e.message);
      forceOffConfirmOpen = false;
    } finally {
      busy = '';
    }
  }

  async function restartVm(vm) {
    const key = `${vm.id}:reboot`;
    if (busy === key) return;
    busy = key;
    closeMenu();
    try {
      toast.info(t('vms.starting'));
      await api.rebootVM(vm.id);
      toast.success(t('vms.restarted'));
      // Console drops during reboot; refetch a ticket once it is back.
      dropTicket(vm.id);
      later(() => fetchTicketFor(vm.id), 2500);
      await loadVMs();
    } catch (e) {
      toast.error(e.message);
    } finally {
      busy = '';
    }
  }

  // --- Snapshot / ISO menu (same flow as MultiView tiles) ---
  function closeMenu() {
    menuOpen = false;
    submenu = null;
  }

  function handleGlobalClick(e) {
    if (menuOpen && !e.target?.closest?.('[data-ws-menu]')) closeMenu();
  }

  function defaultSnapshotName() {
    const d = new Date();
    const p = (n) => String(n).padStart(2, '0');
    return `snap-${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}`;
  }

  async function createSnapshot(vm) {
    const key = `${vm.id}:snapshot`;
    if (busy === key) return;
    busy = key;
    closeMenu();
    try {
      const snapName = defaultSnapshotName();
      const res = await api.createSnapshot(vm.id, {
        name: snapName,
        description: t('vms.quickSnapshotDesc'),
      });
      await api.waitJob(res.job, {
        title: `${t('vmDetail.createSnapshot')}: ${snapName} (${vm.alias || vm.name})`,
        kind: 'snapshot',
      });
      toast.success(t('vmDetail.snapshotCreated'));
    } catch (e) {
      toast.error(e.message);
    } finally {
      busy = '';
    }
  }

  async function openSnapshots(vm) {
    submenu = 'snapshots';
    if (snapshotsLoading) return;
    snapshotList = [];
    snapshotsLoading = true;
    try {
      snapshotList = await api.listSnapshots(vm.id);
    } catch (e) {
      toast.error(e.message);
      closeMenu();
    } finally {
      snapshotsLoading = false;
    }
  }

  function askRevert(snap) {
    revertConfirmSnapshot = snap;
    revertConfirmOpen = true;
    closeMenu();
  }

  async function doRevert() {
    const vm = selectedVm;
    if (!vm || !revertConfirmSnapshot) return;
    revertLoading = true;
    try {
      await api.revertSnapshot(vm.id, revertConfirmSnapshot.name);
      revertConfirmOpen = false;
      toast.success(t('vmDetail.reverted'));
      dropTicket(vm.id);
      await loadVMs();
    } catch (e) {
      toast.error(e.message);
      revertConfirmOpen = false;
    } finally {
      revertLoading = false;
    }
  }

  async function openIso(vm) {
    submenu = 'iso';
    if (!isoList && !isosLoading) {
      isosLoading = true;
      try {
        isoList = await api.listISOs();
      } catch (e) {
        toast.error(e.message);
        closeMenu();
        return;
      } finally {
        isosLoading = false;
      }
    }
    if (!disks[vm.id]) {
      try {
        disks = { ...disks, [vm.id]: await api.listDisks(vm.id) };
      } catch (e) {
        toast.error(e.message);
        closeMenu();
      }
    }
  }

  function cdrom(vm) {
    return (disks[vm.id] || vm.disks || []).find((d) => d.device === 'cdrom') || null;
  }

  async function setIso(vm, source) {
    const cd = cdrom(vm);
    if (!cd) return;
    const key = `${vm.id}:iso`;
    if (busy === key) return;
    busy = key;
    try {
      await api.updateDiskSource(vm.id, cd.target, source);
      toast.success(source ? t('vmDetail.isoChanged') : t('vmDetail.isoEjected'));
      disks = { ...disks, [vm.id]: await api.listDisks(vm.id) };
      closeMenu();
    } catch (e) {
      toast.error(e.message);
    } finally {
      busy = '';
    }
  }

  // --- Console helpers ---
  function popoutConsole(vm) {
    // Mirrors VmList/VmDetail openConsole: ticket-authenticated,
    // server-rendered console page in a new tab.
    window.open(
      `/console/${vm.id}?vt=${encodeURIComponent(tickets[vm.id] || '')}`,
      '_blank',
      'noopener,noreferrer'
    );
  }

  function reconnect(vm) {
    const el = document.querySelector(`iframe[data-vm-id="${vm.id}"]`);
    if (el) {
      el.src = el.src;
    } else {
      dropTicket(vm.id);
      fetchTicketFor(vm.id);
    }
    toast.info(t('multiview.reconnectTitle'));
  }

  function toggleFullscreen() {
    const el = document.getElementById('workstation-container');
    if (!document.fullscreenElement) {
      el?.requestFullscreen().catch(() => {});
    } else {
      document.exitFullscreen().catch(() => {});
    }
  }

  /** setTimeout that is cancelled automatically on unmount. */
  const pendingTimers = new SvelteSet();
  function later(fn, ms) {
    const t = setTimeout(() => {
      pendingTimers.delete(t);
      fn();
    }, ms);
    pendingTimers.add(t);
    return t;
  }

  onMount(() => {
    loadVMs();
    pollInterval = setInterval(loadVMs, 15000);
    const fsHandler = () => {
      isFullscreen = !!document.fullscreenElement;
    };
    document.addEventListener('fullscreenchange', fsHandler);
    document.addEventListener('click', handleGlobalClick);

    // Realtime state updates: react to power changes without waiting for
    // the 15 s poll. A VM that left running loses its ticket; one that
    // entered running (and is selected) gets one after a short grace
    // period, exactly like MultiView.
    const offVmState = events.onVmState((e) => {
      const vm = vms.find((v) => v.id === e.vm_id);
      if (!vm || vm.state === e.state) return;
      vms = vms.map((v) => (v.id === e.vm_id ? { ...v, state: e.state } : v));
      if (e.state !== 'running') {
        dropTicket(e.vm_id);
      } else if (e.vm_id === selectedVmId) {
        later(() => fetchTicketFor(e.vm_id), 2500);
      }
    });

    return () => {
      document.removeEventListener('fullscreenchange', fsHandler);
      document.removeEventListener('click', handleGlobalClick);
      offVmState();
      if (pollInterval) clearInterval(pollInterval);
    };
  });

  onDestroy(() => {
    if (pollInterval) clearInterval(pollInterval);
    for (const t of pendingTimers) clearTimeout(t);
    pendingTimers.clear();
  });
</script>

<div
  class="w-full flex-1 flex flex-col min-h-0 min-w-0 p-2 sm:p-3 overflow-hidden"
  id="workstation-container"
>
  {#if error}
    <Alert variant="error" class="mb-2 shrink-0">{error}</Alert>
  {/if}

  <div class="flex-1 flex gap-3 min-h-0 min-w-0">
    <!-- Left: machine library -->
    <aside
      class="w-[300px] sm:w-[340px] shrink-0 flex flex-col min-h-0 rounded-2xl bg-card border border-border shadow-md overflow-hidden"
    >
      <div class="px-3 pt-3 pb-2 border-b border-border shrink-0 space-y-2">
        <div class="flex items-center gap-2">
          <div
            class="w-8 h-8 rounded-xl bg-accent/15 border border-accent/30 flex items-center justify-center text-accent shrink-0"
          >
            <Icon name="monitor" size={16} />
          </div>
          <div class="min-w-0">
            <h1 class="text-sm font-bold text-foreground tracking-tight truncate">
              {t('workstation.title')}
            </h1>
            <p class="text-[11px] text-muted-foreground truncate">
              {t('workstation.subtitle')}
            </p>
          </div>
        </div>
        <div class="relative">
          <Icon
            name="search"
            size={14}
            class="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <input
            type="text"
            placeholder={t('vms.searchPlaceholder')}
            bind:value={search}
            class="w-full pl-8.5 pr-3 py-1.5 text-xs rounded-lg bg-background border border-border text-foreground placeholder:text-muted-foreground focus:outline-none focus:border-accent transition-colors"
          />
        </div>
      </div>

      <div class="flex-1 overflow-y-auto p-2 space-y-1 min-h-0">
        {#if loading && vms.length === 0}
          <div class="flex flex-col items-center gap-2 py-10 text-muted-foreground">
            <Spinner size="md" />
          </div>
        {:else if filteredVms.length === 0}
          <p class="px-2 py-6 text-xs text-muted-foreground text-center">
            {vms.length === 0 ? t('vms.noVms') : t('workstation.libraryEmpty')}
          </p>
        {:else}
          {#each filteredVms as vm (vm.id)}
            {@const tags = vmTags(vm)}
            <div
              role="button"
              tabindex="0"
              class="group w-full text-left px-2.5 py-2 rounded-xl border cursor-pointer transition-all {selectedVmId ===
              vm.id
                ? 'bg-accent/10 border-accent/60 ring-1 ring-accent/30'
                : 'bg-background border-border hover:border-border-hover'}"
              onclick={() => selectVm(vm.id)}
              onkeydown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  selectVm(vm.id);
                }
              }}
            >
              <div class="flex items-center gap-2 min-w-0">
                <span
                  class="w-2 h-2 rounded-full shrink-0 {stateDotClass(vm.state)}"
                  title={stateLabel(vm.state)}
                ></span>
                <span class="text-xs font-bold text-foreground truncate min-w-0 flex-1" title={vm.name}>
                  {vm.alias || vm.name}
                </span>
                {#if auth.canMutate()}
                  <span
                    class="flex items-center gap-0.5 shrink-0 opacity-0 group-hover:opacity-100 focus-within:opacity-100 transition-opacity"
                  >
                    {#if vm.state === 'running'}
                      <button
                        class="p-1 rounded text-muted-foreground hover:text-warning hover:bg-muted transition-colors"
                        title={t('vms.shutdown')}
                        aria-label={t('vms.shutdown')}
                        onclick={(e) => {
                          e.stopPropagation();
                          askShutdown(vm);
                        }}
                      >
                        <Icon name="power" size={12} />
                      </button>
                    {:else}
                      <button
                        class="p-1 rounded text-muted-foreground hover:text-success hover:bg-muted transition-colors disabled:opacity-50"
                        title={t('multiview.powerOn')}
                        aria-label={t('multiview.powerOn')}
                        disabled={busy === `${vm.id}:start`}
                        onclick={(e) => {
                          e.stopPropagation();
                          powerOn(vm);
                        }}
                      >
                        <Icon name="play" size={12} />
                      </button>
                    {/if}
                  </span>
                {/if}
              </div>
              <div class="flex items-center gap-1.5 mt-1 text-[10px] text-muted-foreground pl-4">
                <span class="truncate">{stateLabel(vm.state)}</span>
                <span>·</span>
                <span class="font-mono">{vm.vcpus ?? '—'} vCPU</span>
                <span>·</span>
                <span class="font-mono">{formatRAM(vm)}</span>
              </div>
              {#if tags.length > 0}
                <div class="flex flex-wrap gap-1 mt-1 pl-4">
                  {#each tags.slice(0, 3) as tag (tag)}
                    <span
                      class="px-1.5 py-0.5 text-[9px] font-semibold rounded-full bg-muted text-muted-foreground border border-border truncate max-w-[100px]"
                      >{tag}</span
                    >
                  {/each}
                </div>
              {/if}
            </div>
          {/each}
        {/if}
      </div>
    </aside>

    <!-- Right: console panel -->
    <section class="flex-1 flex flex-col min-h-0 min-w-0 rounded-2xl bg-card border border-border shadow-md overflow-hidden">
      {#if !selectedVm}
        <div class="flex-1 flex flex-col items-center justify-center gap-3 p-8 text-center">
          <div
            class="w-12 h-12 rounded-2xl bg-muted border border-border flex items-center justify-center text-muted-foreground shadow-inner"
          >
            <Icon name="monitor" size={24} />
          </div>
          <p class="text-sm font-bold text-foreground/80">{t('workstation.noSelection')}</p>
          <p class="text-xs text-muted-foreground max-w-xs">{t('workstation.noSelectionDesc')}</p>
        </div>
      {:else}
        <!-- Console header -->
        <div class="h-11 px-3 flex items-center justify-between gap-2 border-b border-border shrink-0">
          <div class="flex items-center gap-2 min-w-0">
            <span
              class="w-2.5 h-2.5 rounded-full shrink-0 {stateDotClass(selectedVm.state)}"
              title={stateLabel(selectedVm.state)}
            ></span>
            <span class="font-bold text-sm text-foreground truncate" title={selectedVm.name}>
              {selectedVm.alias || selectedVm.name}
            </span>
            <span class="text-[11px] text-muted-foreground shrink-0 hidden sm:inline">
              {stateLabel(selectedVm.state)}
            </span>
            {#if selectedVm.state === 'running'}
              <span
                class="text-[10px] font-mono font-semibold px-1.5 py-0.5 rounded bg-accent/15 text-accent border border-accent/30 shrink-0"
                title={t('workstation.vncBadge')}
              >
                {t('workstation.vncBadge')}{selectedVm.vnc_port ? ` :${selectedVm.vnc_port}` : ''}
              </span>
            {/if}
          </div>

          <div class="flex items-center gap-1 shrink-0">
            {#if auth.canMutate()}
              {#if selectedVm.state === 'running'}
                <Button
                  size="xs"
                  variant="outline"
                  disabled={busy === `${selectedVm.id}:shutdown`}
                  onclick={() => askShutdown(selectedVm)}
                  title={t('vms.shutdown')}
                >
                  <Icon name="power" size={12} class="mr-1 text-warning" />
                  <span class="hidden md:inline">{t('vms.shutdown')}</span>
                </Button>
                <Button
                  size="xs"
                  variant="outline"
                  disabled={busy === `${selectedVm.id}:reboot`}
                  onclick={() => restartVm(selectedVm)}
                  title={t('workstation.restart')}
                >
                  <Icon name="rotateCcw" size={12} class="mr-1" />
                  <span class="hidden md:inline">{t('workstation.restart')}</span>
                </Button>
                <Button
                  size="xs"
                  variant="outline"
                  class="text-destructive hover:text-destructive"
                  onclick={() => askForceOff(selectedVm)}
                  title={t('vms.forceOff')}
                >
                  <Icon name="x" size={12} class="mr-1" />
                  <span class="hidden md:inline">{t('vms.forceOff')}</span>
                </Button>
              {:else}
                <Button
                  size="xs"
                  variant="primary"
                  disabled={busy === `${selectedVm.id}:start`}
                  onclick={() => powerOn(selectedVm)}
                  title={t('multiview.powerOn')}
                >
                  <Icon name="play" size={12} class="mr-1" />
                  <span>{t('multiview.powerOn')}</span>
                </Button>
              {/if}

              <!-- Snapshot / ISO "⋯" menu -->
              <div class="relative" data-ws-menu>
                <button
                  class="p-1.5 rounded-lg text-muted-foreground hover:text-accent hover:bg-muted transition-colors {menuOpen
                    ? 'text-accent bg-muted'
                    : ''}"
                  title={t('multiview.moreActions')}
                  aria-label={t('multiview.moreActions')}
                  onclick={() => {
                    if (menuOpen) closeMenu();
                    else menuOpen = true;
                  }}
                >
                  <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
                    <circle cx="5" cy="12" r="1.6" />
                    <circle cx="12" cy="12" r="1.6" />
                    <circle cx="19" cy="12" r="1.6" />
                  </svg>
                </button>
                {#if menuOpen}
                  <div
                    class="absolute right-0 top-full mt-1 w-60 rounded-xl border border-border bg-popover text-popover-foreground p-1.5 shadow-2xl z-50 animate-fadeIn"
                    data-ws-menu
                  >
                    {#if submenu === null}
                      <button
                        class="flex items-center gap-2.5 w-full px-2.5 py-1.5 text-xs font-medium text-foreground rounded-lg hover:bg-accent/15 hover:text-accent transition-colors text-left disabled:opacity-50 disabled:pointer-events-none"
                        disabled={busy === `${selectedVm.id}:snapshot`}
                        onclick={() => createSnapshot(selectedVm)}
                      >
                        <Icon name="camera" size={14} class="text-accent" />
                        <span>{t('vmDetail.createSnapshot')}</span>
                      </button>
                      <button
                        class="flex items-center gap-2.5 w-full px-2.5 py-1.5 text-xs font-medium text-foreground rounded-lg hover:bg-accent/15 hover:text-accent transition-colors text-left"
                        onclick={() => openSnapshots(selectedVm)}
                      >
                        <Icon name="rotateCcw" size={14} class="text-warning" />
                        <span>{t('vmDetail.revert')}</span>
                        <Icon name="chevronRight" size={12} class="ml-auto text-muted-foreground" />
                      </button>
                      <button
                        class="flex items-center gap-2.5 w-full px-2.5 py-1.5 text-xs font-medium text-foreground rounded-lg hover:bg-accent/15 hover:text-accent transition-colors text-left"
                        onclick={() => openIso(selectedVm)}
                      >
                        <Icon name="disc" size={14} class="text-accent" />
                        <span>{t('vmDetail.changeIso')}</span>
                        <Icon name="chevronRight" size={12} class="ml-auto text-muted-foreground" />
                      </button>
                    {:else if submenu === 'snapshots'}
                      <button
                        class="flex items-center gap-2 w-full px-2.5 py-1.5 text-xs font-semibold text-muted-foreground hover:text-foreground rounded-lg hover:bg-muted transition-colors text-left border-b border-border mb-1 pb-2 rounded-b-none"
                        onclick={() => (submenu = null)}
                      >
                        <Icon name="chevronLeft" size={12} />
                        <span>{t('vmDetail.revert')}</span>
                      </button>
                      {#if snapshotsLoading}
                        <div class="flex items-center justify-center gap-2 py-3 text-xs text-muted-foreground">
                          <Spinner size="sm" />
                        </div>
                      {:else if snapshotList.length === 0}
                        <p class="px-2.5 py-2 text-xs text-muted-foreground">
                          {t('vmDetail.noSnapshots')}
                        </p>
                      {:else}
                        {#each snapshotList as snap (snap.name)}
                          <button
                            class="flex items-center gap-2 w-full px-2.5 py-1.5 text-xs text-foreground rounded-lg hover:bg-warning/15 hover:text-warning transition-colors text-left"
                            title={t('vms.snapRevertConfirmTitle')}
                            onclick={() => askRevert(snap)}
                          >
                            <Icon name="rotateCcw" size={12} class="shrink-0" />
                            <span class="truncate font-mono">{snap.name}</span>
                          </button>
                        {/each}
                      {/if}
                    {:else if submenu === 'iso'}
                      <button
                        class="flex items-center gap-2 w-full px-2.5 py-1.5 text-xs font-semibold text-muted-foreground hover:text-foreground rounded-lg hover:bg-muted transition-colors text-left border-b border-border mb-1 pb-2 rounded-b-none"
                        onclick={() => (submenu = null)}
                      >
                        <Icon name="chevronLeft" size={12} />
                        <span>{t('vmDetail.changeIso')}</span>
                      </button>
                      {#if !disks[selectedVm.id] || isosLoading}
                        <div class="flex items-center justify-center gap-2 py-3 text-xs text-muted-foreground">
                          <Spinner size="sm" />
                        </div>
                      {:else if !cdrom(selectedVm)}
                        <p class="px-2.5 py-2 text-xs text-muted-foreground flex items-start gap-2">
                          <Icon name="info" size={13} class="shrink-0 mt-0.5" />
                          <span>{t('multiview.noCdromHint')}</span>
                        </p>
                      {:else}
                        <p class="px-2.5 py-1 text-[10px] text-muted-foreground truncate">
                          {cdrom(selectedVm).source
                            ? t('vmDetail.currentIso', {
                                name: cdrom(selectedVm).source.split('/').pop(),
                              })
                            : t('vmDetail.noIsoAttached')}
                        </p>
                        {#each isoList || [] as iso (iso.path || iso.name)}
                          <button
                            class="flex items-center gap-2 w-full px-2.5 py-1.5 text-xs text-foreground rounded-lg hover:bg-accent/15 hover:text-accent transition-colors text-left disabled:opacity-50 disabled:pointer-events-none"
                            disabled={busy === `${selectedVm.id}:iso`}
                            onclick={() => setIso(selectedVm, iso.path || iso.name)}
                          >
                            <Icon name="disc" size={12} class="shrink-0" />
                            <span class="truncate">{iso.name || iso.path}</span>
                          </button>
                        {/each}
                        {#if cdrom(selectedVm).source}
                          <button
                            class="flex items-center gap-2 w-full px-2.5 py-1.5 text-xs text-foreground rounded-lg hover:bg-destructive/15 hover:text-destructive transition-colors text-left mt-1 border-t border-border pt-2 rounded-t-none disabled:opacity-50 disabled:pointer-events-none"
                            disabled={busy === `${selectedVm.id}:iso`}
                            onclick={() => setIso(selectedVm, '')}
                          >
                            <Icon name="x" size={12} class="shrink-0" />
                            <span>{t('vmDetail.isoEjected')}</span>
                          </button>
                        {/if}
                      {/if}
                    {/if}
                  </div>
                {/if}
              </div>
            {/if}

            <span class="w-px h-5 bg-border mx-0.5 shrink-0"></span>
            <button
              class="p-1.5 rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
              title={t('multiview.reconnectTitle')}
              onclick={() => reconnect(selectedVm)}
            >
              <Icon name="refresh" size={14} />
            </button>
            <button
              class="p-1.5 rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
              title={t('multiview.popoutTitle')}
              onclick={() => popoutConsole(selectedVm)}
            >
              <Icon name="externalLink" size={14} />
            </button>
            <button
              class="p-1.5 rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
              title={t('multiview.fullscreenTitle')}
              onclick={toggleFullscreen}
            >
              <Icon name={isFullscreen ? 'minimize' : 'maximize'} size={14} />
            </button>
          </div>
        </div>

        <!-- Console canvas -->
        <div class="flex-1 relative min-h-0 min-w-0 bg-black flex items-center justify-center overflow-hidden">
          {#if selectedVm.state === 'running'}
            {#if tickets[selectedVm.id]}
              <iframe
                src="/console/{selectedVm.id}?vt={encodeURIComponent(tickets[selectedVm.id])}&embedded=1"
                data-vm-id={selectedVm.id}
                title={t('multiview.consoleTitle', { name: selectedVm.name })}
                class="w-full h-full border-0 bg-black"
                allow="fullscreen; clipboard-read; clipboard-write"
              ></iframe>
            {:else if loadingTickets[selectedVm.id]}
              <div class="flex flex-col items-center gap-2 text-muted-foreground text-sm font-mono">
                <Spinner size="md" />
                <span>{t('multiview.connectingVnc')}</span>
              </div>
            {:else}
              <div class="flex flex-col items-center gap-3 p-8 text-center">
                <Button size="sm" variant="outline" onclick={() => fetchTicketFor(selectedVm.id)}>
                  <Icon name="refresh" size={13} class="mr-1" />
                  <span>{t('multiview.reconnectTitle')}</span>
                </Button>
              </div>
            {/if}
          {:else}
            <div class="flex flex-col items-center justify-center p-8 text-center gap-3">
              <div
                class="w-12 h-12 rounded-2xl bg-muted border border-border flex items-center justify-center text-muted-foreground shadow-inner"
              >
                <Icon name="power" size={24} />
              </div>
              <p class="text-sm font-bold text-foreground/80">
                {selectedVm.alias || selectedVm.name}
                — {stateLabel(selectedVm.state)}
              </p>
              {#if auth.canMutate()}
                <Button size="sm" onclick={() => powerOn(selectedVm)} disabled={busy === `${selectedVm.id}:start`}>
                  <Icon name="play" size={14} class="mr-1 text-success" />
                  <span>{t('multiview.powerOn')}</span>
                </Button>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
    </section>
  </div>
</div>

<!-- Shutdown confirmation -->
<ConfirmDialog
  bind:open={shutdownConfirmOpen}
  title={t('workstation.confirmShutdownTitle', { name: shutdownConfirmVm?.alias || shutdownConfirmVm?.name || '' })}
  description={t('workstation.confirmShutdownDesc')}
  confirmLabel={t('vms.shutdown')}
  loading={busy === `${shutdownConfirmVm?.id}:shutdown`}
  onConfirm={doShutdown}
/>

<!-- Force off confirmation (destructive, same copy as VmDetail) -->
<ConfirmDialog
  bind:open={forceOffConfirmOpen}
  title={t('vmDetail.forceOffTitle')}
  description={t('vmDetail.forceOffDesc')}
  confirmLabel={t('vms.forceOff')}
  variant="destructive"
  loading={busy === `${forceOffConfirmVm?.id}:forceoff`}
  onConfirm={doForceOff}
/>

<!-- Revert-to-snapshot confirmation (destructive: VM is shut off) -->
<ConfirmDialog
  bind:open={revertConfirmOpen}
  title={t('vms.snapRevertConfirmTitle')}
  description={t('vms.snapRevertConfirmDesc', { name: revertConfirmSnapshot?.name || '' })}
  confirmLabel={t('vmDetail.revert')}
  variant="destructive"
  loading={revertLoading}
  onConfirm={doRevert}
>
  {#if selectedVm?.state === 'running'}
    <p
      class="flex items-start gap-2 text-xs text-warning bg-warning/10 border border-warning/30 rounded-lg p-2.5"
    >
      <Icon name="alertTriangle" size={14} class="shrink-0 mt-0.5" />
      <span>{t('multiview.revertRunningWarn')}</span>
    </p>
  {/if}
</ConfirmDialog>

<style>
  @keyframes fadeIn {
    from {
      opacity: 0;
      transform: translateY(-4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
  .animate-fadeIn {
    animation: fadeIn 0.15s cubic-bezier(0.16, 1, 0.3, 1) forwards;
  }
</style>
