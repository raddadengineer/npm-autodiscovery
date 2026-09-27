// NPM Auto-Discovery Dashboard Single Page Application
let activeTab = 'proxies-view';
let proxiesData = [];
let containersData = [];
let currentLogFilter = 'all';
let autoScrollLogs = true;

document.addEventListener('DOMContentLoaded', () => {
  initTabs();
  initSearchAndFilters();
  initSyncButton();
  initEventStream();
  loadInitialData();

  // Periodic polling for status and tables every 5 seconds as fallback
  setInterval(fetchStatus, 5000);
  setInterval(fetchProxies, 6000);
  setInterval(fetchContainers, 10000);
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

  const containersSearch = document.getElementById('containers-search');
  if (containersSearch) {
    containersSearch.addEventListener('input', (e) => {
      renderContainersTable(e.target.value.toLowerCase());
    });
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
  await fetchContainers();
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

    // Metric Cards
    document.getElementById('active-proxies-count').textContent = status.active_proxies;
    document.getElementById('tab-proxies-count').textContent = status.active_proxies;
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

// Render Proxies Table
function renderProxiesTable(filterText = '') {
  const tbody = document.getElementById('proxies-tbody');
  const emptyState = document.getElementById('proxies-empty');
  tbody.innerHTML = '';

  const filtered = proxiesData.filter(p => {
    if (!filterText) return true;
    const searchStr = `${p.domain_names.join(' ')} ${p.container_name} ${p.forward_host}:${p.forward_port}`.toLowerCase();
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
    }

    tr.innerHTML = `
      <td><span class="badge badge-emerald">● Live</span></td>
      <td><div class="domain-chip-group">${domainChips}</div></td>
      <td>
        <span class="target-badge">
          <span class="scheme-tag">${escapeHtml(proxy.forward_scheme)}://</span>
          ${escapeHtml(proxy.forward_host)}:${proxy.forward_port}
        </span>
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
          <span class="container-sub">${escapeHtml(proxy.image || (proxy.source_file ? 'manifest: ' + proxy.source_file : proxy.container_id.substring(0, 12)))}</span>
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
    if (!filterText) return true;
    const search = `${c.name} ${c.image} ${c.id}`.toLowerCase();
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

    const domainText = c.domains && c.domains.length > 0 ? c.domains.join(', ') : `<span class="text-muted">—</span>`;
    const portText = c.port ? c.port : `<span class="text-muted">—</span>`;

    tr.innerHTML = `
      <td>
        <div class="container-info">
          <span class="container-title">${escapeHtml(c.name)}</span>
          <span class="container-sub">${escapeHtml(c.id)}</span>
        </div>
      </td>
      <td><span style="font-family: var(--font-mono); font-size: 0.8rem;">${escapeHtml(c.image)}</span></td>
      <td><span class="badge ${c.state === 'running' ? 'badge-cyan' : 'badge-gray'}">${escapeHtml(c.state)}</span></td>
      <td>${statusBadge}</td>
      <td>${domainText}</td>
      <td>${portText}</td>
      <td>
        <button class="btn btn-secondary btn-sm" onclick="showGenerateLabelsModal('${escapeHtml(c.id)}')">
          Compose Labels
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

  const timeStr = new Date(log.timestamp).toLocaleTimeString();
  const categoryClass = `tag-${log.category || 'system'}`;
  const levelClass = `lvl-${log.level || 'info'}`;

  line.innerHTML = `
    <span class="terminal-time">[${timeStr}]</span>
    <span class="terminal-tag ${categoryClass}">${escapeHtml(log.category || 'sys')}</span>
    <span class="terminal-level ${levelClass}">${escapeHtml(log.level || 'info').toUpperCase()}</span>
    <span class="terminal-msg">${escapeHtml(log.message)}</span>
    ${log.details ? `<span class="terminal-details">(${escapeHtml(log.details)})</span>` : ''}
  `;

  // Apply current filter visibility
  if (!shouldShowLog(log.category, log.level)) {
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
    line.style.display = shouldShowLog(cat, lvl) ? 'flex' : 'none';
  });
}

function shouldShowLog(cat, lvl) {
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

  title.textContent = `Compose Labels for "${container.name}"`;
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
