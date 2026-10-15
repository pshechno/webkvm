package libvirt

import (
	"testing"

	"libvirt.org/go/libvirt"
)

// The SSE channel must translate native lifecycle events into the state
// vocabulary the frontend understands; anything unmapped falls back to
// "unknown" and the caller resolves the live state instead.
func TestStateToString(t *testing.T) {
	cases := map[libvirt.DomainEventType]string{
		libvirt.DOMAIN_EVENT_STARTED:     "running",
		libvirt.DOMAIN_EVENT_RESUMED:     "running",
		libvirt.DOMAIN_EVENT_STOPPED:     "shutoff",
		libvirt.DOMAIN_EVENT_SHUTDOWN:    "shutoff",
		libvirt.DOMAIN_EVENT_SUSPENDED:   "paused",
		libvirt.DOMAIN_EVENT_PMSUSPENDED: "paused",
		libvirt.DOMAIN_EVENT_CRASHED:     "crashed",
		libvirt.DOMAIN_EVENT_DEFINED:     "unknown",
		libvirt.DOMAIN_EVENT_UNDEFINED:   "unknown",
	}
	for ev, want := range cases {
		if got := stateToString(ev); got != want {
			t.Errorf("stateToString(%v) = %q, want %q", ev, got, want)
		}
	}
}

func TestStateToStringFromState(t *testing.T) {
	cases := map[libvirt.DomainState]string{
		libvirt.DOMAIN_RUNNING:  "running",
		libvirt.DOMAIN_BLOCKED:  "running",
		libvirt.DOMAIN_SHUTOFF:  "shutoff",
		libvirt.DOMAIN_SHUTDOWN: "shutoff",
		libvirt.DOMAIN_PAUSED:   "paused",
		libvirt.DOMAIN_CRASHED:  "crashed",
		libvirt.DOMAIN_NOSTATE:  "unknown",
	}
	for s, want := range cases {
		if got := stateToStringFromState(s); got != want {
			t.Errorf("stateToStringFromState(%v) = %q, want %q", s, got, want)
		}
	}
}
