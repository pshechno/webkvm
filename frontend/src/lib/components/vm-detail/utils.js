// Pure display/format helpers shared by the VmDetail section components.
// Logic moved verbatim from routes/VmDetail.svelte — behaviour unchanged.

export function bytesToStr(b) {
  if (!b) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let n = b;
  while (n >= 1024 && i < u.length - 1) {
    n /= 1024;
    i++;
  }
  return n.toFixed(i > 0 ? 1 : 0) + ' ' + u[i];
}

export function diskLabel(d) {
  if (d.device === 'cdrom') return d.name || (d.source ? d.source.split('/').pop() : '(empty)');
  if (d.zvol) return `zvol: ${d.zvol}`;
  if (d.block_dev) return d.block_dev;
  return d.name || (d.source ? d.source.split('/').pop() : d.target);
}

// Build a snapshot tree from the flat list. Roots have parent_name == "".
export function buildSnapshotTree(flat) {
  if (!Array.isArray(flat) || flat.length === 0) return { roots: [], byId: {} };
  const byId = {};
  for (const s of flat) byId[s.name] = { ...s, children: [] };
  const roots = [];
  for (const k of Object.keys(byId)) {
    const node = byId[k];
    if (node.parent_name && byId[node.parent_name]) {
      byId[node.parent_name].children.push(node);
    } else {
      roots.push(node);
    }
  }
  // Sort by creation time ascending so children appear below parents.
  const sortRec = (nodes) => {
    nodes.sort((a, b) => (a.creation_time || 0) - (b.creation_time || 0));
    nodes.forEach((n) => sortRec(n.children));
  };
  sortRec(roots);
  return { roots, byId };
}

export function formatSnapshotDate(epoch) {
  if (!epoch) return '—';
  const d = new Date(epoch * 1000);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  return `${y}-${m}-${day} ${hh}:${mm}`;
}

// NIC models offered when editing an interface (unchanged labels).
export const networkModels = [
  { value: 'virtio', label: 'virtio (recommended)' },
  { value: 'e1000e', label: 'e1000e (Intel, ideal for Windows)' },
  { value: 'e1000', label: 'e1000 (legacy Intel)' },
  { value: 'rtl8139', label: 'rtl8139 (Realtek, very compatible)' },
  { value: 'pcnet', label: 'pcnet (AMD, legacy)' },
];
