package syncer

import (
	"testing"

	"github.com/raddadengineer/npm-autodiscovery/internal/npm"
)

func TestMatchCertificate(t *testing.T) {
	certs := []npm.Certificate{
		{
			ID:          1,
			NiceName:    "*.domain.com, domain.com",
			DomainNames: []string{"*.domain.com", "domain.com"},
			Provider:    "letsencrypt",
		},
		{
			ID:          2,
			NiceName:    "*.halnt.dev, halnt.dev",
			DomainNames: []string{"*.halnt.dev", "halnt.dev"},
			Provider:    "letsencrypt",
		},
		{
			ID:          3,
			NiceName:    "special.domain.com",
			DomainNames: []string{"special.domain.com"},
			Provider:    "other",
		},
	}

	tests := []struct {
		name       string
		domains    []string
		wantCertID int
		wantFound  bool
	}{
		{
			name:       "Wildcard match on halnt.dev (e.g. vw.halnt.dev)",
			domains:    []string{"vw.halnt.dev"},
			wantCertID: 2,
			wantFound:  true,
		},
		{
			name:       "Wildcard match on domain.com (e.g. api.domain.com)",
			domains:    []string{"api.domain.com"},
			wantCertID: 1,
			wantFound:  true,
		},
		{
			name:       "Exact match takes precedence over wildcard (special.domain.com)",
			domains:    []string{"special.domain.com"},
			wantCertID: 3,
			wantFound:  true,
		},
		{
			name:       "Root domain exact match (halnt.dev)",
			domains:    []string{"halnt.dev"},
			wantCertID: 2,
			wantFound:  true,
		},
		{
			name:       "Local domain has no matching certificate (e.g. worker.local)",
			domains:    []string{"worker.local"},
			wantCertID: 0,
			wantFound:  false,
		},
		{
			name:       "Internal domain has no matching certificate (e.g. test.internal)",
			domains:    []string{"test.internal"},
			wantCertID: 0,
			wantFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := MatchCertificate(certs, tt.domains)
			if !tt.wantFound {
				if matched != nil {
					t.Fatalf("expected no certificate match for %v, got cert ID #%d", tt.domains, matched.ID)
				}
				return
			}

			if matched == nil {
				t.Fatalf("expected certificate ID #%d for %v, got nil", tt.wantCertID, tt.domains)
			}

			if matched.ID != tt.wantCertID {
				t.Errorf("expected certificate ID #%d, got #%d", tt.wantCertID, matched.ID)
			}
		})
	}
}
