package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"webkvm/internal/audit"
	"webkvm/internal/auth"
	"webkvm/internal/compute"
	"webkvm/internal/models"
)

// dhcpBackend is a minimal compute.Backend stub: ListNetworks serves a
// fixed bridge view and UpdateNetwork records the reservation set it
// was given (the handlers do read-modify-write through UpdateNetwork,
// so capturing those calls is the whole contract under test).
type dhcpBackend struct {
	compute.Backend
	mu      sync.Mutex
	nets    []models.Network
	updates []models.UpdateNetworkRequest
}

func (b *dhcpBackend) ListNetworks() ([]models.Network, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]models.Network, len(b.nets))
	copy(out, b.nets)
	return out, nil
}

func (b *dhcpBackend) UpdateNetwork(name string, req models.UpdateNetworkRequest) (models.Network, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.updates = append(b.updates, req)
	for i, n := range b.nets {
		if n.Name == name {
			if req.Reservations != nil {
				b.nets[i].Reservations = req.Reservations
			}
			return b.nets[i], nil
		}
	}
	return models.Network{}, compute.ErrNetworkNotFound
}

func newDHCPHandler(t *testing.T, be *dhcpBackend) http.Handler {
	t.Helper()
	aud, err := audit.New(t.TempDir() + "/audit.log")
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{compute: be, audit: aud}
	// The handlers read the network name via chi.URLParam, so requests
	// must go through a mux that populates the route context.
	r := chi.NewRouter()
	r.Get("/api/networks/{id}/dhcp", h.ListDHCPReservations)
	r.Post("/api/networks/{id}/dhcp", h.AddDHCPReservation)
	r.Delete("/api/networks/{id}/dhcp/{mac}", h.DeleteDHCPReservation)
	return r
}

func dhcpNet() models.Network {
	on := true
	_ = on
	return models.Network{
		Name:      "nat0",
		Kind:      "nat",
		CIDR:      "192.168.100.0/24",
		DHCP:      true,
		DHCPStart: "192.168.100.100",
		DHCPEnd:   "192.168.100.200",
		Reservations: []models.DHCPReservation{
			{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.100.10", Name: "existing"},
		},
	}
}

func adminReq(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-User", "root")
	req.Header.Set("X-Role", models.RoleAdmin)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestDHCPReservations_AddRemove(t *testing.T) {
	be := &dhcpBackend{nets: []models.Network{dhcpNet()}}
	mux := newDHCPHandler(t, be)

	// Add a new reservation: 200, whole merged set returned, one
	// UpdateNetwork carrying exactly the merged set.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, adminReq(http.MethodPost, "/api/networks/nat0/dhcp",
		`{"mac":"AA:BB:CC:DD:EE:02","ip":"192.168.100.11","name":"web"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("add expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(be.updates) != 1 || len(be.updates[0].Reservations) != 2 {
		t.Fatalf("expected one UpdateNetwork with 2 reservations, got %+v", be.updates)
	}
	if be.updates[0].Reservations[1].MAC != "aa:bb:cc:dd:ee:02" {
		t.Fatalf("MAC not lowercased: %+v", be.updates[0].Reservations[1])
	}

	// Re-adding the same MAC replaces the entry (and moves its IP) rather
	// than duplicating it.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, adminReq(http.MethodPost, "/api/networks/nat0/dhcp",
		`{"mac":"aa:bb:cc:dd:ee:02","ip":"192.168.100.12"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("re-add expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(be.updates[1].Reservations) != 2 || be.updates[1].Reservations[1].IP != "192.168.100.12" {
		t.Fatalf("re-add should replace in place, got %+v", be.updates[1].Reservations)
	}

	// Delete by MAC: entry removed, remaining set persisted.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, adminReq(http.MethodDelete, "/api/networks/nat0/dhcp/aa:bb:cc:dd:ee:01", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(be.updates[2].Reservations) != 1 || be.updates[2].Reservations[0].MAC != "aa:bb:cc:dd:ee:02" {
		t.Fatalf("delete should keep only the other entry, got %+v", be.updates[2].Reservations)
	}

	// Deleting an unknown MAC is a 404, not a silent no-op.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, adminReq(http.MethodDelete, "/api/networks/nat0/dhcp/11:22:33:44:55:66", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete unknown MAC expected 404, got %d", rec.Code)
	}

	// List reflects the post-delete state.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, adminReq(http.MethodGet, "/api/networks/nat0/dhcp", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "aa:bb:cc:dd:ee:02") || strings.Contains(rec.Body.String(), "aa:bb:cc:dd:ee:01") {
		t.Fatalf("list body unexpected: %s", rec.Body.String())
	}
}

func TestDHCPReservations_Validation(t *testing.T) {
	be := &dhcpBackend{nets: []models.Network{dhcpNet()}}
	mux := newDHCPHandler(t, be)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"bad MAC", `{"mac":"nope","ip":"192.168.100.5"}`, http.StatusBadRequest},
		{"bad IP", `{"mac":"aa:bb:cc:dd:ee:09","ip":"999.1.1.1"}`, http.StatusBadRequest},
		{"IPv6", `{"mac":"aa:bb:cc:dd:ee:09","ip":"fd00::1"}`, http.StatusBadRequest},
		{"outside subnet", `{"mac":"aa:bb:cc:dd:ee:09","ip":"10.0.0.5"}`, http.StatusBadRequest},
		{"duplicate IP", `{"mac":"aa:bb:cc:dd:ee:09","ip":"192.168.100.10"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, adminReq(http.MethodPost, "/api/networks/nat0/dhcp", tc.body))
			if rec.Code != tc.want {
				t.Fatalf("expected %d, got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
	if len(be.updates) != 0 {
		t.Fatalf("no rejected request may reach UpdateNetwork, got %+v", be.updates)
	}
}

func TestDHCPReservations_Guards(t *testing.T) {
	off := dhcpNet()
	off.DHCP = false
	direct := models.Network{Name: "vmbr0", Kind: "direct", DHCP: false}

	t.Run("DHCP off -> 400", func(t *testing.T) {
		be := &dhcpBackend{nets: []models.Network{off}}
		mux := newDHCPHandler(t, be)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, adminReq(http.MethodPost, "/api/networks/nat0/dhcp",
			`{"mac":"aa:bb:cc:dd:ee:09","ip":"192.168.100.5"}`))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("direct network -> 400", func(t *testing.T) {
		be := &dhcpBackend{nets: []models.Network{direct}}
		mux := newDHCPHandler(t, be)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, adminReq(http.MethodPost, "/api/networks/vmbr0/dhcp",
			`{"mac":"aa:bb:cc:dd:ee:09","ip":"192.168.100.5"}`))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("unknown network -> 404", func(t *testing.T) {
		be := &dhcpBackend{nets: []models.Network{dhcpNet()}}
		mux := newDHCPHandler(t, be)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, adminReq(http.MethodGet, "/api/networks/nope/dhcp", ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

// TestDHCPReservations_RBAC mirrors TestNetworksRBAC_RouteMapping for the
// three new routes: listing is for any authenticated user, mutations are
// admin-only (they rewrite dnsmasq config and restart it).
func TestDHCPReservations_RBAC(t *testing.T) {
	okStub := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	r := chi.NewRouter()
	r.Route("/api/networks", func(r chi.Router) {
		r.Get("/{id}/dhcp", okStub)
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole(models.RoleAdmin))
			r.Post("/{id}/dhcp", okStub)
			r.Delete("/{id}/dhcp/{mac}", okStub)
		})
	})

	operator := http.Header{"X-User": {"op"}, "X-Role": {models.RoleOperator}}
	admin := http.Header{"X-User": {"root"}, "X-Role": {models.RoleAdmin}}
	cases := []struct {
		method, path string
		hdr          http.Header
		want         int
	}{
		{http.MethodGet, "/api/networks/nat0/dhcp", operator, http.StatusOK},
		{http.MethodPost, "/api/networks/nat0/dhcp", operator, http.StatusForbidden},
		{http.MethodDelete, "/api/networks/nat0/dhcp/aa:bb:cc:dd:ee:ff", operator, http.StatusForbidden},
		{http.MethodGet, "/api/networks/nat0/dhcp", admin, http.StatusOK},
		{http.MethodPost, "/api/networks/nat0/dhcp", admin, http.StatusOK},
		{http.MethodDelete, "/api/networks/nat0/dhcp/aa:bb:cc:dd:ee:ff", admin, http.StatusOK},
	}
	for i, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		for k, vs := range tc.hdr {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("case %d (%s %s): expected %d, got %d", i, tc.method, tc.path, tc.want, rec.Code)
		}
	}
}
