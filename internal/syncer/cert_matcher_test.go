package syncer

import (
	"testing"

	"github.com/raddadengineer/npm-autodiscovery/internal/npm"
)

func TestCertMatcherDomainScoring(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		host      string
		wantScore int
	}{
		{
			name:      "Exact match",
			pattern:   "vw.halnt.dev",
			host:      "vw.halnt.dev",
			wantScore: 100,
		},
		{
			name:      "Exact match case insensitive",
			pattern:   "*.halnt.dev",
			host:      "WiFi.halnt.dev",
			wantScore: 80,
		},
		{
			name:      "Wildcard single level match",
			pattern:   "*.halnt.dev",
			host:      "vw.halnt.dev",
			wantScore: 80,
		},
		{
			name:      "Wildcard multi-level match fallback",
			pattern:   "*.halnt.dev",
			host:      "sub.vw.halnt.dev",
			wantScore: 40,
		},
		{
			name:      "Wildcard does not match apex domain",
			pattern:   "*.halnt.dev",
			host:      "halnt.dev",
			wantScore: 0,
		},
		{
			name:      "No match for local domain",
			pattern:   "*.halnt.dev",
			host:      "worker.local",
			wantScore: 0,
		},
		{
			name:      "No match for whoami.local",
			pattern:   "*.raddadengineer.com",
			host:      "whoami.local",
			wantScore: 0,
		},
		{
			name:      "Match for raddadengineer domain",
			pattern:   "*.raddadengineer.com",
			host:      "dashboard.raddadengineer.com",
			wantScore: 80,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := MatchDomain(tt.pattern, tt.host)
			if score != tt.wantScore {
				t.Errorf("MatchDomain(%q, %q) = %d; want %d", tt.pattern, tt.host, score, tt.wantScore)
			}
		})
	}
}

func TestFindBestMatchingCertificate(t *testing.T) {
	certs := []npm.Certificate{
		{
			ID:          1,
			NiceName:    "Raddad Engineer Wildcard",
			DomainNames: []string{"*.raddadengineer.com", "raddadengineer.com"},
		},
		{
			ID:          2,
			NiceName:    "Halnt Dev Wildcard",
			DomainNames: []string{"*.halnt.dev", "halnt.dev"},
		},
		{
			ID:          3,
			NiceName:    "Specific VW Host Cert",
			DomainNames: []string{"vw.halnt.dev"},
		},
	}

	// 1. xxx.local should match nothing -> nil
	localCert := FindBestMatchingCertificate(certs, "worker.local")
	if localCert != nil {
		t.Errorf("Expected nil certificate for worker.local, got ID #%d", localCert.ID)
	}

	whoamiCert := FindBestMatchingCertificate(certs, "whoami.local")
	if whoamiCert != nil {
		t.Errorf("Expected nil certificate for whoami.local, got ID #%d", whoamiCert.ID)
	}

	// 2. xxx.raddadengineer.com should match cert #1
	radCert := FindBestMatchingCertificate(certs, "portal.raddadengineer.com")
	if radCert == nil || radCert.ID != 1 {
		t.Errorf("Expected cert ID #1 for portal.raddadengineer.com, got %v", radCert)
	}

	// 3. xxx.halnt.dev should match cert #2 (wildcard)
	halntCert := FindBestMatchingCertificate(certs, "grafana.halnt.dev")
	if halntCert == nil || halntCert.ID != 2 {
		t.Errorf("Expected cert ID #2 for grafana.halnt.dev, got %v", halntCert)
	}

	// 4. Exact match precedence: vw.halnt.dev has an exact cert (#3) vs wildcard (#2)
	vwCert := FindBestMatchingCertificate(certs, "vw.halnt.dev")
	if vwCert == nil || vwCert.ID != 3 {
		t.Errorf("Expected exact match cert ID #3 for vw.halnt.dev, got %v", vwCert)
	}

	// 5. Multiple domains check
	multiCert := FindBestMatchingCertificateForDomains(certs, []string{"whoami.local", "api.raddadengineer.com"})
	if multiCert == nil || multiCert.ID != 1 {
		t.Errorf("Expected cert ID #1 for multi-domains fallback, got %v", multiCert)
	}
}
