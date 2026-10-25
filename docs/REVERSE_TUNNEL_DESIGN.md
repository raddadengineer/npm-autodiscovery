# Reverse Tunnel & Ingress Data Plane Specification (Option C)

**Document Version:** 1.0  
**Target Milestone:** NPM Auto-Discovery v2.1  
**Status:** Architecture Design / Implementation Blueprint  

---

## 1. Executive Summary & Problem Statement

### 1.1 The Problem
In standard multi-node deployments of `npm-autodiscovery`:
1. **Host Port Leaks:** Every target container on a remote worker node must publish a port on the host network (`ports: - "8081:80"` in `docker-compose.yml`).
2. **Firewall Holes & Insecurity:** The worker machine must keep these published ports open to the local network or public internet so the central Nginx Proxy Manager (NPM) can reach them.
3. **Port Conflicts:** Multiple containers on the same worker host cannot bind the same host port.
4. **NAT / Network Isolation Barriers:** Remote worker nodes behind NAT, CGNAT, firewalls, or in different VPCs/clouds cannot be easily reached by central NPM without external VPNs, complex static subnet routing, or port forwarding.

### 1.2 The Solution: Option C (Reverse Tunnel Architecture)
Implement an **In-App Reverse Tunnel** that splits system responsibilities into two distinct planes:
* **Control Plane:** Worker agents auto-discover local containers and push metadata/rules to the Central Controller via `/api/cluster/report` (already existing).
* **Data Plane (New):** Worker agents dial an **outbound persistent multiplexed tunnel** (WebSocket + Yamux) back to the Central Controller. 

Central NPM simply forwards traffic to Central `npm-autodiscovery`'s internal Gateway, which multiplexes incoming HTTP requests over the existing reverse tunnel directly to the worker. The worker agent dials the target container on its local Docker bridge network.

### 1.3 Key Architectural Benefits
* **Zero Published Ports:** Containers omit `ports:` completely from `docker-compose.yml`.
* **Zero Worker Inbound Ports:** The worker node maintains a 100% closed firewall with zero open listening ports; it only initiates an outbound TLS connection.
* **Network Agnostic:** Works across home labs, private subnets, cellular connections (CGNAT), and cloud VPS without VPNs or port forwarding.
* **Hardened Security Hop:** Traffic between NPM and Central is encrypted with TLS and authenticated via a pre-shared gateway secret (`X-Tunnel-Secret`).

---

## 2. End-to-End System Architecture

```
[ Public Client: Browser / App ]
               │
               ▼  (Public HTTPS :443 / SSL Terminated by NPM)
┌──────────────────────────────────────────────────────────────────────┐
│  CENTRAL HOST                                                        │
│                                                                      │
│  ┌─────────────────────────┐                                         │
│  │   Nginx Proxy Manager   │ (LXC, VM, Bare-Metal, or Docker)        │
│  └────────────┬────────────┘                                         │
│               │                                                      │
│               │  Internal Upstream Hop:                              │
│               │  • Scheme: HTTPS (or HTTP)                           │
│               │  • Target: 192.168.1.10:8443 (TUNNEL_GATEWAY_HOST)   │
│               │  • Header: X-Tunnel-Secret: <token>                  │
│               │  • Header: Host: worker.halnt.dev                    │
│               ▼                                                      │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ npm-autodiscovery (Central Controller)                         │  │
│  │                                                                │  │
│  │  1. Gateway Listener (:8443 / TLS)                             │  │
│  │     Validates X-Tunnel-Secret & extracts Host                  │  │
│  │                                                                │  │
│  │  2. In-Memory Router (internal/router)                         │  │
│  │     Matches "worker.halnt.dev" -> Node "worker-01"             │  │
│  │                                                                │  │
│  │  3. Tunnel Session Manager (internal/tunnel)                   │  │
│  │     Picks active Yamux session for "worker-01"                 │  │
│  │     Opens multiplexed virtual stream                           │  │
│  └────────────────────────────────┬───────────────────────────────┘  │
└───────────────────────────────────┼──────────────────────────────────┘
                                    │
                                    │  Persistent Outbound Tunnel
                                    │  (WebSocket + Yamux over TLS)
                                    │  Auth: Bearer <CLUSTER_TOKEN>
                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│  REMOTE WORKER NODE (worker-01)                                      │
│                                                                      │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ npm-autodiscovery (Worker Agent)                               │  │
│  │                                                                │  │
│  │  1. Tunnel Worker Client (internal/tunnel)                     │  │
│  │     Accepts multiplexed Yamux stream                           │  │
│  │                                                                │  │
│  │  2. Local Forwarder (httputil.ReverseProxy)                    │  │
│  │     Dials local container directly:                            │  │
│  │     http://worker-whoami:8081 or http://172.18.0.5:8081        │  │
│  └────────────────────────────────┬───────────────────────────────┘  │
│                                   │                                  │
│                                   ▼  Internal Docker Bridge Only     │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ Target Container: worker-whoami                                │  │
│  │ Listening internally on :8081                                  │  │
│  │ NO PUBLISHED HOST PORTS (ports: omitted!)                      │  │
│  └────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 3. Core Protocol & Transport Specification

### 3.1 Transport: WebSocket Upgraded to Yamux
* **Why WebSocket?**
  * Traverses enterprise proxies, NATs, and cloud load balancers without special firewall rules.
  * Standard HTTP/1.1 or HTTP/2 upgrade semantics.
  * Authenticated during standard HTTP handshake before streaming raw bytes.
* **Why Yamux (`github.com/hashicorp/yamux`)?**
  * Production-proven multiplexing protocol used in HashiCorp Consul, Nomad, and Vault.
  * Multiplexes hundreds of concurrent HTTP requests and full-duplex WebSockets over a single underlying connection.
  * Built-in flow control (sliding window), keep-alives, and connection health pinging.

### 3.2 Tunnel Handshake & Authentication Sequence
1. Worker starts and reads `MAIN_NODE_URL=https://central-ip:8080` and `CLUSTER_TOKEN=secret`.
2. Worker initiates an HTTP GET with WebSocket Upgrade headers to:
   ```http
   GET /api/tunnel/connect HTTP/1.1
   Host: central-ip:8080
   Upgrade: websocket
   Connection: Upgrade
   X-Node-ID: worker-node-01
   Authorization: Bearer <CLUSTER_TOKEN>
   ```
3. Central validates `CLUSTER_TOKEN`. If unauthorized, responds `401 Unauthorized`.
4. Central upgrades the connection to WebSocket.
5. Central wraps the `websocket.Conn` into a `net.Conn` stream adapter.
6. Central initializes `yamux.Server(conn, config)` (or `yamux.Client`).
7. The active session is registered in the Central `SessionRegistry` keyed by `NodeID: "worker-node-01"`.
8. Background pings (every 15s) detect broken connections; upon disconnection, the worker triggers an immediate exponential-backoff reconnect loop (1s, 2s, 4s... max 30s).

---

## 4. Securing the NPM ⟷ Central Gateway Hop

Because NPM and Central `npm-autodiscovery` might run on different machines, VMs, or subnets without sharing a Docker network, the gateway hop must be hardened against eavesdropping and unauthorized access.

### 4.1 Layer 1: Transport Encryption (TLS/HTTPS)
* Central Gateway binds to `TUNNEL_GATEWAY_PORT` (e.g. `8443`) with TLS enabled.
* **Certificate Strategy:**
  1. **Self-Signed Auto-Generation:** If no certificate is supplied, Central automatically generates an internal 2048-bit RSA or Ed25519 self-signed TLS cert on boot stored in `/data/certs/gateway.crt`.
  2. **Custom Certificate:** Users can provide `TUNNEL_TLS_CERT` and `TUNNEL_TLS_KEY` pointing to their existing private PKI or wildcard cert.
* **NPM Configuration:** NPM proxy hosts are configured with `forward_scheme: "https"`.

### 4.2 Layer 2: Authentication & Anti-Tampering (`X-Tunnel-Secret`)
To prevent other devices on the LAN from sending requests directly to Central Gateway and bypassing NPM access lists:
* Central enforces a pre-shared secret:
  ```env
  TUNNEL_SECRET=my-custom-tunnel-secret
  ```
  *(If empty, automatically derived via HMAC of `CLUSTER_TOKEN`).*
* The Central Syncer automatically injects the secret into NPM's proxy host configuration via the **Custom Nginx Configuration** block:
  ```nginx
  # Managed by NPM-AutoDiscovery Tunnel Gateway
  proxy_set_header X-Tunnel-Secret "my-custom-tunnel-secret";
  ```
* When an HTTP request reaches Central Gateway:
  1. Gateway checks for `X-Tunnel-Secret`.
  2. If missing or invalid ➔ responds immediately with `403 Forbidden` (`{"error": "Unauthorized tunnel gateway access"}`).
  3. If valid ➔ removes `X-Tunnel-Secret` from request headers before forwarding so the backend container never sees internal tunnel credentials.

### 4.3 Layer 3: Co-Located Alternative (Unix Domain Socket)
If NPM and Central run on the **same physical host**:
* Central can optionally listen on a Unix Domain Socket: `/var/run/npm-tunnel/gateway.sock`.
* Shared via Docker volume mount between NPM and Central.
* NPM forwards directly via `proxy_pass http://unix:/var/run/npm-tunnel/gateway.sock;`.
* **Zero TCP network exposure; 100% kernel-level memory routing.**

---

## 5. Package Structure & Implementation Blueprint

```
internal/
├── router/
│   ├── rule.go         # RouteRule definition & thread-safe RoutingTable
│   ├── parser.go       # Parses container labels into RouteRules
│   └── gateway.go      # HTTP/HTTPS Gateway listener (intercepts from NPM)
└── tunnel/
    ├── types.go        # Protocol messages, node registration headers
    ├── adapter.go      # Net.Conn wrapper around Gorilla WebSocket
    ├── server.go       # Central tunnel listener & Yamux session manager
    ├── client.go       # Worker tunnel dialer & reconnect manager
    └── forwarder.go    # Worker-side local HTTP reverse proxy
```

### 5.1 `internal/router/rule.go`
```go
package router

import (
	"sync"
)

// RouteRule maps an incoming domain/path to a specific worker node and target.
type RouteRule struct {
	Domain      string `json:"domain"`       // e.g. "worker.halnt.dev"
	Path        string `json:"path"`         // e.g. "/" or "/api"
	NodeID      string `json:"node_id"`      // e.g. "worker-node-01"
	TargetHost  string `json:"target_host"`  // e.g. "worker-whoami" or "172.18.0.5"
	TargetPort  int    `json:"target_port"`  // e.g. 8081
	Scheme      string `json:"scheme"`       // "http" or "https"
	StripPrefix string `json:"strip_prefix"` // optional path prefix strip
}

// Table holds active route rules indexed by domain.
type Table struct {
	mu     sync.RWMutex
	routes map[string][]*RouteRule // Domain -> list of path rules
}

func NewTable() *Table {
	return &Table{routes: make(map[string][]*RouteRule)}
}

func (t *Table) Set(domain string, rules []*RouteRule) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.routes[domain] = rules
}

func (t *Table) Match(host, path string) (*RouteRule, bool) {
	t.mu.RUnlock()
	defer t.mu.RUnlock()

	rules, exists := t.routes[host]
	if !exists {
		return nil, false
	}
	// Best-match path lookup (longest prefix match)
	var best *RouteRule
	for _, r := range rules {
		if path == r.Path || (len(r.Path) > 0 && strings.HasPrefix(path, r.Path)) {
			if best == nil || len(r.Path) > len(best.Path) {
				best = r
			}
		}
	}
	return best, best != nil
}
```

### 5.2 `internal/tunnel/server.go` (Central Session Manager)
```go
package tunnel

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

type Server struct {
	mu          sync.RWMutex
	sessions    map[string]*yamux.Session // NodeID -> Session
	tokenSecret string
	upgrader    websocket.Upgrader
}

func NewServer(tokenSecret string) *Server {
	return &Server{
		sessions:    make(map[string]*yamux.Session),
		tokenSecret: tokenSecret,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// HandleTunnelConnect handles WebSocket upgrade from remote worker nodes.
func (s *Server) HandleTunnelConnect(w http.ResponseWriter, r *http.Request) {
	nodeID := r.Header.Get("X-Node-ID")
	token := extractBearerToken(r)

	if s.tokenSecret != "" && token != s.tokenSecret {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	wsConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	netConn := NewWSNetConnAdapter(wsConn)
	session, err := yamux.Client(netConn, yamux.DefaultConfig())
	if err != nil {
		wsConn.Close()
		return
	}

	s.mu.Lock()
	s.sessions[nodeID] = session
	s.mu.Unlock()

	// Wait until session terminates, then cleanup
	<-session.CloseChan()

	s.mu.Lock()
	if s.sessions[nodeID] == session {
		delete(s.sessions, nodeID)
	}
	s.mu.Unlock()
}

// DialNode opens a multiplexed stream to a specific worker node.
func (s *Server) DialNode(ctx context.Context, nodeID string) (net.Conn, error) {
	s.mu.RLock()
	session, ok := s.sessions[nodeID]
	s.mu.RUnlock()

	if !ok || session.IsClosed() {
		return nil, fmt.Errorf("node '%s' is offline or disconnected", nodeID)
	}
	return session.Open()
}
```

### 5.3 `internal/router/gateway.go` (Central Ingress Gateway)
```go
package router

import (
	"fmt"
	"net/http"
	"net/http/httputil"

	"github.com/raddadengineer/npm-autodiscovery/internal/tunnel"
)

type Gateway struct {
	table        *Table
	tunnelServer *tunnel.Server
	secretToken  string
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Verify Gateway Secret from NPM
	if g.secretToken != "" {
		if r.Header.Get("X-Tunnel-Secret") != g.secretToken {
			http.Error(w, "Forbidden: Invalid or missing tunnel secret", http.StatusForbidden)
			return
		}
	}

	// 2. Lookup Rule by Host & Path
	rule, found := g.table.Match(r.Host, r.URL.Path)
	if !found {
		http.Error(w, fmt.Sprintf("No active route registered for host: %s", r.Host), http.StatusBadGateway)
		return
	}

	// 3. Dial Worker Node Stream via Tunnel
	stream, err := g.tunnelServer.DialNode(r.Context(), rule.NodeID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Worker node '%s' unavailable: %v", rule.NodeID, err), http.StatusServiceUnavailable)
		return
	}
	defer stream.Close()

	// 4. Reverse Proxy over the Yamux Stream
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = rule.Scheme
			req.URL.Host = fmt.Sprintf("%s:%d", rule.TargetHost, rule.TargetPort)
			// Remove internal security header before forwarding to container
			req.Header.Del("X-Tunnel-Secret")
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return stream, nil
			},
		},
	}

	proxy.ServeHTTP(w, r)
}
```

---

## 6. Syncer & NPM Integration (`mapper.go`)

When a container is discovered on a worker node running with `TUNNEL_ENABLED=true`:

1. **Host Resolution Logic (`mapper.go`):**
   ```go
   if proxyCfg.TunnelEnabled || s.cfg.TunnelMode {
       // Target becomes the Central Gateway that NPM can reach
       gatewayHost := s.cfg.TunnelGatewayHost
       if gatewayHost == "" {
           gatewayHost = s.cfg.HostIP // fallback to central HostIP
       }
       gatewayPort := s.cfg.TunnelGatewayPort
       if gatewayPort <= 0 {
           gatewayPort = 8443
       }
       scheme := "https"
       if !s.cfg.TunnelTLSEnabled {
           scheme = "http"
       }
       return gatewayHost, gatewayPort, scheme, "reverse-tunnel-gateway", nil
   }
   ```
2. **NPM Advanced Config Injection:**
   The syncer appends the secret header directive into the proxy host configuration payload:
   ```nginx
   # --- Injected by NPM-AutoDiscovery (Reverse Tunnel Gateway) ---
   proxy_set_header X-Tunnel-Secret "super-secret-token";
   proxy_ssl_verify off;
   ```

---

## 7. Configuration Reference

### 7.1 Central Node Configuration (`.env`)

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `TUNNEL_ENABLED` | bool | `true` | Enables the Central reverse tunnel server & gateway. |
| `TUNNEL_GATEWAY_HOST` | string | Auto / `HOST_IP` | IP or hostname reachable by NPM (e.g. `192.168.1.10` or `npm-autodiscovery`). |
| `TUNNEL_GATEWAY_PORT` | int | `8443` | Port where the Central Gateway listens for NPM proxy traffic. |
| `TUNNEL_TLS_ENABLED` | bool | `true` | Enables HTTPS on the Gateway listener with auto-generated/custom TLS. |
| `TUNNEL_TLS_CERT` | string | `""` | Optional path to custom TLS certificate file (`.crt` / `.pem`). |
| `TUNNEL_TLS_KEY` | string | `""` | Optional path to custom TLS private key file (`.key`). |
| `TUNNEL_SECRET` | string | `""` | Pre-shared token required in `X-Tunnel-Secret` header from NPM. |
| `CLUSTER_TOKEN` | string | *Required* | Pre-shared key for worker node authentication. |

### 7.2 Remote Worker Node Configuration (`.env`)

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `MAIN_NODE_URL` | string | *Required* | URL of central controller (e.g. `https://192.168.1.10:8080`). |
| `CLUSTER_TOKEN` | string | *Required* | Pre-shared key matching Central's `CLUSTER_TOKEN`. |
| `TUNNEL_ENABLED` | bool | `true` | Enables the outbound data plane reverse tunnel. |
| `HOST_ID` | string | Hostname | Unique node identifier (e.g. `worker-node-01`). |

---

## 8. User Experience & Verification

### 8.1 Zero-Port Container Deployment
On any worker node, users deploy services without defining `ports:`:

```yaml
# docker-compose.worker.yml
services:
  worker-whoami:
    image: raddadengineer/whoami:latest
    container_name: worker-whoami
    restart: unless-stopped
    # NOTICE: No ports: section exists!
    labels:
      - "npm.frontend.domain=worker.halnt.dev"
      - "npm.frontend.port=8081"
      - "npm.forward_scheme=http"
      - "npm.websocket=true"
      - "npm.block_exploits=true"
```

### 8.2 What Happens Automatically:
1. Worker Agent reads labels, detects port `8081` on `worker-whoami`.
2. Worker Agent reports route to Central over `/api/cluster/report`.
3. Central registers route in `internal/router/rule.go`: `worker.halnt.dev -> worker-node-01 -> worker-whoami:8081`.
4. Central configures NPM:
   * **Domain:** `worker.halnt.dev`
   * **Forward Host:** `192.168.1.10` *(Central IP)*
   * **Forward Port:** `8443`
   * **Forward Scheme:** `https`
   * **Advanced Config:** `proxy_set_header X-Tunnel-Secret "<token>";`
5. Public user visits `https://worker.halnt.dev` ➔ NPM forwards to Central over encrypted TLS ➔ Central streams through the tunnel to Worker ➔ Worker dials `worker-whoami:8081` ➔ Response returned seamlessly.

---

## 9. Implementation Roadmap

- [ ] **Phase 1: Tunnel Transport (`internal/tunnel`)**
  - Implement WebSocket `net.Conn` adapter (`adapter.go`).
  - Implement Central Yamux server and session registry (`server.go`).
  - Implement Worker persistent dialer with exponential backoff reconnects (`client.go`).
- [ ] **Phase 2: Router & Gateway (`internal/router`)**
  - Implement rule table and longest-prefix route matcher (`rule.go`).
  - Implement Gateway HTTP/HTTPS reverse proxy listener (`gateway.go`).
  - Implement `X-Tunnel-Secret` validation middleware.
- [ ] **Phase 3: Central Syncer Integration (`internal/syncer`)**
  - Update `mapper.go` to route tunneled services to `TUNNEL_GATEWAY_HOST:TUNNEL_GATEWAY_PORT`.
  - Automatically inject `proxy_set_header X-Tunnel-Secret` into NPM payload.
- [ ] **Phase 4: Web Dashboard Integration**
  - Add "Tunnel Status" badge to Node cards (e.g. `● Tunnel Active (Latency: 4ms)`).
  - Add active stream counter and throughput metrics to Prometheus/OTel.
