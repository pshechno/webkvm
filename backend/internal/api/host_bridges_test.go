package api

import "testing"

func TestFilterHostBridges(t *testing.T) {
	// Fixture listing as /sys/class/net would enumerate it (order is
	// kernel-dependent, hence the sorted output contract).
	names := []string{
		"vnet3", "lo", "vmbr0", "eth0", "virbr0", "br-1a2b3c", "docker0", "bond0", "wlpxy",
	}
	isBridge := func(name string) bool {
		switch name {
		case "vmbr0", "virbr0", "br-1a2b3c", "docker0":
			return true
		}
		return false
	}

	got := filterHostBridges(names, isBridge)
	want := []string{"br-1a2b3c", "docker0", "virbr0", "vmbr0"}
	if len(got) != len(want) {
		t.Fatalf("got %d bridges, want %d: %+v", len(got), len(want), got)
	}
	for i, br := range got {
		if br.Name != want[i] {
			t.Errorf("bridge[%d] = %q, want %q (output must be sorted)", i, br.Name, want[i])
		}
		if br.State != "" {
			t.Errorf("bridge %q: state should be filled in by the handler, got %q", br.Name, br.State)
		}
	}
}

func TestFilterHostBridgesEmptyAndNil(t *testing.T) {
	isBridge := func(string) bool { return true }
	if got := filterHostBridges(nil, isBridge); got == nil || len(got) != 0 {
		t.Errorf("nil input: want empty non-nil slice, got %+v", got)
	}
	// Only excluded names -> still empty, but a JSON-encoded [] not null.
	if got := filterHostBridges([]string{"lo", "vnet0", "vnet12"}, isBridge); len(got) != 0 {
		t.Errorf("want no bridges from lo/vnet*, got %+v", got)
	}
}
