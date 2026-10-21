package libvirt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Fixture shaped like a real EXTERNAL snapshot XML: the two data disks
// are snapshot='external' with per-disk overlay files, the cdrom is
// excluded (snapshot='no').
const externalSnapshotXML = `<domainsnapshot>
  <name>1972set_up</name>
  <state>shutoff</state>
  <disks>
    <disk name='vda' snapshot='external' type='file'>
      <source file='/mnt/storage/work/VMS/kvm/isg_sync_ssd_3.1972set_up'/>
      <driver type='qcow2'/>
    </disk>
    <disk name='sda' snapshot='external' type='file'>
      <source file='/mnt/storage/work/VMS/kvm/isg_sync_ssd_3-data.1972set_up'/>
      <driver type='qcow2'/>
    </disk>
    <disk name='hdc' snapshot='no' type='block'>
      <driver type='raw'/>
      <target dev='hdc' bus='ide'/>
    </disk>
  </disks>
  <domain type='kvm'>
    <devices>
      <disk type='file' device='disk'>
        <driver name='qemu' type='qcow2'/>
        <source file='/mnt/storage/work/VMS/kvm/isg_sync_ssd_3.qcow2'/>
        <target dev='vda' bus='virtio'/>
      </disk>
      <disk type='file' device='disk'>
        <driver name='qemu' type='qcow2'/>
        <source file='/mnt/storage/work/VMS/kvm/isg_sync_ssd_3-data.qcow2'/>
        <target dev='sda' bus='scsi'/>
      </disk>
      <disk type='block' device='cdrom'>
        <driver name='qemu' type='raw'/>
        <target dev='hdc' bus='ide'/>
        <readonly/>
      </disk>
    </devices>
  </domain>
</domainsnapshot>`

func TestSnapshotExternalDiskOverlays(t *testing.T) {
	overlays, excluded := snapshotExternalDiskOverlays(externalSnapshotXML)

	wantOverlays := map[string]string{
		"vda": "/mnt/storage/work/VMS/kvm/isg_sync_ssd_3.1972set_up",
		"sda": "/mnt/storage/work/VMS/kvm/isg_sync_ssd_3-data.1972set_up",
	}
	for dev, want := range wantOverlays {
		if got := overlays[dev]; got != want {
			t.Errorf("overlays[%q] = %q, want %q", dev, got, want)
		}
	}
	if len(overlays) != len(wantOverlays) {
		t.Errorf("overlays = %v, want exactly %v", overlays, wantOverlays)
	}
	if !excluded["hdc"] {
		t.Errorf("excluded[hdc] = false, want true (snapshot='no')")
	}
	if len(excluded) != 1 {
		t.Errorf("excluded = %v, want only hdc", excluded)
	}
}

func TestSnapshotInternalDiskOverlays(t *testing.T) {
	// INTERNAL snapshot: disks carry snapshot='internal' and no overlay
	// source — both results must stay empty so the -l code path runs.
	xml := `<domainsnapshot>
  <name>internal1</name>
  <disks>
    <disk name='vda' snapshot='internal' type='file'/>
    <disk name='sda' snapshot='internal' type='file'/>
  </disks>
  <domain type='kvm'>
    <devices>
      <disk type='file' device='disk'>
        <source file='/mnt/pool/vm.qcow2'/>
        <target dev='vda' bus='virtio'/>
      </disk>
    </devices>
  </domain>
</domainsnapshot>`
	overlays, excluded := snapshotExternalDiskOverlays(xml)
	if len(overlays) != 0 {
		t.Errorf("overlays = %v, want empty (internal snapshot)", overlays)
	}
	if len(excluded) != 0 {
		t.Errorf("excluded = %v, want empty (internal snapshot)", excluded)
	}
}

func TestConvertDiskFromSnapshotExternal(t *testing.T) {
	qemuImg, err := exec.LookPath("qemu-img")
	if err != nil {
		t.Skip("qemu-img not available")
	}

	dir := t.TempDir()
	base := filepath.Join(dir, "base.qcow2")
	overlay := filepath.Join(dir, "base.snap")
	dst := filepath.Join(dir, "dst.qcow2")

	for _, f := range []string{base, overlay} {
		cmd := exec.Command(qemuImg, "create", "-f", "qcow2", f, "8M")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("qemu-img create %s: %v (%s)", f, err, out)
		}
	}
	// Rebase the overlay on top of base so convert reads the chain and
	// produces a flat copy.
	cmd := exec.Command(qemuImg, "rebase", "-F", "qcow2", "-b", base, overlay)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("qemu-img rebase: %v (%s)", err, out)
	}

	// External path: plain convert of the overlay, no -l.
	if err := convertDiskFromSnapshot(base, dst, "qcow2", "1972set_up", overlay); err != nil {
		t.Fatalf("convertDiskFromSnapshot (external): %v", err)
	}
	if info, err := os.Stat(dst); err != nil || info.Size() == 0 {
		t.Fatalf("dst not created properly: %v", err)
	}
	// The flat copy must not depend on the backing chain.
	out, err := exec.Command(qemuImg, "info", "--backing-file", dst).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "does not have a backing file") && !strings.Contains(string(out), "no backing file") {
		// `info --backing-file` errors when there is no backing file;
		// anything else means the image still has one.
		if err == nil {
			t.Errorf("dst unexpectedly reports a backing file:\n%s", out)
		}
	}

	// Unreadable overlay must produce the explicit "not readable" error.
	badDst := filepath.Join(dir, "bad.qcow2")
	err = convertDiskFromSnapshot(base, badDst, "qcow2", "1972set_up", filepath.Join(dir, "missing.overlay"))
	if err == nil || !strings.Contains(err.Error(), "not readable") {
		t.Errorf("missing overlay: err = %v, want 'not readable'", err)
	}
	if _, serr := os.Stat(badDst); !os.IsNotExist(serr) {
		t.Errorf("failed convert left %s behind", badDst)
	}
}
