package syncer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/raddadengineer/npm-autodiscovery/internal/config"
)

func TestParseNPMAccessLogsRealClientIP(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "npm-logs-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldEnv := os.Getenv("NPM_LOGS_DIR")
	defer os.Setenv("NPM_LOGS_DIR", oldEnv)
	os.Setenv("NPM_LOGS_DIR", tmpDir)

	// Create a mock proxy log file: proxy-host-10_access.log
	logFile := filepath.Join(tmpDir, "proxy-host-10_access.log")
	nowStr := time.Now().Format("02/Jan/2006:15:04:05 -0700")

	logContent := "[" + nowStr + `] - 200 200 - GET https test.domain.com "/api/v1/health" [Client 203.0.113.195] [Length 512] [Gzip -] [Sent-to 172.18.0.12:8080] "curl/7.88"` + "\n"
	logContent += "[" + nowStr + `] - 502 502 - POST https test.domain.com "/api/v1/data" [Client 198.51.100.42] [Length 256]` + "\n"

	if err := os.WriteFile(logFile, []byte(logContent), 0644); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}

	st := newNPMMetricsState()
	st.events = nil

	s := &Syncer{
		cfg:        &config.Config{HostIP: "192.168.1.100"},
		npmMetrics: st,
	}

	hosts := []npmHostUnified{
		{
			Domain:    "test.domain.com",
			NPMHostID: 10,
			Online:    true,
			Enabled:   true,
		},
	}

	cutoff := time.Now().Add(-1 * time.Hour)
	samples, ok := s.parseNPMAccessLogsIfAvailable(hosts, cutoff)
	if !ok {
		t.Fatalf("expected parseNPMAccessLogsIfAvailable to return true")
	}

	if len(samples) != 2 {
		t.Fatalf("expected 2 metric samples, got %d", len(samples))
	}

	// Verify events were populated in npmMetrics with actual client IPs
	s.npmMetrics.mu.RLock()
	events := s.npmMetrics.events
	s.npmMetrics.mu.RUnlock()

	if len(events) != 2 {
		t.Fatalf("expected 2 events in state, got %d", len(events))
	}

	// Check first event client IP
	if events[0].ClientIP != "203.0.113.195" {
		t.Errorf("expected client IP 203.0.113.195, got %s", events[0].ClientIP)
	}
	if events[0].Path != "/api/v1/health" {
		t.Errorf("expected path /api/v1/health, got %s", events[0].Path)
	}
	if events[0].Upstream != "172.18.0.12:8080" {
		t.Errorf("expected upstream 172.18.0.12:8080, got %s", events[0].Upstream)
	}

	// Check second event client IP
	if events[1].ClientIP != "198.51.100.42" {
		t.Errorf("expected client IP 198.51.100.42, got %s", events[1].ClientIP)
	}
	if events[1].StatusCode != 502 {
		t.Errorf("expected status code 502, got %d", events[1].StatusCode)
	}
}

func TestDetectLocalOutboundIP(t *testing.T) {
	ip := detectLocalOutboundIP()
	if ip == "" {
		t.Errorf("detectLocalOutboundIP returned empty string")
	}
}
