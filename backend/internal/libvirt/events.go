package libvirt

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"libvirt.org/go/libvirt"
	"webkvm/internal/events"
	"webkvm/internal/safego"
)

// StartEventLoop keeps the SSE channel fed with VM state changes.
//
// Two independent mechanisms run side by side, on purpose:
//
//  1. Native libvirt domain lifecycle events. The libvirt event loop
//     itself (EventRegisterDefaultImpl / EventRunDefaultImpl) is owned by
//     connect.go's ensureEventLoop, which runs it in a dedicated locked
//     OS thread and restarts it if it ever exits. This file only
//     registers the callback — and re-registers it whenever the
//     libvirt connection drops and is re-established (a fresh
//     *libvirt.Connect does not carry the old registration).
//  2. A polling loop that diffs ListAllDomains() states and broadcasts
//     only on change. It is cheap and always on: even if the native
//     path stops delivering (event loop hiccup, missed re-register,
//     older libvirt without the event API), VM states still update
//     within one poll interval. Duplicates with the native path are
//     harmless — clients just re-render the same state.
//
// The returned function stops both loops. It is safe to call multiple times.
func (c *Connector) StartEventLoop(ctx context.Context, hub *events.Hub, pollInterval time.Duration) (stop func()) {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}

	stopCh := make(chan struct{})
	stopped := atomic.Bool{}

	// Polling safety net — always running.
	go pollLoop(ctx, hub, c, pollInterval, stopCh)

	// Native event registration with reconnect handling.
	go eventRegisterLoop(ctx, hub, c, stopCh)

	return func() {
		if !stopped.CompareAndSwap(false, true) {
			return
		}
		close(stopCh)
	}
}

// eventRegisterLoop registers the domain lifecycle callback and keeps it
// registered: if the libvirt connection is lost (and ensureConnected
// later re-establishes it on some other call site), the old callback
// dies with the old connection, so we deregister it and re-register on
// the new one. Retries with a capped backoff while registration fails.
func eventRegisterLoop(ctx context.Context, hub *events.Hub, c *Connector, stop <-chan struct{}) {
	defer safego.Recover("libvirt_event_register_loop")

	const (
		retryMin = time.Second
		retryMax = 30 * time.Second
	)
	retry := retryMin

	var (
		registered *libvirt.Connect // connection the callback is currently registered on
		callbackID int
	)
	defer func() {
		if registered != nil {
			if err := registered.DomainEventDeregister(callbackID); err != nil {
				slog.Debug("event_loop_deregister_failed", "err", err)
			}
		}
	}()

	for {
		cur := c.Get()
		switch {
		case registered == nil:
			// Not registered yet (or the old connection died): try to
			// register on the current connection.
			if cur != nil && c.IsConnected() {
				id, err := cur.DomainEventLifecycleRegister(nil,
					func(_ *libvirt.Connect, d *libvirt.Domain, event *libvirt.DomainEventLifecycle) {
						c.broadcastLifecycleEvent(hub, d, event)
					})
				if err != nil {
					slog.Warn("event_loop_libvirt_register_failed_polling", "err", err)
				} else {
					registered, callbackID, retry = cur, id, retryMin
					slog.Info("event_loop_active", "callback_id", id)
				}
			}
		case cur != registered || !c.IsConnected():
			// Connection was replaced (reconnect) or died: the callback
			// is gone with it. Deregister best-effort on the old
			// connection and wait for the new one to come up.
			_ = registered.DomainEventDeregister(callbackID)
			registered = nil
			slog.Warn("event_loop_connection_lost_reregistering")
		}

		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-time.After(retry):
			if registered == nil {
				retry *= 2
				if retry > retryMax {
					retry = retryMax
				}
			}
		}
	}
}

// broadcastLifecycleEvent maps a native lifecycle event to a hub event.
// Events libvirt does not let us translate directly (and UNDEFINED, which
// may mean "undefined while shutoff" — i.e. the VM is gone entirely) are
// resolved against the domain's actual live state instead of guessing.
func (c *Connector) broadcastLifecycleEvent(hub *events.Hub, d *libvirt.Domain, event *libvirt.DomainEventLifecycle) {
	if d == nil || event == nil {
		return
	}
	name, _ := d.GetName()
	uuid, _ := d.GetUUIDString()

	state := stateToString(event.Event)
	if state == "unknown" {
		s, _, err := d.GetState()
		if err != nil {
			// The domain can no longer be inspected at all — most likely
			// undefined while shut off, so it is really gone.
			slog.Debug("event_loop_domain_gone_after_event", "vm_id", uuid, "event", event.Event)
			hub.Broadcast(events.Event{
				Type:      "vm.removed",
				VmID:      uuid,
				Timestamp: time.Now().Unix(),
			})
			return
		}
		state = stateToStringFromState(s)
	}

	hub.Broadcast(events.Event{
		Type:      "vm.state",
		VmID:      uuid,
		State:     state,
		Name:      name,
		Timestamp: time.Now().Unix(),
	})
}

func pollLoop(ctx context.Context, hub *events.Hub, c *Connector, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	last := map[string]string{} // vm uuid -> state
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			if !c.IsConnected() {
				continue
			}
			conn := c.Get()
			if conn == nil {
				continue
			}
			doms, err := conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE | libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
			if err != nil {
				continue
			}
			current := map[string]string{}
			for _, dom := range doms {
				uuid, _ := dom.GetUUIDString()
				name, _ := dom.GetName()
				state, _, _ := dom.GetState()
				s := stateToStringFromState(state)
				current[uuid] = s

				if prev, ok := last[uuid]; !ok || prev != s {
					hub.Broadcast(events.Event{
						Type:      "vm.state",
						VmID:      uuid,
						State:     s,
						Name:      name,
						Timestamp: time.Now().Unix(),
					})
				}
				dom.Free()
			}
			// Broadcast "removed" for VMs that disappeared
			for uuid := range last {
				if _, ok := current[uuid]; !ok {
					hub.Broadcast(events.Event{
						Type:      "vm.removed",
						VmID:      uuid,
						Timestamp: time.Now().Unix(),
					})
				}
			}
			last = current
		}
	}
}

// stateToString maps a libvirt DomainEventType to the canonical state
// string the frontend uses ("running" | "shutoff" | "paused" | "crashed").
// Events that don't map cleanly return "unknown"; the caller then reads
// the domain's actual state rather than broadcasting a guess.
func stateToString(t libvirt.DomainEventType) string {
	switch t {
	case libvirt.DOMAIN_EVENT_STARTED:
		return "running"
	case libvirt.DOMAIN_EVENT_STOPPED, libvirt.DOMAIN_EVENT_SHUTDOWN:
		return "shutoff"
	case libvirt.DOMAIN_EVENT_SUSPENDED, libvirt.DOMAIN_EVENT_PMSUSPENDED:
		return "paused"
	case libvirt.DOMAIN_EVENT_RESUMED:
		return "running"
	case libvirt.DOMAIN_EVENT_CRASHED:
		return "crashed"
	default:
		return "unknown"
	}
}

func stateToStringFromState(s libvirt.DomainState) string {
	switch s {
	case libvirt.DOMAIN_RUNNING, libvirt.DOMAIN_BLOCKED:
		return "running"
	case libvirt.DOMAIN_SHUTOFF, libvirt.DOMAIN_SHUTDOWN:
		return "shutoff"
	case libvirt.DOMAIN_PAUSED:
		return "paused"
	case libvirt.DOMAIN_CRASHED:
		return "crashed"
	default:
		return "unknown"
	}
}
