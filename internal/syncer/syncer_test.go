package syncer

import (
	"testing"

	"github.com/raddadengineer/npm-autodiscovery/internal/config"
	"github.com/raddadengineer/npm-autodiscovery/internal/npm"
)

func TestMultiHostIsolation(t *testing.T) {
	cfgHostA := &config.Config{
		HostID: "node-alpha",
	}

	syncerA := &Syncer{
		cfg: cfgHostA,
	}

	hostAlphaProxy := &npm.ProxyHost{
		ID:          10,
		DomainNames: []string{"alpha.example.com"},
		Meta: map[string]interface{}{
			"managed_by":     ManagedByTag,
			"host_id":        "node-alpha",
			"container_id":   "cid-alpha-123",
			"container_name": "web-alpha",
		},
	}

	hostBetaProxy := &npm.ProxyHost{
		ID:          20,
		DomainNames: []string{"beta.example.com"},
		Meta: map[string]interface{}{
			"managed_by":     ManagedByTag,
			"host_id":        "node-beta",
			"container_id":   "cid-beta-456",
			"container_name": "web-beta",
		},
	}

	// 1. Host Alpha checking its own proxy
	if !syncerA.isManagedByThisHostAndContainer(hostAlphaProxy, "cid-alpha-123", "web-alpha") {
		t.Errorf("Host Alpha should manage its own proxy")
	}

	// 2. Host Alpha MUST NOT manage Host Beta's proxy
	if syncerA.isManagedByThisHostAndContainer(hostBetaProxy, "cid-beta-456", "web-beta") {
		t.Errorf("Host Alpha MUST NOT match or delete Host Beta's proxy!")
	}

	// 3. getManagedInfo extracts host_id correctly
	isManagedA, _, hostIDA, cidA, _, _ := getManagedInfo(hostAlphaProxy)
	if !isManagedA || hostIDA != "node-alpha" || cidA != "cid-alpha-123" {
		t.Errorf("Failed extracting Alpha managed info: isManaged=%v, hostID=%s, cid=%s", isManagedA, hostIDA, cidA)
	}

	isManagedB, _, hostIDB, cidB, _, _ := getManagedInfo(hostBetaProxy)
	if !isManagedB || hostIDB != "node-beta" || cidB != "cid-beta-456" {
		t.Errorf("Failed extracting Beta managed info: isManaged=%v, hostID=%s, cid=%s", isManagedB, hostIDB, cidB)
	}

	// 4. Fallback extraction from AdvancedConfig header
	hostLegacyWithHeader := &npm.ProxyHost{
		ID:             30,
		DomainNames:    []string{"gamma.example.com"},
		AdvancedConfig: "# Managed by NPM-AutoDiscovery [host_id: node-gamma]\nclient_max_body_size 10M;",
	}
	isManagedG, _, hostIDG, _, _, _ := getManagedInfo(hostLegacyWithHeader)
	if !isManagedG || hostIDG != "node-gamma" {
		t.Errorf("Failed extracting host_id from AdvancedConfig: isManaged=%v, hostID=%s", isManagedG, hostIDG)
	}
}

func TestIsUpdateRequired(t *testing.T) {
	existing := &npm.ProxyHost{
		DomainNames:   []string{"app.example.com"},
		ForwardHost:   "192.168.1.10",
		ForwardPort:   8080,
		ForwardScheme: "http",
		CertificateID: 0,
		AccessListID:  0,
	}

	desired := &npm.ProxyHostRequest{
		DomainNames:   []string{"app.example.com"},
		ForwardHost:   "192.168.1.10",
		ForwardPort:   8080,
		ForwardScheme: "http",
		CertificateID: 0,
		AccessListID:  0,
	}

	// 1. Equal
	req, reason := isUpdateRequired(existing, desired)
	if req {
		t.Errorf("Expected no update required, got reason: %s", reason)
	}

	// 2. Certificate ID updated from 0 to 2
	desired.CertificateID = 2
	req, reason = isUpdateRequired(existing, desired)
	if !req || reason != "certificate ID changed from 0 to 2" {
		t.Errorf("Expected update required for cert ID, got req=%v, reason=%s", req, reason)
	}

	// Reset
	existing.CertificateID = 2
	req, _ = isUpdateRequired(existing, desired)
	if req {
		t.Errorf("Expected no update when both have cert ID 2")
	}

	// 3. Float64 cert ID from JSON unmarshaling vs int cert ID
	existing.CertificateID = float64(2)
	desired.CertificateID = 2
	req, _ = isUpdateRequired(existing, desired)
	if req {
		t.Errorf("Expected float64(2) and int(2) to match normalized")
	}

	// 4. AccessListID change
	desired.AccessListID = 5
	req, reason = isUpdateRequired(existing, desired)
	if !req {
		t.Errorf("Expected update required for access list ID change")
	}
}
