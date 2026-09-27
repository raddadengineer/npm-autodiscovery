# NPM Auto-Discovery 🚀
> Automated Docker service discovery and dynamic proxy host provisioning for **Nginx Proxy Manager (NPM)**.

[![Go Report Card](https://goreportcard.com/badge/github.com/sampson/npm-autodiscovery)](https://goreportcard.com/report/github.com/sampson/npm-autodiscovery)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white)](Dockerfile)

---

## 📖 Overview

While **Traefik** has native support for reading Docker container labels to dynamically create ingress routes, **Nginx Proxy Manager (NPM)** traditionally requires users to manually log in to the web UI and configure every single Proxy Host.

**NPM Auto-Discovery** bridges this gap. Operating as a lightweight sidecar container alongside Nginx Proxy Manager, it listens directly to the Docker daemon socket (`/var/run/docker.sock`) for container lifecycle events (`start`, `die`, `stop`, `destroy`). When a container with NPM discovery labels is detected, it automatically creates, updates, or cleans up proxy hosts via the official Nginx Proxy Manager REST API.

It also comes bundled with a **real-time Single Page Application dashboard** built directly into the single binary (no Node.js or heavy dependencies required in production), giving you full visibility over active ingress routes, live container discovery logs, and one-click manual synchronization.

---

## ✨ Features

- **⚡ Native Docker Socket Event Listener:** Subscribes to real-time container events (`start`, `die`, `destroy`, `stop`, `kill`) with automatic backoff reconnection.
- **🔄 Official NPM API Integration:** Authenticates with NPM's REST API using JWT tokens with automatic renewal prior to expiration and on HTTP 401 retries.
- **🎯 Smart Network & IP Mapping:** Supports three host resolution strategies (`auto`, `name`, `ip`) to ensure NPM can reach container endpoints across Docker bridge networks and Docker DNS.
- **🛡️ Idempotent Reconciler:** Diffs desired container configuration with existing NPM proxy hosts. Updates only when configuration changes occur, avoiding unnecessary Nginx reloads.
- **🧹 Automatic Orphan Pruning:** Automatically purges proxy hosts when containers stop or die, and cleans up orphaned routes during periodic full scans (`POLL_INTERVAL`).
- **📊 Real-Time Cyberpunk Dashboard:** Embedded Web UI featuring live Server-Sent Events (SSE) log streaming, search & filtering, stats cards, and a Docker Compose label generator.
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

## 🏷️ Docker Container Label Reference

Add these labels to any Docker container (via `docker-compose.yml` or `docker run -l`) to control proxying:

| Label | Type | Default | Description | Example |
| :--- | :--- | :--- | :--- | :--- |
| `npm.frontend.domain` | string | *Required* | Domain name(s) for the proxy host. Supports comma-separated lists. | `app.example.com, web.example.com` |
| `npm.frontend.port` | integer | Auto-detect | Target container internal port. Auto-detects if container exposes a single port. | `80`, `3000`, `8080` |
| `npm.forward_scheme` | string | `http` | Upstream protocol scheme (`http` or `https`). | `http` |
| `npm.forward_host` | string | Auto | Custom target host or IP override. | `192.168.1.50` or `my-service` |
| `npm.ssl.enabled` | boolean | `false` | Enable SSL for this proxy host. | `true` |
| `npm.ssl.forced` | boolean | `false` | Force HTTP to HTTPS redirection. | `true` |
| `npm.certificate_id` | int/str | `0` | NPM Certificate ID (e.g. `1`) or `"new"` for Let's Encrypt. | `1` |
| `npm.websocket` | boolean | `true` | Enable WebSocket upgrade support (`proxy_set_header Upgrade`). | `true` |
| `npm.block_exploits` | boolean | `true` | Enable NPM exploit block filters. | `true` |
| `npm.caching` | boolean | `false` | Enable Nginx static asset caching. | `false` |
| `npm.hsts` | boolean | `false` | Enable HTTP Strict Transport Security (HSTS). | `true` |
| `npm.http2` | boolean | `false` | Enable HTTP/2 protocol support. | `true` |
| `npm.enabled` | boolean | `true` | Explicitly enable or disable discovery for this container. | `false` |
| `npm.advanced_config`| string | `""` | Custom Nginx directives appended to the proxy host configuration. | `client_max_body_size 100M;` |

*(Note: Short labels like `npm.domain` and `npm.port` are also supported interchangeably).*

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
| `FORWARD_HOST_STRATEGY`| `auto` | Target resolution strategy: `auto`, `name`, or `ip` (see below). |
| `POLL_INTERVAL` | `30s` | Frequency for full container scans and orphan pruning. |
| `DEFAULT_FORWARD_SCHEME`| `http` | Default forward scheme if container label is omitted (`http` or `https`). |
| `DEFAULT_SSL_ENABLED` | `false` | Default SSL enablement if not specified in labels. |
| `DEFAULT_SSL_FORCED` | `false` | Default SSL force redirect if not specified in labels. |
| `DEFAULT_WEBSOCKET` | `true` | Default WebSocket upgrade policy. |
| `DEFAULT_BLOCK_EXPLOITS`| `true` | Default exploit blocking policy. |
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

### How Multi-Host Support Works:
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

If you already have Nginx Proxy Manager running, simply run NPM Auto-Discovery as a standalone container:

```bash
docker compose up -d
```

By default, `docker-compose.yml` runs **only** the `npm-autodiscovery` agent. Configure `NPM_URL` in `.env` or pass it directly:

```yaml
version: '3.8'

services:
  npm-autodiscovery:
    image: raddadengineer/npm-autodiscovery:latest
    build: .
    container_name: npm-autodiscovery
    restart: unless-stopped
    ports:
      - '8080:8080' # Auto-Discovery Dashboard
    environment:
      - NPM_URL=http://nginx-proxy-manager:81 # or http://192.168.1.100:81
      - NPM_USER=admin@example.com
      - NPM_PASS=changeme
      - NPM_NETWORK=npm-network
      - FORWARD_HOST_STRATEGY=auto
      - POLL_INTERVAL=30s
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks:
      - npm-network

networks:
  npm-network:
    name: npm-network
    driver: bridge # or external: true if network already exists
```

### Option B: All-in-One Development Stack (With NPM & Demo App)

To spin up NPM Auto-Discovery alongside a new Nginx Proxy Manager instance and a demo container, use the `--profile with-npm` or `--profile all-in-one` flag:

```bash
docker compose --profile all-in-one up -d
```

### Starting the Stack
```bash
docker compose up -d
```
1. Open NPM at `http://localhost:81` to configure your initial admin account if starting fresh.
2. Open the NPM Auto-Discovery Dashboard at `http://localhost:8080` to watch containers being discovered in real time!

---

## 🖥️ Management Dashboard & REST API

The application serves a single-page web app and REST API on `PORT` (`8080`):

### REST Endpoints

- `GET /api/status`: Returns Docker & NPM connection health, container counts, active proxy counts, and uptime.
- `GET /api/proxies`: Returns list of all active auto-discovered proxy hosts with target IPs, domains, and SSL status.
- `GET /api/containers`: Returns all running Docker containers and their discovery readiness.
- `POST /api/sync`: Forces an immediate full container scan and synchronization with NPM.
- `GET /api/events/stream`: Live Server-Sent Events (SSE) log stream for real-time terminal output.
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
