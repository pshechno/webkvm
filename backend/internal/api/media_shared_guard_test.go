package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"webkvm/internal/compute"
	"webkvm/internal/config"
	"webkvm/internal/models"
)

// The one-writer attachment guard must not block read-only media: QEMU
// opens a cdrom's ISO read-only, so one install image may boot any
// number of VMs. These tests pin the single exemption of
// sourceMayBeShared (cdrom device) and the cases it must keep refusing
// (data disk images, even when the file sits in an ISO pool, and files
// outside every pool).
func TestSourceMayBeShared(t *testing.T) {
	root := t.TempDir()
	isos := filepath.Join(root, "isos")
	disks := filepath.Join(root, "disks")
	for _, d := range []string{isos, disks} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	iso := filepath.Join(isos, "debian.iso")
	disk := filepath.Join(disks, "vm.qcow2")
	for _, f := range []string{iso, disk} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := &Handler{compute: &fakePoolBackend{pools: []models.StoragePool{
		{Name: "ISOS", Type: "dir", Path: isos, Purpose: compute.PoolPurposeISO},
		{Name: "webkvm-disks", Type: "dir", Path: disks, Purpose: compute.PoolPurposeDisk},
	}}}

	cases := []struct {
		name   string
		device string
		source string
		want   bool
	}{
		{"cdrom device is shared regardless of location", "cdrom", disk, true},
		{"cdrom device without a source", "cdrom", "", true},
		{"file in an ISO pool is still a writer on a data disk", "disk", iso, false},
		{"disk image in a disk pool is not shared", "disk", disk, false},
		{"file outside every pool is not shared", "disk", filepath.Join(root, "loose.img"), false},
		{"empty source is not shared", "disk", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := h.sourceMayBeShared(c.device, c.source); got != c.want {
				t.Errorf("sourceMayBeShared(%q, %q) = %v, want %v", c.device, c.source, got, c.want)
			}
		})
	}
}

// The one-writer guard itself is unchanged: a data disk image already
// attached to another VM is still refused, while re-attaching to the
// same VM stays idempotent. The cdrom exemption lives in
// sourceMayBeShared and is decided by the handlers before the guard.
func TestCheckSourceNotAttachedElsewhereStillGuardsDataDisks(t *testing.T) {
	disks := t.TempDir()
	disk := filepath.Join(disks, "vm.qcow2")
	if err := os.WriteFile(disk, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &Handler{compute: &fakePoolBackend{
		pools: []models.StoragePool{
			{Name: "webkvm-disks", Type: "dir", Path: disks, Purpose: compute.PoolPurposeDisk},
		},
		attachments: map[string][]models.VolumeAttachment{
			"webkvm-disks/vm.qcow2": {{VMID: "vm-1", VMName: "one", Target: "vda"}},
		},
	}}
	if err := h.checkSourceNotAttachedElsewhere("vm-2", disk); err == nil {
		t.Error("data disk attached to another VM was not refused")
	}
	if err := h.checkSourceNotAttachedElsewhere("vm-1", disk); err != nil {
		t.Errorf("idempotent re-attach to the same VM was refused: %v", err)
	}
}

// ListPools flags dir pools whose path lives outside the managed pools
// directory (and that are not built-in) as external: WebKVM only looks
// at the files there, it never creates or deletes them. Everything
// under PoolsDir, every built-in pool and every non-dir pool stays
// non-external.
func TestListPoolsMarksExternal(t *testing.T) {
	root := t.TempDir()
	shelf := filepath.Join(root, "isos")
	if err := os.Mkdir(shelf, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{DataDir: filepath.Join(root, "data")}
	extra := filepath.Join(cfg.PoolsDir(), "extra-isos")
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		cfg: cfg,
		compute: &fakePoolBackend{pools: []models.StoragePool{
			{Name: "webkvm-isos", Type: "dir", Path: cfg.ISOPoolPath(), Purpose: compute.PoolPurposeISO},
			{Name: "webkvm-disks", Type: "dir", Path: cfg.DiskPoolPath(), Purpose: compute.PoolPurposeDisk},
			{Name: "extra-isos", Type: "dir", Path: extra, Purpose: compute.PoolPurposeISO},
			{Name: "shelf", Type: "dir", Path: shelf, Purpose: compute.PoolPurposeISO},
			{Name: "nas", Type: "netfs", Path: "/mnt/nas", Purpose: compute.PoolPurposeISO},
		}},
	}

	rr := httptest.NewRecorder()
	h.ListPools(rr, httptest.NewRequest(http.MethodGet, "/api/storage/pools", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("ListPools status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var pools []models.StoragePool
	if err := json.Unmarshal(rr.Body.Bytes(), &pools); err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(pools))
	for _, p := range pools {
		got[p.Name] = p.External
	}
	want := map[string]bool{
		"webkvm-isos":  false, // built-in
		"webkvm-disks": false, // built-in
		"extra-isos":   false, // dir pool inside PoolsDir
		"shelf":        true,  // dir pool outside PoolsDir, not built-in
		"nas":          false, // not a dir pool
	}
	for name, wantExternal := range want {
		if got[name] != wantExternal {
			t.Errorf("pool %q external = %v, want %v", name, got[name], wantExternal)
		}
	}
	if len(got) != len(want) {
		t.Errorf("ListPools returned %d pools, want %d", len(got), len(want))
	}
}
