import { describe, it, expect, vi, afterEach } from 'vitest';
import { mount, unmount, flushSync } from 'svelte';
import VmSnapshotsCard from './VmSnapshotsCard.svelte';

let cleanup = [];
function render(props = {}) {
  const target = document.createElement('div');
  document.body.appendChild(target);
  const app = mount(VmSnapshotsCard, { target, props });
  flushSync();
  cleanup.push(() => {
    unmount(app);
    target.remove();
  });
  return target;
}

afterEach(() => {
  cleanup.forEach((fn) => fn());
  cleanup = [];
});

// Regression guard for the revert-gating removal: reverting used to be
// disabled for disk-only snapshots on a running VM (isRevertBlocked),
// and the whole card could crash on prop/import mistakes — a crash class
// the production build does not catch (see TODO.md). These tests mount
// the real component and assert the buttons stay usable and handlers
// receive the snapshot id.
const snapshots = [
  { id: 'base', name: 'base', creation_time: 100, parent_name: '', current: false, has_memory: false },
  { id: 'child', name: 'child', creation_time: 200, parent_name: 'base', current: true, has_memory: true },
  // Orphan: its parent was deleted (deleting a snapshot keeps
  // descendants now) — buildSnapshotTree must re-root it.
  { id: 'orphan', name: 'orphan', creation_time: 300, parent_name: 'gone', current: false, has_memory: false },
];

const baseProps = {
  vmId: 'vm-1',
  snapshots,
  schedSnap: '',
  schedSnapMax: 3,
  schedSaving: false,
};

describe('VmSnapshotsCard', () => {
  it('renders orphans as roots and never disables the revert button', () => {
    const root = render({ ...baseProps });
    const revert = [...root.querySelectorAll('button')].filter(
      (b) => b.getAttribute('title') === 'Revert'
    );
    expect(revert.length).toBe(3);
    // Reverting is allowed while the VM runs: a memory snapshot reverts
    // live, a disk-only one is forced by the backend (REVERT_FORCE).
    expect(revert.every((b) => !b.disabled)).toBe(true);
    const names = [...root.querySelectorAll('p.text-sm')].map((p) =>
      p.textContent.trim()
    );
    expect(names.some((n) => n.startsWith('orphan'))).toBe(true);
  });

  it('passes the snapshot id to the revert and delete handlers', () => {
    const onRevert = vi.fn();
    const onDelete = vi.fn();
    const root = render({ ...baseProps, onRevert, onDelete });
    const buttons = [...root.querySelectorAll('button')];
    const revert = buttons.find((b) => b.getAttribute('title') === 'Revert');
    revert.click();
    // First node in creation order is 'base', not the current one.
    expect(onRevert).toHaveBeenCalledWith('base', false);
    const del = buttons.find((b) => b.textContent.trim() === 'Delete');
    del.click();
    expect(onDelete).toHaveBeenCalledWith('base');
  });
});
