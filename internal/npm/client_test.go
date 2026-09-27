package npm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNPMClientAuthAndCRUD(t *testing.T) {
	var tokenCalls int
	var hostsCalls int
	var createCalls int

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tokens":
			tokenCalls++
			var req AuthRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.Identity != "admin@test.com" || req.Secret != "secret123" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"message":"Invalid credentials"}}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(TokenResponse{
				Token:   "mock-jwt-token-xyz",
				Expires: time.Now().Add(2 * time.Hour).Format(time.RFC3339),
			})

		case "/api/nginx/proxy-hosts":
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer mock-jwt-token-xyz" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			if r.Method == http.MethodGet {
				hostsCalls++
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]ProxyHost{
					{
						ID:          1,
						DomainNames: []string{"existing.local"},
						ForwardHost: "172.18.0.2",
						ForwardPort: 80,
					},
				})
			} else if r.Method == http.MethodPost {
				createCalls++
				var req ProxyHostRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(ProxyHost{
					ID:          2,
					DomainNames: req.DomainNames,
					ForwardHost: req.ForwardHost,
					ForwardPort: req.ForwardPort,
				})
			}

		case "/api/nginx/proxy-hosts/1":
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
			}

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	client := NewClient(mockServer.URL, "admin@test.com", "secret123", 5*time.Second)

	ctx := context.Background()

	// 1. Authenticate
	if err := client.Authenticate(ctx); err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	// 2. GetProxyHosts
	hosts, err := client.GetProxyHosts(ctx)
	if err != nil {
		t.Fatalf("GetProxyHosts failed: %v", err)
	}
	if len(hosts) != 1 || hosts[0].DomainNames[0] != "existing.local" {
		t.Errorf("Unexpected hosts: %v", hosts)
	}

	// 3. CreateProxyHost
	created, err := client.CreateProxyHost(ctx, &ProxyHostRequest{
		DomainNames:   []string{"new.local"},
		ForwardHost:   "172.18.0.3",
		ForwardPort:   3000,
		ForwardScheme: "http",
	})
	if err != nil {
		t.Fatalf("CreateProxyHost failed: %v", err)
	}
	if created.ID != 2 || created.DomainNames[0] != "new.local" {
		t.Errorf("Unexpected created host: %+v", created)
	}

	// 4. DeleteProxyHost
	if err := client.DeleteProxyHost(ctx, 1); err != nil {
		t.Fatalf("DeleteProxyHost failed: %v", err)
	}
}
