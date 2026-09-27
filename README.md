# NPM Auto-Discovery v2.0 🚀
>
> Automated Docker, Proxmox LXC & LXD service discovery, Layer 4 stream proxying, and dynamic ingress provisioning for **Nginx Proxy Manager (NPM)**.

[![Go Report Card](https://goreportcard.com/badge/github.com/raddadengineer/npm-autodiscovery)](https://goreportcard.com/report/github.com/raddadengineer/npm-autodiscovery)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white)](Dockerfile)

---

## 📖 Overview

> **Imagine the magic of Traefik’s zero-touch container auto-discovery combined with the rock-solid reliability, visual simplicity, and high-performance Nginx core of Nginx Proxy Manager.**

For years, developers, sysadmins, and homelab enthusiasts faced a frustrating compromise: choose **Traefik** for its hands-free label-based dynamic routing while wrestling with steep learning curves and read-only dashboards—or choose **Nginx Proxy Manager (NPM)** for its clean UI and effortless visual SSL certificate management, at the cost of typing every single proxy host into a form by hand.

**NPM Auto-Discovery v2.0 shatters this compromise.** Operating as an ultra-fast, lightweight sidecar or standalone agent, it turns Nginx Proxy Manager into an autonomous, self-healing ingress controller across your entire compute stack: Docker containers, Proxmox VE LXC containers, Canonical LXD / Incus instances, and declarative GitOps manifests (`routes.yaml`). It intercepts service lifecycle events in real time, generates declarative Nginx middlewares, eliminates cold-start `502 Bad Gateway` errors, orchestrates Layer 4 TCP/UDP streams, balances scaled upstreams, and syncs proxy hosts seamlessly through the official NPM REST API.

It also embeds a **real-time Cyberpunk SPA dashboard** directly inside the single statically compiled binary (zero Node.js or runtime bloat), giving you complete visibility over active ingress routes, live container discovery telemetry, and one-click manual reconciliation.

---

## ✨ Features (v2.0)

- **⚡ Native Multi-Engine Discovery:** Real-time Docker socket events, Proxmox VE REST API token polling (with tags/notes parsing), and Canonical LXD / Incus Unix domain socket lifecycle streaming.
- **🛡️ Zero-502 HealthCheck-Aware Ingress:** Holds incoming ingress during container warm-up (`starting` state) and smoothly routes traffic only after healthy checks succeed, with configurable timeouts and fallbacks.
- **🔧 Declarative Nginx Middlewares:** Traefik-style 1-line label middlewares for URL path prefix stripping (`npm.middleware.strip_prefix`), IP CIDR whitelisting, token-bucket rate limiting, CORS preflights, and security headers.
- **📍 Path-Based Microservice Aggregation:** Mounts separate containers onto subpaths (e.g. `/api`, `/ws`, `/auth`) under a single parent domain, synthesized automatically into NPM custom locations.
- **⚖️ Dynamic Upstream Load Balancing:** Bundles horizontally scaled containers (`docker compose up --scale svc=3`) into native Nginx `upstream` blocks with `round_robin`, `least_conn`, or `ip_hash`.
- **🎮 Layer 4 TCP/UDP Stream Discovery:** Automatically discovers and provisions Layer 4 TCP/UDP proxying for game servers (Minecraft, Valheim), databases (Postgres, Redis), MQTT, and DNS.
- **📈 Prometheus & OTel Metrics Endpoint:** Exposes `/metrics` standard endpoint tracking active proxies, stream counts, sync latency histograms, and container health states.
- **🎯 Smart Network & IP Mapping:** Supports three host resolution strategies (`auto`, `name`, `ip`) and CIDR subnet filtering to route across Docker bridge networks, physical LANs, and hypervisor bridges.
- **🧹 Automatic Orphan Pruning:** Automatically purges proxy hosts and streams when containers stop or die, with multi-host collision prevention (`HOST_ID`).
- **📊 Real-Time Cyberpunk Dashboard:** Embedded Web UI featuring live Server-Sent Events (SSE) log streaming, search & filtering, stats cards, and a Docker / PVE / LXD configuration generator.
- **📦 Ultra-Lightweight Single Binary:** Statically compiled with Go, embedding the entire frontend. The multi-stage Alpine Docker image is only **~15MB** in size and consumes **<15MB RAM**.

---

## 🏗️ Architecture

```
                   ┌──────────────────────────────────────────────┐
                   │               Docker Daemon                  │
                   │           (/var/run/docker.sock)             │
                   └───────┬──────────────────────────────▲───────┘
                           │                              │
                           │ Container Events             │ Container Inspect
                           ▼                              │
         ┌─────────────────────────────────────────────────────────────┐
         │                NPM Auto-Discovery Agent                     │
         │                                                             │
         │  ┌──────────────────┐  ┌──────────────────┐  ┌───────────┐  │
         │  │  Event Listener  │  │  Label Evaluator │  │ Host Diffe│  │
         │  └────────┬─────────┘  └────────┬─────────┘  └─────┬─────┘  │
         │           └─────────────────────┼──────────────────┘        │
         │                                 ▼                           │
         │                 JWT Auth & Ingress Syncer                   │
         └─────────┬───────────────────────────────────────────┬───────┘
                   │                                           │
  HTTP REST API    │ (Create/Update/Delete)       Server-Sent  │ (Live Logs &
  to NPM (Port 81) │                              Events (SSE) │  Status)
                   ▼                                           ▼
┌──────────────────────────────────────┐     ┌───────────────────────────────┐
│         Nginx Proxy Manager          │     │    Web Management Dashboard   │
│   (Official REST API & Nginx Core)   │     │    (Embedded SPA on Port 8080)│
└──────────────────────────────────────┘     └───────────────────────────────┘
```

---

## 🏷️ Container Label & Metadata Reference (v2.0)

Add these labels to any Docker container (via `docker-compose.yml` or `docker run -l`) to configure dynamic ingress:

### Core Ingress & SSL

| Label | Type | Default | Description | Example |
| :--- | :--- | :--- | :--- | :--- |
| `npm.frontend.domain` | string | *Required* | Domain name(s) for the proxy host. Supports comma-separated lists. | `app.example.com, web.example.com` |
| `npm.frontend.port` | integer | Auto-detect | Target container internal port. Auto-detects if container exposes a single port. | `80`, `3000`, `8080` |
| `npm.forward_scheme` | string | `http` | Upstream protocol scheme (`http` or `https`). | `http` |
| `npm.forward_host` | string | Auto | Custom target host or IP override. | `192.168.1.50` or `my-service` |
| `npm.ssl.enabled` | boolean | `false` | Enable SSL for this proxy host. | `true` |
| `npm.ssl.forced` | boolean | `false` | Force HTTP to HTTPS redirection. | `true` |
| `npm.certificate_id` | int/str | Auto | NPM Certificate ID (e.g. `1`), `"new"`, or `"none"` / `0` to disable. If omitted, automatically matches existing NPM certificates by domain (e.g. `*.halnt.dev`, `*.domain.com` vs HTTP-only `.local`). | `2` |
| `npm.websocket` | boolean | `true` | Enable WebSocket upgrade support (`proxy_set_header Upgrade`). | `true` |
| `npm.block_exploits` | boolean | `true` | Enable NPM exploit block filters. | `true` |
| `npm.caching` | boolean | `false` | Enable Nginx static asset caching. | `false` |
| `npm.hsts` | boolean | `false` | Enable HTTP Strict Transport Security (HSTS). | `true` |
| `npm.http2` | boolean | `false` | Enable HTTP/2 protocol support. | `true` |
| `npm.enabled` | boolean | `true` | Explicitly enable or disable discovery for this container. | `false` |
| `npm.advanced_config` | string | `""` | Custom Nginx directives appended to the proxy host configuration. | `client_max_body_size 100M;` |

### Zero-502 HealthCheck-Aware Routing

| Label | Type | Default | Description | Example |
| :--- | :--- | :--- | :--- | :--- |
| `npm.healthcheck.enabled` | boolean | `false` | Holds ingress during warmup (`starting`); activates only upon `healthy`. | `true` |
| `npm.healthcheck.timeout` | duration | `60s` | Maximum wait time for health check before triggering fallback. | `45s`, `2m` |
| `npm.healthcheck.fallback_action` | string | `disable` | Action on warmup failure: `disable`, `keep`, or `redirect`. | `disable` |
| `npm.healthcheck.redirect_target` | string | `""` | Maintenance redirect URL when fallback is `redirect`. | `https://status.example.com` |

### Declarative Nginx Middlewares Engine

| Label | Type | Default | Description | Example |
| :--- | :--- | :--- | :--- | :--- |
| `npm.middleware.strip_prefix` | string | `""` | Strips URL path prefix before routing upstream. | `/api` |
| `npm.middleware.ip_whitelist` | string | `""` | Comma-separated CIDR allowlist. Denies all unlisted IPs. | `192.168.1.0/24, 10.0.0.0/8` |
| `npm.middleware.rate_limit` | string | `""` | Token-bucket rate limit (Nginx `limit_req_zone`). | `10r/s`, `60r/m` |
| `npm.middleware.rate_burst` | integer | `10` | Maximum burst capacity for rate limiter. | `20` |
| `npm.middleware.cors` | boolean | `false` | Injects automated CORS preflight (OPTIONS 204) and headers. | `true` |
| `npm.middleware.security_headers` | boolean | `false` | Injects hardened enterprise security headers (SAMEORIGIN, HSTS). | `true` |

### Microservice Subpaths & Dynamic Upstreams

| Label | Type | Default | Description | Example |
| :--- | :--- | :--- | :--- | :--- |
| `npm.frontend.path` | string | `/` | Subpath mount point under domain. Multiple containers aggregate automatically! | `/api`, `/ws` |
| `npm.location.<path>.forward_host` | string | Auto | Target forward host for subpath `<path>`. | `api-service` |
| `npm.location.<path>.forward_port` | integer | Auto | Target forward port for subpath `<path>`. | `8080` |
| `npm.location.<path>.strip_prefix` | boolean | `false` | Strip prefix for this custom location block. | `true` |
| `npm.upstream.balance` | string | `round_robin` | Balancing algorithm for scaled replicas: `round_robin`, `least_conn`, `ip_hash`. | `least_conn` |
| `npm.upstream.fail_timeout` | duration | `10s` | Upstream `fail_timeout` recovery window. | `15s` |
| `npm.upstream.max_fails` | integer | `3` | Max failures before marking upstream backend down. | `5` |

### Layer 4 TCP/UDP Streams

| Label | Type | Default | Description | Example |
| :--- | :--- | :--- | :--- | :--- |
| `npm.stream.port` | integer | *Required* | Public port for NPM to listen on for Layer 4 stream. | `25565`, `5432`, `53` |
| `npm.stream.forward_port` | integer | Auto | Target port on container. Defaults to stream port. | `25565` |
| `npm.stream.tcp` | boolean | `true` | Enable TCP stream proxying. | `true` |
| `npm.stream.udp` | boolean | `false` | Enable UDP stream proxying (e.g. DNS, gaming, voice). | `true` |

### Hypervisor Metadata (Proxmox VE & LXD / Incus)

- **Proxmox VE (Tags):** `pct set <vmid> -tags "npm.domain=app.lan,npm.port=80,npm.ssl=true"`
- **Proxmox VE (Notes):** Add YAML block to container notes textarea (`npm.domain: app.lan`, `npm.port: 8080`).
- **Canonical LXD / Incus:** `incus config set <instance> user.npm.domain "app.lan"` and `user.npm.port "8080"`.

---

## ⚙️ Configuration (Environment Variables)

| Variable | Default | Description |
| :--- | :--- | :--- |
| `NPM_URL` | `http://127.0.0.1:81` | URL of the Nginx Proxy Manager admin UI and REST API. |
| `NPM_USER` | `admin@example.com` | NPM administrator email address. |
| `NPM_PASS` | `changeme` | NPM administrator password. |
| `HOST_ID` | Hostname | Unique identifier for this node (e.g. `worker-01`). Protects proxies from cross-host pruning. |
| `HOST_IP` | `""` | Public/LAN IP or hostname of this Docker host when NPM is located on another machine. |
| `USE_HOST_PORT` | `false` | When `HOST_IP` is set, maps forward target to the container's published host port. |
| `DOCKER_SOCKET` | `/var/run/docker.sock` | Path to the Docker daemon socket (or `tcp://host:2375`). |
| `NPM_NETWORK` | `""` | Name of the shared Docker bridge network connecting NPM and target containers. |
| `FORWARD_HOST_STRATEGY` | `auto` | Target resolution strategy: `auto`, `name`, or `ip` (see below). |
| `POLL_INTERVAL` | `30s` | Frequency for full container scans and orphan pruning. |
| `PVE_ENABLED` | `false` | Enable Proxmox VE (PVE) LXC container auto-discovery. |
| `PVE_URL` | `""` | Proxmox API URL (e.g. `https://192.168.1.30:8006`). |
| `PVE_TOKEN_ID` / `PVE_TOKEN_SECRET` | `""` | Proxmox API Token credentials. |
| `PVE_PREFERRED_INTERFACE` | `eth0` | Preferred network interface for LXC container IP discovery. |
| `PVE_ALLOWED_SUBNETS` | `""` | Optional comma-separated CIDR filter for LXC IPs (e.g. `192.168.1.0/24`). |
| `LXD_ENABLED` | `false` | Enable Canonical LXD / LinuxContainers Incus auto-discovery. |
| `LXD_SOCKET` | `/var/snap/lxd/...` | Path to LXD or Incus Unix domain socket. |
| `ROUTES_FILE` / `ROUTES_DIR` | `""` | Path to declarative static YAML file or directory (`routes.yaml`). |
| `DEFAULT_FORWARD_SCHEME` | `http` | Default forward scheme if container label is omitted (`http` or `https`). |
| `DEFAULT_SSL_ENABLED` | `false` | Default SSL enablement if not specified in labels. |
| `DEFAULT_SSL_FORCED` | `false` | Default SSL force redirect if not specified in labels. |
| `DEFAULT_WEBSOCKET` | `true` | Default WebSocket upgrade policy. |
| `DEFAULT_BLOCK_EXPLOITS` | `true` | Default exploit blocking policy. |
| `LABEL_PREFIX` | `npm.` | Custom label prefix to match. |
| `PORT` | `8080` | Port for the built-in Web Management Dashboard and REST API. |

---

## 🌐 Multi-Host / Distributed Discovery Architecture

You can run **multiple `npm-autodiscovery` agents on separate Docker hosts**, all synchronizing with a **single, central Nginx Proxy Manager instance**!

```
┌─────────────────────────────────┐       ┌─────────────────────────────────┐
│       Docker Host 1 (Edge)      │       │     Docker Host 2 (Worker)      │
│  ┌───────────────────────────┐  │       │  ┌───────────────────────────┐  │
│  │   npm-autodiscovery       │  │       │  │   npm-autodiscovery       │  │
│  │   (HOST_ID: worker-01)    │  │       │  │   (HOST_ID: worker-02)    │  │
│  │   (HOST_IP: 192.168.1.20) │  │       │  │   (HOST_IP: 192.168.1.30) │  │
│  └─────────────┬─────────────┘  │       │  └─────────────┬─────────────┘  │
│                │                │       │                │                │
│    Containers with labels       │       │    Containers with labels       │
└────────────────┼────────────────┘       └────────────────┼────────────────┘
                 │                                         │
                 │   NPM REST API (Tagged with Host ID)    │
                 └───────────────────┬─────────────────────┘
                                     ▼
                  ┌─────────────────────────────────────┐
                  │      Central Nginx Proxy Manager    │
                  │        (e.g., at 192.168.1.10)      │
                  └─────────────────────────────────────┘
```

### How Multi-Host Support Works

1. **Host Isolation & Ownership Protection:**
   Every proxy host created in NPM is automatically tagged with that node's `HOST_ID` in its metadata and Nginx config header (e.g. `# Managed by NPM-AutoDiscovery [host_id: worker-02]`). An agent on Host 1 will **never** overwrite, touch, or prune proxy hosts that belong to Host 2!
2. **Cross-Host Traffic Routing:**
   When an agent runs on a host separate from NPM, set `HOST_IP=<this-machine-ip>` and `USE_HOST_PORT=true`.
   NPM will then forward ingress traffic to `<this-machine-ip>:<container-published-port>` instead of trying to reach an internal Docker bridge IP that only exists on the remote node.

---

### 🌐 Network Resolution Strategies

NPM Auto-Discovery provides three host resolution strategies (`FORWARD_HOST_STRATEGY`):

1. **`auto` (Recommended):** If `NPM_NETWORK` is set and the container is connected to that network, resolves using the container's Docker name (leveraging Docker's internal DNS). If not, falls back to the container's internal network IP address.
2. **`name`:** Resolves using the Docker container name (e.g., `web-service`). Ideal when all containers and NPM reside on the same user-defined Docker bridge network.
3. **`ip`:** Directly queries the container's assigned IP address on the shared network or bridge (e.g., `172.20.0.5`).

---

## 🚀 Deployment Options

### Option A: Standalone Deployment (Existing NPM Instance)

If you already have Nginx Proxy Manager running, simply configure your `.env` file and run NPM Auto-Discovery:

1. **Create your `.env` configuration file:**
```bash
cp .env.example .env
```

2. **Configure your NPM instance connection in `.env`:**
```dotenv
# Target Nginx Proxy Manager instance
NPM_URL=http://nginx-proxy-manager:81 # or http://192.168.1.100:81
NPM_USER=admin@example.com
NPM_PASS=changeme
NPM_NETWORK=npm-network
FORWARD_HOST_STRATEGY=auto
POLL_INTERVAL=30s
```

3. **Start the agent using Docker Compose:**
```bash
docker compose up -d
```

`docker-compose.yml` loads configuration from `.env` via `env_file`:

```yaml
version: '3.8'

services:
  npm-autodiscovery:
    image: raddadengineer/npm-autodiscovery:latest
    container_name: npm-autodiscovery
    restart: unless-stopped
    ports:
      - '${PORT:-8080}:${PORT:-8080}' # Auto-Discovery Dashboard
    env_file:
      - .env
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks:
      - npm-network

networks:
  npm-network:
    name: ${NPM_NETWORK:-npm-network}
    driver: bridge # or external: true if network already exists
```

### Option B: All-in-One Development Stack (With NPM & Demo App)

To spin up NPM Auto-Discovery alongside a new Nginx Proxy Manager instance and a demo target service in a single command, you can use the built-in Compose profiles:

```bash
cp .env.example .env
docker compose --profile all-in-one up -d
```

Or deploy using this complete, self-contained `docker-compose.yml`:

```yaml
version: '3.8'

services:
  # 1. Nginx Proxy Manager
  npm:
    image: 'jc21/nginx-proxy-manager:latest'
    container_name: nginx-proxy-manager
    restart: unless-stopped
    ports:
      - '80:80'     # Public HTTP Traffic
      - '443:443'   # Public HTTPS Traffic
      - '81:81'     # NPM Admin Web UI & REST API
    environment:
      DISABLE_IPV6: 'true'
    volumes:
      - npm_data:/data
      - npm_letsencrypt:/etc/letsencrypt
    networks:
      - npm-network

  # 2. NPM Auto-Discovery Agent
  npm-autodiscovery:
    image: raddadengineer/npm-autodiscovery:latest
    container_name: npm-autodiscovery
    restart: unless-stopped
    ports:
      - '${PORT:-8080}:${PORT:-8080}' # Auto-Discovery Management Dashboard
    env_file:
      - .env
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks:
      - npm-network

  # 3. Demo Target Service (Auto-Discovered & Provisioned)
  demo-whoami:
    image: raddadengineer/whoami:latest
    container_name: demo-whoami
    restart: unless-stopped
    networks:
      - npm-network
    labels:
      - "npm.frontend.domain=whoami.local"
      - "npm.frontend.port=80"
      - "npm.forward_scheme=http"
      - "npm.websocket=true"
      - "npm.block_exploits=true"
      - "npm.ssl.enabled=false"

volumes:
  npm_data:
    driver: local
  npm_letsencrypt:
    driver: local

networks:
  npm-network:
    name: ${NPM_NETWORK:-npm-network}
    driver: bridge
```

### Option C: Multi-Host / Remote Worker Node Deployment (Headless or Standalone)

To run an agent on a separate machine (Host 2, Host 3, etc.) that discovers local containers and routes traffic back to a central Nginx Proxy Manager instance (Host 1):

1. **On the worker machine, create your `.env` configuration:**
```bash
cp .env.worker.example .env
```

2. **Configure node identity, cross-host routing, and headless telemetry push in `.env`:**
```dotenv
# Central NPM instance reachable from this worker
NPM_URL=http://192.168.1.10:81
NPM_USER=admin@example.com
NPM_PASS=changeme

# Unique worker ID (prevents collision with peer nodes)
HOST_ID=worker-node-01

# Worker machine IP reachable by central NPM
HOST_IP=192.168.1.20

# Route ingress traffic to published host ports on HOST_IP
USE_HOST_PORT=true

# --- Unified Cluster & Headless Worker Mode ---
# Disable local dashboard completely (0 open listening ports on the worker node)
DASHBOARD_ENABLED=false

# Push worker telemetry, containers, proxies, and logs to central controller
MAIN_NODE_URL=http://192.168.1.10:8080
CLUSTER_TOKEN=your-cluster-secret-token
PUSH_INTERVAL=15s
```

3. **Deploy using `docker-compose.worker.yml`:**
```bash
docker compose -f docker-compose.worker.yml up -d
```

Central NPM will automatically register `worker.local` routing to `http://192.168.1.20:8081` tagged with `[host_id: worker-node-01]`, preventing collision or accidental deletion by other nodes.

When `DASHBOARD_ENABLED=false` is set:
- **Zero Open Ports:** The HTTP server is not initialized on the worker host; no extra ports need to be exposed or mapped in Docker.
- **Centralized Visibility:** All discovered containers, active proxies, L4 streams, and live logs from the worker node appear directly on your main node's dashboard under **Cluster Nodes**, complete with host badges and filter controls.

### Quickstart Guide

1. **Initial NPM Setup:** Open `http://localhost:81` in your browser. If starting fresh, log in with default credentials (`admin@example.com` / `changeme`) and set your administrator email and password.
2. **Auto-Discovery Dashboard:** Open `http://localhost:8080` to watch the agent authenticate with NPM, discover `demo-whoami`, and provision the proxy host in real time!
3. **Verify Routing:** Add `127.0.0.1 whoami.local` to your `/etc/hosts` and open `http://whoami.local` in your browser. Traffic will route through NPM straight to the demo container!

---

## 🖥️ Management Dashboard & REST API

The application serves a single-page web app and REST API on `PORT` (`8080`):

### REST Endpoints

- `GET /api/status`: Returns Docker & NPM connection health, container counts, active proxy counts, cluster health summary, and uptime.
- `GET /api/proxies?node=<node_id>`: Returns list of all active auto-discovered proxy hosts (aggregated across all cluster nodes or filtered by node ID) with target IPs, domains, and SSL status.
- `GET /api/streams?node=<node_id>`: Returns list of all active TCP/UDP L4 streams across nodes.
- `GET /api/containers?node=<node_id>`: Returns running Docker containers and their discovery readiness.
- `GET /api/cluster/nodes`: Returns status, last heartbeat, container/proxy counts, and platform details for all worker nodes in the cluster.
- `POST /api/cluster/report`: Secure ingestion endpoint for remote worker nodes to push periodic telemetry reports (requires `X-Cluster-Token` or Bearer authentication if configured).
- `POST /api/sync`: Forces an immediate full container scan and synchronization with NPM.
- `GET /api/events/stream`: Live Server-Sent Events (SSE) log stream for real-time terminal output across all nodes.
- `GET /api/events`: Fetches recent log history buffer.
- `GET /api/config`: Returns active non-sensitive configuration settings.

---

## 🛠️ Development & Building Locally

### Requirements

- Go 1.22+
- Docker Engine / Docker Desktop

### Run Locally

```bash
# Run tests
go test -v ./...

# Build binary
go build -o npm-autodiscovery ./cmd/agent

# Run with custom parameters
NPM_URL=http://localhost:81 \
NPM_USER=admin@example.com \
NPM_PASS=changeme \
PORT=8080 \
./npm-autodiscovery
```

---

## 📄 License

MIT License. See [LICENSE](LICENSE) for details.
