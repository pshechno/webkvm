package libvirt

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"testing"

	"webkvm/internal/models"
)

// The per-VM "eject ISO on shutdown" flag must survive the XML round-trip
// through the <webkvm:meta> element (write path of SetVMMeta, read path
// of GetVMMeta).
func TestVMMetaEjectISOOnShutdownRoundTrip(t *testing.T) {
	root := xmlMetaRoot{
		XMLNS:              webkvmNamespace,
		Alias:              "test",
		EjectISOOnShutdown: true,
		UpdatedAt:          1234,
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	if err := xml.NewEncoder(&buf).Encode(root); err != nil {
		t.Fatal(err)
	}

	// The encoded metadata must carry the namespaced-free JSON-visible
	// tag libvirt stores.
	if !bytes.Contains(buf.Bytes(), []byte("eject_iso_on_shutdown")) {
		t.Errorf("encoded metadata missing eject_iso_on_shutdown:\n%s", buf.String())
	}

	var back xmlMetaRoot
	if err := xml.NewDecoder(bytes.NewReader(buf.Bytes())).Decode(&back); err != nil {
		t.Fatal(err)
	}
	if !back.EjectISOOnShutdown {
		t.Error("EjectISOOnShutdown lost in XML round-trip")
	}
}

// The install_mode flag (boot from cdrom until the OS is installed, then
// eject + boot from disk) must survive the same XML round-trip.
func TestVMMetaInstallModeRoundTrip(t *testing.T) {
	root := xmlMetaRoot{
		XMLNS:       webkvmNamespace,
		Alias:       "test",
		InstallMode: true,
		UpdatedAt:   1234,
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	if err := xml.NewEncoder(&buf).Encode(root); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("install_mode")) {
		t.Errorf("encoded metadata missing install_mode:\n%s", buf.String())
	}

	var back xmlMetaRoot
	if err := xml.NewDecoder(bytes.NewReader(buf.Bytes())).Decode(&back); err != nil {
		t.Fatal(err)
	}
	if !back.InstallMode {
		t.Error("InstallMode lost in XML round-trip")
	}
}

// Default (false) must stay omitted so old metadata keeps parsing.
func TestVMMetaInstallModeOmittedByDefault(t *testing.T) {
	root := xmlMetaRoot{XMLNS: webkvmNamespace, Alias: "x"}
	var buf bytes.Buffer
	if err := xml.NewEncoder(&buf).Encode(root); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("install_mode")) {
		t.Errorf("flag should be omitted when false:\n%s", buf.String())
	}
	var back xmlMetaRoot
	if err := xml.NewDecoder(bytes.NewReader(buf.Bytes())).Decode(&back); err != nil {
		t.Fatal(err)
	}
	if back.InstallMode {
		t.Error("flag should decode as false when absent")
	}
}

// Default (false) must stay omitted so old metadata keeps parsing.
func TestVMMetaEjectISOOnShutdownOmittedByDefault(t *testing.T) {
	root := xmlMetaRoot{XMLNS: webkvmNamespace, Alias: "x"}
	var buf bytes.Buffer
	if err := xml.NewEncoder(&buf).Encode(root); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("eject_iso_on_shutdown")) {
		t.Errorf("flag should be omitted when false:\n%s", buf.String())
	}
	var back xmlMetaRoot
	if err := xml.NewDecoder(bytes.NewReader(buf.Bytes())).Decode(&back); err != nil {
		t.Fatal(err)
	}
	if back.EjectISOOnShutdown {
		t.Error("flag should decode as false when absent")
	}
}

// VMMetaUpdate must expose the flag under eject_iso_on_shutdown and
// model under model for the PATCH endpoints.
func TestVMMetaUpdateAndNetIfaceJSONTags(t *testing.T) {
	b, err := json.Marshal(map[string]any{
		"eject_iso_on_shutdown": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var upd models.VMMetaUpdate
	if err := json.Unmarshal(b, &upd); err != nil {
		t.Fatal(err)
	}
	if upd.EjectISOOnShutdown == nil || !*upd.EjectISOOnShutdown {
		t.Error("VMMetaUpdate.EjectISOOnShutdown did not bind eject_iso_on_shutdown")
	}

	var upd2 models.VMMetaUpdate
	if err := json.Unmarshal([]byte(`{"install_mode":true}`), &upd2); err != nil {
		t.Fatal(err)
	}
	if upd2.InstallMode == nil || !*upd2.InstallMode {
		t.Error("VMMetaUpdate.InstallMode did not bind install_mode")
	}

	var req models.UpdateNetIfaceRequest
	if err := json.Unmarshal([]byte(`{"model":"e1000e"}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.Model == nil || *req.Model != "e1000e" {
		t.Error("UpdateNetIfaceRequest.Model did not bind model")
	}
}

func TestValidNICModels(t *testing.T) {
	for _, m := range []string{"virtio", "e1000", "e1000e", "rtl8139", "pcnet"} {
		if !models.ValidNICModels[m] {
			t.Errorf("model %q should be valid", m)
		}
	}
	for _, m := range []string{"vmxnet3", "", "ne2k_pci"} {
		if models.ValidNICModels[m] {
			t.Errorf("model %q should be invalid", m)
		}
	}
}
