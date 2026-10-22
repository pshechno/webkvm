<script>
  import Alert from '$lib/components/Alert.svelte';
  import Spinner from '$lib/components/Spinner.svelte';
  import VmDetailSkeleton from '$lib/components/VmDetailSkeleton.svelte';
  import ProgressBar from '$lib/components/ProgressBar.svelte';
  import PasswordModal from '$lib/components/PasswordModal.svelte';
  import CredentialsModal from '$lib/components/CredentialsModal.svelte';
  import TerminalPanel from '$lib/components/TerminalPanel.svelte';
  import CronPicker from '$lib/components/CronPicker.svelte';
  import Gauge from '$lib/components/Gauge.svelte';
  import CpuFlagPicker from '$lib/components/CpuFlagPicker.svelte';
  import CpuPrioritySelector from '$lib/components/CpuPrioritySelector.svelte';
  import StatusBadge from '$lib/components/StatusBadge.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import BlockCard from '$lib/components/BlockCard.svelte';
  import SettingRow from '$lib/components/SettingRow.svelte';
  import PciPreflightPanel from '$lib/components/PciPreflightPanel.svelte';
  import PciGroupCard from '$lib/components/PciGroupCard.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import Checkbox from '$lib/components/Checkbox.svelte';
  import Tabs from '$lib/components/Tabs.svelte';
  import { upsertTask, updateTask, finishTask } from '$lib/stores/tasks.svelte.js';
  import { onMount, onDestroy } from 'svelte';
  import { fade } from 'svelte/transition';
  import { api, auth } from '$lib/stores/auth.svelte.js';
  import {
    capabilities,
    loadCapabilities,
    supports,
    unavailableReason,
  } from '$lib/stores/capabilities.svelte.js';
  import { t } from '../lib/i18n.svelte.js';
  import { bytesToStr, diskLabel, networkModels } from '$lib/components/vm-detail/utils.js';
  import VmSnapshotsCard from '$lib/components/vm-detail/VmSnapshotsCard.svelte';
  import VmDisksCard from '$lib/components/vm-detail/VmDisksCard.svelte';
  import VmNetCard from '$lib/components/vm-detail/VmNetCard.svelte';
  import VmUsbCard from '$lib/components/vm-detail/VmUsbCard.svelte';
  import VmPciCard from '$lib/components/vm-detail/VmPciCard.svelte';
  import VmSharedFoldersCard from '$lib/components/vm-detail/VmSharedFoldersCard.svelte';
  import VmCloudInitCard from '$lib/components/vm-detail/VmCloudInitCard.svelte';
  import VmFirewallCard from '$lib/components/vm-detail/VmFirewallCard.svelte';
  import VmHistoryCard from '$lib/components/vm-detail/VmHistoryCard.svelte';
  import VmAlertsCard from '$lib/components/vm-detail/VmAlertsCard.svelte';
  import VmGuestCard from '$lib/components/vm-detail/VmGuestCard.svelte';
  import VmHardwareCard from '$lib/components/vm-detail/VmHardwareCard.svelte';
  import VmScheduleCard from '$lib/components/vm-detail/VmScheduleCard.svelte';
  import { vmDiskPools, deployablePools, movablePools } from '$lib/purpose.js';
  import { isContainer } from '$lib/utils/computeType.js';
  import { networkLabel, networkLabelFor } from '$lib/utils/networkLabel.js';
  import { vmIps } from '$lib/utils/vmIps.js';
  import { formatRate, formatBytes } from '$lib/utils/format.js';
  import { events } from '$lib/stores/events.svelte.js';
  import { navigate, getRoute } from '$lib/router.svelte.js';
  import { toast } from '$lib/components/ui/toast';
  import { Button, buttonVariants } from '$lib/components/ui/button';
  import { cn } from '$lib/utils.js';
  import { Input } from '$lib/components/ui/input';
  import { Card } from '$lib/components/ui/card';
  import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
  import DeleteVmDialog from '$lib/components/DeleteVmDialog.svelte';
  import BatchCloneModal from '$lib/components/BatchCloneModal.svelte';
  import * as Dialog from '$lib/components/ui/dialog';
  import Chart from '$lib/components/Chart.svelte';
  import Switch from '$lib/components/Switch.svelte';
  import {
    Play,
    PowerOff,
    Power as PowerIcon,
    RotateCw,
    Pause,
    PlayCircle,
    Terminal,
    Trash2,
    CopyPlus,
    Pencil,
    Info,
    Download,
    KeyRound,
  } from '@lucide/svelte';

  let { vmId } = $props();

  let vm = $state(null);
  // v1.4 Fase 4: an LXD container has no KVM concepts (no chipset/UEFI/
  // TPM, no cdrom, no VNC, no per-disk attach). Sections and controls
  // below are gated on this derived flag (PLAN-LXD 5.4).
  const isContainerVm = $derived(vm && isContainer(vm));
  let snapshots = $state([]);
  let bootDevice = $state('hd');
  let loading = $state(true);
  let error = $state('');
  let actionLoading = $state('');
  // Firewall rules + port forwards (local editing state).
  let fwRules = $state([]);
  let fwForwards = $state([]);
  // VM metadata (template flag, owner, etc.).
  let vmMeta = $state(null);
  // Power & Snapshot schedule.
  let schedStart = $state('');
  let schedStop = $state('');
  let schedSnap = $state('');
  let schedSnapMax = $state(3);
  let schedSaving = $state(false);
  // Inline notice shown when the user tries an action that requires
  // the VM to be shut off while it isn't. Shown as a pop-up dialog.
  let blockedNotice = $state('');
  let showBlocked = $state(false);
  // Password reset modal.
  let showPasswordModal = $state(false);
  let resetUsername = $state('');
  let resetPasswordValue = $state('');
  // Reset password error dialog.
  let showResetError = $state(false);
  let resetError = $state('');
  // Derived: `actionLoading` is a string ('' when idle). Without this
  // coercion, `disabled={actionLoading}` becomes `disabled=""` in
  // the rendered HTML, which is truthy and dims every button even
  // when no action is in flight.
  const busy = $derived(!!actionLoading);

  // Autostart: separate flag from actionLoading because the
  // autostart toggle is a fast PATCH-style operation that
  // shouldn't dim the Start/Shutdown buttons while in flight.
  // The visual state lives on `vm.autostart` (the Switch
  // mirrors it via its `checked` prop); on failure we restore
  // the previous value.
  let autostartSaving = $state(false);

  // Metrics (Phase 21): in-memory time series per metric, updated via
  // SSE and on mount via a single REST fetch.
  let metrics = $state(null);
  const last60 = (arr) => (Array.isArray(arr) ? arr.slice(-60) : []);
  const cpuPoints = $derived(last60(metrics?.cpu?.points));
  const ramPoints = $derived(last60(metrics?.ram?.points));
  const diskRPoints = $derived(last60(metrics?.disk_read?.points));
  const diskWPoints = $derived(last60(metrics?.disk_write?.points));
  const netRxPoints = $derived(last60(metrics?.net_rx?.points));
  const netTxPoints = $derived(last60(metrics?.net_tx?.points));




  // Change ISO state
  let showChangeISO = $state(false);
  let cISOTarget = $state('');
  let cISOSource = $state('');
  // First cdrom of the VM — drives the quick ISO row on the overview
  // tab (null when the VM has no cdrom at all).
  const cdromDisk = $derived((vm?.disks || []).find((d) => d.device === 'cdrom') || null);

  // Clone state
  let showClone = $state(false);
  let showBatchClone = $state(false);
  let cName = $state('');
  let cPool = $state('webkvm-disks');
  let cLinked = $state(false);
  let cKeepMAC = $state(false);

  // Move-storage state. Unlike a clone this produces no new instance:
  // the same VM ends up with its disks in a different pool.
  let showMove = $state(false);
  let movePool = $state('');
  let moveProgress = $state(null); // { pct, stage } while the job runs
  // Where the instance's storage lives now, read off its first writable
  // disk. Used only to drop that pool from the destination list — the
  // backend is the authority and rejects a same-pool move outright.
  const currentPool = $derived(
    (vm?.disks || []).find((d) => d.device !== 'cdrom' && d.pool)?.pool || ''
  );
  // Only pools of the instance's own world are offered: a VM cannot
  // live in a container pool and vice versa.
  //
  // A VM flagged as a template gains one extra destination: the
  // template pools, which exist to keep golden images apart from the
  // disks in daily use. Containers never get them — Incus owns their
  // storage and knows nothing of a libvirt template pool.
  const movePoolOptions = $derived(
    [
      ...deployablePools(pools, isContainerVm ? 'container' : 'vm'),
      ...(vmMeta?.template && !isContainerVm ? movablePools(pools, 'template') : []),
    ].filter((p) => p.name !== currentPool)
  );

  // Export state
  let showExport = $state(false);
  let exportTarget = $state('vmware');
  let exportProgress = $state(null);
  let exportAbort = $state(null);

  // Identity / metadata state (Phase 16)
  let showIdentity = $state(false);
  let identityTab = $state('alias'); // 'alias' | 'cover' | 'network' | 'notes' | 'groups'
  let eAlias = $state('');
  let eNotes = $state('');
  let eNotesOriginal = $state(''); // tracks the last-saved value for blur autosave
  // Selected group names for this VM. A Set, not free text — a group
  // name can itself contain spaces (e.g. "APPS WEBS"), and the old
  // comma-or-space-separated text field silently split those into
  // separate, unregistered tags that never matched the real group.
  let eGroupsSet = $state(new Set());
  let eGroupsList = $state([]); // groups available to assign

  let coverFile = $state(null);
  let coverPreview = $state(null);
  let uploadingCover = $state(false);
  // The cover picker reads from the media library instead of keeping its
  // own private store of images: an image uploaded once should be usable
  // as a cover here, as an avatar, or both, without a second copy.
  let mediaItems = $state([]);
  let mediaLoading = $state(false);
  let mediaError = $state('');
  let vlanSupportByNetwork = $state({}); // networkName -> { supported, reason }
  let ifaceEdits = $state({}); // mac -> { mac, network, vlan, busy, error }
  let savingIdentity = $state(false);
  let notesStatus = $state(''); // '' | 'saving' | 'saved' | 'error'
  let notesError = $state('');

  // Confirm dialogs
  let confirmState = $state({
    open: false,
    title: '',
    description: '',
    confirmLabel: t('common.confirm'),
    variant: 'default',
    onConfirm: () => {},
    loading: false,
  });

  let pools = $state([]);
  let networks = $state([]);
  // Existing Linux bridges on the host (vmbr*/br*/...): the "host
  // bridges" group of the change-network picker. Fails soft to [].
  let hostBridges = $state([]);
  let isos = $state([]);
  // Current user's network allowlist (null until loaded). Empty/absent or
  // admin = every network; otherwise restricts the dropdowns below. Mirrors
  // VmCreate.svelte's myAllowedNetworks/vmNetworks exactly.
  let myAllowedNetworks = $state(null);
  const vmNetworks = $derived.by(() => {
    if (auth.role === 'admin') return networks;
    if (myAllowedNetworks && myAllowedNetworks.length) {
      return networks.filter((n) => myAllowedNetworks.includes(n.name));
    }
    return networks;
  });

  // Which of the 4 tabs is showing. 'overview' bundles the spec/metrics/
  // serial-console/firewall/schedule cards that used to be freestanding
  // blocks — see BlockCard/vmLayout.svelte.js.
  let activeSection = $state('overview');
  const sectionTabs = $derived(
    [
      { id: 'overview', label: t('vmDetail.overview') },
      { id: 'terminal', label: isContainerVm ? 'Terminal Shell' : 'Consola Serie' },
      { id: 'hardware', label: t('vmDetail.hardware') },
      !isContainerVm ? { id: 'cloudinit', label: 'Cloud-Init' } : null,
      { id: 'disks', label: t('vmDetail.disks') },
      { id: 'net', label: t('vmDetail.networkInterfaces') },
      // Guest-agent telemetry is a KVM-only concept: containers have
      // no qemu-guest-agent and the host sees their state directly.
      !isContainerVm ? { id: 'guest', label: t('vmDetail.guestTab') } : null,
      { id: 'history', label: t('vmDetail.history') },
      { id: 'alerts', label: t('vmDetail.alerts') },
      { id: 'snaps', label: t('vmDetail.snapshots') },
    ].filter(Boolean)
  );

  function gotoSerial() {
    activeSection = 'terminal';
    queueMicrotask(() => {
      document.getElementById('vm-serial-block')?.scrollIntoView({
        behavior: 'smooth',
        block: 'start',
      });
    });
  }

  async function openConsole(mode = 'vnc') {
    try {
      const { vnc_ticket } = await api.getVNCTicket(vmId);
      const modeParam = mode === 'spice' ? '&mode=spice' : '';
      window.open(
        `/console/${vmId}?vt=${encodeURIComponent(vnc_ticket)}${modeParam}`,
        '_blank',
        'noopener,noreferrer'
      );
    } catch (e) {
      toast.error(e.message);
    }
  }

  // Set while a delete is in flight / done: every loader and event
  // handler bails out so the page stops hitting endpoints of a VM that
  // no longer exists (/boot 500, /metrics 404) before navigating away.
  // Not $state — nothing renders from it.
  let deleting = false;
  let unsubscribeEvents = () => {};

  function onDeleteStart() {
    deleting = true;
  }
  function onDeleteFailed() {
    deleting = false;
  }
  function onVmDeleted() {
    deleting = true;
    unsubscribeEvents();
    clearAllTimers();
    navigate('/vms');
  }

  onMount(() => {
    // A route without an id (e.g. an inaccessible VM link) must not
    // fire /api/vms/undefined/* requests.
    if (!vmId) {
      loading = false;
      return;
    }
    loadCapabilities();
    load();
    loadMetrics();
    // Deep link: /vms/:id?serial=1 scrolls to the embedded serial console.
    if (getRoute().query?.serial === '1') later(gotoSerial, 400);
    const offMetrics = events.onVmMetrics((e) => {
      if (deleting || e.vm_id !== vmId) return;
      metrics = e.data;
    });

    // Subscribe to VM state events for this VM
    const off = events.onVmState((e) => {
      if (deleting || e.vm_id !== vmId) return;
      if (vm && vm.state !== e.state) {
        vm = { ...vm, state: e.state, name: e.name || vm.name };
        // Light refetch to update uptime/ip
        load(true);
      }
    });
    let unsubscribed = false;
    unsubscribeEvents = () => {
      if (unsubscribed) return;
      unsubscribed = true;
      off();
      offMetrics();
    };
    return unsubscribeEvents;
  });

  // One-shot timers (deep-link scroll, notes/export feedback)
  // are tracked and cancelled on unmount.
  let timers = [];

  function later(fn, ms) {
    const id = setTimeout(() => {
      timers = timers.filter((x) => x !== id);
      fn();
    }, ms);
    timers.push(id);
    return id;
  }

  function clearAllTimers() {
    for (const id of timers) clearTimeout(id);
    timers = [];
  }

  onDestroy(() => {
    unsubscribeEvents();
    clearAllTimers();
  });

  async function loadMetrics() {
    if (!vmId || deleting) return;
    try {
      metrics = await api.getVMMetrics(vmId);
    } catch {
      metrics = null;
    }
  }

  async function load(silent = false) {
    if (!vmId || deleting) return;
    if (!silent) loading = true;
    error = '';
    try {
      const [vmData, snapData, bootData] = await Promise.all([
        api.getVM(vmId),
        api.listSnapshots(vmId).catch(() => []),
        api.getBootDevice(vmId).catch(() => ({ boot_device: 'hd' })),
      ]);
      vm = vmData;
      snapshots = snapData;
      if (bootData) bootDevice = bootData.boot_device;
      api
        .getVMMeta(vmId)
        .then((m) => (vmMeta = m))
        .catch(() => {});
      api
        .getVMFirewall(vmId)
        .then((fw) => {
          fwRules = (fw.rules || []).map((r) => ({ ...r }));
          fwForwards = (fw.forwards || []).map((f) => ({ ...f }));
        })
        .catch(() => {});
      api
        .getVMSchedule(vmId)
        .then((sc) => {
          schedStart = sc.start_cron || '';
          schedStop = sc.stop_cron || '';
          schedSnap = sc.snapshot_cron || '';
          schedSnapMax = sc.snapshot_max || 3;
        })
        .catch(() => {});
      Promise.all([
        api
          .listNetworks()
          .then((n) => (networks = n))
          .catch(() => {}),
        api
          .listHostBridges()
          .then((b) => (hostBridges = b))
          .catch(() => (hostBridges = [])),
        api
          .listPools()
          .then((p) => (pools = p))
          .catch(() => {}),
        api
          .listISOs()
          .then((i) => (isos = i))
          .catch(() => {}),
        api
          .me()
          .then((me) => (myAllowedNetworks = me?.allowed_networks || []))
          .catch(() => (myAllowedNetworks = [])),
        api
          .getVMMeta(vmId)
          .then((m) => {
            // Cover changes are picked up by re-render, alias/groups
            // in vm are already populated from ListVMs. We only need
            // to keep them in sync after detail reloads.
            if (vm && m) {
              vm = { ...vm, alias: m.alias || '', cover: m.cover || '', groups: m.groups || [] };
            }
          })
          .catch(() => {}),
      ]);
    } catch (e) {
      if (
        e.status === 403 ||
        (e.message && (e.message.toLowerCase().includes('forbidden') || e.message.includes('403')))
      ) {
        toast.error(t('vmDetail.noAccess'));
        error = '';
        navigate('/vms');
        return;
      }
      error = e.message;
    } finally {
      if (!silent) loading = false;
    }
  }

  async function openIdentity() {
    identityTab = 'alias';
    coverFile = null;
    coverPreview = null;
    notesStatus = '';
    notesError = '';
    try {
      const meta = await api.getVMMeta(vmId);
      eAlias = meta.alias || '';
      eNotes = meta.notes || '';
      eNotesOriginal = meta.notes || '';
      eGroupsSet = new Set(meta.groups || []);
    } catch {
      eAlias = vm?.alias || '';
      eNotes = '';
      eNotesOriginal = '';
      eGroupsSet = new Set(vm?.groups || []);
    }
    // Initialize iface edit state for each network interface.
    const edits = {};
    for (const iface of vm.networks || []) {
      edits[iface.mac] = {
        mac: iface.mac,
        network: iface.network,
        vlan: '',
        busy: false,
        error: '',
      };
    }
    ifaceEdits = edits;
    vlanSupportByNetwork = {};
    // Pre-fetch VLAN support for each network this VM uses.
    const uniqueNets = [...new Set((vm.networks || []).map((n) => n.network).filter(Boolean))];
    await Promise.all(
      uniqueNets.map(async (net) => {
        try {
          vlanSupportByNetwork[net] = await api.checkVLANSupport(net);
        } catch (e) {
          vlanSupportByNetwork[net] = { supported: false, reason: e.message };
        }
      })
    );
    // Load available groups (for the alias/cover tab also has a groups list).
    try {
      const grp = await api.listGroups();
      eGroupsList = grp.groups || [];
    } catch {
      eGroupsList = [];
    }
    showIdentity = true;
    if (identityTab === 'cover') loadMediaLibrary();
  }

  async function saveIdentityBasics() {
    // Save alias + notes + groups in a single PUT.
    savingIdentity = true;
    try {
      const groups = Array.from(eGroupsSet);
      await api.updateVMMeta(vmId, {
        alias: eAlias,
        notes: eNotes,
        groups: groups,
      });
      eNotesOriginal = eNotes;
      vm = { ...vm, alias: eAlias, groups };
      toast.success('Identity updated');
      // Close on success — leaving the dialog open with no visible
      // change (besides a toast easy to miss) read as "the button
      // doesn't do anything" even though the save worked. On error,
      // stay open so the message and fields remain visible to retry.
      showIdentity = false;
    } catch (e) {
      toast.error(e.message);
    } finally {
      savingIdentity = false;
    }
  }

  // Notes blur-autosave: only fires if the value actually changed since
  // the last save (openIdentity / successful save updates eNotesOriginal).
  async function saveNotesIfChanged() {
    if (eNotes === eNotesOriginal) return;
    notesStatus = 'saving';
    notesError = '';
    try {
      await api.updateVMMeta(vmId, { notes: eNotes });
      eNotesOriginal = eNotes;
      notesStatus = 'saved';
      later(() => {
        if (notesStatus === 'saved') notesStatus = '';
      }, 2000);
    } catch (e) {
      notesStatus = 'error';
      notesError = e.message;
    }
  }

  function onCoverPicked(e) {
    const f = e.target.files?.[0];
    if (!f) return;
    coverFile = f;
    const reader = new FileReader();
    reader.onload = () => (coverPreview = reader.result);
    reader.readAsDataURL(f);
  }

  async function loadMediaLibrary() {
    if (mediaLoading) return;
    mediaLoading = true;
    mediaError = '';
    try {
      const res = await api.listMedia();
      const items = Array.isArray(res) ? res : res?.items || [];
      // Only raster formats make sense as a cover, matching what the
      // upload path accepts.
      mediaItems = items.filter((i) => !/\.(svg|ico)$/i.test(i.id || ''));
    } catch (e) {
      mediaError = e.message;
    } finally {
      mediaLoading = false;
    }
  }

  // Picking an existing image never copies bytes: it stores a reference
  // to the library item, so deleting or reusing it stays coherent.
  async function pickCoverFromLibrary(item) {
    uploadingCover = true;
    try {
      const res = await api.applyMediaUsage({
        media_id: item.id,
        usage: 'vm_cover',
        vm_id: vmId,
      });
      vm = { ...vm, cover: res.url };
      coverFile = null;
      coverPreview = null;
      toast.success(t('vmDetail.coverUpdated'));
    } catch (e) {
      toast.error(e.message);
    } finally {
      uploadingCover = false;
    }
  }

  // Uploading from the cover tab goes through the media library, so the
  // image shows up in Multimedia like any other. The old per-VM upload
  // wrote to a separate directory that nothing else could see.
  async function uploadCover() {
    if (!coverFile) return;
    uploadingCover = true;
    try {
      const item = await api.uploadMedia(coverFile);
      const res = await api.applyMediaUsage({
        media_id: item.id,
        usage: 'vm_cover',
        vm_id: vmId,
      });
      vm = { ...vm, cover: res.url };
      toast.success(t('vmDetail.coverUpdated'));
      coverFile = null;
      coverPreview = null;
      loadMediaLibrary();
    } catch (e) {
      toast.error(e.message);
    } finally {
      uploadingCover = false;
    }
  }

  async function removeCover() {
    try {
      await api.deleteCover(vmId);
      vm = { ...vm, cover: '' };
      toast.success(t('vmDetail.coverRemoved'));
    } catch (e) {
      toast.error(e.message);
    }
  }

  async function saveIface(mac) {
    const cur = ifaceEdits[mac];
    if (!cur) return;
    const newMac = cur.mac.trim();
    if (newMac && newMac !== mac) {
      if (!requireShutoff(t('vmDetail.requireShutoffEditingNic'))) return;
    }
    cur.error = '';
    cur.busy = true;
    ifaceEdits = { ...ifaceEdits };
    const vlanRaw = cur.vlan.trim();
    let vlanTag = null;
    if (vlanRaw !== '') {
      const n = parseInt(vlanRaw, 10);
      if (isNaN(n) || n < 0 || n > 4094) {
        cur.error = t('vmDetail.vlanTagRange');
        cur.busy = false;
        ifaceEdits = { ...ifaceEdits };
        return;
      }
      vlanTag = n;
    }
    const payload = {};
    if (newMac && newMac !== mac) payload.mac = newMac;
    if (cur.network && cur.network !== (vm.networks.find((i) => i.mac === mac)?.network || '')) {
      payload.network = cur.network;
    }
    if (vlanTag !== null) payload.vlan_tag = vlanTag;
    if (Object.keys(payload).length === 0) {
      cur.busy = false;
      ifaceEdits = { ...ifaceEdits };
      toast.info(t('vmDetail.nothingToSave'));
      return;
    }
    try {
      await api.updateNetIface(vmId, mac, payload);
      toast.success(t('vmDetail.networkInterfaceUpdated'));
      await load();
      // Re-seed the edit state for the (possibly new) MAC.
      const updatedIface = (vm.networks || []).find(
        (i) => i.mac === newMac || i.mac === cur.network
      );
      if (newMac && newMac !== mac) {
        delete ifaceEdits[mac];
      }
      if (updatedIface) {
        ifaceEdits[updatedIface.mac] = {
          mac: updatedIface.mac,
          network: updatedIface.network,
          vlan: '',
          busy: false,
          error: '',
        };
      }
    } catch (e) {
      // Revert: keep the form value the user typed, but show the error.
      cur.error = e.message;
    } finally {
      cur.busy = false;
      ifaceEdits = { ...ifaceEdits };
    }
  }

  async function toggleLinkState(iface) {
    actionLoading = 'netlink';
    try {
      const nextState = iface.link_state === 'down' ? 'up' : 'down';
      await api.updateNetIface(vmId, iface.mac, { link_state: nextState });
      toast.success(nextState === 'up' ? 'Cable de red conectado' : 'Cable de red desconectado');
      await load();
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }

  // Model change redefines the NIC, so it only works on a shut-off VM.
  // The select is disabled while running; requireShutoff is the belt to
  // that suspenders.
  async function changeNetModel(iface) {
    if (!iface?.mac) return;
    if (!requireShutoff(t('vmDetail.requireShutoffChangeNetModel'))) {
      await load(); // restore the previous model in the select
      return;
    }
    actionLoading = 'netmodel';
    try {
      await api.updateNetIface(vmId, iface.mac, { model: iface.model });
      toast.success(t('vmDetail.netModelChanged'));
      await load();
    } catch (e) {
      toast.error(e.message);
      await load();
    } finally {
      actionLoading = '';
    }
  }

  // Change the interface's source (WebKVM network or host bridge). The
  // PATCH takes just { network: <name> }: the backend decides libvirt
  // network vs Linux bridge by probing the name on the host. Network and
  // bridge changes apply live on a running VM (with a transparent
  // detach+attach fallback), so on a running VM we confirm first — the
  // interface is reconnected and connectivity drops for a moment.
  async function applyNetNetwork(iface, value) {
    const sep = value.indexOf(':');
    const kind = value.slice(0, sep);
    const name = value.slice(sep + 1);
    // The PATCH carries the plain name either way: the backend switches
    // between <source network='…'/> and <source bridge='…'/> by probing
    // whether the name is a real Linux bridge on the host.
    actionLoading = 'netchange-' + iface.mac;
    try {
      await api.updateNetIface(vmId, iface.mac, { network: name });
      toast.success(t('vmDetail.networkChanged'));
      await load();
    } catch (e) {
      toast.error(e.message);
      await load(); // restore the previous source in the select
    } finally {
      actionLoading = '';
    }
  }

  function changeNetNetwork(iface, value) {
    if (!iface?.mac || !value) return;
    if (vm.state === 'running' || vm.state === 'paused') {
      askConfirm({
        title: t('vmDetail.changeNetworkTitle'),
        description: t('vmDetail.changeNetworkDesc'),
        confirmLabel: t('common.confirm'),
        onConfirm: async () => {
          confirmState.loading = true;
          await applyNetNetwork(iface, value);
          confirmState.open = false;
          confirmState.loading = false;
        },
      });
      return;
    }
    applyNetNetwork(iface, value);
  }

  function askConfirm(opts) {
    confirmState = { ...opts, open: true, loading: false };
  }

  // The edit form lives in VmHardwareCard. These request an edit
  // session (seeding the form from the current VM) and jump to the
  // hardware tab, as the inline editor always did.
  let hwEditRequest = $state(null);
  function openEdit() {
    activeSection = 'hardware';
    hwEditRequest = { tab: 'general', n: (hwEditRequest?.n || 0) + 1 };
  }
  function openEditTab(tab = 'general') {
    activeSection = 'hardware';
    hwEditRequest = { tab, n: (hwEditRequest?.n || 0) + 1 };
  }

  // Delete flow: see lib/components/DeleteVmDialog.svelte. VmDetail only
  // owns the trigger flag and the disks it passes in as a prop.
  let showDeleteFlow = $state(false);
  const vmDisks = $derived(vm?.disks?.filter((d) => d.device === 'disk') ?? []);

  let showAppCreds = $state(false);

  // Visible diagnostics: any render/JS error shows as a toast (this is
  // how the "JS: TypeError ..." overlay originated). Keeping it in one
  // effect avoids the never-assigned boundaryMsg/boundaryStack block
  // that used to render a second, dead error panel.
  $effect(() => {
    const onErr = (e) => {
      const msg = (e && (e.message || e.reason)) || 'error desconocido';
      console.error('[WebKVM UI]', e);
      try {
        toast.error('JS: ' + msg, { duration: 0 });
      } catch (_e) {
        // ignore toast error
      }
    };
    window.addEventListener('error', onErr);
    window.addEventListener('unhandledrejection', onErr);
    return () => {
      window.removeEventListener('error', onErr);
      window.removeEventListener('unhandledrejection', onErr);
    };
  });
  let appCreds = $state(null);
  // Embedded serial console panel (lazy: WS opens on toggle / ?serial=1).

  // App credentials only exist for VMs deployed from an appliance;
  // vmMeta.app_info is loaded by load().
  const appInfo = $derived.by(() => {
    if (!vmMeta?.app_info) return null;
    try {
      return JSON.parse(vmMeta.app_info);
    } catch {
      return null;
    }
  });

  function showAppCredentials() {
    if (appInfo) {
      appCreds = appInfo;
      showAppCreds = true;
    }
  }

  async function resetPassword() {
    actionLoading = 'resetPassword';
    try {
      const result = await api.resetVMPassword(vm.id);
      resetUsername = result.username || 'admin';
      resetPasswordValue = result.password;
      showPasswordModal = true;
    } catch (e) {
      resetError = e.message;
      showResetError = true;
    } finally {
      actionLoading = '';
    }
  }

  async function doAction(action) {
    if (action === 'forceOffVM') {
      askConfirm({
        title: t('vmDetail.forceOffTitle'),
        description: t('vmDetail.forceOffDesc'),
        confirmLabel: t('vmDetail.forceOff'),
        variant: 'destructive',
        onConfirm: async () => {
          // Unlike every other confirm-dialog action here, this one
          // never toggled confirmState.loading — the Confirm button
          // stayed enabled with no spinner for the whole call (up to
          // 30s for forceRebootVM, which waits for shutoff before
          // restarting), looking exactly like "accepted the click but
          // never closes". doActionRun still closes the dialog itself
          // on success; this only covers the wait and the error case.
          confirmState.loading = true;
          await doActionRun(action);
          confirmState.loading = false;
        },
      });
      return;
    }
    if (action === 'forceRebootVM') {
      askConfirm({
        title: t('vmDetail.forceRebootTitle'),
        description: t('vmDetail.forceRebootDesc'),
        confirmLabel: t('vmDetail.forceReboot'),
        variant: 'destructive',
        onConfirm: async () => {
          confirmState.loading = true;
          await doActionRun(action);
          confirmState.loading = false;
        },
      });
      return;
    }
    if (action === 'deleteVM') {
      showDeleteFlow = true;
      return;
    }
    await doActionRun(action);
  }

  // toggleAutostart flips libvirtd's per-VM autostart flag. It
  // is intentionally separate from doActionRun (which dims the
  // whole Actions card) — the round-trip is fast and the user
  // reported that accidentally toggling autostart was the
  // annoyance that motivated this control, so a quick, narrow
  // visual confirmation is the right feedback.
  //
  // On failure we restore the previous value (the Switch's
  // local `checked` already flipped optimistically via the
  // Switch's onclick handler; we re-set it from the still-
  // unchanged vm.autostart). The toast is sticky so the user
  // doesn't miss the libvirt error.
  async function toggleAutostart(next) {
    if (!vm) return;
    const previous = vm.autostart;
    autostartSaving = true;
    try {
      await api.setVMAutostart(vmId, next);
      // Reflect the new value on the VM object so any other
      // UI reading vm.autostart (e.g. a future list view
      // badge) stays in sync.
      vm = { ...vm, autostart: next };
      toast.success(next ? t('vmDetail.autostartEnabled') : t('vmDetail.autostartDisabled'), {
        duration: 3000,
      });
    } catch (e) {
      // Roll the Switch back to the previous value.
      vm = { ...vm, autostart: previous };
      toast.error(t('vmDetail.setAutostartFailed', { error: e.message }), { duration: 0 });
    } finally {
      autostartSaving = false;
    }
  }

  async function doActionRun(action) {
    actionLoading = action;
    try {
      if (action === 'forceRebootVM') {
        await api.forceOffVM(vmId);
        await waitForState('shutoff', 30000);
        await api.startVM(vmId);
      } else {
        await api[action](vmId);
      }
      const labels = {
        startVM: t('vmDetail.actionStarted'),
        shutdownVM: t('vmDetail.actionShutDown'),
        rebootVM: t('vmDetail.actionRebooted'),
        forceRebootVM: t('vmDetail.actionForceRebooted'),
        suspendVM: t('vmDetail.actionSuspended'),
        resumeVM: t('vmDetail.actionResumed'),
        forceOffVM: t('vmDetail.actionForceOff'),
      };
      toast.success(
        t('vmDetail.vmActionToast', { label: labels[action] || t('vmDetail.actionUpdated') })
      );
      confirmState.open = false;
      await load();
    } catch (e) {
      toast.error(e.message, { duration: 0 });
    } finally {
      actionLoading = '';
    }
  }

  async function waitForState(targetState, timeoutMs = 30000) {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      const cur = await api.getVM(vmId);
      if (cur.state === targetState) return;
      await new Promise((r) => setTimeout(r, 500));
    }
    throw new Error(t('vmDetail.timeoutState', { state: targetState }));
  }

  // Admin-only "share with all users" flag on a template. Same
  // optimistic pattern as toggleAutostart: the Switch has already
  // flipped locally; on failure we restore vmMeta.shared.
  let shareSaving = $state(false);
  async function toggleShared(next) {
    if (!vmId || !vmMeta?.template) return;
    const previous = !!vmMeta.shared;
    shareSaving = true;
    try {
      const m = await api.updateVMMeta(vmId, { shared: next });
      vmMeta = m && typeof m === 'object' ? m : { ...vmMeta, shared: next };
      toast.success(next ? t('vmDetail.shareTemplateOn') : t('vmDetail.shareTemplateOff'), {
        duration: 3000,
      });
    } catch (e) {
      vmMeta = { ...vmMeta, shared: previous };
      toast.error(t('vmDetail.shareTemplateFailed', { error: e.message }), { duration: 0 });
    } finally {
      shareSaving = false;
    }
  }

  // Eject-ISO-on-shutdown flag lives in VM meta. Older backends may not
  // know the field yet: they either return the meta unchanged or reject
  // the PUT — both are handled without breaking the toggle.
  async function toggleEjectIsoOnShutdown(next) {
    if (!vmId) return;
    const previous = !!vmMeta?.eject_iso_on_shutdown;
    vmMeta = { ...(vmMeta || {}), eject_iso_on_shutdown: next };
    actionLoading = 'ejectmeta';
    try {
      const m = await api.updateVMMeta(vmId, { eject_iso_on_shutdown: next });
      vmMeta = m && typeof m === 'object' ? m : { ...vmMeta, eject_iso_on_shutdown: next };
      toast.success(t('vmDetail.ejectIsoOnShutdownSaved'), { duration: 3000 });
    } catch (e) {
      vmMeta = { ...(vmMeta || {}), eject_iso_on_shutdown: previous };
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }

  async function toggleTemplate() {
    if (!vmId || vm.state !== 'shutoff') return;
    actionLoading = 'template';
    try {
      if (vmMeta?.template) {
        await api.unsetVMTemplate(vmId);
        toast.success(t('vmDetail.unsetTemplateDone'));
      } else {
        await api.makeVMTemplate(vmId);
        toast.success(t('vmDetail.makeTemplateDone'));
      }
      const m = await api.getVMMeta(vmId);
      vmMeta = m;
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }

  async function saveSchedule() {
    if (!vmId) return;
    schedSaving = true;
    try {
      await api.setVMSchedule(vmId, {
        start_cron: schedStart.trim(),
        stop_cron: schedStop.trim(),
        snapshot_cron: schedSnap.trim(),
        snapshot_max: Math.min(999, Math.max(1, Number(schedSnapMax) || 3)),
      });
      toast.success(t('vmDetail.scheduleSaved'));
    } catch (e) {
      toast.error(e.message);
    } finally {
      schedSaving = false;
    }
  }

  // Returns true on success so the snapshots card can clear its form
  // (the original inline version cleared it only after a successful
  // create). On error the toast is shown and the form keeps its values.
  async function createSnapshot(payload) {
    actionLoading = 'snapshot';
    try {
      const res = await api.createSnapshot(vmId, {
        name: payload.name,
        description: payload.description,
        memory: payload.memory,
      });
      await api.waitJob(res.job, {
        title: `Creando instantánea: ${payload.name} (${vm?.name || vmId})`,
        kind: 'snapshot',
      });
      toast.success(t('vmDetail.snapshotCreated'));
      await load();
      return true;
    } catch (e) {
      toast.error(e.message);
      return false;
    } finally {
      actionLoading = '';
    }
  }

  async function revertSnapshot(sid, isCurrent = false) {
    if (!requireShutoff(t('vmDetail.requireShutoffRevertingSnapshot'))) return;
    askConfirm({
      title: t('vmDetail.revertSnapshotTitle'),
      // Reverting the "current" snapshot rolls back to the latest one —
      // spell that out so the operator isn't surprised.
      description: isCurrent
        ? t('vmDetail.revertCurrentSnapshotDesc')
        : t('vmDetail.revertSnapshotDesc'),
      confirmLabel: t('vmDetail.revert'),
      variant: 'destructive',
      onConfirm: async () => {
        confirmState.loading = true;
        try {
          await api.revertSnapshot(vmId, sid);
          confirmState.open = false;
          toast.success(t('vmDetail.reverted'));
          await load();
        } catch (e) {
          toast.error(e.message);
          confirmState.loading = false;
        }
      },
    });
  }

  function deleteSnapshot(sid) {
    askConfirm({
      title: t('vmDetail.deleteSnapshotTitle'),
      description: t('vmDetail.deleteSnapshotDesc'),
      confirmLabel: t('common.delete'),
      variant: 'destructive',
      onConfirm: async () => {
        confirmState.loading = true;
        try {
          await api.deleteSnapshot(vmId, sid);
          confirmState.open = false;
          toast.success(t('vmDetail.snapshotDeleted'));
          await load();
        } catch (e) {
          toast.error(e.message);
          confirmState.loading = false;
        }
      },
    });
  }

  async function changeISO() {
    if (!cISOTarget) return;
    actionLoading = 'changeiso';
    try {
      await api.updateDiskSource(vmId, cISOTarget, cISOSource);
      showChangeISO = false;
      toast.success(t('vmDetail.isoChanged'));
      await load();
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }

  // Eject = point the cdrom's source at nothing (empty source), the
  // same UpdateDisk endpoint the Change ISO dialog uses.
  async function ejectISO(dev) {
    actionLoading = 'ejectiso';
    try {
      await api.updateDiskSource(vmId, dev, '');
      toast.success(t('vmDetail.isoEjected'));
      await load();
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }

  // requireShutoff gates disk operations that cannot be applied to a
  // running VM (resize uses qemu-img on a live disk; detach risks the
  // guest still having the device mounted). Returns false and explains
  // why when the VM isn't shut off.
  function requireShutoff(action) {
    if (!vm) return false;
    if (vm.state !== 'shutoff') {
      const msg = t('vmDetail.requireShutoffMsg', { action, state: vm.state });
      blockedNotice = msg;
      showBlocked = true;
      toast.warning(msg, { duration: 5000 });
      return false;
    }
    return true;
  }

  // Once the VM is actually shut off, close the pop-up and clear the notice.
  $effect(() => {
    if (vm && vm.state === 'shutoff') {
      blockedNotice = '';
      showBlocked = false;
    }
  });

  function removeDisk(target) {
    askConfirm({
      title: t('vmDetail.removeDiskTitle', { target }),
      description: t('vmDetail.removeDiskDesc'),
      confirmLabel: t('vmDetail.remove'),
      variant: 'destructive',
      onConfirm: async () => {
        confirmState.loading = true;
        try {
          await api.deleteDisk(vmId, target);
          confirmState.open = false;
          toast.success(t('vmDetail.diskRemoved'));
          await load();
        } catch (e) {
          toast.error(e.message);
          confirmState.loading = false;
        }
      },
    });
  }

  function removeNet(mac) {
    askConfirm({
      title: t('vmDetail.removeNetTitle'),
      description: t('vmDetail.removeNetDesc', { mac }),
      confirmLabel: t('vmDetail.remove'),
      variant: 'destructive',
      onConfirm: async () => {
        confirmState.loading = true;
        try {
          await api.deleteNetIface(vmId, mac);
          confirmState.open = false;
          toast.success(t('vmDetail.netRemoved'));
          await load();
        } catch (e) {
          toast.error(e.message);
          confirmState.loading = false;
        }
      },
    });
  }

  async function cloneVM() {
    if (!cName) return;
    if (!requireShutoff(t('vmDetail.requireShutoffCloning'))) return;
    actionLoading = 'clone';
    try {
      const res = await api.cloneVM(vmId, {
        name: cName,
        pool: cPool,
        linked: cLinked,
        keep_mac: cKeepMAC,
      });
      const cloned = await api.waitJob(res.job, {
        title: `Clonando VM: ${vm?.name || vmId} ➔ ${cName}`,
        kind: 'clone',
      });
      showClone = false;
      toast.success(t('vmDetail.vmCloned', { name: cloned?.name || cName }));
      await load();
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
    }
  }

  // Stage names come from the backend as stable identifiers. Mapping
  // them explicitly (rather than interpolating into t()) keeps every
  // key greppable, so check-i18n can still tell used from orphaned.
  const MOVE_STAGES = {
    move_preparing: 'vmDetail.movePreparing',
    move_copying: 'vmDetail.moveCopying',
    move_cleanup: 'vmDetail.moveCleanup',
    move_done: 'vmDetail.moveDone',
  };
  function moveStageLabel(stage) {
    const key = MOVE_STAGES[stage];
    return key ? t(key) : '';
  }

  // Move this instance's storage to another pool. Cold only: the disk
  // is copied byte-for-byte, which cannot be done safely underneath a
  // running hypervisor process.
  async function moveStorage() {
    if (!movePool) return;
    if (!requireShutoff(t('vmDetail.requireShutoffMoving'))) return;
    actionLoading = 'move';
    moveProgress = { pct: 0, stage: 'move_preparing' };
    try {
      const res = await api.moveVMStorage(vmId, movePool);
      await api.waitJob(res.id, {
        title: `Moviendo almacenamiento: ${vm?.name || vmId} ➔ ${movePool}`,
        kind: 'storage',
        // A cross-device copy of a large disk can run well past the
        // default 10-minute ceiling; a timeout here would report
        // failure on a move that is still progressing fine.
        timeout: 4 * 60 * 60 * 1000,
        onPoll: (job) => {
          moveProgress = { pct: job.progress || 0, stage: job.stage || '' };
        },
      });
      showMove = false;
      toast.success(t('vmDetail.storageMoved', { pool: movePool }));
      await load();
    } catch (e) {
      toast.error(e.message);
    } finally {
      actionLoading = '';
      moveProgress = null;
    }
  }

  // Export
  function exportVM() {
    // Containers can export while running (Incus daemon handles it natively).
    // KVM VMs must be shut off.
    if (!isContainerVm && !requireShutoff(t('vmDetail.requireShutoffExporting'))) return;
    // Default target differs by type: containers default to Incus backup,
    // KVM VMs default to VMware OVA.
    exportTarget = isContainerVm ? 'backup' : 'vmware';
    exportProgress = null;
    showExport = true;
  }

  // Download a .rdp / .vv console file with the session cookie
  // (credentials: include — never a token in the URL), then save it as a blob.
  async function downloadConsoleFile(kind) {
    const url = kind === 'rdp' ? api.getRDPUrl(vmId) : api.getSPICEUrl(vmId);
    try {
      const res = await fetch(url, {
        credentials: 'include',
      });
      if (!res.ok)
        throw new Error((await res.json().catch(() => ({}))).error || `HTTP ${res.status}`);
      const blob = await res.blob();
      const objectUrl = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = objectUrl;
      a.download = kind === 'rdp' ? `${vm.name || vmId}.rdp` : `${vm.name || vmId}.vv`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(objectUrl);
    } catch (e) {
      toast.error(e.message);
    }
  }

  async function startExport() {
    // Containers can export while running; KVM VMs need to be shut off.
    if (!isContainerVm && !requireShutoff(t('vmDetail.requireShutoffExporting'))) {
      return;
    }
    // The export dialog's confirm button keys off actionLoading ===
    // 'export'; without this it was never set, so the button stayed
    // enabled and a double-click fired a second export on top of the
    // first.
    actionLoading = 'export';
    try {
      await performExport();
    } finally {
      actionLoading = '';
    }
  }

  async function performExport() {
    let label;
    if (exportTarget === 'backup') {
      label = isContainerVm
        ? t('vmDetail.exportIncusBackupLabel')
        : t('vmDetail.exportBuildingBackup');
    } else if (exportTarget === 'proxmox') {
      label = t('vmDetail.exportBuildingProxmox');
    } else {
      label = t('vmDetail.exportBuildingOva', { target: exportTarget });
    }
    exportProgress = { received: 0, total: 0, percent: 0, label };
    const taskId = 'export:' + vm.name;
    upsertTask({
      id: taskId,
      kind: 'export',
      title: vm.name,
      pct: 0,
      message: label,
      status: 'running',
    });
    const ac = new AbortController();
    exportAbort = ac;
    try {
      const opts = {
        signal: ac.signal,
        onProgress: (p) => {
          exportProgress = { received: p.received, total: p.total, percent: p.percent, label };
          updateTask(taskId, {
            pct: Math.round(p.percent || 0),
            message: label,
          });
        },
      };
      if (exportTarget === 'backup') {
        opts.format = 'backup';
        opts.compress = true;
      } else if (exportTarget === 'proxmox') {
        opts.format = 'proxmox';
      } else {
        opts.format = 'ova';
        opts.target = exportTarget;
      }
      const result = await api.exportVM(vm.name, opts);
      exportProgress = {
        received: result.size,
        total: result.size,
        percent: 100,
        label: t('vmDetail.exportDone'),
      };
      finishTask(taskId, 'success', t('vmDetail.exportComplete'), 100);
      toast.success(t('vmDetail.exportComplete'));
      later(() => {
        showExport = false;
        exportProgress = null;
        exportAbort = null;
      }, 800);
    } catch (e) {
      if (e.name === 'AbortError') {
        exportProgress = {
          received: 0,
          total: 0,
          percent: 0,
          label: t('vmDetail.exportCancelled'),
        };
        finishTask(taskId, 'error', t('vmDetail.exportCancelled'), 0);
        later(() => {
          showExport = false;
          exportProgress = null;
          exportAbort = null;
        }, 600);
      } else {
        finishTask(taskId, 'error', e.message, exportProgress?.percent || 0);
        toast.error(e.message);
        showExport = false;
        exportProgress = null;
        exportAbort = null;
      }
    }
  }

  function cancelExport() {
    if (exportAbort) exportAbort.abort();
  }

  function onBack() {
    navigate('/vms');
  }

  function formatUptime(s) {
    if (!s) return '—';
    const d = Math.floor(s / 86400),
      h = Math.floor((s % 86400) / 3600),
      m = Math.floor((s % 3600) / 60);
    const parts = [];
    if (d) parts.push(d + 'd');
    if (h) parts.push(h + 'h');
    if (m) parts.push(m + 'm');
    return parts.join(' ') || '<1m';
  }

  // Build a snapshot tree from the flat list. Roots have parent_name == "".
  // (buildSnapshotTree/formatSnapshotDate/bytesToStr live in
  // lib/components/vm-detail/utils.js, shared with the section cards.)

  // Deep-link from external routes: ?tab=snapshots or ?tab=hardware
  $effect(() => {
    const r = getRoute();
    if (r.query?.tab === 'snapshots') activeSection = 'snaps';
    if (r.query?.tab === 'hardware') activeSection = 'hardware';
  });
</script>

<div class="p-3 sm:p-5 w-full max-w-[1700px] mx-auto">
  <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
    <div class="flex items-center gap-3 flex-wrap min-w-0 flex-1">
      <button
        onclick={onBack}
        class="p-1.5 rounded-md hover:bg-muted text-muted-foreground hover:text-foreground transition-colors shrink-0"
        aria-label="Back"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
          <polyline points="15 18 9 12 15 6" />
        </svg>
      </button>
      {#if vm}
        <h1 class="text-xl font-semibold tracking-tight truncate min-w-0">{vm.alias || vm.name}</h1>
        {#if vm.alias && vm.alias !== vm.name}
          <span class="text-xs text-muted-foreground font-mono truncate shrink-0">({vm.name})</span>
        {/if}
        <StatusBadge state={vm.state} size="sm" class="shrink-0" />
        {#if vm.state === 'running' && vmIps(vm).length}
          <span
            class="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-accent/10 border border-accent/20 text-accent font-mono truncate shrink-0 max-w-[24rem]"
          >
            {vmIps(vm).join(', ')}
          </span>
        {/if}
        {#if vmMeta?.template}
          <span
            class="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-warning/10 border border-warning/30 text-warning font-medium shrink-0 whitespace-nowrap"
          >
            {t('vmDetail.templateBadge')}
          </span>
          {#if vmMeta?.shared}
            <span
              class="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-accent/10 border border-accent/20 text-accent font-medium shrink-0 whitespace-nowrap"
              title={t('vmDetail.sharedBadgeTitle')}
            >
              {t('vmDetail.sharedBadge')}
            </span>
          {/if}
        {/if}
      {/if}
    </div>
    {#if vm && auth.canMutate()}
      <!-- Wraps (and may shrink) instead of shrink-0: on a phone the
           share switch + template button overflowed the viewport and
           clipped "Quitar como plantilla". -->
      <div class="flex flex-wrap items-center gap-2 min-w-0 max-w-full">
        {#if auth.isAdmin() && vmMeta?.template}
          <div class="max-w-xs min-w-0">
            <Switch
              size="sm"
              checked={!!vmMeta?.shared}
              disabled={shareSaving}
              label={t('vmDetail.shareTemplate')}
              description={t('vmDetail.shareTemplateDesc')}
              ariaLabel={t('vmDetail.shareTemplate')}
              onchange={toggleShared}
            />
          </div>
        {/if}
        <Button
          size="sm"
          variant="outline"
          onclick={toggleTemplate}
          disabled={vm.state !== 'shutoff' || actionLoading === 'template'}
        >
          {vmMeta?.template ? t('vmDetail.unsetTemplate') : t('vmDetail.makeTemplate')}
        </Button>
      </div>
    {/if}
  </div>

  {#if error && !error.toLowerCase().includes('forbidden')}
    <Alert variant="error">{error}</Alert>
  {/if}

  {#if loading}
    <VmDetailSkeleton />
  {:else if vm}
    <!-- minmax(0,1fr) + min-w-0: a plain `1fr` track has an auto minimum,
         so the tab row's min-content (~930px with all KVM tabs) widened
         the page past the viewport and clipped the sidebar actions. The
         tab strip scrolls horizontally instead (Tabs has overflow-x-auto). -->
    <div class="grid grid-cols-1 xl:grid-cols-[minmax(0,1fr)_280px] gap-5">
      <!-- Main column -->
      <div class="space-y-5 min-w-0">
        <!-- Overview -->
        {#snippet sec_overview()}
          <BlockCard bid="overview" title={t('vmDetail.overview')}>
            {#snippet headerActions()}
              {#if auth.canMutate()}
                <Button
                  size="sm"
                  variant="outline"
                  class="!h-7 !text-xs"
                  onclick={() => openEditTab('general')}
                >
                  <Pencil class="w-3 h-3 mr-1.5" />
                  {t('vmDetail.editSettings')}
                </Button>
              {/if}
            {/snippet}

            {#if vm.cover}
              <div
                class="relative w-full h-32 rounded-xl overflow-hidden border border-border bg-muted/40 shadow-2xs mb-4"
              >
                <img src={vm.cover} alt="" class="w-full h-full object-cover" />
                <div
                  class="absolute inset-0 bg-gradient-to-t from-background/90 via-background/20 to-transparent flex items-end p-3"
                >
                  <div class="flex items-center gap-2">
                    <span
                      class="text-xs font-semibold px-2.5 py-0.5 rounded-full bg-background/80 backdrop-blur border border-border/80"
                    >
                      {vm.alias || vm.name}
                    </span>
                    {#if vm.os_type}
                      <span
                        class="text-[11px] px-2 py-0.5 rounded-full bg-accent/20 text-accent font-medium backdrop-blur"
                      >
                        {vm.os_type}
                      </span>
                    {/if}
                  </div>
                </div>
              </div>
            {/if}

            <div class="grid grid-cols-1 sm:grid-cols-3 gap-3 mb-4">
              <div class="border border-border rounded-md p-3 bg-background">
                <p class="text-2xl font-semibold tnum">{vm.vcpus}</p>
                <p class="text-xs text-muted-foreground mt-0.5">{t('common.vcpu')}</p>
                {#if vm.cpu_usage != null}<p class="text-xs text-accent mt-0.5 tnum">
                    {vm.cpu_usage.toFixed(1)}% used
                  </p>{/if}
              </div>
              <div class="border border-border rounded-md p-3 bg-background">
                <p class="text-2xl font-semibold tnum">{vm.ram_mb}</p>
                <p class="text-xs text-muted-foreground mt-0.5">{t('vmDetail.ramLabel')}</p>
                {#if vm.ram_used_mb != null}<p class="text-xs text-accent mt-0.5 tnum">
                    {vm.ram_used_mb} MB used
                  </p>{/if}
              </div>
              <div class="border border-border rounded-md p-3 bg-background">
                <p class="text-lg font-semibold tnum">{formatUptime(vm.uptime_sec)}</p>
                <p class="text-xs text-muted-foreground mt-0.5">{t('vmDetail.uptime')}</p>
              </div>
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-1.5 text-sm">
              <div class="flex gap-2">
                <span class="text-muted-foreground shrink-0">{t('vmDetail.typeLabel')}</span>
                <span
                  class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded border text-[10px] uppercase tracking-wider font-medium {isContainerVm
                    ? 'border-warning/30 bg-warning/10 text-warning'
                    : 'border-accent/30 bg-accent/10 text-accent'}"
                >
                  {isContainerVm ? 'Incus' : 'KVM'}
                </span>
              </div>
              {#if vm.os_type}
                <div class="flex gap-2">
                  <span class="text-muted-foreground shrink-0">{t('vmDetail.os')}</span><span
                    class="truncate">{vm.os_type}{vm.os_version ? ' ' + vm.os_version : ''}</span
                  >
                </div>
              {/if}
              {#if !isContainerVm}
                <div class="flex gap-2">
                  <span class="text-muted-foreground shrink-0">{t('vmDetail.chipset')}</span><span
                    >{vm.chipset}</span
                  >
                </div>
                <div class="flex gap-2">
                  <span class="text-muted-foreground shrink-0">{t('vmDetail.secureBoot')}</span
                  ><span>{vm.secure_boot ? t('common.yes') : t('common.no')}</span>
                </div>
                <div class="flex gap-2">
                  <span class="text-muted-foreground shrink-0">{t('vmDetail.tpm')}</span><span
                    >{vm.tpm_enabled ? t('common.yes') : t('common.no')}</span
                  >
                </div>
                <div class="flex gap-2">
                  <span class="text-muted-foreground shrink-0">{t('vmDetail.bios')}</span><span
                    class="capitalize">{vm.firmware || '?'}</span
                  >
                </div>
                <div class="flex gap-2">
                  <span class="text-muted-foreground shrink-0">{t('vmDetail.cpuMode')}</span><span
                    >{vm.cpu_mode || 'host-passthrough'}</span
                  >
                </div>
                <div class="flex gap-2">
                  <span class="text-muted-foreground shrink-0">{t('vmDetail.video')}</span><span
                    >{vm.video_model || 'virtio'}</span
                  >
                </div>
                <div class="flex items-center gap-2">
                  <span class="text-muted-foreground shrink-0">{t('vmDetail.boot')}</span>
                  <select
                    bind:value={bootDevice}
                    onchange={() =>
                      api
                        .setBootDevice(vmId, bootDevice)
                        .then(() => load())
                        .catch((e) => toast.error(e.message))}
                    class="input !py-1 !text-xs w-auto"
                  >
                    <option value="hd">{t('vmDetail.hardDisk')}</option>
                    <option value="cdrom">{t('vmDetail.cdrom')}</option>
                    <option value="network">{t('vmDetail.network')}</option>
                  </select>
                </div>
              {/if}
            </div>
            {#if !isContainerVm && cdromDisk && auth.canMutate()}
              <!-- Quick ISO row: shows what is in the drive and offers
                   the same Change ISO dialog used on the disks tab,
                   plus a one-click eject. -->
              <div
                class="mt-3 pt-3 border-t border-border flex items-center justify-between gap-3 flex-wrap"
              >
                <div class="flex items-center gap-2 min-w-0">
                  <span class="text-muted-foreground shrink-0 text-sm"
                    >{t('vmDetail.iso')}</span
                  >
                  <span
                    class="text-sm truncate {cdromDisk.source
                      ? ''
                      : 'text-muted-foreground italic'}"
                    title={cdromDisk.source || ''}
                  >
                    {cdromDisk.source ? diskLabel(cdromDisk) : t('vmDetail.noIsoAttached')}
                  </span>
                </div>
                <div class="flex items-center gap-1 shrink-0">
                  <button
                    onclick={() => {
                      cISOTarget = cdromDisk.target;
                      cISOSource = cdromDisk.source || '';
                      showChangeISO = true;
                    }}
                    class="text-xs text-accent hover:text-accent-hover px-2 py-1 rounded hover:bg-muted"
                    >{t('vmDetail.changeIso')}</button
                  >
                  {#if cdromDisk.source}
                    <button
                      onclick={() => ejectISO(cdromDisk.target)}
                      disabled={actionLoading === 'ejectiso'}
                      class="text-xs text-muted-foreground hover:text-destructive px-2 py-1 rounded hover:bg-destructive/10 disabled:opacity-50 disabled:cursor-not-allowed"
                      >{t('vmDetail.eject')}</button
                    >
                  {/if}
                </div>
              </div>
            {/if}
          </BlockCard>
        {/snippet}

        <!-- Hardware & Settings: read-only inventory. Editing is
             consolidated into a single entry point (the "Editar
             ajustes" button in this card's header) instead of a
             separate "Editar" affordance per row/section, which used
             to make ~15 buttons all opening the same dialog. The `tab`
             param is unused visually now but kept so a future
             "double-click a row to jump to its tab" shortcut can reuse
             it without re-plumbing every call site. -->{#snippet sec_hardware()}
          <VmHardwareCard
            {vmId}
            {vm}
            {isContainerVm}
            {vmNetworks}
            bind:groupsList={eGroupsList}
            bind:actionLoading
            request={hwEditRequest}
            onLoad={load}
          />
        {/snippet}

        {#snippet sec_metrics()}
          <BlockCard bid="metrics" title={t('vmDetail.metrics')}>
            <div class="flex items-center justify-between mb-3">
              <span class="text-xs text-muted-foreground tnum"
                >{t('vmDetail.updatedAt', {
                  time: metrics?.sampled_at
                    ? new Date(metrics.sampled_at * 1000).toLocaleTimeString()
                    : '—',
                })}</span
              >
            </div>
            {#if vm?.state !== 'running'}
              <p class="text-sm text-muted-foreground">
                {t('vmDetail.metricsOff')}
              </p>
            {:else if !metrics || (cpuPoints.length === 0 && ramPoints.length === 0)}
              <p class="text-sm text-muted-foreground">{t('vmDetail.collectingSamples')}</p>
            {:else}
              <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                  <div class="flex items-baseline justify-between mb-1.5">
                    <span class="text-xs font-medium text-muted-foreground uppercase tracking-wider"
                      >{t('vms.cpu')}</span
                    >
                    <span class="text-sm tnum"
                      >{cpuPoints.length
                        ? cpuPoints[cpuPoints.length - 1].v.toFixed(1)
                        : '0.0'}%</span
                    >
                  </div>
                  <Chart points={cpuPoints} yMax={100} height={70} />
                </div>
                <div>
                  <div class="flex items-baseline justify-between mb-1.5">
                    <span class="text-xs font-medium text-muted-foreground uppercase tracking-wider"
                      >{t('common.ram')}</span
                    >
                    <span class="text-sm tnum"
                      >{ramPoints.length
                        ? ramPoints[ramPoints.length - 1].v.toFixed(1)
                        : '0.0'}%</span
                    >
                  </div>
                  <Chart points={ramPoints} yMax={100} height={70} />
                </div>
                <div>
                  <div class="flex items-baseline justify-between mb-1.5">
                    <span class="text-xs font-medium text-muted-foreground uppercase tracking-wider"
                      >{t('vmDetail.diskRead')}</span
                    >
                    <span class="text-sm tnum"
                      >{diskRPoints.length
                        ? formatRate(diskRPoints[diskRPoints.length - 1].v)
                        : '0 B/s'}</span
                    >
                  </div>
                  <Chart points={diskRPoints} height={70} color="var(--success)" />
                </div>
                <div>
                  <div class="flex items-baseline justify-between mb-1.5">
                    <span class="text-xs font-medium text-muted-foreground uppercase tracking-wider"
                      >{t('vmDetail.diskWrite')}</span
                    >
                    <span class="text-sm tnum"
                      >{diskWPoints.length
                        ? formatRate(diskWPoints[diskWPoints.length - 1].v)
                        : '0 B/s'}</span
                    >
                  </div>
                  <Chart points={diskWPoints} height={70} color="var(--warning)" />
                </div>
                <div>
                  <div class="flex items-baseline justify-between mb-1.5">
                    <span class="text-xs font-medium text-muted-foreground uppercase tracking-wider"
                      >{t('vmDetail.netRx')}</span
                    >
                    <span class="text-sm tnum"
                      >{netRxPoints.length
                        ? formatRate(netRxPoints[netRxPoints.length - 1].v)
                        : '0 B/s'}</span
                    >
                  </div>
                  <Chart points={netRxPoints} height={70} color="var(--info, var(--accent))" />
                </div>
                <div>
                  <div class="flex items-baseline justify-between mb-1.5">
                    <span class="text-xs font-medium text-muted-foreground uppercase tracking-wider"
                      >{t('vmDetail.netTx')}</span
                    >
                    <span class="text-sm tnum"
                      >{netTxPoints.length
                        ? formatRate(netTxPoints[netTxPoints.length - 1].v)
                        : '0 B/s'}</span
                    >
                  </div>
                  <Chart points={netTxPoints} height={70} color="var(--info, var(--accent))" />
                </div>
              </div>
            {/if}
          </BlockCard>
        {/snippet}

        {#snippet sec_cloudinit()}
          <VmCloudInitCard
            {vmId}
            {vm}
            {isContainerVm}
            {vmMeta}
            active={activeSection}
            bind:actionLoading
            onLoad={load}
            onToggleEjectIso={toggleEjectIsoOnShutdown}
          />
        {/snippet}

        {#snippet sec_disks()}
          <VmDisksCard
            {vmId}
            {vm}
            {isContainerVm}
            {pools}
            {isos}
            bind:actionLoading
            {requireShutoff}
            onLoad={load}
            onSetISO={(disk) => {
              cISOTarget = disk.target;
              cISOSource = disk.source || '';
              showChangeISO = true;
            }}
            onRemove={(disk) => {
              if (!isContainerVm && vm.disks.length <= 1 && disk.device !== 'cdrom') {
                if (!requireShutoff(t('vmDetail.removing'))) return;
              }
              removeDisk(disk.target);
            }}
          />
        {/snippet}

        {#snippet sec_shared_folders()}
          <VmSharedFoldersCard {vmId} {vm} {isContainerVm} bind:actionLoading onLoad={load} />
        {/snippet}

        {#snippet sec_net()}
          <VmNetCard
            {vmId}
            {vm}
            {isContainerVm}
            hostNetworks={networks}
            netOptions={vmNetworks}
            {hostBridges}
            bind:actionLoading
            canMutate={auth.canMutate()}
            onModelChange={changeNetModel}
            onToggleLink={toggleLinkState}
            onRemove={removeNet}
            onLoad={load}
            onChangeNetwork={changeNetNetwork}
          />
        {/snippet}

        {#snippet sec_usb()}
          <VmUsbCard {vmId} {vm} bind:actionLoading onLoad={load} />
        {/snippet}

        {#snippet sec_pci()}
          <VmPciCard {vmId} {vm} bind:actionLoading onLoad={load} />
        {/snippet}

        {#snippet sec_guest()}
          <VmGuestCard
            {vmId}
            {vm}
            active={activeSection}
            bind:actionLoading
            isDeleting={() => deleting}
          />
        {/snippet}

        {#snippet snapRetention()}
          <div class="pt-3 border-t border-border/50 space-y-2">
            <label for="sched-snap-max" class="text-xs text-muted-foreground block">
              {t('vmDetail.snapMaxKeep')}
            </label>
            <div class="flex flex-wrap items-center gap-2">
              <select
                value={schedSnapMax}
                onchange={(e) => (schedSnapMax = Number(e.target.value) || 3)}
                class="input !h-8 text-xs w-auto"
              >
                <option value={1}>1</option>
                <option value={3}>3</option>
                <option value={5}>5</option>
                <option value={7}>7</option>
                <option value={14}>14</option>
                <option value={30}>30</option>
              </select>
              <span class="text-xs text-muted-foreground">·</span>
              <Input
                id="sched-snap-max"
                type="number"
                min="1"
                max="999"
                bind:value={schedSnapMax}
                placeholder={t('vmDetail.retentionCustom')}
                class="!h-8 w-24 tnum text-xs"
              />
              <span class="text-xs text-muted-foreground">
                {t('vmDetail.retentionCount', { n: schedSnapMax }) ||
                  `${schedSnapMax} instantáneas`}
              </span>
            </div>
          </div>
        {/snippet}

        {#snippet sec_snaps()}
          <VmSnapshotsCard
            {vmId}
            {snapshots}
            bind:actionLoading
            bind:schedSnap
            bind:schedSnapMax
            {schedSaving}
            onCreate={createSnapshot}
            onRevert={revertSnapshot}
            onDelete={deleteSnapshot}
            onSaveSchedule={saveSchedule}
            retention={snapRetention}
          />
        {/snippet}

        {#snippet sec_firewall()}
          <VmFirewallCard {vmId} bind:fwRules bind:fwForwards />
        {/snippet}

        {#snippet sec_schedule()}
          <VmScheduleCard
            bind:schedStart
            bind:schedStop
            bind:schedSnap
            {schedSaving}
            onSaveSchedule={saveSchedule}
            retention={snapRetention}
          />
        {/snippet}

        {#snippet sec_history()}
          <VmHistoryCard {vmId} {vm} active={activeSection} isDeleting={() => deleting} />
        {/snippet}

        {#snippet sec_alerts()}
          <VmAlertsCard {vmId} isDeleting={() => deleting} />
        {/snippet}

        {#snippet sec_serial()}
          <div id="vm-serial-block">
            <BlockCard
              bid="serial"
              title={isContainerVm ? 'Terminal Shell (xterm)' : t('vmDetail.serialConsoleTitle')}
            >
              <TerminalPanel mode="vm" {vmId} />
            </BlockCard>
          </div>
        {/snippet}

        <Tabs tabs={sectionTabs} bind:active={activeSection} class="mb-1" />

        <!-- Each panel stays mounted (CSS-hidden, not {#if}-removed) so
             switching tabs never tears down the serial WebSocket or a
             VNC connection living inside one of these cards. -->
        <div class="space-y-5 {activeSection === 'overview' ? '' : 'hidden'}">
          {@render sec_overview()}
          {@render sec_metrics()}
          {@render sec_firewall()}
          {@render sec_schedule()}
        </div>
        <div class="space-y-5 {activeSection === 'terminal' ? '' : 'hidden'}">
          {@render sec_serial()}
        </div>
        <div class={activeSection === 'hardware' ? '' : 'hidden'}>
          {@render sec_hardware()}
        </div>
        {#if !isContainerVm}
          <div class={activeSection === 'cloudinit' ? '' : 'hidden'}>
            {@render sec_cloudinit()}
          </div>
        {/if}
        <div class={activeSection === 'disks' ? '' : 'hidden'}>
          {@render sec_disks()}
          {#if auth.isAdmin() && !isContainerVm}
            {@render sec_shared_folders()}
          {/if}
        </div>
        <div class={activeSection === 'net' ? '' : 'hidden'}>
          {@render sec_net()}
          {#if auth.isAdmin()}
            {@render sec_usb()}
            {#if !isContainerVm}
              {@render sec_pci()}
            {/if}
          {/if}
        </div>
        {#if !isContainerVm}
          <div class={activeSection === 'guest' ? '' : 'hidden'}>
            {@render sec_guest()}
          </div>
        {/if}
        <div class={activeSection === 'snaps' ? '' : 'hidden'}>
          {@render sec_snaps()}
        </div>
        <div class={activeSection === 'history' ? '' : 'hidden'}>
          {@render sec_history()}
        </div>
        <div class={activeSection === 'alerts' ? '' : 'hidden'}>
          {@render sec_alerts()}
        </div>
      </div>

      <!-- Sidebar -->
      <div class="space-y-4">
        {#if auth.canMutate()}
          <Card class="p-4">
            <h2 class="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              {t('vmDetail.power')}
            </h2>
            <div class="space-y-2">
              {#key vm.state}
                <div transition:fade={{ duration: 150 }}>
                  {#if vm.state === 'shutoff' && vmMeta?.template}
                    <p class="text-xs text-muted-foreground text-center py-2">
                      {t('vmDetail.templateStartBlocked')}
                    </p>
                  {:else if vm.state === 'shutoff'}
                    <Button
                      onclick={() => doAction('startVM')}
                      disabled={busy}
                      class="w-full {actionLoading === 'startVM' ? 'opacity-100!' : ''}"
                    >
                      {#if actionLoading === 'startVM'}<Spinner
                          size="sm"
                          color="text-white"
                        />{:else}<Play class="w-4 h-4 mr-1.5" />{t('vmDetail.start')}{/if}
                    </Button>
                  {:else if vm.state === 'running'}
                    <Button
                      variant="outline"
                      onclick={() => doAction('shutdownVM')}
                      disabled={busy}
                      class="w-full {actionLoading === 'shutdownVM' ? 'opacity-100!' : ''}"
                    >
                      {#if actionLoading === 'shutdownVM'}<Spinner
                          size="sm"
                          color="text-white"
                        />{:else}<PowerOff class="w-4 h-4 mr-1.5" />{t('vmDetail.shutdown')}{/if}
                    </Button>
                    <Button
                      variant="destructive"
                      onclick={() => doAction('forceOffVM')}
                      disabled={busy}
                      class="w-full {actionLoading === 'forceOffVM' ? 'opacity-100!' : ''}"
                    >
                      {#if actionLoading === 'forceOffVM'}<Spinner
                          size="sm"
                          color="text-white"
                        />{:else}<PowerIcon class="w-4 h-4 mr-1.5" />{t('vmDetail.forceOff')}{/if}
                    </Button>
                    <Button
                      variant="destructive"
                      onclick={() => doAction('forceRebootVM')}
                      disabled={busy}
                      class="w-full {actionLoading === 'forceRebootVM' ? 'opacity-100!' : ''}"
                    >
                      {#if actionLoading === 'forceRebootVM'}<Spinner
                          size="sm"
                          color="text-white"
                        />{:else}<RotateCw class="w-4 h-4 mr-1.5" />{t('vmDetail.forceReboot')}{/if}
                    </Button>
                    <Button
                      variant="outline"
                      onclick={() => doAction('suspendVM')}
                      disabled={busy}
                      class="w-full {actionLoading === 'suspendVM' ? 'opacity-100!' : ''}"
                    >
                      {#if actionLoading === 'suspendVM'}<Spinner
                          size="sm"
                          color="text-white"
                        />{:else}<Pause class="w-4 h-4 mr-1.5" />{t('vmDetail.suspend')}{/if}
                    </Button>
                  {:else if vm.state === 'paused'}
                    <Button
                      onclick={() => doAction('resumeVM')}
                      disabled={busy}
                      class="w-full {actionLoading === 'resumeVM' ? 'opacity-100!' : ''}"
                    >
                      {#if actionLoading === 'resumeVM'}<Spinner
                          size="sm"
                          color="text-white"
                        />{:else}<PlayCircle class="w-4 h-4 mr-1.5" />{t('vmDetail.resume')}{/if}
                    </Button>
                  {/if}
                </div>
              {/key}
            </div>
          </Card>
        {/if}

        <Card class="p-4">
          <h2 class="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
            {t('vmDetail.actions')}
          </h2>
          <div class="space-y-2">
            {#if isContainerVm}
              <Button onclick={gotoSerial} class="w-full">
                <Terminal class="w-4 h-4 mr-1.5" />
                Terminal Shell (xterm)
              </Button>
            {:else}
              {#if (vm.graphics_type || 'both') === 'both'}
                <div class="grid grid-cols-2 gap-2">
                  <Button onclick={() => openConsole('vnc')} class="w-full text-xs">
                    <Terminal class="w-3.5 h-3.5 mr-1" />
                    noVNC
                  </Button>
                  <Button
                    variant="secondary"
                    onclick={() => openConsole('spice')}
                    class="w-full text-xs"
                  >
                    <Terminal class="w-3.5 h-3.5 mr-1" />
                    SPICE
                  </Button>
                </div>
              {:else if vm.graphics_type === 'spice'}
                <Button
                  variant="secondary"
                  onclick={() => openConsole('spice')}
                  class="w-full text-xs"
                >
                  <Terminal class="w-3.5 h-3.5 mr-1" />
                  Consola SPICE
                </Button>
              {:else if vm.graphics_type === 'vnc'}
                <Button onclick={() => openConsole('vnc')} class="w-full text-xs">
                  <Terminal class="w-3.5 h-3.5 mr-1" />
                  Consola noVNC
                </Button>
              {/if}
              {#if vm.serial_port !== false}
                <Button variant="outline" onclick={gotoSerial} class="w-full">
                  <Terminal class="w-4 h-4 mr-1.5" />
                  {t('vmDetail.serialConsole')}
                </Button>
              {/if}
            {/if}
            {#if appInfo}
              <Button variant="outline" onclick={showAppCredentials} class="w-full">
                <KeyRound class="w-4 h-4 mr-1.5" />
                {t('vmDetail.appCredentials')}
              </Button>
            {/if}
            {#if auth.isAdmin() && !isContainerVm}
              <Button
                variant="outline"
                onclick={resetPassword}
                disabled={busy || (vm && vm.state !== 'running')}
                title={vm && vm.state !== 'running' ? t('vmDetail.resetPasswordHint') : ''}
                class="w-full"
              >
                {t('vmDetail.resetPassword')}
              </Button>
            {/if}
            {#if auth.canMutate()}
              <Button
                variant="destructive"
                onclick={() => doAction('deleteVM')}
                disabled={busy}
                class="w-full"
              >
                <Trash2 class="w-4 h-4 mr-1.5" />
                {t('vmDetail.deleteVM')}
              </Button>
              {#if !isContainerVm}
                <Button
                  variant="outline"
                  onclick={() => {
                    cName = vm.name + '-clone';
                    cPool = vmDiskPools(pools)[0]?.name || 'webkvm-disks';
                    cKeepMAC = false;
                    showClone = true;
                  }}
                  class="w-full"
                >
                  <CopyPlus class="w-4 h-4 mr-1.5" />
                  {t('vmDetail.cloneVM')}
                </Button>
                <Button
                  variant="outline"
                  onclick={() => (showBatchClone = true)}
                  class="w-full text-xs"
                >
                  <CopyPlus class="w-4 h-4 mr-1.5" />
                  Clonado por Lotes...
                </Button>
              {/if}
              {#if movePoolOptions.length > 0}
                <!-- Hidden outright when there is nowhere to move to:
                     a button that can only ever fail is worse than no
                     button. -->
                <Button
                  variant="outline"
                  onclick={() => {
                    movePool = movePoolOptions[0]?.name || '';
                    showMove = true;
                  }}
                  class="w-full"
                >
                  <Icon name="hard-drive" size={16} class="mr-1.5" />
                  {t('vmDetail.moveStorage')}
                </Button>
              {/if}
              <Button variant="outline" onclick={openEdit} class="w-full">
                <Pencil class="w-4 h-4 mr-1.5" />
                {t('vmDetail.editSettings')}
              </Button>
              <Button variant="outline" onclick={openIdentity} class="w-full">
                <Info class="w-4 h-4 mr-1.5" />
                {t('vmDetail.identityNotes')}
              </Button>
              <Button
                variant="outline"
                onclick={exportVM}
                disabled={actionLoading === 'export'}
                class="w-full"
              >
                {#if actionLoading === 'export'}<Spinner size="sm" color="text-white" />{t(
                    'vmDetail.exportingShort'
                  )}{:else}<Download class="w-4 h-4 mr-1.5" />{t('vmDetail.exportBackup')}{/if}
              </Button>
            {/if}
            {#if !isContainerVm}
              <div class="grid grid-cols-2 gap-2">
                <Button
                  variant="outline"
                  class="w-full justify-center"
                  onclick={() => downloadConsoleFile('rdp')}
                >
                  RDP
                </Button>
                <Button
                  variant="outline"
                  class="w-full justify-center"
                  onclick={() => downloadConsoleFile('spice')}
                  disabled={capabilities.loaded &&
                    capabilities.parsed &&
                    !capabilities.spiceSupported}
                  title={capabilities.loaded && capabilities.parsed && !capabilities.spiceSupported
                    ? 'SPICE is not available on this host (QEMU has no spice module)'
                    : ''}
                >
                  SPICE
                </Button>
              </div>
            {/if}
            <!-- Autostart toggle: lives at the bottom of the
						     Actions card so it doesn't compete with the
						     primary action (Open Console). The Switch's
						     `checked` is bound to vm.autostart; onchange
						     fires toggleAutostart() which PATCHes the
						     server and rolls back on failure. -->
            <div class="pt-3 mt-2 border-t border-border">
              <Switch
                checked={!!vm.autostart}
                disabled={autostartSaving}
                onchange={toggleAutostart}
                label={t('vmDetail.autostart')}
                description={autostartSaving
                  ? t('vmDetail.notesSaving')
                  : vm.autostart
                    ? t('vmDetail.autostartDescOn')
                    : t('vmDetail.autostartDescOff')}
              />
            </div>
          </div>
        </Card>
      </div>
    </div>
  {/if}
</div>

<PasswordModal
  bind:open={showPasswordModal}
  username={resetUsername}
  password={resetPasswordValue}
  title={t('vmDetail.passwordResetTitle')}
/>

<CredentialsModal bind:open={showAppCreds} info={appCreds} />

<!-- Reset password error: guest agent not available or reset failed -->
<Dialog.Root bind:open={showResetError}>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>{t('vmDetail.passwordResetFailedTitle')}</Dialog.Title>
      <Dialog.Description>
        {t('vmDetail.passwordResetFailedDesc')}
      </Dialog.Description>
    </Dialog.Header>
    {#if resetError}
      <p class="text-sm text-muted-foreground break-words">{resetError}</p>
    {/if}
    <p class="text-xs text-muted-foreground mt-2">
      {t('vmDetail.passwordResetGuestAgentHint')}
      <code class="block mt-1 p-2 rounded bg-muted font-mono text-xs"
        >sudo apt install qemu-guest-agent && sudo systemctl enable --now qemu-guest-agent</code
      >
    </p>
    <Dialog.Footer class="gap-2">
      <Button onclick={() => (showResetError = false)}>OK</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Pop-up: action blocked because the VM isn't shut off -->
<Dialog.Root bind:open={showBlocked}>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>{t('vmDetail.requireShutoffTitle')}</Dialog.Title>
      <Dialog.Description>
        {t('vmDetail.blockedDialogDesc')}
      </Dialog.Description>
    </Dialog.Header>
    {#if blockedNotice}
      <p class="text-sm text-muted-foreground">{blockedNotice}</p>
    {/if}
    <Dialog.Footer class="gap-2">
      <Button onclick={() => (showBlocked = false)}>OK</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Shared confirm dialog (replaces all window.confirm) -->
<ConfirmDialog
  bind:open={confirmState.open}
  title={confirmState.title}
  description={confirmState.description}
  confirmLabel={confirmState.confirmLabel}
  variant={confirmState.variant}
  loading={confirmState.loading}
  onConfirm={confirmState.onConfirm}
/>

<DeleteVmDialog
  bind:open={showDeleteFlow}
  vmId={vm?.id}
  vmName={vm?.name}
  {vmDisks}
  active={vm?.state === 'running' || vm?.state === 'paused'}
  isContainer={isContainerVm}
  {onDeleteStart}
  {onDeleteFailed}
  onDeleted={onVmDeleted}
/>

<!-- Change ISO Dialog -->
<Dialog.Root bind:open={showChangeISO}>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>{t('vmDetail.changeIsoTitle', { target: cISOTarget })}</Dialog.Title>
    </Dialog.Header>
    <div>
      <label for="ciso-src" class="block text-sm font-medium mb-1.5">{t('vmDetail.iso')}</label>
      <select id="ciso-src" bind:value={cISOSource} class="input">
        <option value="">{t('vmDetail.ejectNoIso')}</option>
        {#each isos as iso (iso.path || iso.name)}<option value={iso.path}>{iso.name}</option
          >{/each}
      </select>
    </div>
    <Dialog.Footer class="gap-2">
      <Button
        variant="outline"
        onclick={() => (showChangeISO = false)}
        disabled={actionLoading === 'changeiso'}>{t('common.cancel')}</Button
      >
      <Button onclick={changeISO} disabled={actionLoading === 'changeiso'}>
        {#if actionLoading === 'changeiso'}<Spinner size="sm" color="text-white" />{:else}{t(
            'vmDetail.changeIso'
          )}{/if}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Clone Dialog -->
<Dialog.Root bind:open={showClone}>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>{t('vmDetail.cloneTitle')}</Dialog.Title>
      <Dialog.Description>{t('vmDetail.cloneDesc')}</Dialog.Description>
    </Dialog.Header>
    <div class="space-y-3">
      {#if vm?.state !== 'shutoff'}
        <div
          class="flex items-start gap-2 p-2.5 rounded-lg border border-warning/30 bg-warning-subtle text-warning text-xs"
        >
          <Icon name="alert-triangle" size={14} class="shrink-0 mt-0.5" />
          <span>{t('vmDetail.cloneRequiresShutoff')}</span>
        </div>
      {/if}
      <div>
        <label for="clone-name" class="block text-sm font-medium mb-1.5"
          >{t('vmDetail.newName')}</label
        >
        <Input id="clone-name" bind:value={cName} type="text" />
      </div>
      <div>
        <label for="clone-pool" class="block text-sm font-medium mb-1.5"
          >{t('vmDetail.storagePool')}</label
        >
        <select id="clone-pool" bind:value={cPool} class="input">
          {#each vmDiskPools(pools) as p (p.name)}<option value={p.name}>{p.name}</option>{/each}
        </select>
      </div>
      <div class="rounded-lg border border-border p-3 space-y-2">
        <div class="flex items-center justify-between gap-2">
          <span class="text-sm font-medium">{t('vmDetail.linkedClone')}</span>
          <Switch bind:checked={cLinked} ariaLabel={t('vmDetail.linkedClone')} />
        </div>
        <p class="text-xs text-muted-foreground">
          {cLinked ? t('vmDetail.linkedCloneHelperOn') : t('vmDetail.linkedCloneHelperOff')}
        </p>
      </div>
      <div class="rounded-lg border border-border p-3 space-y-2">
        <label class="flex items-center gap-2 text-sm font-medium cursor-pointer select-none">
          <input
            type="checkbox"
            bind:checked={cKeepMAC}
            class="w-4 h-4 rounded border-border"
          />
          {t('vmDetail.keepMAC')}
        </label>
        <p class="text-xs text-muted-foreground">{t('vmDetail.keepMACHint')}</p>
      </div>
    </div>
    <Dialog.Footer class="gap-2">
      <Button
        variant="outline"
        onclick={() => (showClone = false)}
        disabled={actionLoading === 'clone'}>{t('common.cancel')}</Button
      >
      <Button onclick={cloneVM} disabled={actionLoading === 'clone' || !cName}>
        {#if actionLoading === 'clone'}<Spinner size="sm" color="text-white" />{:else}{t(
            'vmDetail.clone'
          )}{/if}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<BatchCloneModal
  {vmId}
  open={showBatchClone}
  onClose={() => (showBatchClone = false)}
  onSuccess={() => {
    navigate('/vms');
  }}
/>

<!-- Move storage to another pool. Not a clone: the same instance ends
     up with its disks somewhere else, and the originals are removed
     once the copy is complete and the VM points at it. -->
<Dialog.Root bind:open={showMove}>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>{t('vmDetail.moveStorageTitle')}</Dialog.Title>
      <Dialog.Description>{t('vmDetail.moveStorageDesc')}</Dialog.Description>
    </Dialog.Header>
    <div class="space-y-3">
      {#if vm?.state !== 'shutoff'}
        <div
          class="flex items-start gap-2 p-2.5 rounded-lg border border-warning/30 bg-warning-subtle text-warning text-xs"
        >
          <Icon name="alert-triangle" size={14} class="shrink-0 mt-0.5" />
          <span>{t('vmDetail.moveRequiresShutoff')}</span>
        </div>
      {/if}
      {#if currentPool}
        <div class="text-xs text-muted-foreground">
          {t('vmDetail.moveCurrentPool', { pool: currentPool })}
        </div>
      {/if}
      <div>
        <label for="move-pool" class="block text-sm font-medium mb-1.5"
          >{t('vmDetail.moveDestPool')}</label
        >
        <select id="move-pool" bind:value={movePool} class="input" disabled={!!moveProgress}>
          {#each movePoolOptions as p (p.name)}<option value={p.name}>{p.name}</option>{/each}
        </select>
      </div>
      {#if moveProgress}
        <div class="space-y-1.5">
          <div class="flex items-center justify-between text-xs">
            <span class="text-muted-foreground">{moveStageLabel(moveProgress.stage)}</span>
            <span class="tnum">{Math.round(moveProgress.pct)}%</span>
          </div>
          <div class="h-1.5 rounded-full bg-muted overflow-hidden">
            <div
              class="h-full bg-accent transition-[width] duration-300"
              style="width: {Math.max(2, Math.min(100, moveProgress.pct))}%"
            ></div>
          </div>
        </div>
      {:else}
        <p class="text-xs text-muted-foreground">{t('vmDetail.moveStorageHint')}</p>
      {/if}
    </div>
    <Dialog.Footer class="gap-2">
      <Button
        variant="outline"
        onclick={() => (showMove = false)}
        disabled={actionLoading === 'move'}>{t('common.cancel')}</Button
      >
      <Button onclick={moveStorage} disabled={actionLoading === 'move' || !movePool}>
        {#if actionLoading === 'move'}<Spinner size="sm" color="text-white" />{:else}{t(
            'vmDetail.moveStorageConfirm'
          )}{/if}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Export Dialog -->
<Dialog.Root bind:open={showExport}>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title
        >{isContainerVm
          ? t('vmDetail.exportContainerTitle')
          : t('vmDetail.exportTitle')}</Dialog.Title
      >
    </Dialog.Header>
    {#if !exportProgress}
      <p class="text-sm text-muted-foreground">
        {isContainerVm ? t('vmDetail.exportContainerDesc') : t('vmDetail.exportDesc')}
      </p>
      <div class="space-y-2 mt-2">
        {#if !isContainerVm}
          <label
            class="flex items-start gap-3 p-3 rounded border border-border cursor-pointer hover:border-border-hover"
          >
            <input type="radio" bind:group={exportTarget} value="vmware" class="mt-1" />
            <div>
              <div class="text-sm font-medium">{t('vmDetail.exportVmware')}</div>
              <div class="text-xs text-muted-foreground">{t('vmDetail.exportVmwareDesc')}</div>
            </div>
          </label>
          <label
            class="flex items-start gap-3 p-3 rounded border border-border cursor-pointer hover:border-border-hover"
          >
            <input type="radio" bind:group={exportTarget} value="libvirt" class="mt-1" />
            <div>
              <div class="text-sm font-medium">{t('vmDetail.exportLibvirt')}</div>
              <div class="text-xs text-muted-foreground">{t('vmDetail.exportLibvirtDesc')}</div>
            </div>
          </label>
          <label
            class="flex items-start gap-3 p-3 rounded border border-border cursor-pointer hover:border-border-hover"
          >
            <input type="radio" bind:group={exportTarget} value="backup" class="mt-1" />
            <div>
              <div class="text-sm font-medium">{t('vmDetail.exportBackupLabel')}</div>
              <div class="text-xs text-muted-foreground">{t('vmDetail.exportBackupDesc')}</div>
            </div>
          </label>
        {:else}
          <label
            class="flex items-start gap-3 p-3 rounded border border-border cursor-pointer hover:border-border-hover"
          >
            <input type="radio" bind:group={exportTarget} value="backup" class="mt-1" />
            <div>
              <div class="text-sm font-medium">{t('vmDetail.exportIncusBackupLabel')}</div>
              <div class="text-xs text-muted-foreground">{t('vmDetail.exportIncusBackupDesc')}</div>
            </div>
          </label>
          <label
            class="flex items-start gap-3 p-3 rounded border border-border cursor-pointer hover:border-border-hover"
          >
            <input type="radio" bind:group={exportTarget} value="proxmox" class="mt-1" />
            <div>
              <div class="text-sm font-medium">{t('vmDetail.exportProxmoxLabel')}</div>
              <div class="text-xs text-muted-foreground">{t('vmDetail.exportProxmoxDesc')}</div>
            </div>
          </label>
        {/if}
      </div>
      <Dialog.Footer class="gap-2">
        <Button variant="outline" onclick={() => (showExport = false)}>{t('common.cancel')}</Button>
        <Button onclick={startExport}>{t('vmDetail.export')}</Button>
      </Dialog.Footer>
    {:else}
      <div class="space-y-2">
        <ProgressBar
          value={exportProgress.total > 0 ? exportProgress.percent : undefined}
          label={exportProgress.label}
          showValue
          size="md"
        />
        <div class="flex justify-between text-xs text-muted-foreground tnum">
          <span
            >{(exportProgress.received / 1e9).toFixed(2)} GB
            {exportProgress.total > 0
              ? `/ ${(exportProgress.total / 1e9).toFixed(2)} GB`
              : ''}</span
          >
        </div>
      </div>
      <Dialog.Footer>
        <Button variant="outline" onclick={cancelExport}>{t('common.cancel')}</Button>
      </Dialog.Footer>
    {/if}
  </Dialog.Content>
</Dialog.Root>

<!-- Identity & Notes Dialog -->
<Dialog.Root bind:open={showIdentity}>
  <Dialog.Content class="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
    <Dialog.Header>
      <Dialog.Title>{t('vmDetail.identityTitle')}</Dialog.Title>
      <Dialog.Description>{t('vmDetail.identityDesc')}</Dialog.Description>
    </Dialog.Header>

    <div class="flex gap-1 border-b border-border mb-4 overflow-x-auto">
      {#each [['alias', t('vmDetail.tabAlias')], ['cover', t('vmDetail.tabCover')], ['network', t('vmDetail.tabNetwork')], ['notes', t('vmDetail.tabNotes')], ['groups', t('vmDetail.tabGroups')]] as [k, label], __i (__i)}
        <button
          onclick={() => {
            identityTab = k;
            if (k === 'cover') loadMediaLibrary();
          }}
          class="px-3 py-2 text-sm border-b-2 -mb-px transition-colors whitespace-nowrap {identityTab ===
          k
            ? 'border-accent text-foreground'
            : 'border-transparent text-muted-foreground hover:text-foreground'}">{label}</button
        >
      {/each}
    </div>

    {#if identityTab === 'alias'}
      <div class="space-y-3">
        <div>
          <label for="ident-alias" class="block text-sm font-medium mb-1.5"
            >{t('vmDetail.aliasLabel')}</label
          >
          <Input id="ident-alias" bind:value={eAlias} placeholder={vm?.name} />
          <p class="text-xs text-muted-foreground mt-1">{t('vmDetail.aliasHelper')}</p>
        </div>
        <div class="flex justify-end gap-2 pt-2">
          <Button variant="outline" onclick={() => (showIdentity = false)}
            >{t('common.close')}</Button
          >
          <Button onclick={saveIdentityBasics} disabled={savingIdentity}
            >{savingIdentity ? t('vmDetail.notesSaving') : t('common.save')}</Button
          >
        </div>
      </div>
    {:else if identityTab === 'cover'}
      <div class="space-y-3">
        <div
          class="aspect-video w-full border border-border rounded-md bg-muted overflow-hidden flex items-center justify-center"
        >
          {#if coverPreview}
            <img src={coverPreview} alt="cover preview" class="w-full h-full object-cover" />
          {:else if vm?.cover}
            <img src={vm.cover} alt="cover" class="w-full h-full object-cover" />
          {:else}
            <div class="text-center text-muted-foreground p-6">
              <svg
                class="w-10 h-10 mx-auto mb-2 opacity-40"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                viewBox="0 0 24 24"
                ><rect x="3" y="3" width="18" height="18" rx="2" /><circle
                  cx="8.5"
                  cy="8.5"
                  r="1.5"
                /><path d="m21 15-5-5L5 21" /></svg
              >
              <p class="text-xs">{t('vmDetail.noCover')}</p>
            </div>
          {/if}
        </div>
        <div class="flex items-center gap-2">
          <label class={cn(buttonVariants({ variant: 'outline' }), 'cursor-pointer')}>
            {coverFile ? t('vmDetail.changeLabel') : t('vmDetail.chooseImage')}
            <input
              type="file"
              accept="image/png,image/jpeg,image/webp"
              class="hidden"
              onchange={onCoverPicked}
            />
          </label>
          {#if coverFile}
            <span class="text-xs text-muted-foreground truncate"
              >{coverFile.name} ({(coverFile.size / 1024).toFixed(0)} KB)</span
            >
            <Button size="sm" onclick={uploadCover} disabled={uploadingCover}
              >{uploadingCover ? t('vmDetail.uploading') : t('vmDetail.upload')}</Button
            >
          {/if}
          {#if vm?.cover}
            <Button size="sm" variant="outline" onclick={removeCover}
              >{t('vmDetail.removeCurrent')}</Button
            >
          {/if}
        </div>
        <p class="text-xs text-muted-foreground">{t('vmDetail.coverHint')}</p>

        <!-- Library picker: the same images available in Multimedia, so a
             cover can be reused without uploading it a second time. -->
        <div class="border-t border-border pt-3 space-y-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-medium">{t('vmDetail.coverFromLibrary')}</span>
            {#if mediaLoading}
              <span class="text-xs text-muted-foreground">{t('common.loading')}</span>
            {/if}
          </div>
          {#if mediaError}
            <p class="text-xs text-destructive">{mediaError}</p>
          {:else if !mediaLoading && mediaItems.length === 0}
            <p class="text-xs text-muted-foreground">{t('vmDetail.coverLibraryEmpty')}</p>
          {:else}
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 max-h-52 overflow-y-auto">
              {#each mediaItems as item (item.id)}
                <button
                  type="button"
                  class={cn(
                    'aspect-video rounded border overflow-hidden bg-muted transition-opacity hover:opacity-80 disabled:opacity-50',
                    vm?.cover === `/api/media/${item.id}/raw`
                      ? 'border-primary ring-1 ring-primary'
                      : 'border-border'
                  )}
                  title={item.name || item.id}
                  disabled={uploadingCover}
                  onclick={() => pickCoverFromLibrary(item)}
                >
                  <img
                    src="/api/media/{item.id}/raw"
                    alt={item.name || item.id}
                    loading="lazy"
                    class="w-full h-full object-cover"
                  />
                </button>
              {/each}
            </div>
          {/if}
        </div>
      </div>
    {:else if identityTab === 'network'}
      <div class="space-y-3">
        {#if vm?.state !== 'shutoff'}
          <div
            class="p-2.5 border border-accent/30 bg-accent/10 rounded-md text-accent text-xs flex items-start gap-2"
          >
            <svg
              class="w-4 h-4 shrink-0 mt-0.5"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              viewBox="0 0 24 24"
              ><path d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg
            >
            <span
              >El cambio de red o puente se aplica en caliente. La modificación de la dirección MAC
              física requiere apagar la máquina.</span
            >
          </div>
        {/if}
        {#if !vm?.networks || vm.networks.length === 0}
          <p class="text-sm text-muted-foreground">{t('vmDetail.noNetworksToEdit')}</p>
        {:else}
          <div class="space-y-2">
            {#each vm.networks as iface (iface.mac || iface.target || iface.name)}
              {@const edit = ifaceEdits[iface.mac] || {
                mac: iface.mac,
                network: iface.network,
                vlan: '',
                busy: false,
                error: '',
              }}
              {@const support = vlanSupportByNetwork[iface.network]}
              <div class="border border-border rounded-md bg-background p-3 space-y-2">
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-2">
                  <div>
                    <div class="block text-xs font-medium text-muted-foreground mb-1">
                      {t('vmDetail.mac')}
                    </div>
                    <Input
                      bind:value={edit.mac}
                      disabled={vm.state !== 'shutoff'}
                      class="font-mono text-xs"
                    />
                  </div>
                  <div>
                    <div class="block text-xs font-medium text-muted-foreground mb-1">
                      {t('vmDetail.tabNetwork')}
                    </div>
                    <select bind:value={edit.network} class="input !text-xs">
                      {#each vmNetworks as n (n)}
                        <option value={n.name}>{n.name}</option>
                      {/each}
                    </select>
                  </div>
                  <div>
                    <div class="block text-xs font-medium text-muted-foreground mb-1">
                      {t('vmDetail.vlanTag')}
                      <span class="text-muted-foreground font-normal">{t('vmDetail.vlanHint')}</span
                      >
                    </div>
                    <Input
                      bind:value={edit.vlan}
                      disabled={support && !support.supported}
                      placeholder="—"
                      class="tnum text-xs"
                    />
                  </div>
                </div>
                {#if support && !support.supported}
                  <p class="text-xs text-warning">
                    {t('vmDetail.vlanUnavailable', { reason: support.reason })}
                  </p>
                {/if}
                {#if edit.error}
                  <p class="text-xs text-destructive">{edit.error}</p>
                {/if}
                <div class="flex justify-end">
                  <Button
                    size="xs"
                    onclick={() => saveIface(iface.mac)}
                    disabled={edit.busy || (vm.state !== 'shutoff' && edit.mac !== iface.mac)}
                  >
                    {#if edit.busy}<Spinner size="xs" color="text-white" />{:else}{t(
                        'vmDetail.saveInterface'
                      )}{/if}
                  </Button>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {:else if identityTab === 'notes'}
      <div class="space-y-3">
        <div>
          <label for="ident-notes" class="block text-sm font-medium mb-1.5"
            >{t('vmDetail.notesLabel')}</label
          >
          <textarea
            id="ident-notes"
            bind:value={eNotes}
            onblur={() => saveNotesIfChanged()}
            rows="8"
            class="input !text-sm font-mono"
            placeholder={t('vmDetail.notesPlaceholder')}
          ></textarea>
          <p class="text-xs text-muted-foreground mt-1 flex items-center gap-1.5">
            {#if notesStatus === 'saving'}
              <Spinner size="xs" />
              <span>{t('vmDetail.notesSaving')}</span>
            {:else if notesStatus === 'saved'}
              <svg
                class="w-3 h-3 text-success"
                fill="none"
                stroke="currentColor"
                stroke-width="2.5"
                viewBox="0 0 24 24"><polyline points="20 6 9 17 4 12" /></svg
              >
              <span class="text-success">{t('vmDetail.notesSaved')}</span>
            {:else if notesStatus === 'error'}
              <svg
                class="w-3 h-3 text-destructive"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                viewBox="0 0 24 24"
                ><circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line
                  x1="12"
                  y1="16"
                  x2="12.01"
                  y2="16"
                /></svg
              >
              <span class="text-destructive">{notesError}</span>
            {:else}
              <span>{t('vmDetail.autoSaves')}</span>
            {/if}
          </p>
        </div>
      </div>
    {:else if identityTab === 'groups'}
      <div class="space-y-3">
        <div>
          <span class="block text-sm font-medium mb-1.5">{t('vmDetail.groupsLabel')}</span>
          {#if eGroupsList.length === 0}
            <p class="text-sm text-muted-foreground">{t('vmDetail.noGroupsCreateFirst')}</p>
          {:else}
            <!-- Toggle chips only — never free text. A group name can
                 contain spaces (e.g. "APPS WEBS"); typed comma/space-
                 separated text used to split that into unregistered
                 fragments that never matched the real group. -->
            <div class="flex flex-wrap gap-1.5">
              {#each eGroupsList as g (g.name)}
                {@const selected = eGroupsSet.has(g.name)}
                <button
                  type="button"
                  onclick={() => {
                    const next = new Set(eGroupsSet);
                    if (next.has(g.name)) next.delete(g.name);
                    else next.add(g.name);
                    eGroupsSet = next;
                  }}
                  aria-pressed={selected}
                  class="text-xs px-2.5 py-1 rounded-full border transition-colors {selected
                    ? 'border-transparent text-white'
                    : ''}"
                  style={selected
                    ? `background-color: ${g.color}`
                    : `border-color: ${g.color}40; background-color: ${g.color}15; color: ${g.color}`}
                  >{g.name}</button
                >
              {/each}
            </div>
          {/if}
          <p class="text-xs text-muted-foreground mt-1.5">{@html t('vmDetail.groupsHelper')}</p>
        </div>
        <div class="flex justify-end gap-2 pt-2">
          <Button variant="outline" onclick={() => (showIdentity = false)}
            >{t('common.close')}</Button
          >
          <Button onclick={saveIdentityBasics} disabled={savingIdentity}
            >{savingIdentity ? t('vmDetail.notesSaving') : t('common.save')}</Button
          >
        </div>
      </div>
    {/if}
  </Dialog.Content>
</Dialog.Root>
