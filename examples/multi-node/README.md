# Multi-Node / Multi-Instance Deployment Example 🌐

This example demonstrates deploying NPM Auto-Discovery across multiple independent Docker hosts communicating with a single central Nginx Proxy Manager instance.

## Topology Overview

```
┌──────────────────────────────────────┐       ┌──────────────────────────────────────┐
│  Host 1: Central Controller          │       │  Host 2: Worker Node 01              │
│  IP: 192.168.1.10                    │       │  IP: 192.168.1.20                    │
│                                      │       │                                      │
│  ┌────────────────────────────────┐  │       │  ┌────────────────────────────────┐  │
│  │ Nginx Proxy Manager (Port 81)  │  │       │  │ npm-autodiscovery (agent)      │  │
│  └────────────────────────────────┘  │       │  │ HOST_ID: worker-01             │  │
│  ┌────────────────────────────────┐  │       │  │ HOST_IP: 192.168.1.20          │  │
│  │ npm-autodiscovery (agent)      │  │       │  │ USE_HOST_PORT: true            │  │
│  │ HOST_ID: central-01            │  │       │  └────────────────┬───────────────┘  │
│  └────────────────────────────────┘  │       │                   │                  │
│                                      │       │  ┌────────────────▼───────────────┐  │
│                                      │       │  │ App Container (8081:80)        │  │
│                                      │       │  │ npm.frontend.domain=app.lan    │  │
│                                      │       │  └────────────────────────────────┘  │
└──────────────────┬───────────────────┘       └───────────────────┬──────────────────┘
                   │                                               │
                   └────────── Central NPM REST API ───────────────┘
```

## Step 1: Deploy Central Controller (Host 1)

1. On **Host 1** (`192.168.1.10`), navigate to `central-node`:
   ```bash
   cd central-node
   cp .env.example .env
   docker compose up -d
   ```
2. Log into NPM at `http://192.168.1.10:81` with `admin@example.com` / `changeme` and complete the initial setup.

## Step 2: Deploy Worker Node (Host 2)

1. On **Host 2** (`192.168.1.20`), navigate to `worker-node`:
   ```bash
   cd worker-node
   cp .env.example .env
   ```
2. In `.env`, ensure `NPM_URL` points to Host 1 (`http://192.168.1.10:81`), and `HOST_IP` is set to Host 2's IP (`192.168.1.20`):
   ```env
   NPM_URL=http://192.168.1.10:81
   HOST_ID=worker-01
   HOST_IP=192.168.1.20
   USE_HOST_PORT=true
   ```
3. Start the worker agent:
   ```bash
   docker compose up -d
   ```

## How It Works

- **No Overwrite/Collision:** The worker agent tags its proxy host entries with `[host_id: worker-01]`. The central agent (or other worker nodes) will never overwrite or prune routes created by another host.
- **Cross-Host Routing:** When `USE_HOST_PORT=true` is enabled, the agent discovers the container's published host port (`8081`) and tells central NPM to forward traffic to `http://192.168.1.20:8081`.
