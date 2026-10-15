package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"webkvm/internal/compute"
	"webkvm/internal/models"

	"github.com/go-chi/chi/v5"
)

// DHCP reservation (static lease) management for nat/isolated bridges
// with dnsmasq DHCP enabled. Reservations live on the network record
// (netStore, surfaced as models.Network.Reservations) and are rendered
// into the bridge's dnsmasq config as dhcp-host= lines by
// libvirt.UpdateNetwork — so every mutation here is a read-modify-write
// of that set through the same UpdateNetwork path the edit dialog uses,
// never a second writer with its own config format.

// findNetwork returns the current view of one bridge by name.
func (h *Handler) findNetwork(name string) (models.Network, bool, error) {
	nets, err := h.compute.ListNetworks()
	if err != nil {
		return models.Network{}, false, err
	}
	for _, n := range nets {
		if n.Name == name {
			return n, true, nil
		}
	}
	return models.Network{}, false, nil
}

// ListDHCPReservations handles GET /api/networks/{id}/dhcp.
func (h *Handler) ListDHCPReservations(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.checkNetworkAccess(r, id); err != nil {
		jsonErr(w, http.StatusForbidden, err.Error())
		return
	}
	nw, ok, err := h.findNetwork(id)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		jsonErr(w, http.StatusNotFound, fmt.Sprintf("network %q not found", id))
		return
	}
	// Return [] (not null) so clients can iterate unconditionally.
	empty := []models.DHCPReservation{}
	if nw.Reservations == nil {
		jsonResp(w, http.StatusOK, empty)
		return
	}
	jsonResp(w, http.StatusOK, nw.Reservations)
}

// AddDHCPReservation handles POST /api/networks/{id}/dhcp with a
// models.DHCPReservation body {mac, ip, name}. A reservation for a MAC
// already in the set replaces that entry (a MAC can only ever hold one
// fixed IP), mirroring how dnsmasq treats duplicate dhcp-host MACs.
func (h *Handler) AddDHCPReservation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.checkNetworkAccess(r, id); err != nil {
		jsonErr(w, http.StatusForbidden, err.Error())
		return
	}
	var req models.DHCPReservation
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	mac := strings.ToLower(strings.TrimSpace(req.MAC))
	if _, err := net.ParseMAC(mac); err != nil {
		jsonErr(w, http.StatusBadRequest, fmt.Sprintf("invalid MAC %q", req.MAC))
		return
	}
	ip := net.ParseIP(strings.TrimSpace(req.IP))
	if ip == nil || ip.To4() == nil {
		jsonErr(w, http.StatusBadRequest, fmt.Sprintf("invalid IPv4 %q", req.IP))
		return
	}

	nw, ok, err := h.findNetwork(id)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		jsonErr(w, http.StatusNotFound, fmt.Sprintf("network %q not found", id))
		return
	}
	// Only nat/isolated bridges run the per-bridge dnsmasq that serves
	// fixed leases; direct bridges pass traffic straight through.
	if nw.Kind != "nat" && nw.Kind != "isolated" {
		jsonErr(w, http.StatusBadRequest, "DHCP reservations are only supported on nat/isolated networks")
		return
	}
	if !nw.DHCP {
		jsonErr(w, http.StatusBadRequest, "DHCP must be enabled on the network before adding reservations")
		return
	}

	// Store the normalized form (lowercase MAC, canonical IPv4, trimmed
	// name) — the same shape libvirt.normalizeReservations persists.
	req = models.DHCPReservation{MAC: mac, IP: ip.String(), Name: strings.TrimSpace(req.Name)}

	// Pre-validate against the bridge's live CIDR so the common failure
	// (IP outside the subnet, or an IP another reservation already pins)
	// is a clean 400 here instead of a 500 from the libvirt layer's
	// normalizeReservations backstop.
	ip = ip.To4()
	if nw.CIDR != "" {
		_, ipnet, perr := net.ParseCIDR(nw.CIDR)
		if perr != nil || !ipnet.Contains(ip) {
			jsonErr(w, http.StatusBadRequest, fmt.Sprintf("%s is outside %s", req.IP, nw.CIDR))
			return
		}
	}
	for _, cur := range nw.Reservations {
		if strings.ToLower(cur.MAC) == mac {
			continue // same MAC being re-pointed is fine
		}
		if net.ParseIP(cur.IP).Equal(ip) {
			jsonErr(w, http.StatusBadRequest, fmt.Sprintf("IP %s is already reserved for %s", req.IP, cur.MAC))
			return
		}
	}

	merged := make([]models.DHCPReservation, 0, len(nw.Reservations)+1)
	replaced := false
	for _, cur := range nw.Reservations {
		if strings.ToLower(cur.MAC) == mac {
			merged = append(merged, req)
			replaced = true
			continue
		}
		merged = append(merged, cur)
	}
	if !replaced {
		merged = append(merged, req)
	}

	if _, err := h.compute.UpdateNetwork(id, models.UpdateNetworkRequest{Reservations: merged}); err != nil {
		jsonErr(w, updateNetworkErrStatus(err), err.Error())
		return
	}
	h.audit.Log(auditFor(r, "network.dhcp.add", id, map[string]any{"mac": mac, "ip": req.IP}))
	jsonResp(w, http.StatusOK, merged)
}

// DeleteDHCPReservation handles DELETE /api/networks/{id}/dhcp/{mac}.
func (h *Handler) DeleteDHCPReservation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.checkNetworkAccess(r, id); err != nil {
		jsonErr(w, http.StatusForbidden, err.Error())
		return
	}
	// Same percent-encoding note as ReleaseNetworkLease: a MAC's ':'
	// reaches chi as %3A, so URLParam alone would hand back "%3A".
	mac, err := url.PathUnescape(chi.URLParam(r, "mac"))
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid mac parameter")
		return
	}
	mac = strings.ToLower(strings.TrimSpace(mac))

	nw, ok, err := h.findNetwork(id)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		jsonErr(w, http.StatusNotFound, fmt.Sprintf("network %q not found", id))
		return
	}
	remaining := make([]models.DHCPReservation, 0, len(nw.Reservations))
	found := false
	for _, cur := range nw.Reservations {
		if strings.ToLower(cur.MAC) == mac {
			found = true
			continue
		}
		remaining = append(remaining, cur)
	}
	if !found {
		jsonErr(w, http.StatusNotFound, fmt.Sprintf("no reservation for MAC %q on %q", mac, id))
		return
	}

	if _, err := h.compute.UpdateNetwork(id, models.UpdateNetworkRequest{Reservations: remaining}); err != nil {
		jsonErr(w, updateNetworkErrStatus(err), err.Error())
		return
	}
	h.audit.Log(auditFor(r, "network.dhcp.remove", id, map[string]any{"mac": mac}))
	jsonResp(w, http.StatusOK, remaining)
}

// updateNetworkErrStatus maps UpdateNetwork failures to the same codes
// the sibling network handlers use: 404 for a missing bridge, 400 for
// validation (e.g. an IP outside the bridge CIDR, rejected by
// libvirt.normalizeReservations), 500 otherwise.
func updateNetworkErrStatus(err error) int {
	if errors.Is(err, compute.ErrNetworkNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
