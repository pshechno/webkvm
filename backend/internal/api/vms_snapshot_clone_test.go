package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webkvm/internal/compute"
	"webkvm/internal/models"

	"github.com/go-chi/chi/v5"
)

// snapCloneBackend records what CloneFromSnapshot (handler) does to the
// compute layer so the happy path and the fast-fail validations can be
// observed without a hypervisor.
type snapCloneBackend struct {
	compute.Backend
	state      models.VMState // source VM state; "" = shutoff
	getErr     error
	cloneErr   error // CloneFromSnapshot fails with this (e.g. snapshot not found)
	cloneCalls []string
	started    []string
	updates    []models.VMMetaUpdate
}

func (b *snapCloneBackend) GetDomain(id string) (models.VM, error) {
	if b.getErr != nil {
		return models.VM{}, b.getErr
	}
	state := b.state
	if state == "" {
		state = models.VMStateShutoff
	}
	return models.VM{ID: id, Name: id, State: state}, nil
}

func (b *snapCloneBackend) CloneFromSnapshot(domainID, snapID string, req models.CloneFromSnapshotRequest) (models.VM, error) {
	if b.cloneErr != nil {
		return models.VM{}, b.cloneErr
	}
	b.cloneCalls = append(b.cloneCalls, domainID+"/"+snapID+"->"+req.Name)
	return models.VM{ID: "clone-1", Name: req.Name}, nil
}

func (b *snapCloneBackend) StartDomain(id string) error {
	b.started = append(b.started, id)
	return nil
}

func (b *snapCloneBackend) UpdateVMMeta(id string, upd models.VMMetaUpdate) (models.VMMeta, error) {
	b.updates = append(b.updates, upd)
	return models.VMMeta{}, nil
}

func snapCloneRequest(t *testing.T, h *Handler, b *snapCloneBackend, id, sid, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/vms/"+id+"/snapshots/"+sid+"/clone", strings.NewReader(body))
	req.Header.Set("X-User", "root")
	req.Header.Set("X-Role", models.RoleAdmin)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", id)
	rc.URLParams.Add("sid", sid)
	rec := httptest.NewRecorder()
	h.CloneFromSnapshot(rec, req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc)))
	// The clone itself runs in a background job; give it a moment and
	// re-check the terminal state so the assertions below are stable.
	if rec.Code == http.StatusAccepted {
		var jobID string
		if dec := decodeJSONMap(rec.Body.Bytes()); dec != nil {
			jobID, _ = dec["job"].(string)
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			j, ok := getJob(jobID)
			if ok && (j.Status == "done" || j.Status == "error") {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	return rec
}

func decodeJSONMap(b []byte) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

func TestCloneFromSnapshot_Validation(t *testing.T) {
	h := &Handler{compute: &snapCloneBackend{}}

	// Missing name.
	rec := snapCloneRequest(t, h, nil, "vm1", "s1", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty name, got %d: %s", rec.Code, rec.Body.String())
	}
	// Invalid JSON body.
	rec = snapCloneRequest(t, h, nil, "vm1", "s1", `{name`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid body, got %d: %s", rec.Code, rec.Body.String())
	}
	// Invalid VM name.
	rec = snapCloneRequest(t, h, nil, "vm1", "s1", `{"name":"../etc"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid VM name, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCloneFromSnapshot_RejectsRunningVM(t *testing.T) {
	b := &snapCloneBackend{state: models.VMStateRunning}
	h := &Handler{compute: b}

	rec := snapCloneRequest(t, h, b, "vm1", "s1", `{"name":"vm1-fromsnap"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a running source VM, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(b.cloneCalls) != 0 {
		t.Fatalf("clone must not be attempted for a running VM: %v", b.cloneCalls)
	}
}

func TestCloneFromSnapshot_SourceLookupFailed(t *testing.T) {
	b := &snapCloneBackend{getErr: errors.New("no such domain")}
	h := &Handler{compute: b}

	rec := snapCloneRequest(t, h, b, "vm1", "s1", `{"name":"vm1-fromsnap"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown source VM, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(b.cloneCalls) != 0 {
		t.Fatalf("clone must not be attempted: %v", b.cloneCalls)
	}
}

func TestCloneFromSnapshot_HappyPath(t *testing.T) {
	b := &snapCloneBackend{}
	h := &Handler{compute: b}

	rec := snapCloneRequest(t, h, b, "vm1", "golden", `{"name":"vm1-fromsnap","start":true}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 with a job, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(b.cloneCalls) != 1 || b.cloneCalls[0] != "vm1/golden->vm1-fromsnap" {
		t.Fatalf("unexpected clone calls: %v", b.cloneCalls)
	}
	if len(b.started) != 1 || b.started[0] != "clone-1" {
		t.Fatalf("start=true must boot the new VM: %v", b.started)
	}
	// The copy must not inherit the source's Shared flag or owner.
	if len(b.updates) != 1 {
		t.Fatalf("expected exactly one meta update, got %v", b.updates)
	}
	upd := b.updates[0]
	if upd.Shared == nil || *upd.Shared {
		t.Fatalf("clone must not be shared: %+v", upd)
	}
	if upd.OwnerID == nil || *upd.OwnerID != "root" {
		t.Fatalf("clone must be owned by the caller: %+v", upd)
	}
	// The job must have finished successfully carrying the new VM.
	m := decodeJSONMap(rec.Body.Bytes())
	jobID, _ := m["job"].(string)
	j, ok := getJob(jobID)
	if !ok || j.Status != "done" {
		t.Fatalf("job %q status=%v ok=%v", jobID, j, ok)
	}
	if vm, vok := j.Result.(models.VM); !vok || vm.ID != "clone-1" {
		t.Fatalf("job result must be the new VM, got %#v", j.Result)
	}
}

func TestCloneFromSnapshot_SnapshotNotFound(t *testing.T) {
	b := &snapCloneBackend{cloneErr: errors.New("lookup snapshot \"ghost\": not found")}
	h := &Handler{compute: b}

	rec := snapCloneRequest(t, h, b, "vm1", "ghost", `{"name":"vm1-fromsnap"}`)
	// The snapshot lookup happens on the compute layer inside the job,
	// so the request itself is accepted (202) and the job carries the error.
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	m := decodeJSONMap(rec.Body.Bytes())
	jobID, _ := m["job"].(string)
	j, ok := getJob(jobID)
	if !ok || j.Status != "error" {
		t.Fatalf("job must fail for a missing snapshot, got status=%v ok=%v", j.Status, ok)
	}
	if !strings.Contains(j.Error, "ghost") {
		t.Fatalf("job error must name the snapshot: %q", j.Error)
	}
}
