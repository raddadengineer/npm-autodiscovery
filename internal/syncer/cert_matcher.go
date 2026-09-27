package syncer

import (
	"strings"

	"github.com/raddadengineer/npm-autodiscovery/internal/npm"
)

// MatchDomain evaluates how well a certificate domain pattern matches a target host domain.
// Returns a match score:
//   100: Exact match (e.g. "vw.halnt.dev" == "vw.halnt.dev")
//    80: Direct single-level wildcard match (e.g. "*.halnt.dev" matches "vw.halnt.dev")
//    40: Multi-level wildcard match (e.g. "*.halnt.dev" matches "sub.vw.halnt.dev")
//     0: No match (e.g. "*.halnt.dev" vs "worker.local")
func MatchDomain(pattern, host string) int {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	host = strings.ToLower(strings.TrimSpace(host))

	// Clean any port suffix or trailing dots
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	host = strings.TrimSuffix(host, ".")
	pattern = strings.TrimSuffix(pattern, ".")

	if pattern == "" || host == "" {
		return 0
	}

	// 1. Exact match has highest precedence
	if pattern == host {
		return 100
	}

	// 2. Wildcard pattern handling: "*.domain.tld"
	if strings.HasPrefix(pattern, "*.") {
		baseDomain := pattern[2:] // e.g. "halnt.dev"

		// Wildcard does not match the apex domain directly (e.g. "*.halnt.dev" does not match "halnt.dev")
		if host == baseDomain {
			return 0
		}

		suffix := "." + baseDomain
		if strings.HasSuffix(host, suffix) {
			prefix := strings.TrimSuffix(host, suffix)
			if len(prefix) > 0 {
				// Single-level wildcard match (no further dots in the prefix): score 80
				if !strings.Contains(prefix, ".") {
					return 80
				}
				// Multi-level wildcard match (fallback): score 40
				return 40
			}
		}
	}

	return 0
}

// FindBestMatchingCertificate returns the certificate in certs with the highest match score for domain.
func FindBestMatchingCertificate(certs []npm.Certificate, domain string) *npm.Certificate {
	var bestCert *npm.Certificate
	bestScore := 0

	for i := range certs {
		cert := &certs[i]
		for _, certDomain := range cert.DomainNames {
			score := MatchDomain(certDomain, domain)
			if score > bestScore {
				bestScore = score
				bestCert = cert
			}
		}
	}

	if bestScore > 0 {
		return bestCert
	}
	return nil
}

// FindBestMatchingCertificateForDomains returns the best matching certificate across a list of domains.
// The primary domain (domains[0]) is prioritized.
func FindBestMatchingCertificateForDomains(certs []npm.Certificate, domains []string) *npm.Certificate {
	if len(domains) == 0 || len(certs) == 0 {
		return nil
	}

	// First attempt matching the primary domain
	primaryCert := FindBestMatchingCertificate(certs, domains[0])
	if primaryCert != nil {
		return primaryCert
	}

	// Fallback to checking secondary domains
	for _, d := range domains[1:] {
		if cert := FindBestMatchingCertificate(certs, d); cert != nil {
			return cert
		}
	}

	return nil
}
