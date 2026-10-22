package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"webkvm/internal/models"
)

// sysClassNet is the sysfs directory enumerating the host's network
// interfaces. A var so the handler stays trivial and all logic that is
// worth testing lives in the pure filter below.
const sysClassNet = "/sys/class/net"

// filterHostBridges is the pure core of ListHostBridges: given the entry
// names of /sys/class/net and a predicate reporting whether an interface
// is a Linux bridge (its sysfs dir has a "bridge" subdirectory), it
// returns the usable host bridge list, sorted by name for deterministic
// output.
//
// Loopback and per-VM tap devices (vnet*) are excluded — vnet devices are
// never bridges anyway, but the guard costs nothing and mirrors
// ListHostInterfaces. libvirt NAT bridges (virbr*) and other system
// bridges (vmbr*, br-*, lxdbr*, ...) are genuine Linux bridges, so they
// stay: an operator may legitimately point a VM NIC at any of them.
func filterHostBridges(names []string, isBridge func(string) bool) []models.HostBridge {
	out := []models.HostBridge{}
	for _, name := range names {
		if name == "lo" || strings.HasPrefix(name, "vnet") {
			continue
		}
		if !isBridge(name) {
			continue
		}
		out = append(out, models.HostBridge{Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ListHostBridges lists existing Linux bridges on the host
// (GET /api/host/bridges). Read-only diagnostics like its neighbour
// ListHostInterfaces: available to every authenticated user, no role
// gate — the pickers that consume it enforce the network allowlist
// server-side on the PATCH anyway.
func (h *Handler) ListHostBridges(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(sysClassNet)
	if err != nil {
		// No /sys/class/net (exotic container?) — an empty list, not a 500.
		jsonResp(w, http.StatusOK, []models.HostBridge{})
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	out := filterHostBridges(names, func(name string) bool {
		_, err := os.Stat(filepath.Join(sysClassNet, name, "bridge"))
		return err == nil
	})
	for i := range out {
		if data, rerr := os.ReadFile(filepath.Join(sysClassNet, out[i].Name, "operstate")); rerr == nil {
			out[i].State = strings.TrimSpace(string(data))
		} else {
			out[i].State = "unknown"
		}
	}
	jsonResp(w, http.StatusOK, out)
}
