package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Default histogram buckets in seconds for reconciliation loops
var defaultSyncBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}

// Registry stores all application metrics for Prometheus scraping.
type Registry struct {
	mu sync.RWMutex

	startTime time.Time
	version   string

	// Gauges: host_id + source -> count
	activeProxies map[proxyKey]int
	activeStreams map[proxyKey]int

	// Counters: action + status -> count
	eventsTotal map[eventKey]int64

	// Health transitions: status -> count
	healthStatusTotal map[string]int64

	// Histogram: sync duration
	syncBuckets []float64
	syncCounts  []int64
	syncSum     float64
	syncTotal   int64
}

type proxyKey struct {
	hostID string
	source string
}

type eventKey struct {
	action string
	status string
}

var (
	defaultRegistry *Registry
	once            sync.Once
)

// Default returns the singleton metrics registry.
func Default() *Registry {
	once.Do(func() {
		defaultRegistry = NewRegistry("1.1.0")
	})
	return defaultRegistry
}

// NewRegistry initializes a new metrics registry.
func NewRegistry(version string) *Registry {
	return &Registry{
		startTime:         time.Now(),
		version:           version,
		activeProxies:     make(map[proxyKey]int),
		activeStreams:     make(map[proxyKey]int),
		eventsTotal:       make(map[eventKey]int64),
		healthStatusTotal: make(map[string]int64),
		syncBuckets:       defaultSyncBuckets,
		syncCounts:        make([]int64, len(defaultSyncBuckets)),
	}
}

// SetActiveProxies updates the active proxy count gauge for a host and source.
func (r *Registry) SetActiveProxies(hostID, source string, count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeProxies[proxyKey{hostID: hostID, source: source}] = count
}

// SetActiveStreams updates the active Layer 4 streams count gauge for a host and source.
func (r *Registry) SetActiveStreams(hostID, source string, count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeStreams[proxyKey{hostID: hostID, source: source}] = count
}

// IncEvents increments the Docker event counter for a specific action and status.
func (r *Registry) IncEvents(action, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.eventsTotal[eventKey{action: action, status: status}]++
}

// IncHealthStatus increments the health status transition counter.
func (r *Registry) IncHealthStatus(status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.healthStatusTotal[status]++
}

// ObserveSyncDuration records the duration of a full reconciliation cycle.
func (r *Registry) ObserveSyncDuration(duration time.Duration) {
	seconds := duration.Seconds()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.syncTotal++
	r.syncSum += seconds

	for i, b := range r.syncBuckets {
		if seconds <= b {
			r.syncCounts[i]++
		}
	}
}

// FormatMetrics renders all metrics into standard Prometheus exposition format.
func (r *Registry) FormatMetrics() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder

	// 1. Build Info
	sb.WriteString("# HELP npm_autodiscovery_build_info Build and version metadata\n")
	sb.WriteString("# TYPE npm_autodiscovery_build_info gauge\n")
	sb.WriteString(fmt.Sprintf("npm_autodiscovery_build_info{version=%q} 1\n\n", r.version))

	// 2. Uptime
	uptimeSec := time.Since(r.startTime).Seconds()
	sb.WriteString("# HELP npm_autodiscovery_uptime_seconds Total seconds since process startup\n")
	sb.WriteString("# TYPE npm_autodiscovery_uptime_seconds gauge\n")
	sb.WriteString(fmt.Sprintf("npm_autodiscovery_uptime_seconds %.2f\n\n", uptimeSec))

	// 3. Active Proxies Gauge
	sb.WriteString("# HELP npm_autodiscovery_active_proxies Total number of active proxy hosts managed\n")
	sb.WriteString("# TYPE npm_autodiscovery_active_proxies gauge\n")
	type proxyEntry struct {
		key proxyKey
		val int
	}
	var proxies []proxyEntry
	for k, v := range r.activeProxies {
		proxies = append(proxies, proxyEntry{key: k, val: v})
	}
	sort.Slice(proxies, func(i, j int) bool {
		if proxies[i].key.hostID != proxies[j].key.hostID {
			return proxies[i].key.hostID < proxies[j].key.hostID
		}
		return proxies[i].key.source < proxies[j].key.source
	})
	for _, p := range proxies {
		sb.WriteString(fmt.Sprintf("npm_autodiscovery_active_proxies{host_id=%q,source=%q} %d\n", p.key.hostID, p.key.source, p.val))
	}
	sb.WriteString("\n")

	// 3b. Active Streams Gauge (Phase 2)
	sb.WriteString("# HELP npm_autodiscovery_active_streams Total number of active Layer 4 streams managed\n")
	sb.WriteString("# TYPE npm_autodiscovery_active_streams gauge\n")
	var streams []proxyEntry
	for k, v := range r.activeStreams {
		streams = append(streams, proxyEntry{key: k, val: v})
	}
	sort.Slice(streams, func(i, j int) bool {
		if streams[i].key.hostID != streams[j].key.hostID {
			return streams[i].key.hostID < streams[j].key.hostID
		}
		return streams[i].key.source < streams[j].key.source
	})
	for _, s := range streams {
		sb.WriteString(fmt.Sprintf("npm_autodiscovery_active_streams{host_id=%q,source=%q} %d\n", s.key.hostID, s.key.source, s.val))
	}
	sb.WriteString("\n")

	// 4. Events Counter
	sb.WriteString("# HELP npm_autodiscovery_events_total Total Docker events processed\n")
	sb.WriteString("# TYPE npm_autodiscovery_events_total counter\n")
	type eventEntry struct {
		key eventKey
		val int64
	}
	var events []eventEntry
	for k, v := range r.eventsTotal {
		events = append(events, eventEntry{key: k, val: v})
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].key.action != events[j].key.action {
			return events[i].key.action < events[j].key.action
		}
		return events[i].key.status < events[j].key.status
	})
	for _, ev := range events {
		sb.WriteString(fmt.Sprintf("npm_autodiscovery_events_total{action=%q,status=%q} %d\n", ev.key.action, ev.key.status, ev.val))
	}
	sb.WriteString("\n")

	// 5. Health Status Transitions Counter
	if len(r.healthStatusTotal) > 0 {
		sb.WriteString("# HELP npm_autodiscovery_health_status_total Total container health status events observed\n")
		sb.WriteString("# TYPE npm_autodiscovery_health_status_total counter\n")
		var healthKeys []string
		for k := range r.healthStatusTotal {
			healthKeys = append(healthKeys, k)
		}
		sort.Strings(healthKeys)
		for _, k := range healthKeys {
			sb.WriteString(fmt.Sprintf("npm_autodiscovery_health_status_total{status=%q} %d\n", k, r.healthStatusTotal[k]))
		}
		sb.WriteString("\n")
	}

	// 6. Sync Duration Histogram
	sb.WriteString("# HELP npm_autodiscovery_sync_duration_seconds Latency of reconciliation loop\n")
	sb.WriteString("# TYPE npm_autodiscovery_sync_duration_seconds histogram\n")
	for i, b := range r.syncBuckets {
		sb.WriteString(fmt.Sprintf("npm_autodiscovery_sync_duration_seconds_bucket{le=\"%g\"} %d\n", b, r.syncCounts[i]))
	}
	sb.WriteString(fmt.Sprintf("npm_autodiscovery_sync_duration_seconds_bucket{le=\"+Inf\"} %d\n", r.syncTotal))
	sb.WriteString(fmt.Sprintf("npm_autodiscovery_sync_duration_seconds_sum %.4f\n", r.syncSum))
	sb.WriteString(fmt.Sprintf("npm_autodiscovery_sync_duration_seconds_count %d\n", r.syncTotal))

	return sb.String()
}

// Handler returns an HTTP HandlerFunc serving Prometheus formatted metrics.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.FormatMetrics()))
	}
}

// Convenience package-level methods forwarding to Default registry
func SetActiveProxies(hostID, source string, count int) {
	Default().SetActiveProxies(hostID, source, count)
}

func SetActiveStreams(hostID, source string, count int) {
	Default().SetActiveStreams(hostID, source, count)
}

func IncEvents(action, status string) {
	Default().IncEvents(action, status)
}

func IncHealthStatus(status string) {
	Default().IncHealthStatus(status)
}

func ObserveSyncDuration(duration time.Duration) {
	Default().ObserveSyncDuration(duration)
}

func Handler() http.HandlerFunc {
	return Default().Handler()
}
