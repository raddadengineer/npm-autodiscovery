// NPM Auto-Discovery Dashboard Single Page Application
let activeTab = 'proxies-view';
let proxiesData = [];
let streamsData = [];
let containersData = [];
let clusterData = [];
let selectedContainersNode = 'all';
let selectedProxiesNode = 'all';
let selectedStreamsNode = 'all';
let selectedEventsNode = 'all';
let currentLogFilter = 'all';
let autoScrollLogs = true;

document.addEventListener('DOMContentLoaded', () => {
  initTabs();
  initSearchAndFilters();
  initSyncButton();
  initEventStream();
  initDocsSubtabs();
  initDocsSearch();
  initAddNodeGenerator();
  loadInitialData();

  // Periodic polling for status and tables every 5 seconds as fallback
  setInterval(fetchStatus, 5000);
  setInterval(fetchProxies, 6000);
  setInterval(fetchStreams, 6000);
  setInterval(fetchContainers, 10000);
  setInterval(fetchClusterNodes, 8000);
});

// Tab navigation handler
function initTabs() {
  document.querySelectorAll('.tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const targetTab = btn.getAttribute('data-tab');
      switchTab(targetTab);
    });
  });
}

function switchTab(tabId) {
  activeTab = tabId;
  document.querySelectorAll('.tab-btn').forEach(b => {
    b.classList.toggle('active', b.getAttribute('data-tab') === tabId);
  });
  document.querySelectorAll('.tab-content').forEach(content => {
    content.classList.toggle('active', content.id === tabId);
  });

  if (tabId === 'containers-view') {
    fetchContainers();
  } else if (tabId === 'streams-view') {
    fetchStreams();
  } else if (tabId === 'cluster-view') {
    fetchClusterNodes();
  }
}

// Search and Filter controls
function initSearchAndFilters() {
  const proxiesSearch = document.getElementById('proxies-search');
  if (proxiesSearch) {
    proxiesSearch.addEventListener('input', (e) => {
      renderProxiesTable(e.target.value.toLowerCase());
    });
  }

  const streamsSearch = document.getElementById('streams-search');
  if (streamsSearch) {
    streamsSearch.addEventListener('input', (e) => {
      renderStreamsTable(e.target.value.toLowerCase());
    });
  }

  const containersSearch = document.getElementById('containers-search');
  if (containersSearch) {
    containersSearch.addEventListener('input', (e) => {
      renderContainersTable(e.target.value.toLowerCase());
    });
  }

  const clusterSearch = document.getElementById('cluster-search');
  if (clusterSearch) {
    clusterSearch.addEventListener('input', (e) => {
      renderClusterNodes(e.target.value.toLowerCase());
    });
  }

  const containersNodeFilter = document.getElementById('containers-node-filter');
  if (containersNodeFilter) {
    containersNodeFilter.addEventListener('change', (e) => {
      selectedContainersNode = e.target.value;
      renderContainersTable();
    });
  }

  const proxiesNodeFilter = document.getElementById('proxies-node-filter');
  if (proxiesNodeFilter) {
    proxiesNodeFilter.addEventListener('change', (e) => {
      selectedProxiesNode = e.target.value;
      renderProxiesTable();
    });
  }

  const streamsNodeFilter = document.getElementById('streams-node-filter');
  if (streamsNodeFilter) {
    streamsNodeFilter.addEventListener('change', (e) => {
      selectedStreamsNode = e.target.value;
      renderStreamsTable();
    });
  }

  const eventsNodeFilter = document.getElementById('events-node-filter');
  if (eventsNodeFilter) {
    eventsNodeFilter.addEventListener('change', (e) => {
      selectedEventsNode = e.target.value;
      filterTerminalLogs();
    });
  }

  const refreshClusterBtn = document.getElementById('refresh-cluster-btn');
  if (refreshClusterBtn) {
    refreshClusterBtn.addEventListener('click', () => {
      fetchClusterNodes();
      showToast('Cluster node telemetry refreshed', 'info');
    });
  }

  const clusterPill = document.getElementById('cluster-status-pill');
  if (clusterPill) {
    clusterPill.addEventListener('click', () => switchTab('cluster-view'));
  }

  document.querySelectorAll('.filter-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      currentLogFilter = btn.getAttribute('data-filter');
      filterTerminalLogs();
    });
  });

  const clearBtn = document.getElementById('clear-logs-btn');
  if (clearBtn) {
    clearBtn.addEventListener('click', () => {
      const term = document.getElementById('terminal-logs');
      term.innerHTML = '';
      showToast('Live log view cleared', 'info');
    });
  }

  const scrollToggle = document.getElementById('autoscroll-toggle');
  if (scrollToggle) {
    scrollToggle.addEventListener('change', (e) => {
      autoScrollLogs = e.target.checked;
    });
  }
}

// "Sync Now" button handler
function initSyncButton() {
  const syncBtn = document.getElementById('sync-now-btn');
  if (!syncBtn) return;

  syncBtn.addEventListener('click', async () => {
    syncBtn.classList.add('spinning');
    syncBtn.disabled = true;

    try {
      const res = await fetch('/api/sync', { method: 'POST' });
      const data = await res.json();
      showToast(data.message || 'Full sync triggered', 'success');

      // Refresh immediately
      setTimeout(() => {
        fetchStatus();
        fetchProxies();
        fetchStreams();
      }, 500);
    } catch (err) {
      showToast('Sync request failed: ' + err.message, 'error');
    } finally {
      setTimeout(() => {
        syncBtn.classList.remove('spinning');
        syncBtn.disabled = false;
      }, 1000);
    }
  });
}

// Initial Data Fetch
async function loadInitialData() {
  await fetchStatus();
  await fetchProxies();
  await fetchStreams();
  await fetchContainers();
  await fetchClusterNodes();
}

// Fetch System Status & Metrics
async function fetchStatus() {
  try {
    const res = await fetch('/api/status');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const status = await res.json();

    // Node Status Pill
    const nodeText = document.getElementById('node-id-text');
    if (nodeText && status.host_id) {
      nodeText.textContent = status.host_id;
    }

    // Cluster Status Pill
    const clusterText = document.getElementById('cluster-status-text');
    if (clusterText) {
      const total = status.cluster_nodes_count || 1;
      const online = status.cluster_online_nodes || 1;
      if (total > 1) {
        clusterText.textContent = `${online}/${total} Online`;
      } else {
        clusterText.textContent = `1 Node`;
      }
    }
    const tabClusterCount = document.getElementById('tab-cluster-count');
    if (tabClusterCount) {
      tabClusterCount.textContent = status.cluster_nodes_count || 1;
    }

    // Docker Status Pill
    const dockerPill = document.getElementById('docker-status-pill');
    const dockerDot = dockerPill.querySelector('.status-dot');
    const dockerText = document.getElementById('docker-status-text');
    if (status.docker_connected) {
      dockerDot.className = 'status-dot ping-dot connected';
      dockerText.textContent = 'Connected';
    } else {
      dockerDot.className = 'status-dot ping-dot disconnected';
      dockerText.textContent = 'Disconnected';
    }

    // NPM Status Pill
    const npmPill = document.getElementById('npm-status-pill');
    const npmDot = npmPill.querySelector('.status-dot');
    const npmText = document.getElementById('npm-status-text');
    if (status.npm_connected) {
      npmDot.className = 'status-dot ping-dot connected';
      npmText.textContent = 'Connected';
    } else {
      npmDot.className = 'status-dot ping-dot disconnected';
      npmText.textContent = 'Auth Failed';
    }

    // PVE Status Pill & Metrics
    const pvePill = document.getElementById('pve-status-pill');
    if (pvePill) {
      const pveDot = pvePill.querySelector('.status-dot');
      const pveText = document.getElementById('pve-status-text');
      if (!status.pve_enabled) {
        pveDot.className = 'status-dot';
        pveText.textContent = 'Disabled';
      } else if (status.pve_connected) {
        pveDot.className = 'status-dot ping-dot connected';
        pveText.textContent = status.pve_node ? `Node: ${status.pve_node}` : 'Connected';
      } else {
        pveDot.className = 'status-dot ping-dot disconnected';
        pveText.textContent = 'Disconnected';
      }
    }

    if (document.getElementById('pve-containers-count')) {
      document.getElementById('pve-containers-count').textContent = status.pve_running_containers || 0;
    }
    if (document.getElementById('pve-status-detail')) {
      document.getElementById('pve-status-detail').textContent = status.pve_version ? `PVE ${status.pve_version}` : (status.pve_enabled ? (status.pve_node || 'Proxmox API') : 'Disabled');
    }

    // LXD Status Pill & Metrics
    const lxdPill = document.getElementById('lxd-status-pill');
    if (lxdPill) {
      const lxdDot = lxdPill.querySelector('.status-dot');
      const lxdText = document.getElementById('lxd-status-text');
      if (!status.lxd_enabled) {
        lxdDot.className = 'status-dot';
        lxdText.textContent = 'Disabled';
      } else if (status.lxd_connected) {
        lxdDot.className = 'status-dot ping-dot connected';
        lxdText.textContent = 'Connected';
      } else {
        lxdDot.className = 'status-dot ping-dot disconnected';
        lxdText.textContent = 'Disconnected';
      }
    }

    if (document.getElementById('lxd-containers-count')) {
      document.getElementById('lxd-containers-count').textContent = status.lxd_running_containers || 0;
    }
    if (document.getElementById('lxd-status-detail')) {
      document.getElementById('lxd-status-detail').textContent = status.lxd_version ? `v${status.lxd_version}` : (status.lxd_enabled ? 'Unix Socket' : 'Disabled');
    }

    // Metric Cards
    document.getElementById('active-proxies-count').textContent = status.active_proxies;
    document.getElementById('tab-proxies-count').textContent = status.active_proxies;
    if (document.getElementById('active-streams-count')) {
      document.getElementById('active-streams-count').textContent = status.active_streams || 0;
    }
    if (document.getElementById('tab-streams-count')) {
      document.getElementById('tab-streams-count').textContent = status.active_streams || 0;
    }
    document.getElementById('running-containers-count').textContent = status.running_containers;
    document.getElementById('docker-version-text').textContent = status.docker_version || 'Docker Socket';
    document.getElementById('npm-url-text').textContent = status.npm_url || 'NPM API';
    document.getElementById('total-events-count').textContent = status.total_events;

    if (status.last_full_sync && status.last_full_sync !== '0001-01-01T00:00:00Z') {
      const syncDate = new Date(status.last_full_sync);
      document.getElementById('last-sync-time').textContent = syncDate.toLocaleTimeString();
    } else {
      document.getElementById('last-sync-time').textContent = 'Pending';
    }

  } catch (err) {
    console.warn('Failed to fetch status:', err);
  }
}

// Fetch Active Proxies
async function fetchProxies() {
  try {
    const res = await fetch('/api/proxies');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    proxiesData = data.proxies || [];

    // Calculate SSL count
    const sslCount = proxiesData.filter(p => p.ssl_enabled || p.ssl_forced).length;
    document.getElementById('ssl-active-count').textContent = sslCount;

    renderProxiesTable();
  } catch (err) {
    console.warn('Failed to fetch proxies:', err);
  }
}

// Fetch Layer 4 Streams
async function fetchStreams() {
  try {
    const res = await fetch('/api/streams');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    streamsData = data.streams || [];

    if (document.getElementById('tab-streams-count')) {
      document.getElementById('tab-streams-count').textContent = streamsData.length;
    }
    if (document.getElementById('active-streams-count')) {
      document.getElementById('active-streams-count').textContent = streamsData.length;
    }

    renderStreamsTable();
  } catch (err) {
    console.warn('Failed to fetch streams:', err);
  }
}

// Render Layer 4 Streams Table
function renderStreamsTable(filterText = '') {
  const tbody = document.getElementById('streams-tbody');
  const emptyState = document.getElementById('streams-empty');
  if (!tbody) return;
  tbody.innerHTML = '';

  const filtered = streamsData.filter(s => {
    if (selectedStreamsNode !== 'all' && s.host_id && s.host_id.toLowerCase() !== selectedStreamsNode.toLowerCase()) {
      return false;
    }
    if (!filterText) return true;
    const searchStr = `${s.incoming_port} ${s.forwarding_host}:${s.forwarding_port} ${s.container_name || ''} ${s.host_id || ''} ${s.tcp ? 'tcp' : ''} ${s.udp ? 'udp' : ''}`.toLowerCase();
    return searchStr.includes(filterText);
  });

  if (filtered.length === 0) {
    if (emptyState) emptyState.style.display = 'block';
    return;
  }
  if (emptyState) emptyState.style.display = 'none';

  filtered.forEach(stream => {
    const tr = document.createElement('tr');

    let protoBadges = [];
    if (stream.tcp) protoBadges.push(`<span class="badge badge-cyan">TCP</span>`);
    if (stream.udp) protoBadges.push(`<span class="badge badge-violet">UDP</span>`);
    if (protoBadges.length === 0) protoBadges.push(`<span class="badge badge-gray">None</span>`);

    let sourceBadge = `<span class="badge badge-cyan" style="font-size: 0.7rem;">🐳 Docker</span>`;
    if (stream.source === 'iac') {
      sourceBadge = `<span class="badge badge-amber" style="font-size: 0.7rem;" title="IaC Manifest: ${escapeHtml(stream.source_file || 'routes.yaml')}">📄 IaC</span>`;
    } else if (stream.source === 'pve') {
      sourceBadge = `<span class="badge" style="background: rgba(249, 115, 22, 0.15); color: #f97316; font-size: 0.7rem;" title="Proxmox VE LXC">⚡ Proxmox</span>`;
    } else if (stream.source === 'lxd') {
      sourceBadge = `<span class="badge" style="background: rgba(16, 185, 129, 0.15); color: #10b981; font-size: 0.7rem;" title="Canonical LXD / Incus">🐧 LXD/Incus</span>`;
    }

    const lastSyncTime = stream.last_synced ? new Date(stream.last_synced).toLocaleTimeString() : 'N/A';

    tr.innerHTML = `
      <td><span class="badge badge-emerald">● Live</span></td>
      <td>
        <span class="target-badge" style="font-family: var(--font-mono); font-weight: 700; color: #00f2fe;">
          :${stream.incoming_port}
        </span>
      </td>
      <td>
        <span class="target-badge">
          ${escapeHtml(stream.forwarding_host)}:${stream.forwarding_port}
        </span>
      </td>
      <td><div style="display:flex; gap:0.25rem;">${protoBadges.join(' ')}</div></td>
      <td>${sourceBadge}</td>
      <td>
        <div class="container-info">
          <span class="container-title">${escapeHtml(stream.container_name || (stream.source === 'iac' ? 'IaC Manifest' : 'stream'))}</span>
          <span class="container-sub">${escapeHtml(stream.source_file ? stream.source_file : (stream.container_id ? stream.container_id.substring(0, 12) : ''))}</span>
        </div>
      </td>
      <td><span class="badge badge-gray" style="font-family: var(--font-mono); font-size: 0.75rem;">${escapeHtml(stream.host_id || 'local')}</span></td>
      <td><span style="font-family: var(--font-mono); color: var(--text-muted);">#${stream.npm_stream_id}</span></td>
      <td><span style="font-size: 0.8rem; color: var(--text-muted);">${lastSyncTime}</span></td>
    `;
    tbody.appendChild(tr);
  });
}

// Render Proxies Table
function renderProxiesTable(filterText = '') {
  const tbody = document.getElementById('proxies-tbody');
  const emptyState = document.getElementById('proxies-empty');
  tbody.innerHTML = '';

  const filtered = proxiesData.filter(p => {
    if (selectedProxiesNode !== 'all' && p.host_id && p.host_id.toLowerCase() !== selectedProxiesNode.toLowerCase()) {
      return false;
    }
    if (!filterText) return true;
    const searchStr = `${p.domain_names.join(' ')} ${p.container_name} ${p.forward_host}:${p.forward_port} ${p.host_id || ''}`.toLowerCase();
    return searchStr.includes(filterText);
  });

  if (filtered.length === 0) {
    emptyState.style.display = 'block';
    return;
  }
  emptyState.style.display = 'none';

  filtered.forEach(proxy => {
    const tr = document.createElement('tr');

    // Domain links
    const domainChips = proxy.domain_names.map(d => {
      const proto = proxy.forward_scheme || 'http';
      return `<a href="${proto}://${d}" target="_blank" rel="noopener noreferrer" class="domain-chip">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
          <polyline points="15 3 21 3 21 9"></polyline>
          <line x1="10" y1="14" x2="21" y2="3"></line>
        </svg>
        ${escapeHtml(d)}
      </a>`;
    }).join('');

    // SSL badge
    let sslBadge = `<span class="badge badge-gray">Disabled</span>`;
    if (proxy.ssl_forced) {
      sslBadge = `<span class="badge badge-emerald">🔒 Forced HTTPS</span>`;
    } else if (proxy.ssl_enabled) {
      sslBadge = `<span class="badge badge-cyan">🔒 Enabled</span>`;
    }

    const lastSyncTime = proxy.last_synced ? new Date(proxy.last_synced).toLocaleTimeString() : 'N/A';

    let sourceBadge = `<span class="badge badge-cyan" style="font-size: 0.7rem;">🐳 Docker</span>`;
    if (proxy.source === 'iac') {
      sourceBadge = `<span class="badge badge-amber" style="font-size: 0.7rem;" title="IaC Manifest: ${escapeHtml(proxy.source_file || 'routes.yaml')}">📄 IaC</span>`;
    } else if (proxy.source === 'pve') {
      sourceBadge = `<span class="badge" style="background: rgba(249, 115, 22, 0.15); color: #f97316; font-size: 0.7rem;" title="Proxmox VE LXC">⚡ Proxmox</span>`;
    } else if (proxy.source === 'lxd') {
      sourceBadge = `<span class="badge" style="background: rgba(16, 185, 129, 0.15); color: #10b981; font-size: 0.7rem;" title="Canonical LXD / Incus">🐧 LXD/Incus</span>`;
    }

    let locationBadge = '';
    if (proxy.locations && proxy.locations.length > 0) {
      locationBadge = `<div style="margin-top: 4px;"><span class="badge" style="background: rgba(0, 242, 254, 0.12); color: #00f2fe; font-size: 0.7rem;" title="Custom Locations: ${proxy.locations.map(l => l.path).join(', ')}">📍 ${proxy.locations.length} Locations</span></div>`;
    }

    let replicaBadge = '';
    if (proxy.upstream_replicas && proxy.upstream_replicas > 1) {
      replicaBadge = `<div style="margin-top: 4px;"><span class="badge" style="background: rgba(16, 185, 129, 0.12); color: #10b981; font-size: 0.7rem;" title="Load-balanced across ${proxy.upstream_replicas} containers">⚖️ ${proxy.upstream_replicas} Replicas (${escapeHtml(proxy.upstream_balancing || 'round_robin')})</span></div>`;
    }

    tr.innerHTML = `
      <td><span class="badge badge-emerald">● Live</span></td>
      <td><div class="domain-chip-group">${domainChips}</div></td>
      <td>
        <span class="target-badge">
          <span class="scheme-tag">${escapeHtml(proxy.forward_scheme)}://</span>
          ${escapeHtml(proxy.forward_host)}:${proxy.forward_port}
        </span>
        ${locationBadge}
        ${replicaBadge}
      </td>
      <td>
        <div style="display: flex; flex-direction: column; gap: 0.2rem;">
          <span class="badge badge-gray" style="font-family: var(--font-mono); font-size: 0.75rem;">
            ${escapeHtml(proxy.host_id || 'local')}
          </span>
          ${sourceBadge}
        </div>
      </td>
      <td>
        <div class="container-info">
          <span class="container-title">${escapeHtml(proxy.container_name)}</span>
          <span class="container-sub">${escapeHtml(proxy.image || (proxy.source_file ? 'manifest: ' + proxy.source_file : (proxy.container_id ? proxy.container_id.substring(0, 12) : '')))}</span>
        </div>
      </td>
      <td>${sslBadge}</td>
      <td><span class="badge badge-gray">${escapeHtml(proxy.resolution_method || 'auto')}</span></td>
      <td><span style="font-family: var(--font-mono); color: var(--text-muted);">#${proxy.npm_host_id}</span></td>
      <td><span style="font-size: 0.8rem; color: var(--text-muted);">${lastSyncTime}</span></td>
      <td>
        <button class="btn btn-secondary btn-sm" onclick="showProxyModal('${escapeHtml(proxy.domain_names[0])}')">
          Details
        </button>
      </td>
    `;
    tbody.appendChild(tr);
  });
}

// Fetch All Containers
async function fetchContainers() {
  try {
    const res = await fetch('/api/containers');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    containersData = data.containers || [];
    document.getElementById('tab-containers-count').textContent = containersData.length;
    renderContainersTable();
  } catch (err) {
    console.warn('Failed to fetch containers:', err);
  }
}

// Render All Containers Table
function renderContainersTable(filterText = '') {
  const tbody = document.getElementById('containers-tbody');
  tbody.innerHTML = '';

  const filtered = containersData.filter(c => {
    if (selectedContainersNode !== 'all' && c.node_id && c.node_id.toLowerCase() !== selectedContainersNode.toLowerCase()) {
      return false;
    }
    if (!filterText) return true;
    const search = `${c.name} ${c.image} ${c.id} ${c.node_id || ''}`.toLowerCase();
    return search.includes(filterText);
  });

  filtered.forEach(c => {
    const tr = document.createElement('tr');

    let statusBadge = '';
    if (c.discovered) {
      statusBadge = `<span class="badge badge-emerald">✓ Discovered</span>`;
    } else {
      statusBadge = `<span class="badge badge-gray" title="${escapeHtml(c.ignored_reason || '')}">Ignored</span>`;
    }

    let healthBadge = '';
    if (c.health_status === 'healthy') {
      healthBadge = `<span class="badge badge-emerald" style="font-size:0.7rem; margin-left:4px;" title="HealthCheck: Healthy">💚 healthy</span>`;
    } else if (c.health_status === 'starting') {
      healthBadge = `<span class="badge badge-amber" style="font-size:0.7rem; margin-left:4px;" title="HealthCheck: Warming Up (Held by Zero-502)">⏳ starting</span>`;
    } else if (c.health_status === 'unhealthy') {
      healthBadge = `<span class="badge badge-rose" style="font-size:0.7rem; margin-left:4px;" title="HealthCheck: Unhealthy">❌ unhealthy</span>`;
    }

    const domainText = c.domains && c.domains.length > 0 ? c.domains.join(', ') : `<span class="text-muted">—</span>`;
    const portText = c.port ? c.port : `<span class="text-muted">—</span>`;

    let sourceBadge = `<span class="badge badge-cyan" style="font-size:0.65rem;">Docker</span>`;
    if (c.source === 'pve') {
      sourceBadge = `<span class="badge" style="background: rgba(249, 115, 22, 0.15); color: #f97316; font-size:0.65rem;">Proxmox LXC</span>`;
    } else if (c.source === 'lxd') {
      sourceBadge = `<span class="badge" style="background: rgba(16, 185, 129, 0.15); color: #10b981; font-size:0.65rem;">LXD / Incus</span>`;
    }

    const buttonLabel = c.source === 'pve' ? 'PVE Config' : (c.source === 'lxd' ? 'LXD Config' : 'Compose Labels');

    tr.innerHTML = `
      <td>
        <div class="container-info">
          <div style="display:flex; align-items:center; gap:0.4rem;">
            <span class="container-title">${escapeHtml(c.name)}</span>
            ${sourceBadge}
          </div>
          <span class="container-sub">${escapeHtml(c.id)}</span>
        </div>
      </td>
      <td>
        <span class="badge badge-node">🖥️ ${escapeHtml(c.node_id || 'controller')}</span>
      </td>
      <td><span style="font-family: var(--font-mono); font-size: 0.8rem;">${escapeHtml(c.image)}</span></td>
      <td>
        <span class="badge ${c.state === 'running' ? 'badge-cyan' : 'badge-gray'}">${escapeHtml(c.state)}</span>
        ${healthBadge}
      </td>
      <td>${statusBadge}</td>
      <td>${domainText}</td>
      <td>${portText}</td>
      <td>
        <button class="btn btn-secondary btn-sm" onclick="showGenerateLabelsModal('${escapeHtml(c.id)}')">
          ${buttonLabel}
        </button>
      </td>
    `;
    tbody.appendChild(tr);
  });
}

// Live Event Stream via Server-Sent Events (SSE)
function initEventStream() {
  const term = document.getElementById('terminal-logs');
  const evtSource = new EventSource('/api/events/stream');

  evtSource.addEventListener('log', (e) => {
    try {
      const log = JSON.parse(e.data);
      appendTerminalLine(log);
    } catch (err) {
      console.error('Failed to parse SSE log:', err);
    }
  });

  evtSource.onerror = (err) => {
    console.warn('SSE stream error, retrying in background...', err);
  };
}

function appendTerminalLine(log) {
  const term = document.getElementById('terminal-logs');
  const line = document.createElement('div');
  line.className = 'terminal-line';
  line.setAttribute('data-category', log.category || 'system');
  line.setAttribute('data-level', log.level || 'info');
  line.setAttribute('data-node', log.node_id || 'controller');

  const timeStr = new Date(log.timestamp).toLocaleTimeString();
  const categoryClass = `tag-${log.category || 'system'}`;
  const levelClass = `lvl-${log.level || 'info'}`;
  const nodeBadge = log.node_id ? `<span class="terminal-node-badge">[${escapeHtml(log.node_id)}]</span>` : '';

  line.innerHTML = `
    <span class="terminal-time">[${timeStr}]</span>
    ${nodeBadge}
    <span class="terminal-tag ${categoryClass}">${escapeHtml(log.category || 'sys')}</span>
    <span class="terminal-level ${levelClass}">${escapeHtml(log.level || 'info').toUpperCase()}</span>
    <span class="terminal-msg">${escapeHtml(log.message)}</span>
    ${log.details ? `<span class="terminal-details">(${escapeHtml(log.details)})</span>` : ''}
  `;

  // Apply current filter visibility
  if (!shouldShowLog(log.category, log.level, log.node_id)) {
    line.style.display = 'none';
  }

  term.appendChild(line);

  // Keep terminal buffer manageable
  if (term.childElementCount > 400) {
    term.removeChild(term.firstElementChild);
  }

  if (autoScrollLogs) {
    term.scrollTop = term.scrollHeight;
  }
}

function filterTerminalLogs() {
  const lines = document.querySelectorAll('.terminal-line');
  lines.forEach(line => {
    const cat = line.getAttribute('data-category');
    const lvl = line.getAttribute('data-level');
    const node = line.getAttribute('data-node');
    line.style.display = shouldShowLog(cat, lvl, node) ? 'flex' : 'none';
  });
}

function shouldShowLog(cat, lvl, node) {
  if (selectedEventsNode !== 'all' && node && node.toLowerCase() !== selectedEventsNode.toLowerCase()) {
    return false;
  }
  if (currentLogFilter === 'all') return true;
  if (currentLogFilter === 'error') return lvl === 'error' || lvl === 'warn';
  return cat === currentLogFilter;
}

// Modal inspection
function showProxyModal(domainKey) {
  const proxy = proxiesData.find(p => p.domain_names && p.domain_names.includes(domainKey));
  if (!proxy) return;

  const modal = document.getElementById('detail-modal');
  const title = document.getElementById('modal-title');
  const content = document.getElementById('modal-content');

  title.textContent = `Proxy Host Details: ${proxy.domain_names[0]}`;
  content.innerHTML = `
    <div style="display: flex; flex-direction: column; gap: 1rem;">
      <div>
        <label class="text-muted" style="font-size: 0.75rem; text-transform: uppercase;">NPM Host ID</label>
        <div style="font-size: 1.1rem; font-weight: 700; color: var(--accent-cyan);">#${proxy.npm_host_id}</div>
      </div>

      <div>
        <label class="text-muted" style="font-size: 0.75rem; text-transform: uppercase;">Domains</label>
        <div><code>${escapeHtml(proxy.domain_names.join(', '))}</code></div>
      </div>

      <div>
        <label class="text-muted" style="font-size: 0.75rem; text-transform: uppercase;">Forward Destination</label>
        <div><code>${proxy.forward_scheme}://${proxy.forward_host}:${proxy.forward_port}</code></div>
      </div>

      <div>
        <label class="text-muted" style="font-size: 0.75rem; text-transform: uppercase;">Container</label>
        <div><strong>${escapeHtml(proxy.container_name)}</strong> (ID: <code>${escapeHtml(proxy.container_id.substring(0, 12))}</code>)</div>
      </div>

      <div>
        <label class="text-muted" style="font-size: 0.75rem; text-transform: uppercase;">Host Resolution Strategy</label>
        <div><span class="badge badge-gray">${escapeHtml(proxy.resolution_method)}</span></div>
      </div>

      <div>
        <label class="text-muted" style="font-size: 0.75rem; text-transform: uppercase;">Feature Flags</label>
        <div style="display: flex; gap: 0.5rem; flex-wrap: wrap; margin-top: 0.25rem;">
          <span class="badge ${proxy.websocket ? 'badge-emerald' : 'badge-gray'}">WebSocket: ${proxy.websocket ? 'ON' : 'OFF'}</span>
          <span class="badge ${proxy.block_exploits ? 'badge-emerald' : 'badge-gray'}">Block Exploits: ${proxy.block_exploits ? 'ON' : 'OFF'}</span>
          <span class="badge ${proxy.ssl_enabled ? 'badge-emerald' : 'badge-gray'}">SSL: ${proxy.ssl_enabled ? 'ON' : 'OFF'}</span>
          <span class="badge ${proxy.ssl_forced ? 'badge-emerald' : 'badge-gray'}">Force SSL: ${proxy.ssl_forced ? 'ON' : 'OFF'}</span>
        </div>
      </div>

      ${proxy.locations && proxy.locations.length > 0 ? `
      <div>
        <label class="text-muted" style="font-size: 0.75rem; text-transform: uppercase;">Custom Locations / Subpaths (${proxy.locations.length})</label>
        <div style="display: flex; flex-direction: column; gap: 0.35rem; margin-top: 0.35rem;">
          ${proxy.locations.map(loc => `
            <div style="background: rgba(255,255,255,0.05); padding: 0.4rem 0.6rem; border-radius: 4px; font-size: 0.85rem; display: flex; justify-content: space-between; align-items: center;">
              <div><strong>${escapeHtml(loc.path)}</strong> &rarr; <code>${escapeHtml(loc.forward_host)}:${loc.forward_port}</code></div>
              <div style="font-size: 0.75rem; color: var(--text-muted);">${loc.websocket ? '⚡ websocket' : ''}</div>
            </div>
          `).join('')}
        </div>
      </div>
      ` : ''}

      ${proxy.upstream_replicas && proxy.upstream_replicas > 1 ? `
      <div>
        <label class="text-muted" style="font-size: 0.75rem; text-transform: uppercase;">Load Balanced Upstream</label>
        <div style="margin-top: 0.25rem;">
          <span class="badge badge-emerald">⚖️ ${proxy.upstream_replicas} Replicas</span>
          <span class="badge badge-cyan">${escapeHtml(proxy.upstream_balancing || 'round_robin')}</span>
          <code style="margin-left: 0.5rem;">upstream ${escapeHtml(proxy.upstream_name)}</code>
        </div>
        ${proxy.container_names && proxy.container_names.length > 0 ? `
          <div style="font-size: 0.8rem; color: var(--text-muted); margin-top: 0.35rem;">
            Containers: ${escapeHtml(proxy.container_names.join(', '))}
          </div>
        ` : ''}
      </div>
      ` : ''}

      <div class="code-snippet-wrap mt-3">
        <pre><code class="language-json">${escapeHtml(JSON.stringify(proxy, null, 2))}</code></pre>
      </div>
    </div>
  `;

  modal.style.display = 'flex';
}

function showGenerateLabelsModal(containerID) {
  const container = containersData.find(c => c.id === containerID);
  if (!container) return;

  const modal = document.getElementById('detail-modal');
  const title = document.getElementById('modal-title');
  const content = document.getElementById('modal-content');

  const domain = `${container.name}.local`;
  const port = container.port || 80;

  if (container.source === 'pve') {
    title.textContent = `Proxmox VE Auto-Discovery for "${container.name}"`;
    const snippetTags = `npm.domain=${domain}, npm.port=${port}, npm.ssl.enabled=false`;
    const snippetNotes = `npm.domain: ${domain}\nnpm.port: ${port}\nnpm.ssl.enabled: false\nnpm.websocket: true`;
    content.innerHTML = `
      <p class="text-muted mb-3">Add tags or notes to your Proxmox LXC container (VMID <code>${escapeHtml(container.id)}</code>) to enable automated discovery:</p>
      
      <label class="text-muted" style="font-size: 0.75rem; text-transform: uppercase; font-weight:600;">Channel A: Container Tags</label>
      <div class="code-snippet-wrap mb-3">
        <pre><code>${escapeHtml(snippetTags)}</code></pre>
      </div>

      <label class="text-muted" style="font-size: 0.75rem; text-transform: uppercase; font-weight:600;">Channel B: Container Notes (YAML/Key-Value)</label>
      <div class="code-snippet-wrap">
        <pre><code>${escapeHtml(snippetNotes)}</code></pre>
      </div>

      <button class="btn btn-primary mt-4" id="btn-copy-pve-snippet">
        Copy Notes Snippet
      </button>
    `;

    modal.style.display = 'flex';
    document.getElementById('btn-copy-pve-snippet').onclick = () => {
      copyRawSnippet(snippetNotes, 'Proxmox notes snippet copied to clipboard!');
    };
  } else if (container.source === 'lxd') {
    title.textContent = `Canonical LXD / Incus Config for "${container.name}"`;
    const snippetLXD = `lxc config set ${container.name} user.npm.domain="${domain}"
lxc config set ${container.name} user.npm.port="${port}"
lxc config set ${container.name} user.npm.ssl.enabled="false"`;
    content.innerHTML = `
      <p class="text-muted mb-3">Run these CLI commands on your host to attach metadata to your LXD / Incus container:</p>
      
      <div class="code-snippet-wrap">
        <pre><code>${escapeHtml(snippetLXD)}</code></pre>
      </div>

      <button class="btn btn-primary mt-4" id="btn-copy-lxd-snippet">
        Copy CLI Commands
      </button>
    `;

    modal.style.display = 'flex';
    document.getElementById('btn-copy-lxd-snippet').onclick = () => {
      copyRawSnippet(snippetLXD, 'LXD CLI commands copied to clipboard!');
    };
  } else {
    title.textContent = `Compose Labels for "${container.name}"`;
    const snippet = `    labels:
      - "npm.frontend.domain=${domain}"
      - "npm.frontend.port=${port}"
      - "npm.ssl.enabled=false"
      - "npm.websocket=true"
      - "npm.block_exploits=true"`;
    content.innerHTML = `
      <p class="text-muted mb-3">Copy and paste these labels into your <code>docker-compose.yml</code> to enable automated discovery:</p>
      
      <div class="code-snippet-wrap">
        <pre><code>services:
  ${escapeHtml(container.name)}:
    # ...
    labels:
      - "npm.frontend.domain=${domain}"
      - "npm.frontend.port=${port}"
      - "npm.ssl.enabled=false"
      - "npm.websocket=true"
      - "npm.block_exploits=true"</code></pre>
      </div>

      <button class="btn btn-primary mt-4" onclick="copySnippet('${domain}', ${port}, '${escapeHtml(container.name)}')">
        Copy YAML Snippet
      </button>
    `;

    modal.style.display = 'flex';
  }
}

function copyRawSnippet(snippet, successMsg = 'Snippet copied to clipboard!') {
  navigator.clipboard.writeText(snippet);
  showToast(successMsg, 'success');
  closeModal();
}

function copySnippet(domain, port, serviceName) {
  const snippet = `    labels:
      - "npm.frontend.domain=${domain}"
      - "npm.frontend.port=${port}"
      - "npm.ssl.enabled=false"
      - "npm.websocket=true"
      - "npm.block_exploits=true"`;

  navigator.clipboard.writeText(snippet);
  showToast('YAML labels snippet copied to clipboard!', 'success');
  closeModal();
}

function closeModal() {
  document.getElementById('detail-modal').style.display = 'none';
}

// Toast Notifications
function showToast(message, type = 'info') {
  const container = document.getElementById('toast-container');
  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  toast.textContent = message;

  container.appendChild(toast);
  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateX(100%)';
    toast.style.transition = 'all 0.25s ease';
    setTimeout(() => toast.remove(), 250);
  }, 4000);
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

// Docs & Setup Guides Subtab Switcher
function initDocsSubtabs() {
  document.querySelectorAll('.docs-subtab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const guideId = btn.getAttribute('data-guide');
      document.querySelectorAll('.docs-subtab-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');

      document.querySelectorAll('.docs-guide-pane').forEach(pane => {
        pane.classList.toggle('active', pane.id === guideId);
      });
    });
  });
}

// Docs Reference Table Real-Time Filter
function initDocsSearch() {
  const input = document.getElementById('docs-search');
  if (!input) return;

  input.addEventListener('input', (e) => {
    const query = e.target.value.toLowerCase().trim();
    const rows = document.querySelectorAll('#docs-reference-table tbody tr');

    rows.forEach(row => {
      if (row.classList.contains('docs-category-header')) {
        row.style.display = query === '' ? '' : 'table-row';
        return;
      }

      const text = row.textContent.toLowerCase();
      if (!query || text.includes(query)) {
        row.style.display = '';
      } else {
        row.style.display = 'none';
      }
    });
  });
}

// Copy Code from Guide Snippet
function copyDocsGuideCode(paneId, toastMsg) {
  const pane = document.getElementById(paneId);
  if (!pane) return;
  const codeEl = pane.querySelector('code');
  if (!codeEl) return;

  navigator.clipboard.writeText(codeEl.textContent.trim());
  showToast(toastMsg || 'Configuration snippet copied to clipboard!', 'success');
}

// Fetch Cluster Nodes (Multi-Node Control Plane)
async function fetchClusterNodes() {
  try {
    const res = await fetch('/api/cluster/nodes');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    clusterData = data.nodes || [];

    const tabCount = document.getElementById('tab-cluster-count');
    if (tabCount) {
      tabCount.textContent = data.count || clusterData.length;
    }

    updateNodeFilterDropdowns();
    renderClusterNodes();
    checkPendingNodeConnection();
  } catch (err) {
    console.warn('Failed to fetch cluster nodes:', err);
  }
}

function updateNodeFilterDropdowns() {
  const nodes = [...new Set(clusterData.map(n => n.node_id))];
  const dropdowns = [
    { el: document.getElementById('containers-node-filter'), current: selectedContainersNode },
    { el: document.getElementById('proxies-node-filter'), current: selectedProxiesNode },
    { el: document.getElementById('streams-node-filter'), current: selectedStreamsNode },
    { el: document.getElementById('events-node-filter'), current: selectedEventsNode }
  ];

  dropdowns.forEach(({ el, current }) => {
    if (!el) return;
    const existingVal = el.value || current || 'all';
    el.innerHTML = '<option value="all">🌐 All Nodes</option>';
    nodes.forEach(nodeId => {
      const opt = document.createElement('option');
      opt.value = nodeId;
      opt.textContent = `🖥️ ${nodeId}`;
      if (nodeId === existingVal) opt.selected = true;
      el.appendChild(opt);
    });
  });
}

function renderClusterNodes(filterText = '') {
  const grid = document.getElementById('cluster-nodes-grid');
  const emptyState = document.getElementById('cluster-empty');
  if (!grid) return;

  grid.innerHTML = '';

  const filtered = clusterData.filter(n => {
    if (!filterText) return true;
    const search = `${n.node_id} ${n.node_ip} ${n.docker_version || ''} ${n.status}`.toLowerCase();
    return search.includes(filterText);
  });

  if (filtered.length === 0) {
    if (emptyState) emptyState.style.display = 'block';
    return;
  }
  if (emptyState) emptyState.style.display = 'none';

  filtered.forEach(node => {
    try {
      const card = document.createElement('div');
      card.className = 'node-card glass-panel';
      const nodeId = node.node_id || (node.is_controller ? 'controller-main' : 'worker');
      const nodeIp = node.node_ip || '127.0.0.1';
      card.setAttribute('data-node-id', nodeId);

      const isOnline = node.status === 'online';
      const statusDot = isOnline 
        ? '<span class="status-dot ping-dot connected"></span>' 
        : '<span class="status-dot ping-dot disconnected"></span>';
      const statusText = isOnline 
        ? '<span class="badge badge-emerald">Online</span>' 
        : '<span class="badge badge-rose">Offline</span>';

      const roleBadge = node.is_controller 
        ? '<span class="badge badge-cyan" style="font-size:0.72rem;">👑 Central Controller</span>' 
        : '<span class="badge badge-violet" style="font-size:0.72rem;">📡 Remote Worker</span>';

      let lastHeartbeatTime = 'Just now';
      if (node.last_heartbeat) {
        lastHeartbeatTime = formatTimeAgo(new Date(node.last_heartbeat));
      }

      const uptimeStr = formatUptime(node.uptime_seconds || 0);

      card.innerHTML = `
        <div class="node-card-header">
          <div class="node-card-title-group">
            ${statusDot}
            <span class="node-card-title">${escapeHtml(nodeId)}</span>
          </div>
          <div style="display:flex; align-items:center; gap:0.5rem;">
            ${roleBadge}
            ${statusText}
          </div>
        </div>

        <div class="node-meta-grid">
          <div class="node-meta-item">
            <span class="node-meta-label">Reachable IP / Host</span>
            <span class="node-meta-val"><code>${escapeHtml(nodeIp)}</code></span>
          </div>
          <div class="node-meta-item">
            <span class="node-meta-label">Last Heartbeat</span>
            <span class="node-meta-val">${escapeHtml(lastHeartbeatTime)}</span>
          </div>
          <div class="node-meta-item">
            <span class="node-meta-label">Discovered Containers</span>
            <span class="node-meta-val" style="color:var(--accent-cyan); font-weight:700;">${node.container_count || 0}</span>
          </div>
          <div class="node-meta-item">
            <span class="node-meta-label">Provisioned Proxies</span>
            <span class="node-meta-val" style="color:var(--accent-emerald); font-weight:700;">${node.proxy_count || 0}</span>
          </div>
        </div>

        <div class="node-engines-list">
          <div class="node-engine-row">
            <span class="text-muted">🐳 Docker Engine:</span>
            <span>${node.docker_connected ? `<span style="color:#10b981; font-weight:600;">Connected</span> <span style="font-size:0.75rem; color:var(--text-muted); font-family:var(--font-mono);">(${escapeHtml(node.docker_version || 'active')})</span>` : '<span style="color:#f43f5e;">Disconnected</span>'}</span>
          </div>
          <div class="node-engine-row">
            <span class="text-muted">⚡ Proxmox VE:</span>
            <span>${node.pve_connected ? '<span style="color:#10b981; font-weight:600;">Active</span>' : '<span style="color:var(--text-muted);">Disabled</span>'}</span>
          </div>
          <div class="node-engine-row">
            <span class="text-muted">🐧 LXD / Incus:</span>
            <span>${node.lxd_connected ? '<span style="color:#10b981; font-weight:600;">Active</span>' : '<span style="color:var(--text-muted);">Disabled</span>'}</span>
          </div>
        </div>

        <div class="node-card-footer">
          <span style="font-size:0.75rem; color:var(--text-muted);">Uptime: ${escapeHtml(uptimeStr)}</span>
          <button class="btn btn-secondary btn-sm" onclick="filterContainersByNode('${escapeHtml(nodeId)}')">
            Inspect Containers
          </button>
        </div>
      `;

      grid.appendChild(card);
    } catch (err) {
      console.error('Failed to render cluster node card:', err, node);
    }
  });
}

function filterContainersByNode(nodeId) {
  selectedContainersNode = nodeId;
  const select = document.getElementById('containers-node-filter');
  if (select) select.value = nodeId;
  switchTab('containers-view');
  renderContainersTable();
}

function formatTimeAgo(date) {
  if (!date || isNaN(date.getTime())) return 'Just now';
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
  if (seconds < 5) return 'Just now';
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function formatUptime(seconds) {
  seconds = Math.max(0, Math.floor(seconds || 0));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ${minutes % 60}m`;
  const days = Math.floor(hours / 24);
  return `${days}d ${hours % 24}h`;
}

// ==============================================================================
// Remote Worker Node Provisioning & Configuration Generator
// ==============================================================================
let clusterSetupInfo = null;
let activeConfigTab = 'run';
let pendingProvisionNode = null;
let generatedConfigs = {};

function initAddNodeGenerator() {
  const btnOpen = document.getElementById('btn-open-add-node');
  if (btnOpen) btnOpen.addEventListener('click', openAddNodeModal);

  const btnEmpty = document.getElementById('btn-empty-add-node');
  if (btnEmpty) btnEmpty.addEventListener('click', openAddNodeModal);

  const btnRandom = document.getElementById('btn-random-node-id');
  if (btnRandom) {
    btnRandom.addEventListener('click', () => {
      const idInput = document.getElementById('node-agent-id');
      if (idInput) {
        idInput.value = generateRandomNodeName();
        if (generatedConfigs.run) generateAgentConfig(false);
      }
    });
  }

  const btnToken = document.getElementById('btn-generate-cluster-token');
  if (btnToken) {
    btnToken.addEventListener('click', () => {
      const tokenInput = document.getElementById('node-cluster-token');
      if (tokenInput) {
        tokenInput.value = generateSecureClusterToken();
        if (generatedConfigs.run) generateAgentConfig(false);
        showToast('Generated fresh 32-byte cluster secret token', 'info');
      }
    });
  }

  const btnGenerate = document.getElementById('btn-generate-agent-config');
  if (btnGenerate) {
    btnGenerate.addEventListener('click', () => generateAgentConfig(true));
  }

  const addrInput = document.getElementById('node-agent-address');
  if (addrInput) {
    addrInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        generateAgentConfig(true);
      }
    });
    addrInput.addEventListener('input', () => {
      if (generatedConfigs.run) generateAgentConfig(false);
    });
  }

  // Config tab switcher
  document.querySelectorAll('.config-tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const tabKey = btn.getAttribute('data-cfg-tab');
      if (tabKey) switchConfigTab(tabKey);
    });
  });

  const btnCopy = document.getElementById('btn-copy-agent-config');
  if (btnCopy) btnCopy.addEventListener('click', copyCurrentAgentConfig);

  const btnDownload = document.getElementById('btn-download-agent-config');
  if (btnDownload) btnDownload.addEventListener('click', downloadCurrentAgentConfig);

  const addNodeModal = document.getElementById('add-node-modal');
  if (addNodeModal) {
    addNodeModal.addEventListener('click', (e) => {
      if (e.target === addNodeModal) closeAddNodeModal();
    });
  }

  window.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      closeModal();
      closeAddNodeModal();
    }
  });
}

async function openAddNodeModal() {
  const modal = document.getElementById('add-node-modal');
  if (!modal) return;
  modal.style.display = 'flex';

  const idInput = document.getElementById('node-agent-id');
  if (idInput && !idInput.value) {
    idInput.value = generateRandomNodeName();
  }

  const addrInput = document.getElementById('node-agent-address');
  if (addrInput) {
    setTimeout(() => addrInput.focus(), 50);
  }

  try {
    const res = await fetch('/api/cluster/setup-info');
    if (res.ok) {
      clusterSetupInfo = await res.json();
      
      const mainUrlInput = document.getElementById('node-main-url');
      if (mainUrlInput && !mainUrlInput.value) {
        mainUrlInput.value = clusterSetupInfo.main_node_url || window.location.origin;
      }
      
      const npmUrlInput = document.getElementById('node-npm-url');
      if (npmUrlInput && !npmUrlInput.value) {
        npmUrlInput.value = clusterSetupInfo.npm_url || 'http://127.0.0.1:81';
      }

      const tokenInput = document.getElementById('node-cluster-token');
      if (tokenInput && !tokenInput.value) {
        tokenInput.value = clusterSetupInfo.cluster_token || generateSecureClusterToken();
      }
    }
  } catch (err) {
    console.warn('Could not fetch cluster setup info, using fallback defaults:', err);
    const mainUrlInput = document.getElementById('node-main-url');
    if (mainUrlInput && !mainUrlInput.value) {
      mainUrlInput.value = window.location.origin;
    }
    const tokenInput = document.getElementById('node-cluster-token');
    if (tokenInput && !tokenInput.value) {
      tokenInput.value = generateSecureClusterToken();
    }
  }

  if (addrInput && addrInput.value.trim()) {
    generateAgentConfig(false);
  }
}

function closeAddNodeModal() {
  const modal = document.getElementById('add-node-modal');
  if (modal) modal.style.display = 'none';
}

function generateRandomNodeName() {
  const prefixes = ['worker', 'edge', 'node', 'lab', 'host', 'compute'];
  const suffixes = ['alpha', 'beta', 'delta', 'east', 'west', '01', '02', '03', 'prime', 'hub'];
  const p = prefixes[Math.floor(Math.random() * prefixes.length)];
  const s = suffixes[Math.floor(Math.random() * suffixes.length)];
  return `${p}-${s}`;
}

function generateSecureClusterToken() {
  const arr = new Uint8Array(32);
  window.crypto.getRandomValues(arr);
  return Array.from(arr, b => b.toString(16).padStart(2, '0')).join('');
}

function generateAgentConfig(notify = true) {
  const addrInput = document.getElementById('node-agent-address');
  let agentAddress = (addrInput?.value || '').trim();

  // Strip protocol or trailing slashes if user pasted a URL
  agentAddress = agentAddress.replace(/^https?:\/\//i, '').replace(/\/+$/, '');

  if (!agentAddress) {
    if (notify) {
      showToast('Please enter the Agent Address (Host IP or Domain)', 'error');
      if (addrInput) addrInput.focus();
    }
    return;
  }

  const idInput = document.getElementById('node-agent-id');
  let nodeId = (idInput?.value || '').trim();
  if (!nodeId) {
    nodeId = generateRandomNodeName();
    if (idInput) idInput.value = nodeId;
  }

  const mainNodeUrl = (document.getElementById('node-main-url')?.value || window.location.origin).trim().replace(/\/+$/, '');
  const npmUrl = (document.getElementById('node-npm-url')?.value || 'http://127.0.0.1:81').trim().replace(/\/+$/, '');
  const clusterToken = (document.getElementById('node-cluster-token')?.value || '').trim();
  const npmPass = (document.getElementById('node-npm-pass')?.value || 'changeme').trim();
  const headlessMode = document.getElementById('node-headless-mode')?.checked ?? true;
  const useHostPort = document.getElementById('node-use-host-port')?.checked ?? true;
  const autoSSL = document.getElementById('node-auto-ssl')?.checked ?? true;
  const npmUser = (clusterSetupInfo?.npm_user || 'admin@example.com');

  pendingProvisionNode = {
    id: nodeId,
    address: agentAddress
  };

  const targetIpSpan = document.getElementById('watcher-target-ip');
  if (targetIpSpan) targetIpSpan.textContent = agentAddress;

  const watcherCard = document.getElementById('node-live-watcher');
  const watcherDot = document.getElementById('watcher-status-dot');
  const watcherTitle = document.getElementById('watcher-title');
  const watcherSubtitle = document.getElementById('watcher-subtitle');
  const viewBtn = document.getElementById('btn-view-connected-node');
  if (watcherCard) watcherCard.classList.remove('connected');
  if (watcherDot) watcherDot.className = 'status-dot ping-dot waiting';
  if (watcherTitle) watcherTitle.textContent = 'Waiting for Remote Agent Heartbeat...';
  if (watcherSubtitle) {
    watcherSubtitle.innerHTML = `Run the command on your worker node (<code>${escapeHtml(agentAddress)}</code>). Heartbeat will register automatically.`;
  }
  if (viewBtn) viewBtn.style.display = 'none';

  // 1. Docker Run Command
  const dockerRunCmd = `docker run -d \\
  --name npm-autodiscovery-worker \\
  --restart unless-stopped \\
  -v /var/run/docker.sock:/var/run/docker.sock:ro \\
  -e NPM_URL="${npmUrl}" \\
  -e NPM_USER="${npmUser}" \\
  -e NPM_PASS="${npmPass}" \\
  -e HOST_ID="${nodeId}" \\
  -e HOST_IP="${agentAddress}" \\
  -e USE_HOST_PORT="${useHostPort}" \\
  -e AUTO_DETECT_SSL="${autoSSL}" \\
  -e DASHBOARD_ENABLED="${headlessMode ? 'false' : 'true'}" \\
  -e MAIN_NODE_URL="${mainNodeUrl}" \\
  -e CLUSTER_TOKEN="${clusterToken}" \\
  -e PUSH_INTERVAL="15s" \\
  raddadengineer/npm-autodiscovery:latest`;

  // 2. Docker Compose
  const dockerComposeYaml = `services:
  # ============================================================================
  # NPM Auto-Discovery (Remote Worker Agent)
  # Host ID: ${nodeId} (${agentAddress})
  # ============================================================================
  npm-autodiscovery:
    image: raddadengineer/npm-autodiscovery:latest
    container_name: npm-autodiscovery-worker
    restart: unless-stopped
    env_file:
      - .env
    volumes:
      # Read-only Docker socket mapping allows listening to local container events
      - /var/run/docker.sock:/var/run/docker.sock:ro`;

  // 3. .env file
  const envContent = `# ==============================================================================
# NPM Auto-Discovery — Remote Worker Node Configuration (.env)
# Node ID: ${nodeId} | Address: ${agentAddress}
# ==============================================================================

# Central Nginx Proxy Manager Official API Settings
NPM_URL=${npmUrl}
NPM_USER=${npmUser}
NPM_PASS=${npmPass}
NPM_TIMEOUT=15s

# Docker Engine Socket on this worker node
DOCKER_SOCKET=/var/run/docker.sock
DOCKER_TIMEOUT=10s

# Multi-Host / Remote Worker Node Configuration
HOST_ID=${nodeId}
HOST_IP=${agentAddress}
USE_HOST_PORT=${useHostPort}
AUTO_DETECT_SSL=${autoSSL}

# Container Label Prefix
LABEL_PREFIX=npm.

# Headless Worker Mode (0 open listening ports on worker machine)
DASHBOARD_ENABLED=${headlessMode ? 'false' : 'true'}
PORT=8080
LOG_LEVEL=info

# Central Cluster Controller Telemetry Push
MAIN_NODE_URL=${mainNodeUrl}
CLUSTER_TOKEN=${clusterToken}
PUSH_INTERVAL=15s
`;

  // 4. Test Service (whoami)
  const whoamiYaml = `services:
  # ============================================================================
  # Test Target Service on Worker Node (${nodeId})
  # Port 8081 will be routed through central NPM to ${agentAddress}:8081
  # ============================================================================
  worker-whoami:
    image: raddadengineer/whoami:latest
    container_name: worker-whoami
    restart: unless-stopped
    ports:
      - "8081:80"
    labels:
      - "npm.frontend.domain=${nodeId}.local"
      - "npm.frontend.port=80"
      - "npm.forward_scheme=http"
      - "npm.websocket=true"
      - "npm.block_exploits=true"
      - "npm.ssl.enabled=false"`;

  generatedConfigs = {
    run: {
      content: dockerRunCmd,
      filename: 'docker run command',
      downloadName: 'install-worker.sh'
    },
    compose: {
      content: dockerComposeYaml,
      filename: 'docker-compose.worker.yml',
      downloadName: 'docker-compose.worker.yml'
    },
    env: {
      content: envContent,
      filename: '.env',
      downloadName: '.env'
    },
    whoami: {
      content: whoamiYaml,
      filename: 'docker-compose.whoami.yml',
      downloadName: 'docker-compose.whoami.yml'
    }
  };

  const outputArea = document.getElementById('agent-config-output');
  if (outputArea) {
    outputArea.style.display = 'block';
  }

  renderConfigTab();
  checkPendingNodeConnection();

  if (notify) {
    showToast('Agent configuration generated!', 'success');
  }
}

function switchConfigTab(tabKey) {
  activeConfigTab = tabKey;
  document.querySelectorAll('.config-tab-btn').forEach(btn => {
    btn.classList.toggle('active', btn.getAttribute('data-cfg-tab') === tabKey);
  });
  renderConfigTab();
}

function renderConfigTab() {
  const item = generatedConfigs[activeConfigTab];
  if (!item) return;

  const fnEl = document.getElementById('config-code-filename');
  if (fnEl) fnEl.textContent = item.filename;

  const codeEl = document.getElementById('config-code-content');
  if (codeEl) codeEl.textContent = item.content;
}

function copyCurrentAgentConfig() {
  const item = generatedConfigs[activeConfigTab];
  if (!item) return;

  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(item.content).then(() => {
      const copyBtnText = document.getElementById('btn-copy-agent-config-text');
      if (copyBtnText) {
        copyBtnText.textContent = 'Copied!';
        setTimeout(() => { copyBtnText.textContent = 'Copy'; }, 2000);
      }
      showToast(`${item.filename} copied to clipboard!`, 'success');
    }).catch(() => {
      fallbackCopy(item.content, `${item.filename} copied to clipboard!`);
    });
  } else {
    fallbackCopy(item.content, `${item.filename} copied to clipboard!`);
  }
}

function downloadCurrentAgentConfig() {
  const item = generatedConfigs[activeConfigTab];
  if (!item) return;
  downloadTextFile(item.downloadName, item.content);
  showToast(`Downloaded ${item.downloadName}`, 'info');
}

function downloadTextFile(filename, text) {
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function fallbackCopy(text, msg = 'Copied to clipboard!') {
  const textarea = document.createElement('textarea');
  textarea.value = text;
  document.body.appendChild(textarea);
  textarea.select();
  document.execCommand('copy');
  document.body.removeChild(textarea);
  showToast(msg, 'success');
}

function checkPendingNodeConnection() {
  if (!pendingProvisionNode || !clusterData.length) return;
  const match = clusterData.find(n => 
    (pendingProvisionNode.id && n.node_id && n.node_id.toLowerCase() === pendingProvisionNode.id.toLowerCase()) ||
    (pendingProvisionNode.address && n.node_ip === pendingProvisionNode.address)
  );

  const watcherCard = document.getElementById('node-live-watcher');
  const watcherDot = document.getElementById('watcher-status-dot');
  const watcherTitle = document.getElementById('watcher-title');
  const watcherSubtitle = document.getElementById('watcher-subtitle');
  const viewBtn = document.getElementById('btn-view-connected-node');

  if (match && match.status === 'online') {
    if (watcherCard) watcherCard.classList.add('connected');
    if (watcherDot) {
      watcherDot.className = 'status-dot ping-dot connected';
    }
    if (watcherTitle) {
      watcherTitle.innerHTML = `<span style="color:var(--accent-emerald);">🎉 Node Connected & Online!</span>`;
    }
    if (watcherSubtitle) {
      watcherSubtitle.textContent = `Worker '${match.node_id}' (${match.node_ip}) successfully registered! Discovered containers: ${match.container_count || 0}.`;
    }
    if (viewBtn) {
      viewBtn.style.display = 'inline-flex';
      viewBtn.onclick = () => viewConnectedNode(match.node_id);
    }
  }
}

function viewConnectedNode(nodeId) {
  closeAddNodeModal();
  switchTab('cluster-view');
  setTimeout(() => {
    const card = document.querySelector(`.node-card[data-node-id="${nodeId}"]`);
    if (card) {
      card.scrollIntoView({ behavior: 'smooth', block: 'center' });
      card.classList.add('node-card-highlight');
      setTimeout(() => card.classList.remove('node-card-highlight'), 3500);
    }
  }, 100);
}


