package syncer

import (
	"strings"
	"time"

	"github.com/raddadengineer/npm-autodiscovery/internal/npm"
)

// MatchCertificate evaluates a list of NPM certificates and finds the best match for the given domain names.
// It returns nil if no certificate matches any of the domains.
func MatchCertificate(certs []npm.Certificate, domains []string) *npm.Certificate {
	if len(certs) == 0 || len(domains) == 0 {
		return nil
	}

	var bestCert *npm.Certificate
	bestScore := 0
	var bestExpiry time.Time

	for i := range certs {
		cert := &certs[i]
		score := ScoreCertificateForDomains(cert, domains)
		if score == 0 {
			continue
		}

		expiry, _ := time.Parse("2006-01-02 15:04:05", cert.ExpiresOn)
		if expiry.IsZero() {
			expiry, _ = time.Parse(time.RFC3339, cert.ExpiresOn)
		}

		if score > bestScore {
			bestScore = score
			bestCert = cert
			bestExpiry = expiry
		} else if score == bestScore {
			// If score is tied, prefer certificate with later expiration or higher ID
			if expiry.After(bestExpiry) || (expiry.Equal(bestExpiry) && cert.ID > bestCert.ID) {
				bestCert = cert
				bestExpiry = expiry
			}
		}
	}

	return bestCert
}

// ScoreCertificateForDomains calculates how well a certificate covers the provided target domain names.
// Exact match on a domain: 100 points
// Wildcard match (*.domain.tld): 50 points
// Multi-level wildcard match (fallback): 25 points
// Returns 0 if none of the domains match.
func ScoreCertificateForDomains(cert *npm.Certificate, domains []string) int {
	totalScore := 0
	matchedAny := false

	for _, domain := range domains {
		d := strings.ToLower(strings.TrimSpace(domain))
		if d == "" {
			continue
		}

		domainScore := 0
		for _, certDomain := range cert.DomainNames {
			cd := strings.ToLower(strings.TrimSpace(certDomain))
			if cd == "" {
				continue
			}

			// 1. Exact match (e.g. "vw.halnt.dev" == "vw.halnt.dev")
			if cd == d {
				if domainScore < 100 {
					domainScore = 100
				}
				continue
			}

			// 2. Wildcard match (e.g. "*.halnt.dev" matching "vw.halnt.dev")
			if strings.HasPrefix(cd, "*.") {
				baseSuffix := cd[1:] // e.g. ".halnt.dev"
				if strings.HasSuffix(d, baseSuffix) {
					sub := d[:len(d)-len(baseSuffix)]
					if len(sub) > 0 {
						if !strings.Contains(sub, ".") {
							// Single-level standard wildcard match (e.g. "vw.halnt.dev")
							if domainScore < 50 {
								domainScore = 50
							}
						} else {
							// Multi-level match (e.g. "sub.vw.halnt.dev")
							if domainScore < 25 {
								domainScore = 25
							}
						}
					}
				}
			}
		}

		if domainScore > 0 {
			matchedAny = true
			totalScore += domainScore
		}
	}

	if !matchedAny {
		return 0
	}
	return totalScore
}
