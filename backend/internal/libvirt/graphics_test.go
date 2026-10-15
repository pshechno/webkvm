package libvirt

import (
	"strings"
	"testing"
)

// Regression: with autoport the INACTIVE domain XML carries port='-1'.
// The old code silently defaulted to 5900, so the VNC proxy dialed the
// first VM's port and two consoles showed the same machine. A port <= 0
// must produce a "not ready" error instead.
func TestParseGraphicsXMLAutoportError(t *testing.T) {
	xml := `<domain><devices>
<graphics type='vnc' port='-1' autoport='yes' listen='127.0.0.1'/>
</devices></domain>`
	_, err := parseGraphicsXML(xml, "vnc")
	if err == nil {
		t.Fatal("expected error for port=-1 (autoport), got nil")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("error should mention 'not ready', got: %v", err)
	}
}

func TestParseGraphicsXMLLivePort(t *testing.T) {
	xml := `<domain><devices>
<graphics type='vnc' port='5902' autoport='yes' listen='127.0.0.1'/>
</devices></domain>`
	info, err := parseGraphicsXML(xml, "vnc")
	if err != nil {
		t.Fatal(err)
	}
	if info.Port != 5902 {
		t.Errorf("Port = %d, want 5902", info.Port)
	}
	if info.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", info.Host)
	}
	if info.Type != "vnc" {
		t.Errorf("Type = %q, want vnc", info.Type)
	}
}

func TestParseGraphicsXMLPreferredType(t *testing.T) {
	xml := `<domain><devices>
<graphics type='vnc' port='5900' listen='127.0.0.1'/>
<graphics type='spice' port='5901' listen='127.0.0.1'/>
</devices></domain>`
	info, err := parseGraphicsXML(xml, "spice")
	if err != nil {
		t.Fatal(err)
	}
	if info.Port != 5901 || info.Type != "spice" {
		t.Errorf("got %+v, want spice on 5901", info)
	}
	// A type the VM doesn't have must error, not fall back silently.
	if _, err := parseGraphicsXML(xml, "rdp"); err == nil {
		t.Error("expected error for unsupported graphics type")
	}
}

func TestParseGraphicsXMLNoDevice(t *testing.T) {
	if _, err := parseGraphicsXML(`<domain><devices></devices></domain>`, "vnc"); err == nil {
		t.Error("expected error when no graphics device present")
	}
}

// A listen attribute that is empty must default to loopback.
func TestParseGraphicsXMLDefaultListen(t *testing.T) {
	info, err := parseGraphicsXML(`<graphics type='vnc' port='5900'/>`, "vnc")
	if err != nil {
		t.Fatal(err)
	}
	if info.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want default 127.0.0.1", info.Host)
	}
}
