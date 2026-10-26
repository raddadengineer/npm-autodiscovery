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

// Client-Side Route Map
const TAB_ROUTES = {
  'proxies-view': '/proxies',
  'streams-view': '/streams',
  'metrics-view': '/analytics',
  'events-view': '/events',
  'containers-view': '/containers',
  'cluster-view': '/cluster',
  'docs-view': '/docs'
};

const ROUTE_TABS = {
  '/': 'proxies-view',
  '/proxies': 'proxies-view',
  '/streams': 'streams-view',
  '/analytics': 'metrics-view',
  '/metrics-tab': 'metrics-view',
  '/events': 'events-view',
  '/logs': 'events-view',
  '/audit': 'events-view',
  '/containers': 'containers-view',
  '/cluster': 'cluster-view',
  '/docs': 'docs-view'
};

const TAB_TITLES = {
  'proxies-view': 'Active Proxies | NPM Auto-Discovery',
  'streams-view': 'L4 Streams | NPM Auto-Discovery',
  'metrics-view': 'Metrics & Analytics | NPM Auto-Discovery',
  'events-view': 'Audit Log | NPM Auto-Discovery',
  'containers-view': 'All Containers | NPM Auto-Discovery',
  'cluster-view': 'Cluster Nodes | NPM Auto-Discovery',
  'docs-view': 'Documentation & Setup | NPM Auto-Discovery'
};

function getTabFromUrl() {
  // Check hash first (e.g. #/containers or #containers)
  const hash = window.location.hash.replace(/^#\/?/, '').trim();
  if (hash) {
    const hashClean = '/' + hash.replace(/\/+$/, '');
    if (hashClean === '/metrics') return { tab: 'metrics-view', subtab: null };
    if (ROUTE_TABS[hashClean]) return { tab: ROUTE_TABS[hashClean], subtab: null };
    if (hashClean.startsWith('/docs')) {
      const parts = hashClean.split('/');
      return { tab: 'docs-view', subtab: parts[2] ? `guide-${parts[2]}` : null };
    }
  }

  // Check pathname
  const path = window.location.pathname.replace(/\/+$/, '') || '/';
  if (ROUTE_TABS[path]) return { tab: ROUTE_TABS[path], subtab: null };
  if (path.startsWith('/docs')) {
    const parts = path.split('/');
    return { tab: 'docs-view', subtab: parts[2] ? `guide-${parts[2]}` : null };
  }

  return { tab: 'proxies-view', subtab: null };
}

function initRouter() {
  const { tab, subtab } = getTabFromUrl();
  switchTab(tab, false);

  if (tab === 'docs-view' && subtab) {
    activateDocsSubtab(subtab);
  }

  window.addEventListener('popstate', () => {
    const routeInfo = getTabFromUrl();
    switchTab(routeInfo.tab, false);
    if (routeInfo.tab === 'docs-view' && routeInfo.subtab) {
      activateDocsSubtab(routeInfo.subtab);
    }
  });

  window.addEventListener('hashchange', () => {
    const routeInfo = getTabFromUrl();
    switchTab(routeInfo.tab, false);
    if (routeInfo.tab === 'docs-view' && routeInfo.subtab) {
      activateDocsSubtab(routeInfo.subtab);
    }
  });
}

function activateDocsSubtab(guideId) {
  const btn = document.querySelector(`.docs-subtab-btn[data-guide="${guideId}"]`);
  if (btn) {
    document.querySelectorAll('.docs-subtab-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    document.querySelectorAll('.docs-guide-pane').forEach(pane => {
      pane.classList.toggle('active', pane.id === guideId);
    });
  }
}

document.addEventListener('DOMContentLoaded', () => {
  initTabs();
  initRouter();
  initTableHeaders();
  initSearchAndFilters();
  initSyncButton();
  initEventStream();
  initDocsSubtabs();
  initDocsSearch();
  initAddNodeGenerator();
  initProxmoxModal();
  initNPMMetricsTab();
  loadInitialData();

  // Periodic polling for status and tables every 5 seconds as fallback
  setInterval(fetchStatus, 5000);
  setInterval(fetchProxies, 6000);
  setInterval(fetchStreams, 6000);
  setInterval(fetchContainers, 10000);
  setInterval(fetchClusterNodes, 8000);
  setInterval(() => {
    fetchNPMMetrics(false);
  }, 10000);
});

// Tab navigation handler
function initTabs() {
  document.querySelectorAll('.tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const targetTab = btn.getAttribute('data-tab');
      switchTab(targetTab, true);
    });
  });

  // Make top overview cards clickable to switch into their respective areas
  document.querySelectorAll('.metric-card[data-target-tab]').forEach(card => {
    card.addEventListener('click', () => {
      const targetTab = card.getAttribute('data-target-tab');
      if (targetTab) {
        switchTab(targetTab, true);
      }
    });
    card.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        const targetTab = card.getAttribute('data-target-tab');
        if (targetTab) {
          switchTab(targetTab, true);
        }
      }
    });
  });
}

function switchTab(tabId, updateUrl = true) {
  closeColumnFilterPopover();
  activeTab = tabId;
  document.querySelectorAll('.tab-btn').forEach(b => {
    b.classList.toggle('active', b.getAttribute('data-tab') === tabId);
  });
  document.querySelectorAll('.tab-content').forEach(content => {
    content.classList.toggle('active', content.id === tabId);
  });

  if (updateUrl) {
    const targetRoute = TAB_ROUTES[tabId] || '/proxies';
    const currentPath = window.location.pathname.replace(/\/+$/, '') || '/';
    if (currentPath !== targetRoute && window.location.hash.replace(/^#\/?/, '/') !== targetRoute) {
      history.pushState({ tab: tabId }, '', targetRoute);
    }
  }

  if (TAB_TITLES[tabId]) {
    document.title = TAB_TITLES[tabId];
  }

  if (tabId === 'containers-view') {
    fetchContainers();
  } else if (tabId === 'streams-view') {
    fetchStreams();
  } else if (tabId === 'metrics-view') {
    fetchNPMMetrics(false);
  } else if (tabId === 'cluster-view') {
    fetchClusterNodes();
  } else if (tabId === 'proxies-view') {
    fetchProxies();
  }
}

// ==============================================================================
// Interactive Table Header Filtering & Sorting Engine
// ==============================================================================

const tableFilters = {
  proxies: {
    columnFilters: {}, // { [colKey]: { text: '', values: Set() } }
    sort: { colKey: null, direction: null },
  },
  streams: {
    columnFilters: {},
    sort: { colKey: null, direction: null },
  },
  containers: {
    columnFilters: {},
    sort: { colKey: null, direction: null },
  },
};

let activePopover = null;

function getTableData(tableId) {
  if (tableId === 'proxies') return proxiesData || [];
  if (tableId === 'streams') return streamsData || [];
  if (tableId === 'containers') return containersData || [];
  return [];
}

function getColumnValue(tableId, item, colKey) {
  if (tableId === 'proxies') {
    switch (colKey) {
      case 'status':
        return item.enabled === false ? 'Disabled' : 'Live';
      case 'domain':
        return (item.domain_names && item.domain_names.length > 0) ? item.domain_names.join(', ') : '';
      case 'target':
        return `${item.forward_scheme || 'http'}://${item.forward_host}:${item.forward_port}`;
      case 'node':
        return item.host_id || 'controller';
      case 'container':
        return item.container_name || (item.meta && item.meta.source === 'iac' ? 'IaC Manifest' : '') || item.container_id || 'Proxy Host';
      case 'ssl':
        if (item.ssl_forced) return 'Forced';
        if (item.ssl_enabled || (item.certificate_id && item.certificate_id !== 0 && item.certificate_id !== '0')) return 'Enabled';
        return 'Disabled';
      case 'strategy':
        return (item.meta && item.meta.resolution_method) || item.resolution_method || item.forward_host_strategy || 'auto';
      case 'npm_id':
        return item.id ? `#${item.id}` : (item.npm_id ? `#${item.npm_id}` : '');
      case 'synced':
        return item.synced_at || item.last_synced || '';
    }
  } else if (tableId === 'streams') {
    switch (colKey) {
      case 'status':
        return item.enabled === false ? 'Disabled' : 'Live';
      case 'incoming_port':
        return item.incoming_port ? `:${item.incoming_port}` : '';
      case 'target':
        return `${item.forwarding_host}:${item.forwarding_port}`;
      case 'protocols': {
        const p = [];
        if (item.tcp) p.push('TCP');
        if (item.udp) p.push('UDP');
        return p.join(' + ') || 'None';
      }
      case 'source':
        if (item.source === 'iac') return 'IaC';
        if (item.source === 'pve') return 'Proxmox';
        if (item.source === 'lxd') return 'LXD/Incus';
        return 'Docker';
      case 'container':
        return item.container_name || (item.source === 'iac' ? 'IaC Manifest' : 'stream');
      case 'node':
        return item.host_id || 'local';
      case 'npm_id':
        return item.npm_stream_id ? `#${item.npm_stream_id}` : '';
      case 'synced':
        return item.last_synced || '';
    }
  } else if (tableId === 'containers') {
    switch (colKey) {
      case 'container':
        return `${item.name || ''} ${item.id || ''}`.trim();
      case 'node':
        return item.node_id || 'controller';
      case 'image':
        return item.image || '';
      case 'state':
        return item.health_status ? `${item.state} (${item.health_status})` : (item.state || 'unknown');
      case 'discovery':
        if (item.manual_npm || (item.ignored_reason && item.ignored_reason.includes('NPM portal'))) return 'NPM Portal';
        if (item.discovered) return 'Discovered';
        return 'Ignored';
      case 'domains':
        return (item.domains && item.domains.length > 0) ? item.domains.join(', ') : 'None';
      case 'port':
        return item.port ? String(item.port) : 'None';
    }
  }
  return '';
}

function getUniqueColumnValues(tableId, colKey) {
  const data = getTableData(tableId);
  const counts = new Map();
  data.forEach(item => {
    if (colKey === 'domain' && item.domain_names && item.domain_names.length > 0) {
      item.domain_names.forEach(d => {
        counts.set(d, (counts.get(d) || 0) + 1);
      });
    } else if (colKey === 'domains' && item.domains && item.domains.length > 0) {
      item.domains.forEach(d => {
        counts.set(d, (counts.get(d) || 0) + 1);
      });
    } else {
      const val = getColumnValue(tableId, item, colKey);
      if (val !== undefined && val !== null && val !== '') {
        counts.set(val, (counts.get(val) || 0) + 1);
      }
    }
  });

  const list = [];
  counts.forEach((count, val) => {
    list.push({ value: val, count });
  });

  list.sort((a, b) => String(a.value).localeCompare(String(b.value), undefined, { numeric: true }));
  return list;
}

function matchesColumnFilter(tableId, item, colKey, filterState) {
  if (!filterState) return true;
  const rawVal = getColumnValue(tableId, item, colKey);
  const strVal = String(rawVal).toLowerCase();

  // 1. Text filter (substring match)
  if (filterState.text && filterState.text.trim()) {
    const q = filterState.text.trim().toLowerCase();
    if (!strVal.includes(q)) return false;
  }

  // 2. Value set filter
  if (filterState.values && filterState.values.size > 0) {
    if (colKey === 'domain') {
      const domains = item.domain_names || [];
      const hasAny = domains.some(d => filterState.values.has(d));
      if (!hasAny) return false;
    } else if (colKey === 'domains') {
      const domains = item.domains || [];
      const hasAny = domains.some(d => filterState.values.has(d));
      if (!hasAny) return false;
    } else {
      if (!filterState.values.has(rawVal)) {
        return false;
      }
    }
  }

  return true;
}

function compareRows(tableId, a, b, sortState) {
  if (!sortState || !sortState.colKey || !sortState.direction) return 0;
  const valA = getColumnValue(tableId, a, sortState.colKey);
  const valB = getColumnValue(tableId, b, sortState.colKey);

  const numA = parseFloat(String(valA).replace(/[^0-9.-]+/g, ''));
  const numB = parseFloat(String(valB).replace(/[^0-9.-]+/g, ''));
  let res = 0;
  if (!isNaN(numA) && !isNaN(numB) && String(valA).replace(/[^0-9.-]+/g, '') !== '' && String(valB).replace(/[^0-9.-]+/g, '') !== '') {
    res = numA - numB;
  } else {
    res = String(valA).localeCompare(String(valB), undefined, { numeric: true, sensitivity: 'base' });
  }

  return sortState.direction === 'desc' ? -res : res;
}

function triggerTableRender(tableId) {
  if (tableId === 'proxies') {
    const s = document.getElementById('proxies-search');
    renderProxiesTable(s ? s.value.toLowerCase() : '');
  } else if (tableId === 'streams') {
    const s = document.getElementById('streams-search');
    renderStreamsTable(s ? s.value.toLowerCase() : '');
  } else if (tableId === 'containers') {
    const s = document.getElementById('containers-search');
    renderContainersTable(s ? s.value.toLowerCase() : '');
  }
}

function updateHeaderIndicators(tableId) {
  const state = tableFilters[tableId];
  if (!state) return;

  const ths = document.querySelectorAll(`th[data-table="${tableId}"][data-col]`);
  ths.forEach(th => {
    const colKey = th.getAttribute('data-col');
    const f = state.columnFilters[colKey];
    const isFiltered = f && ((f.text && f.text.trim()) || (f.values && f.values.size > 0));

    th.classList.toggle('has-active-filter', !!isFiltered);
    const triggerBtn = th.querySelector('.th-filter-trigger');
    const badge = th.querySelector('.th-filter-badge');
    if (triggerBtn) {
      triggerBtn.classList.toggle('active', !!isFiltered);
    }
    if (badge) {
      badge.style.display = isFiltered ? 'block' : 'none';
    }

    const sortIcon = th.querySelector('.th-sort-icon');
    if (sortIcon) {
      if (state.sort.colKey === colKey && state.sort.direction === 'asc') {
        sortIcon.textContent = '▲';
        sortIcon.className = 'th-sort-icon sorted-asc';
      } else if (state.sort.colKey === colKey && state.sort.direction === 'desc') {
        sortIcon.textContent = '▼';
        sortIcon.className = 'th-sort-icon sorted-desc';
      } else {
        sortIcon.textContent = '↕';
        sortIcon.className = 'th-sort-icon';
      }
    }
  });
}

function renderActiveFiltersBar(tableId) {
  const bar = document.getElementById(`${tableId}-active-filters`);
  if (!bar) return;
  const state = tableFilters[tableId];
  if (!state) return;

  const activeCols = Object.entries(state.columnFilters).filter(([_, f]) => {
    return (f.text && f.text.trim()) || (f.values && f.values.size > 0);
  });

  if (activeCols.length === 0) {
    bar.style.display = 'none';
    bar.innerHTML = '';
    return;
  }

  bar.style.display = 'flex';
  bar.innerHTML = `
    <span class="active-filters-title">
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"></polygon>
      </svg>
      Filtered By:
    </span>
  `;

  activeCols.forEach(([colKey, f]) => {
    const th = document.querySelector(`th[data-table="${tableId}"][data-col="${colKey}"]`);
    const label = th ? th.getAttribute('data-label') || colKey : colKey;

    let desc = '';
    if (f.text && f.text.trim()) {
      desc += `"${f.text.trim()}"`;
    }
    if (f.values && f.values.size > 0) {
      if (desc) desc += ' & ';
      const arr = Array.from(f.values);
      desc += arr.slice(0, 2).join(', ');
      if (arr.length > 2) {
        desc += ` +${arr.length - 2}`;
      }
    }

    const chip = document.createElement('div');
    chip.className = 'active-filter-chip';
    chip.innerHTML = `
      <span><strong>${escapeHtml(label)}:</strong> ${escapeHtml(desc)}</span>
      <button type="button" class="active-filter-chip-remove" title="Remove filter for ${escapeHtml(label)}">&times;</button>
    `;
    chip.querySelector('.active-filter-chip-remove').addEventListener('click', () => {
      delete state.columnFilters[colKey];
      updateHeaderIndicators(tableId);
      triggerTableRender(tableId);
    });
    bar.appendChild(chip);
  });

  const clearAllBtn = document.createElement('button');
  clearAllBtn.type = 'button';
  clearAllBtn.className = 'btn-clear-all-filters';
  clearAllBtn.textContent = 'Clear All Filters';
  clearAllBtn.addEventListener('click', () => {
    state.columnFilters = {};
    updateHeaderIndicators(tableId);
    triggerTableRender(tableId);
  });
  bar.appendChild(clearAllBtn);
}

function closeColumnFilterPopover() {
  if (activePopover) {
    activePopover.remove();
    activePopover = null;
  }
}

function openColumnFilterPopover(th, tableId, colKey, label) {
  if (activePopover && activePopover.getAttribute('data-popover-col') === `${tableId}:${colKey}`) {
    closeColumnFilterPopover();
    return;
  }
  closeColumnFilterPopover();

  const state = tableFilters[tableId];
  if (!state.columnFilters[colKey]) {
    state.columnFilters[colKey] = { text: '', values: new Set() };
  }
  const curFilter = state.columnFilters[colKey];
  const uniqueValues = getUniqueColumnValues(tableId, colKey);

  const popover = document.createElement('div');
  popover.className = 'th-filter-popover';
  popover.setAttribute('data-popover-col', `${tableId}:${colKey}`);

  const draftSelected = new Set(curFilter.values);
  let draftText = curFilter.text || '';

  popover.innerHTML = `
    <div class="popover-header">
      <div class="popover-title">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"></polygon>
        </svg>
        <span>Filter ${escapeHtml(label)}</span>
      </div>
      <button type="button" class="popover-close-btn" title="Close">&times;</button>
    </div>

    <!-- Quick Sort Row -->
    <div class="popover-sort-row">
      <button type="button" class="popover-sort-btn ${state.sort.colKey === colKey && state.sort.direction === 'asc' ? 'active' : ''}" data-sort="asc">
        ▲ Asc
      </button>
      <button type="button" class="popover-sort-btn ${state.sort.colKey === colKey && state.sort.direction === 'desc' ? 'active' : ''}" data-sort="desc">
        ▼ Desc
      </button>
      <button type="button" class="popover-sort-btn" data-sort="none">
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><polyline points="1 4 1 10 7 10"></polyline><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"></path></svg>Clear Sort
      </button>
    </div>

    <!-- Search in Column -->
    <div class="popover-search-wrap">
      <svg class="popover-search-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="11" cy="11" r="8"></circle>
        <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
      </svg>
      <input type="text" class="popover-search-input" placeholder="Search ${escapeHtml(label)}..." value="${escapeHtml(draftText)}" />
    </div>

    <!-- Quick Check Links -->
    <div class="popover-quick-links">
      <button type="button" class="popover-link-btn" id="quick-select-all">Select All</button>
      <button type="button" class="popover-link-btn" id="quick-clear-all">Clear</button>
    </div>

    <!-- Values List -->
    <div class="popover-values-list" id="popover-val-container">
      ${uniqueValues.length === 0 ? '<div style="color:var(--text-muted);font-size:0.75rem;padding:0.4rem;">No values available</div>' : ''}
    </div>

    <!-- Footer Actions -->
    <div class="popover-footer">
      <button type="button" class="popover-btn popover-btn-apply">Apply Filter</button>
      <button type="button" class="popover-btn popover-btn-reset">Reset</button>
    </div>
  `;

  const valContainer = popover.querySelector('#popover-val-container');
  function renderChecklist(searchTerm = '') {
    valContainer.innerHTML = '';
    const term = searchTerm.toLowerCase();
    const visibleValues = uniqueValues.filter(uv => String(uv.value).toLowerCase().includes(term));

    if (visibleValues.length === 0) {
      valContainer.innerHTML = '<div style="color:var(--text-muted);font-size:0.75rem;padding:0.4rem;">No matching values</div>';
      return;
    }

    visibleValues.forEach(uv => {
      const itemEl = document.createElement('label');
      itemEl.className = 'popover-val-item';

      const isChecked = draftSelected.size === 0 || draftSelected.has(uv.value);

      itemEl.innerHTML = `
        <div class="popover-val-left">
          <input type="checkbox" value="${escapeHtml(String(uv.value))}" ${isChecked ? 'checked' : ''} />
          <span class="popover-val-text" title="${escapeHtml(String(uv.value))}">${escapeHtml(String(uv.value))}</span>
        </div>
        <span class="popover-val-count">${uv.count}</span>
      `;

      const cb = itemEl.querySelector('input');
      cb.addEventListener('change', () => {
        if (cb.checked) {
          draftSelected.add(uv.value);
        } else {
          if (draftSelected.size === 0) {
            uniqueValues.forEach(v => draftSelected.add(v.value));
          }
          draftSelected.delete(uv.value);
        }
      });

      valContainer.appendChild(itemEl);
    });
  }

  renderChecklist();

  const searchInput = popover.querySelector('.popover-search-input');
  searchInput.addEventListener('input', (e) => {
    draftText = e.target.value;
    renderChecklist(draftText);
  });

  popover.querySelector('#quick-select-all').addEventListener('click', () => {
    draftSelected.clear();
    popover.querySelectorAll('#popover-val-container input[type="checkbox"]').forEach(cb => {
      cb.checked = true;
    });
  });

  popover.querySelector('#quick-clear-all').addEventListener('click', () => {
    draftSelected.clear();
    uniqueValues.forEach(v => draftSelected.add('__NONE_MATCHING__'));
    popover.querySelectorAll('#popover-val-container input[type="checkbox"]').forEach(cb => {
      cb.checked = false;
    });
  });

  popover.querySelectorAll('.popover-sort-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const dir = btn.getAttribute('data-sort');
      if (dir === 'none') {
        state.sort = { colKey: null, direction: null };
      } else {
        state.sort = { colKey, direction: dir };
      }
      triggerTableRender(tableId);
      updateHeaderIndicators(tableId);
      closeColumnFilterPopover();
    });
  });

  popover.querySelector('.popover-btn-apply').addEventListener('click', () => {
    if (draftSelected.size >= uniqueValues.length || draftSelected.size === 0) {
      curFilter.values = new Set();
    } else {
      curFilter.values = new Set(draftSelected);
    }
    curFilter.text = draftText;

    if (!curFilter.text && curFilter.values.size === 0) {
      delete state.columnFilters[colKey];
    }

    triggerTableRender(tableId);
    updateHeaderIndicators(tableId);
    closeColumnFilterPopover();
  });

  popover.querySelector('.popover-btn-reset').addEventListener('click', () => {
    delete state.columnFilters[colKey];
    triggerTableRender(tableId);
    updateHeaderIndicators(tableId);
    closeColumnFilterPopover();
  });

  popover.querySelector('.popover-close-btn').addEventListener('click', closeColumnFilterPopover);

  document.body.appendChild(popover);
  activePopover = popover;

  const rect = th.getBoundingClientRect();
  let top = rect.bottom + 6;
  let left = rect.left;

  if (left + 280 > window.innerWidth) {
    left = Math.max(10, window.innerWidth - 290);
  }
  if (top + popover.offsetHeight > window.innerHeight) {
    top = Math.max(10, rect.top - popover.offsetHeight - 6);
  }

  popover.style.top = `${top}px`;
  popover.style.left = `${left}px`;
}

document.addEventListener('click', (e) => {
  if (activePopover) {
    if (!activePopover.contains(e.target) && !e.target.closest('.th-filter-trigger')) {
      closeColumnFilterPopover();
    }
  }
});

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && activePopover) {
    closeColumnFilterPopover();
  }
});

window.addEventListener('resize', closeColumnFilterPopover);

function initTableHeaders() {
  const ths = document.querySelectorAll('th[data-table][data-col]');
  ths.forEach(th => {
    const tableId = th.getAttribute('data-table');
    const colKey = th.getAttribute('data-col');
    const label = th.getAttribute('data-label') || th.textContent.trim();

    th.classList.add('filterable');
    th.innerHTML = `
      <div class="th-content-wrapper">
        <button type="button" class="th-label-btn" title="Click to sort by ${escapeHtml(label)}">
          <span>${escapeHtml(label)}</span>
          <span class="th-sort-icon">↕</span>
        </button>
        <div class="th-icons-group">
          <button type="button" class="th-filter-trigger" title="Filter by ${escapeHtml(label)}">
            <svg class="th-filter-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"></polygon>
            </svg>
            <span class="th-filter-badge" style="display:none;"></span>
          </button>
        </div>
      </div>
    `;

    const labelBtn = th.querySelector('.th-label-btn');
    labelBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      const state = tableFilters[tableId];
      if (state.sort.colKey === colKey) {
        if (state.sort.direction === 'asc') {
          state.sort.direction = 'desc';
        } else {
          state.sort = { colKey: null, direction: null };
        }
      } else {
        state.sort = { colKey, direction: 'asc' };
      }
      updateHeaderIndicators(tableId);
      triggerTableRender(tableId);
    });

    const filterBtn = th.querySelector('.th-filter-trigger');
    filterBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      openColumnFilterPopover(th, tableId, colKey, label);
    });
  });
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

  const pruneOfflineBtn = document.getElementById('btn-prune-offline-nodes');
  if (pruneOfflineBtn) {
    pruneOfflineBtn.addEventListener('click', pruneOfflineNodes);
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
  await fetchNPMMetrics(false);
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
    if (filterText) {
      const searchStr = `${s.incoming_port} ${s.forwarding_host}:${s.forwarding_port} ${s.container_name || ''} ${s.host_id || ''} ${s.tcp ? 'tcp' : ''} ${s.udp ? 'udp' : ''}`.toLowerCase();
      if (!searchStr.includes(filterText)) return false;
    }
    for (const [colKey, filterState] of Object.entries(tableFilters.streams.columnFilters)) {
      if (!matchesColumnFilter('streams', s, colKey, filterState)) return false;
    }
    return true;
  });

  if (tableFilters.streams.sort.colKey && tableFilters.streams.sort.direction) {
    filtered.sort((a, b) => compareRows('streams', a, b, tableFilters.streams.sort));
  }

  renderActiveFiltersBar('streams');
  updateHeaderIndicators('streams');

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

    let sourceBadge = `<span class="badge badge-cyan" style="font-size: 0.7rem;"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><rect x="2" y="7" width="20" height="14" rx="2" ry="2"></rect><path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16"></path></svg>Docker</span>`;
    if (stream.source === 'iac') {
      sourceBadge = `<span class="badge badge-amber" style="font-size: 0.7rem;" title="IaC Manifest: ${escapeHtml(stream.source_file || 'routes.yaml')}"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline></svg>IaC</span>`;
    } else if (stream.source === 'pve') {
      sourceBadge = `<span class="badge" style="background: rgba(249, 115, 22, 0.15); color: #f97316; font-size: 0.7rem;" title="Proxmox VE LXC"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon></svg>Proxmox</span>`;
    } else if (stream.source === 'lxd') {
      sourceBadge = `<span class="badge" style="background: rgba(16, 185, 129, 0.15); color: #10b981; font-size: 0.7rem;" title="Canonical LXD / Incus"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><polyline points="4 17 10 11 4 5"></polyline><line x1="12" y1="19" x2="20" y2="19"></line></svg>LXD/Incus</span>`;
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
    if (filterText) {
      const searchStr = `${p.domain_names.join(' ')} ${p.container_name} ${p.forward_host}:${p.forward_port} ${p.host_id || ''}`.toLowerCase();
      if (!searchStr.includes(filterText)) return false;
    }
    for (const [colKey, filterState] of Object.entries(tableFilters.proxies.columnFilters)) {
      if (!matchesColumnFilter('proxies', p, colKey, filterState)) return false;
    }
    return true;
  });

  if (tableFilters.proxies.sort.colKey && tableFilters.proxies.sort.direction) {
    filtered.sort((a, b) => compareRows('proxies', a, b, tableFilters.proxies.sort));
  }

  renderActiveFiltersBar('proxies');
  updateHeaderIndicators('proxies');

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
      sslBadge = `<span class="badge badge-emerald"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path></svg>Forced HTTPS</span>`;
    } else if (proxy.ssl_enabled) {
      sslBadge = `<span class="badge badge-cyan"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path></svg>Enabled</span>`;
    }

    const lastSyncTime = proxy.last_synced ? new Date(proxy.last_synced).toLocaleTimeString() : 'N/A';

    let sourceBadge = `<span class="badge badge-cyan" style="font-size: 0.7rem;"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><rect x="2" y="7" width="20" height="14" rx="2" ry="2"></rect><path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16"></path></svg>Docker</span>`;
    if (proxy.source === 'iac') {
      sourceBadge = `<span class="badge badge-amber" style="font-size: 0.7rem;" title="IaC Manifest: ${escapeHtml(proxy.source_file || 'routes.yaml')}"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline></svg>IaC</span>`;
    } else if (proxy.source === 'pve') {
      sourceBadge = `<span class="badge" style="background: rgba(249, 115, 22, 0.15); color: #f97316; font-size: 0.7rem;" title="Proxmox VE LXC"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon></svg>Proxmox</span>`;
    } else if (proxy.source === 'lxd') {
      sourceBadge = `<span class="badge" style="background: rgba(16, 185, 129, 0.15); color: #10b981; font-size: 0.7rem;" title="Canonical LXD / Incus"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><polyline points="4 17 10 11 4 5"></polyline><line x1="12" y1="19" x2="20" y2="19"></line></svg>LXD/Incus</span>`;
    } else if (proxy.source === 'manual' || proxy.source === 'npm-portal') {
      sourceBadge = `<span class="badge badge-amber" style="font-size: 0.7rem;" title="Configured directly via Nginx Proxy Manager portal"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>NPM Portal</span>`;
    }

    let liveStatusBadge = `<span class="badge badge-emerald">● Live</span>`;
    if (proxy.status === 'disabled') {
      liveStatusBadge = `<span class="badge badge-gray"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><rect x="6" y="4" width="4" height="16"></rect><rect x="14" y="4" width="4" height="16"></rect></svg>Disabled</span>`;
    }

    let stratBadge = `<span class="badge badge-gray">${escapeHtml(proxy.resolution_method || 'auto')}</span>`;
    if (proxy.source === 'manual' || proxy.source === 'npm-portal') {
      stratBadge = `<span class="badge badge-amber" title="Manually configured from NPM portal">Manual (NPM)</span>`;
    }

    let containerTitle = escapeHtml(proxy.container_name || 'NPM Portal Host');
    let containerSub = escapeHtml(proxy.image || (proxy.source_file ? 'manifest: ' + proxy.source_file : (proxy.container_id ? proxy.container_id.substring(0, 12) : '')));
    if (proxy.source === 'manual' || proxy.source === 'npm-portal') {
      if (!proxy.container_id && (!proxy.container_name || proxy.container_name === 'npm-portal')) {
        containerTitle = `<span style="color: var(--text-secondary); font-style: italic;">NPM Portal Host</span>`;
        containerSub = `<span style="color: var(--text-muted);">Target: ${escapeHtml(proxy.forward_host)}:${proxy.forward_port}</span>`;
      } else {
        containerTitle = `<span>${escapeHtml(proxy.container_name)}</span>`;
        containerSub = `<span class="badge badge-amber" style="font-size: 0.65rem;">Linked via NPM</span>`;
      }
    }

    let locationBadge = '';
    if (proxy.locations && proxy.locations.length > 0) {
      locationBadge = `<div style="margin-top: 4px;"><span class="badge" style="background: rgba(0, 242, 254, 0.12); color: #00f2fe; font-size: 0.7rem;" title="Custom Locations: ${proxy.locations.map(l => l.path).join(', ')}"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"></path><circle cx="12" cy="10" r="3"></circle></svg>${proxy.locations.length} Locations</span></div>`;
    }

    let replicaBadge = '';
    if (proxy.upstream_replicas && proxy.upstream_replicas > 1) {
      replicaBadge = `<div style="margin-top: 4px;"><span class="badge" style="background: rgba(16, 185, 129, 0.12); color: #10b981; font-size: 0.7rem;" title="Load-balanced across ${proxy.upstream_replicas} containers"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><path d="m16 16 3-8 3 8c-.87.65-1.92 1-3 1s-2.13-.35-3-1Z"></path><path d="m2 16 3-8 3 8c-.87.65-1.92 1-3 1s-2.13-.35-3-1Z"></path><path d="M7 21h10"></path><path d="M12 3v18"></path><path d="M3 7h2c2 0 5-1 7-2 2 1 5 2 7 2h2"></path></svg>${proxy.upstream_replicas} Replicas (${escapeHtml(proxy.upstream_balancing || 'round_robin')})</span></div>`;
    }

    tr.innerHTML = `
      <td>${liveStatusBadge}</td>
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
          <span class="container-title">${containerTitle}</span>
          <span class="container-sub">${containerSub}</span>
        </div>
      </td>
      <td>${sslBadge}</td>
      <td>${stratBadge}</td>
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
async function fetchContainers(force = false) {
  const refreshBtn = document.getElementById('btn-refresh-containers');
  if (force && refreshBtn) {
    refreshBtn.classList.add('loading');
    const svg = refreshBtn.querySelector('svg');
    if (svg) svg.style.animation = 'spin 0.75s linear infinite';
  }

  try {
    const res = await fetch('/api/containers');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    containersData = data.containers || [];
    const countEl = document.getElementById('tab-containers-count');
    if (countEl) countEl.textContent = containersData.length;
    renderContainersTable();
    if (force) showToast(`Refreshed ${containersData.length} containers`, 'info');
  } catch (err) {
    console.warn('Failed to fetch containers:', err);
    if (force) showToast(`Failed to refresh containers: ${err.message}`, 'error');
  } finally {
    if (force && refreshBtn) {
      setTimeout(() => {
        refreshBtn.classList.remove('loading');
        const svg = refreshBtn.querySelector('svg');
        if (svg) svg.style.animation = '';
      }, 400);
    }
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
    if (filterText) {
      const search = `${c.name} ${c.image} ${c.id} ${c.node_id || ''}`.toLowerCase();
      if (!search.includes(filterText)) return false;
    }
    for (const [colKey, filterState] of Object.entries(tableFilters.containers.columnFilters)) {
      if (!matchesColumnFilter('containers', c, colKey, filterState)) return false;
    }
    return true;
  });

  if (tableFilters.containers.sort.colKey && tableFilters.containers.sort.direction) {
    filtered.sort((a, b) => compareRows('containers', a, b, tableFilters.containers.sort));
  }

  renderActiveFiltersBar('containers');
  updateHeaderIndicators('containers');

  if (filtered.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td colspan="8" style="text-align: center; padding: 3rem 1rem; color: var(--text-muted);">
        <div style="margin-bottom: 0.5rem;">
          <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" style="color: var(--text-muted);">
            <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"></path>
            <polyline points="3.27 6.96 12 12.01 20.73 6.96"></polyline>
            <line x1="12" y1="22.08" x2="12" y2="12"></line>
          </svg>
        </div>
        <div style="font-weight: 600; margin-bottom: 0.25rem;">No containers found</div>
        <div style="font-size: 0.85rem;">
          ${containersData.length === 0 
            ? 'No Docker, Proxmox VE, or LXD containers have been discovered yet. Verify your provider connections and check the live logs.' 
            : 'No containers match your current node or search filter.'}
        </div>
      </td>
    `;
    tbody.appendChild(tr);
    return;
  }

  filtered.forEach(c => {
    const tr = document.createElement('tr');

    let statusBadge = '';
    if (c.manual_npm || (c.ignored_reason && c.ignored_reason.includes('NPM portal'))) {
      statusBadge = `<span class="badge badge-amber" title="Manually configured from NPM portal"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>NPM Portal</span>`;
    } else if (c.discovered) {
      statusBadge = `<span class="badge badge-emerald"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><polyline points="20 6 9 17 4 12"></polyline></svg>Discovered</span>`;
    } else {
      statusBadge = `<span class="badge badge-gray" title="${escapeHtml(c.ignored_reason || '')}">Ignored</span>`;
    }

    let healthBadge = '';
    if (c.health_status === 'healthy') {
      healthBadge = `<span class="badge badge-emerald" style="font-size:0.7rem; margin-left:4px;" title="HealthCheck: Healthy"><svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:2px;"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg>healthy</span>`;
    } else if (c.health_status === 'starting') {
      healthBadge = `<span class="badge badge-amber" style="font-size:0.7rem; margin-left:4px;" title="HealthCheck: Warming Up (Held by Zero-502)"><svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:2px;"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>starting</span>`;
    } else if (c.health_status === 'unhealthy') {
      healthBadge = `<span class="badge badge-rose" style="font-size:0.7rem; margin-left:4px;" title="HealthCheck: Unhealthy"><svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:2px;"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>unhealthy</span>`;
    }

    let domainText = `<span class="text-muted">—</span>`;
    if (c.domains && c.domains.length > 0) {
      domainText = c.domains.map(d => {
        return `<a href="http://${d}" target="_blank" rel="noopener noreferrer" class="domain-chip" style="font-size: 0.72rem; padding: 0.15rem 0.45rem;">
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
            <polyline points="15 3 21 3 21 9"></polyline>
            <line x1="10" y1="14" x2="21" y2="3"></line>
          </svg>
          ${escapeHtml(d)}
        </a>`;
      }).join(' ');
    }
    const portText = c.port ? c.port : `<span class="text-muted">—</span>`;

    let sourceBadge = `<span class="badge badge-cyan" style="font-size:0.65rem;">Docker</span>`;
    if (c.source === 'pve') {
      const typeLabel = (c.image && c.image.toLowerCase() === 'qemu') ? 'Proxmox VM' : 'Proxmox VE';
      sourceBadge = `<span class="badge" style="background: rgba(249, 115, 22, 0.15); color: #f97316; font-size:0.65rem;">${typeLabel}</span>`;
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
        <span class="badge badge-node"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect><rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect><line x1="6" y1="6" x2="6.01" y2="6"></line><line x1="6" y1="18" x2="6.01" y2="18"></line></svg>${escapeHtml(c.node_id || 'controller')}</span>
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
        <div><strong>${escapeHtml(proxy.container_name || 'NPM Portal Host')}</strong> ${proxy.container_id ? `(ID: <code>${escapeHtml(proxy.container_id.substring(0, 12))}</code>)` : `<span class="badge badge-amber" style="font-size:0.7rem; margin-left: 4px;">NPM Portal</span>`}</div>
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
              <div style="font-size: 0.75rem; color: var(--text-muted);">${loc.websocket ? '<svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:2px;"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon></svg>websocket' : ''}</div>
            </div>
          `).join('')}
        </div>
      </div>
      ` : ''}

      ${proxy.upstream_replicas && proxy.upstream_replicas > 1 ? `
      <div>
        <label class="text-muted" style="font-size: 0.75rem; text-transform: uppercase;">Load Balanced Upstream</label>
        <div style="margin-top: 0.25rem;">
          <span class="badge badge-emerald"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><path d="m16 16 3-8 3 8c-.87.65-1.92 1-3 1s-2.13-.35-3-1Z"></path><path d="m2 16 3-8 3 8c-.87.65-1.92 1-3 1s-2.13-.35-3-1Z"></path><path d="M7 21h10"></path><path d="M12 3v18"></path><path d="M3 7h2c2 0 5-1 7-2 2 1 5 2 7 2h2"></path></svg>${proxy.upstream_replicas} Replicas</span>
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
      <p class="text-muted mb-3">Add tags or notes to your Proxmox instance (ID <code>${escapeHtml(container.id)}</code>) to enable automated discovery:</p>
      
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

// Docs View Mode Switcher ('reference' | 'guides' | 'split')
let currentDocsMode = 'reference';
let currentDocsCategory = 'all';

function setDocsMode(mode) {
  currentDocsMode = mode;
  localStorage.setItem('docs_view_mode', mode);

  const container = document.getElementById('docs-grid-container');
  if (container) {
    container.className = `docs-grid mode-${mode}`;
  }

  const btnRef = document.getElementById('btn-docs-reference');
  const btnGuides = document.getElementById('btn-docs-guides');
  const btnSplit = document.getElementById('btn-docs-split');

  if (btnRef) btnRef.classList.toggle('active', mode === 'reference');
  if (btnGuides) btnGuides.classList.toggle('active', mode === 'guides');
  if (btnSplit) btnSplit.classList.toggle('active', mode === 'split');
}

function filterDocsCategory(cat) {
  currentDocsCategory = cat;

  // Update chips active state
  document.querySelectorAll('.docs-chip').forEach(chip => {
    chip.classList.toggle('active', chip.getAttribute('data-category') === cat);
  });

  applyDocsFilters();
}

function clearDocsSearch() {
  const input = document.getElementById('docs-search');
  if (input) {
    input.value = '';
    input.focus();
  }
  const clearBtn = document.getElementById('docs-search-clear');
  if (clearBtn) clearBtn.style.display = 'none';
  applyDocsFilters();
}

function applyDocsFilters() {
  const input = document.getElementById('docs-search');
  const query = (input?.value || '').toLowerCase().trim();
  const clearBtn = document.getElementById('docs-search-clear');
  if (clearBtn) {
    clearBtn.style.display = query ? 'block' : 'none';
  }

  const rows = document.querySelectorAll('#docs-reference-table tbody tr');
  let matchedOptionsCount = 0;
  let totalOptionsCount = 0;

  // First pass: identify category headers and their child rows
  let currentHeader = null;
  let currentHeaderHasMatch = false;
  let currentHeaderCategory = '';

  rows.forEach(row => {
    if (row.classList.contains('docs-category-header')) {
      if (currentHeader) {
        currentHeader.style.display = currentHeaderHasMatch ? '' : 'none';
      }
      currentHeader = row;
      currentHeaderHasMatch = false;
      const hText = row.textContent.toLowerCase();
      if (hText.includes('docker ingress')) currentHeaderCategory = 'docker';
      else if (hText.includes('zero-502') || hText.includes('health')) currentHeaderCategory = 'health';
      else if (hText.includes('middleware')) currentHeaderCategory = 'middleware';
      else if (hText.includes('subpath') || hText.includes('location')) currentHeaderCategory = 'locations';
      else if (hText.includes('balancing') || hText.includes('upstream')) currentHeaderCategory = 'upstream';
      else if (hText.includes('layer 4') || hText.includes('streams')) currentHeaderCategory = 'streams';
      else if (hText.includes('proxmox') || hText.includes('pve')) currentHeaderCategory = 'pve';
      else if (hText.includes('lxd') || hText.includes('incus')) currentHeaderCategory = 'lxd';
      else if (hText.includes('environment')) currentHeaderCategory = 'env';
      else currentHeaderCategory = '';
      return;
    }

    totalOptionsCount++;
    const rowCat = row.getAttribute('data-category') || currentHeaderCategory;
    const text = row.textContent.toLowerCase();

    const matchesCategory = currentDocsCategory === 'all' || rowCat === currentDocsCategory;
    const matchesSearch = !query || text.includes(query);

    if (matchesCategory && matchesSearch) {
      row.style.display = '';
      currentHeaderHasMatch = true;
      matchedOptionsCount++;
    } else {
      row.style.display = 'none';
    }
  });

  // Handle final category header
  if (currentHeader) {
    currentHeader.style.display = currentHeaderHasMatch ? '' : 'none';
  }

  // Update match counter text
  const counterEl = document.getElementById('docs-match-counter');
  if (counterEl) {
    if (query || currentDocsCategory !== 'all') {
      counterEl.textContent = `Showing ${matchedOptionsCount} of ${totalOptionsCount} options`;
    } else {
      counterEl.textContent = `${totalOptionsCount} options & settings documented`;
    }
  }
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

// Docs Reference Table Real-Time Filter & Categorizer
function initDocsSearch() {
  // Automatically tag rows with categories based on category headers
  const rows = document.querySelectorAll('#docs-reference-table tbody tr');
  let currentCategory = 'docker';

  rows.forEach(row => {
    if (row.classList.contains('docs-category-header')) {
      const headerText = row.textContent.toLowerCase();
      if (headerText.includes('docker')) currentCategory = 'docker';
      else if (headerText.includes('health')) currentCategory = 'health';
      else if (headerText.includes('middleware')) currentCategory = 'middleware';
      else if (headerText.includes('subpath') || headerText.includes('location')) currentCategory = 'locations';
      else if (headerText.includes('upstream') || headerText.includes('balancing')) currentCategory = 'upstream';
      else if (headerText.includes('stream')) currentCategory = 'streams';
      else if (headerText.includes('proxmox') || headerText.includes('pve')) currentCategory = 'pve';
      else if (headerText.includes('lxd') || headerText.includes('incus')) currentCategory = 'lxd';
      else if (headerText.includes('environment')) currentCategory = 'env';

      row.setAttribute('data-category', currentCategory);
    } else {
      row.setAttribute('data-category', currentCategory);
    }
  });

  const input = document.getElementById('docs-search');
  if (input) {
    input.addEventListener('input', applyDocsFilters);
  }

  // Restore saved view mode or default intelligently based on screen width
  const savedMode = localStorage.getItem('docs_view_mode') || (window.innerWidth >= 1440 ? 'split' : 'reference');
  setDocsMode(savedMode);
  applyDocsFilters();
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
  if (selectedContainersNode !== 'all' && !nodes.includes(selectedContainersNode)) {
    selectedContainersNode = 'all';
  }
  if (selectedProxiesNode !== 'all' && !nodes.includes(selectedProxiesNode)) {
    selectedProxiesNode = 'all';
  }
  if (selectedStreamsNode !== 'all' && !nodes.includes(selectedStreamsNode)) {
    selectedStreamsNode = 'all';
  }
  if (selectedEventsNode !== 'all' && !nodes.includes(selectedEventsNode)) {
    selectedEventsNode = 'all';
  }

  const dropdowns = [
    { el: document.getElementById('containers-node-filter'), current: selectedContainersNode },
    { el: document.getElementById('proxies-node-filter'), current: selectedProxiesNode },
    { el: document.getElementById('streams-node-filter'), current: selectedStreamsNode },
    { el: document.getElementById('events-node-filter'), current: selectedEventsNode }
  ];

  dropdowns.forEach(({ el, current }) => {
    if (!el) return;
    const existingVal = el.value || current || 'all';
    el.innerHTML = '<option value="all">All Nodes</option>';
    nodes.forEach(nodeId => {
      const opt = document.createElement('option');
      opt.value = nodeId;
      opt.textContent = nodeId;
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

  const offlineNodes = clusterData.filter(n => !n.is_controller && n.status === 'offline');
  const pruneBtn = document.getElementById('btn-prune-offline-nodes');
  const offlineCountSpan = document.getElementById('offline-nodes-count');
  if (pruneBtn) {
    if (offlineNodes.length > 0) {
      pruneBtn.style.display = 'inline-flex';
      if (offlineCountSpan) offlineCountSpan.textContent = offlineNodes.length;
    } else {
      pruneBtn.style.display = 'none';
    }
  }

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
        ? '<span class="badge badge-cyan" style="font-size:0.72rem;"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"></path></svg>Central Controller</span>' 
        : '<span class="badge badge-violet" style="font-size:0.72rem;"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><circle cx="12" cy="12" r="2"></circle><path d="M16.24 7.76a6 6 0 0 1 0 8.49m-8.48-.01a6 6 0 0 1 0-8.49m11.31-2.82a10 10 0 0 1 0 14.14m-14.14 0a10 10 0 0 1 0-14.14"></path></svg>Remote Worker</span>';

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
            <span class="text-muted"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-2px; margin-right:4px;"><rect x="2" y="7" width="20" height="14" rx="2" ry="2"></rect><path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16"></path></svg>Docker Engine:</span>
            <span>${node.docker_connected ? `<span style="color:#10b981; font-weight:600;">Connected</span> <span style="font-size:0.75rem; color:var(--text-muted); font-family:var(--font-mono);">(${escapeHtml(node.docker_version || 'active')})</span>` : '<span style="color:#f43f5e;">Disconnected</span>'}</span>
          </div>
          <div class="node-engine-row" style="cursor: pointer;" onclick="openProxmoxModal('${escapeHtml(nodeId)}')" title="Configure Proxmox VE Discovery">
            <span class="text-muted"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-2px; margin-right:4px;"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon></svg>Proxmox VE:</span>
            <span>
              ${((node.pve_enabled || (node.overview && node.overview.pve_enabled)) && node.pve_connected)
                ? `<span style="color:#10b981; font-weight:600;">Active</span> <span style="font-size:0.75rem; color:var(--text-muted);">(${node.pve_endpoint_count && node.pve_endpoint_count > 1 ? `${node.pve_endpoint_count} endpoints` : escapeHtml(node.pve_version || 'connected')})</span>` 
                : ((node.pve_enabled || (node.overview && node.overview.pve_enabled))
                    ? `<span style="color:#f59e0b; font-weight:600;">Enabled</span> <span style="font-size:0.75rem; color:var(--text-muted);">(${node.pve_endpoint_count && node.pve_endpoint_count > 1 ? `${node.pve_endpoint_count} endpoints` : 'Connecting...'})</span>` 
                    : '<span style="color:var(--text-muted);">Disabled</span>')}
            </span>
          </div>
          <div class="node-engine-row">
            <span class="text-muted"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-2px; margin-right:4px;"><polyline points="4 17 10 11 4 5"></polyline><line x1="12" y1="19" x2="20" y2="19"></line></svg>LXD / Incus:</span>
            <span>${node.lxd_connected ? '<span style="color:#10b981; font-weight:600;">Active</span>' : '<span style="color:var(--text-muted);">Disabled</span>'}</span>
          </div>
        </div>

        <div class="node-card-footer">
          <span style="font-size:0.75rem; color:var(--text-muted);">Uptime: ${escapeHtml(uptimeStr)}</span>
          <div style="display: flex; gap: 0.4rem; flex-wrap: wrap;">
            ${(!node.is_controller && !isOnline) ? `
              <button class="btn btn-danger btn-sm" onclick="removeClusterNode('${escapeHtml(nodeId)}')" title="Remove offline node from cluster registry">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-2px; margin-right:2px;">
                  <polyline points="3 6 5 6 21 6"></polyline>
                  <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
                </svg>
                Remove
              </button>
            ` : ''}
            <button class="btn btn-secondary btn-sm" onclick="openProxmoxModal('${escapeHtml(nodeId)}')">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-1px; margin-right:3px;"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon></svg>Proxmox VE
            </button>
            <button class="btn btn-secondary btn-sm" onclick="filterContainersByNode('${escapeHtml(nodeId)}')">
              Inspect Containers
            </button>
          </div>
        </div>
      `;

      grid.appendChild(card);
    } catch (err) {
      console.error('Failed to render cluster node card:', err, node);
    }
  });
}

async function removeClusterNode(nodeId) {
  if (!nodeId) return;
  if (!confirm(`Are you sure you want to remove offline node "${nodeId}" from the cluster registry?`)) {
    return;
  }

  try {
    const res = await fetch(`/api/cluster/nodes/${encodeURIComponent(nodeId)}`, {
      method: 'DELETE'
    });

    if (!res.ok) {
      const errData = await res.json().catch(() => ({}));
      const errMsg = errData.error || errData.message || (await res.text()) || `HTTP ${res.status}`;
      throw new Error(errMsg);
    }

    showToast(`Removed offline node "${nodeId}" from cluster`, 'success');
    await fetchClusterNodes();
    await fetchStatus();
    await fetchProxies();
    await fetchContainers();
    await fetchStreams();
  } catch (err) {
    console.error('Failed to remove node:', err);
    showToast(`Failed to remove node: ${err.message}`, 'error');
  }
}

async function pruneOfflineNodes() {
  const offlineNodes = clusterData.filter(n => !n.is_controller && n.status === 'offline');
  if (offlineNodes.length === 0) {
    showToast('No offline nodes to prune', 'info');
    return;
  }

  const names = offlineNodes.map(n => n.node_id).join(', ');
  if (!confirm(`Are you sure you want to remove all ${offlineNodes.length} offline node(s) (${names}) from cluster registry?`)) {
    return;
  }

  const btn = document.getElementById('btn-prune-offline-nodes');
  if (btn) btn.disabled = true;

  try {
    const res = await fetch('/api/cluster/nodes?offline=true', {
      method: 'DELETE'
    });

    if (!res.ok) {
      const errData = await res.json().catch(() => ({}));
      const errMsg = errData.error || errData.message || (await res.text()) || `HTTP ${res.status}`;
      throw new Error(errMsg);
    }

    const data = await res.json();
    const count = data.count !== undefined ? data.count : offlineNodes.length;
    showToast(`Successfully pruned ${count} offline node(s)`, 'success');
    await fetchClusterNodes();
    await fetchStatus();
    await fetchProxies();
    await fetchContainers();
    await fetchStreams();
  } catch (err) {
    console.error('Failed to prune offline nodes:', err);
    showToast(`Failed to prune offline nodes: ${err.message}`, 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}

window.removeClusterNode = removeClusterNode;
window.pruneOfflineNodes = pruneOfflineNodes;

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

  const pveToggle = document.getElementById('add-node-pve-enabled');
  if (pveToggle) {
    pveToggle.addEventListener('change', () => {
      if (generatedConfigs.run) generateAgentConfig(false);
    });
  }

  ['add-node-pve-url', 'add-node-pve-node', 'add-node-pve-token-id', 'add-node-pve-token-secret'].forEach(id => {
    const el = document.getElementById(id);
    if (el) {
      el.addEventListener('input', () => {
        if (generatedConfigs.run) generateAgentConfig(false);
      });
    }
  });

  const pveSSL = document.getElementById('add-node-pve-verify-ssl');
  if (pveSSL) {
    pveSSL.addEventListener('change', () => {
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
      closeProxmoxModal();
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

  const pveEnabled = document.getElementById('add-node-pve-enabled')?.checked ?? false;
  const pveUrl = (document.getElementById('add-node-pve-url')?.value || '').trim();
  const pveNode = (document.getElementById('add-node-pve-node')?.value || '').trim();
  const pveTokenId = (document.getElementById('add-node-pve-token-id')?.value || '').trim();
  const pveTokenSecret = (document.getElementById('add-node-pve-token-secret')?.value || '').trim();
  const pveVerifySSL = document.getElementById('add-node-pve-verify-ssl')?.checked ?? false;

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

  let pveDockerRun = '';
  if (pveEnabled && pveUrl) {
    pveDockerRun = ` \\
  -e PVE_ENABLED="true" \\
  -e PVE_URL="${pveUrl}" \\
  -e PVE_TOKEN_ID="${pveTokenId}" \\
  -e PVE_TOKEN_SECRET="${pveTokenSecret}" \\
  -e PVE_NODE="${pveNode}" \\
  -e PVE_VERIFY_SSL="${pveVerifySSL ? 'true' : 'false'}" \\
  -e PVE_PREFERRED_INTERFACE="eth0"`;
  }

  // 1. Docker Run Command
  const dockerRunCmd = `docker run -d \\
  --name npm-autodiscovery-worker \\
  --restart unless-stopped \\
  -v /var/run/docker.sock:/var/run/docker.sock:ro \\
  -v ./data:/data \\
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
  -e PUSH_INTERVAL="15s"${pveDockerRun} \\
  raddadengineer/npm-autodiscovery:latest`;

  let pveComposeBlock = '';
  if (pveEnabled && pveUrl) {
    pveComposeBlock = `
    environment:
      - PVE_ENABLED=true
      - PVE_URL=\${PVE_URL:-${pveUrl}}
      - PVE_TOKEN_ID=\${PVE_TOKEN_ID:-${pveTokenId}}
      - PVE_TOKEN_SECRET=\${PVE_TOKEN_SECRET:-${pveTokenSecret}}
      - PVE_NODE=\${PVE_NODE:-${pveNode}}
      - PVE_VERIFY_SSL=${pveVerifySSL ? 'true' : 'false'}
      - PVE_PREFERRED_INTERFACE=eth0`;
  }

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
      - /var/run/docker.sock:/var/run/docker.sock:ro
      # Persistent configuration & Proxmox discovery endpoints
      - ./data:/data${pveComposeBlock}`;

  let pveEnvBlock = '';
  if (pveEnabled && pveUrl) {
    pveEnvBlock = `
# Proxmox VE (PVE) LXC Auto-Discovery
PVE_ENABLED=true
PVE_URL=${pveUrl}
PVE_TOKEN_ID=${pveTokenId}
PVE_TOKEN_SECRET=${pveTokenSecret}
PVE_NODE=${pveNode}
PVE_VERIFY_SSL=${pveVerifySSL ? 'true' : 'false'}
PVE_PREFERRED_INTERFACE=eth0
`;
  }

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
${pveEnvBlock}`;

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
      watcherTitle.innerHTML = `<span style="color:var(--accent-emerald);"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-2px; margin-right:4px;"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg>Node Connected & Online!</span>`;
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

/* ==============================================================================
   Proxmox VE LXC Discovery Modal Handlers (Multi-Endpoint Support)
   ============================================================================== */
let activePveNodeId = null;
let nodeProxmoxConfigs = [];
let activePveConfigId = null;

function openProxmoxModal(nodeId) {
  activePveNodeId = nodeId;
  const modal = document.getElementById('proxmox-config-modal');
  if (!modal) {
    console.error('Modal #proxmox-config-modal not found');
    return;
  }

  const badge = document.getElementById('pve-modal-node-badge');
  const resultDiv = document.getElementById('pve-test-result');
  if (badge) badge.textContent = nodeId;
  if (resultDiv) {
    resultDiv.style.display = 'none';
    resultDiv.className = '';
    resultDiv.innerHTML = '';
  }

  // Pre-fill from clusterData if available
  const node = (clusterData || []).find(n => n.node_id === nodeId || (n.is_controller && (nodeId === 'controller-main' || nodeId === 'local')));
  if (node && node.pve_configs && node.pve_configs.length > 0) {
    nodeProxmoxConfigs = JSON.parse(JSON.stringify(node.pve_configs));
  } else if (node && node.pve_url && (node.pve_enabled || (node.overview && node.overview.pve_enabled))) {
    nodeProxmoxConfigs = [{
      id: 'default',
      name: 'Primary Proxmox',
      enabled: true,
      url: node.pve_url,
      node: node.pve_node || '',
      token_id: node.pve_token_id || '',
      preferred_interface: node.pve_preferred_interface || 'eth0',
      allowed_subnets: node.pve_allowed_subnets || '',
      verify_ssl: !!node.pve_verify_ssl,
      has_secret: !!node.pve_has_secret
    }];
  } else {
    nodeProxmoxConfigs = [];
  }

  renderPveEndpointPills();
  if (nodeProxmoxConfigs.length > 0) {
    selectPveEndpoint(nodeProxmoxConfigs[0].id || 'default');
  } else {
    addNewPveEndpoint();
  }

  modal.style.display = 'flex';

  // Fetch freshest configs from server
  fetch(`/api/cluster/nodes/${encodeURIComponent(nodeId)}/proxmox`)
    .then(res => {
      if (!res.ok) throw new Error('Status ' + res.status);
      return res.json();
    })
    .then(data => {
      if (activePveNodeId !== nodeId) return;
      if (data.configs && data.configs.length > 0) {
        nodeProxmoxConfigs = data.configs;
      } else if (data.url && data.enabled) {
        nodeProxmoxConfigs = [{
          id: data.id || 'default',
          name: data.name || 'Primary Proxmox',
          enabled: !!data.enabled,
          url: data.url,
          node: data.node || '',
          token_id: data.token_id || '',
          preferred_interface: data.preferred_interface || 'eth0',
          allowed_subnets: data.allowed_subnets || '',
          verify_ssl: !!data.verify_ssl,
          has_secret: !!data.has_secret
        }];
      } else {
        nodeProxmoxConfigs = [];
      }

      renderPveEndpointPills();
      const currentSelected = nodeProxmoxConfigs.find(c => c.id === activePveConfigId);
      if (currentSelected) {
        selectPveEndpoint(currentSelected.id);
      } else if (nodeProxmoxConfigs.length > 0) {
        selectPveEndpoint(nodeProxmoxConfigs[0].id);
      } else {
        addNewPveEndpoint();
      }
    })
    .catch(err => console.warn('Could not fetch node proxmox configs:', err));
}

function renderPveEndpointPills() {
  const container = document.getElementById('pve-endpoints-pills');
  const countSpan = document.getElementById('pve-endpoints-count');
  if (countSpan) countSpan.textContent = nodeProxmoxConfigs.length;
  if (!container) return;

  container.innerHTML = '';

  if (nodeProxmoxConfigs.length === 0 && activePveConfigId === 'new') {
    const pill = document.createElement('div');
    pill.className = 'pve-endpoint-pill active';
    pill.innerHTML = `<span class="pill-dot"></span><span>New Endpoint</span>`;
    container.appendChild(pill);
    return;
  }

  nodeProxmoxConfigs.forEach((cfg, idx) => {
    const pill = document.createElement('div');
    const isAct = (cfg.id === activePveConfigId);
    pill.className = `pve-endpoint-pill ${isAct ? 'active' : ''} ${cfg.enabled ? 'enabled' : 'disabled'}`;
    const nameStr = cfg.name || cfg.url || `Proxmox #${idx + 1}`;
    pill.innerHTML = `<span class="pill-dot"></span><span>${escapeHtml(nameStr)}</span>`;
    pill.onclick = () => selectPveEndpoint(cfg.id);
    container.appendChild(pill);
  });

  if (activePveConfigId === 'new') {
    const pill = document.createElement('div');
    pill.className = 'pve-endpoint-pill active';
    pill.innerHTML = `<span class="pill-dot"></span><span>＋ New Endpoint</span>`;
    container.appendChild(pill);
  }
}

function selectPveEndpoint(configId) {
  activePveConfigId = configId;
  renderPveEndpointPills();

  const cfg = nodeProxmoxConfigs.find(c => c.id === configId);
  const nameInput = document.getElementById('pve-modal-name');
  const idInput = document.getElementById('pve-modal-endpoint-id');
  const enabledInput = document.getElementById('pve-modal-enabled');
  const urlInput = document.getElementById('pve-modal-url');
  const nodeInput = document.getElementById('pve-modal-target-node');
  const tokenIdInput = document.getElementById('pve-modal-token-id');
  const secretInput = document.getElementById('pve-modal-token-secret');
  const ifaceInput = document.getElementById('pve-modal-interface');
  const subnetsInput = document.getElementById('pve-modal-subnets');
  const verifySSLInput = document.getElementById('pve-modal-verify-ssl');
  const delBtn = document.getElementById('btn-delete-pve-endpoint');
  const resultDiv = document.getElementById('pve-test-result');

  if (resultDiv) {
    resultDiv.style.display = 'none';
    resultDiv.innerHTML = '';
  }

  if (cfg) {
    if (nameInput) nameInput.value = cfg.name || '';
    if (idInput) idInput.value = cfg.id || '';
    if (enabledInput) enabledInput.checked = !!cfg.enabled;
    if (urlInput) urlInput.value = cfg.url || '';
    if (nodeInput) nodeInput.value = cfg.node || '';
    if (tokenIdInput) tokenIdInput.value = cfg.token_id || '';
    if (ifaceInput) ifaceInput.value = cfg.preferred_interface || 'eth0';
    if (subnetsInput) subnetsInput.value = cfg.allowed_subnets || '';
    if (verifySSLInput) verifySSLInput.checked = !!cfg.verify_ssl;

    if (secretInput) {
      secretInput.value = '';
      if (cfg.has_secret) {
        secretInput.placeholder = '•••••••• (leave blank to keep existing secret)';
      } else {
        secretInput.placeholder = 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx';
      }
    }

    if (delBtn) delBtn.style.display = 'inline-flex';
  }
}

function addNewPveEndpoint() {
  activePveConfigId = 'new';
  renderPveEndpointPills();

  const nameInput = document.getElementById('pve-modal-name');
  const idInput = document.getElementById('pve-modal-endpoint-id');
  const enabledInput = document.getElementById('pve-modal-enabled');
  const urlInput = document.getElementById('pve-modal-url');
  const nodeInput = document.getElementById('pve-modal-target-node');
  const tokenIdInput = document.getElementById('pve-modal-token-id');
  const secretInput = document.getElementById('pve-modal-token-secret');
  const ifaceInput = document.getElementById('pve-modal-interface');
  const subnetsInput = document.getElementById('pve-modal-subnets');
  const verifySSLInput = document.getElementById('pve-modal-verify-ssl');
  const delBtn = document.getElementById('btn-delete-pve-endpoint');
  const resultDiv = document.getElementById('pve-test-result');

  if (resultDiv) {
    resultDiv.style.display = 'none';
    resultDiv.innerHTML = '';
  }

  if (nameInput) nameInput.value = `Proxmox Endpoint ${nodeProxmoxConfigs.length + 1}`;
  if (idInput) idInput.value = '';
  if (enabledInput) enabledInput.checked = true;
  if (urlInput) {
    urlInput.value = '';
    urlInput.focus();
  }
  if (nodeInput) nodeInput.value = '';
  if (tokenIdInput) tokenIdInput.value = '';
  if (ifaceInput) ifaceInput.value = 'eth0';
  if (subnetsInput) subnetsInput.value = '';
  if (verifySSLInput) verifySSLInput.checked = false;
  if (secretInput) {
    secretInput.value = '';
    secretInput.placeholder = 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx';
  }

  if (delBtn) delBtn.style.display = 'none';
}

async function testProxmoxConnection() {
  const btn = document.getElementById('btn-test-pve-conn');
  const btnText = document.getElementById('btn-test-pve-conn-text');
  const resultDiv = document.getElementById('pve-test-result');
  const configId = (document.getElementById('pve-modal-endpoint-id')?.value || '').trim();
  const url = (document.getElementById('pve-modal-url')?.value || '').trim();
  const tokenId = (document.getElementById('pve-modal-token-id')?.value || '').trim();
  const secret = (document.getElementById('pve-modal-token-secret')?.value || '').trim();
  const node = (document.getElementById('pve-modal-target-node')?.value || '').trim();
  const iface = (document.getElementById('pve-modal-interface')?.value || '').trim();
  const subnets = (document.getElementById('pve-modal-subnets')?.value || '').trim();
  const verifySSL = document.getElementById('pve-modal-verify-ssl')?.checked ?? false;

  if (!url) {
    if (resultDiv) {
      resultDiv.style.display = 'block';
      resultDiv.className = 'pve-test-error';
      resultDiv.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-2px; margin-right:4px;"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>Please enter the Proxmox VE API URL (e.g. <code>https://192.168.1.100:8006</code>)';
    }
    return;
  }

  if (resultDiv) {
    resultDiv.style.display = 'block';
    resultDiv.className = 'pve-test-loading';
    resultDiv.innerHTML = '<span class="status-dot ping-dot waiting" style="display:inline-block; vertical-align:middle; margin-right:6px;"></span> Connecting to Proxmox VE API...';
  }

  if (btn) btn.disabled = true;
  if (btnText) btnText.textContent = 'Testing...';

  try {
    const res = await fetch('/api/proxmox/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        node_id: activePveNodeId,
        config_id: configId,
        url: url,
        token_id: tokenId,
        token_secret: secret,
        node: node,
        preferred_interface: iface || 'eth0',
        allowed_subnets: subnets,
        verify_ssl: verifySSL
      })
    });
    const data = await res.json();
    if (resultDiv) {
      if (data.success) {
        let contentHtml = `<div style="display:flex; align-items:center; gap:6px;"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"></polyline></svg><strong>${escapeHtml(data.message || 'Connected successfully!')}</strong></div>`;

        if (data.nodes && data.nodes.length > 0) {
          contentHtml += `<div style="margin-top: 0.45rem; font-size: 0.78rem; display: flex; align-items: center; gap: 0.45rem; flex-wrap: wrap;">
            <span style="color: var(--text-muted);">Quick-select node:</span>`;
          data.nodes.forEach(n => {
            contentHtml += `<button type="button" class="badge badge-cyan" style="cursor: pointer; border: 1px solid rgba(56, 189, 248, 0.4); background: rgba(56, 189, 248, 0.15); padding: 3px 8px; border-radius: 4px;" onclick="document.getElementById('pve-modal-target-node').value='${escapeHtml(n)}'" title="Set target node to ${escapeHtml(n)}">${escapeHtml(n)}</button>`;
          });
          contentHtml += `<button type="button" class="badge badge-secondary" style="cursor: pointer; border: 1px solid var(--border-color); background: rgba(255, 255, 255, 0.08); padding: 3px 8px; border-radius: 4px;" onclick="document.getElementById('pve-modal-target-node').value=''" title="Discover all cluster nodes">All Nodes (leave blank)</button>
          </div>`;
        }

        if (data.node_warning) {
          contentHtml += `<div style="margin-top: 0.5rem; padding: 0.5rem 0.75rem; border-radius: 6px; background: rgba(245, 158, 11, 0.15); border: 1px solid rgba(245, 158, 11, 0.4); color: #fbbf24; font-size: 0.8rem; line-height: 1.4; display:flex; align-items:flex-start; gap:6px;">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="flex-shrink:0; margin-top:2px;"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
            <div><strong>Node Mismatch:</strong> ${escapeHtml(data.node_warning)}</div>
          </div>`;
        }

        if (data.warning) {
          contentHtml += `<div style="margin-top: 0.5rem; padding: 0.5rem 0.75rem; border-radius: 6px; background: rgba(239, 68, 68, 0.15); border: 1px solid rgba(239, 68, 68, 0.4); color: #f87171; font-size: 0.8rem; line-height: 1.4; display:flex; align-items:flex-start; gap:6px;">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="flex-shrink:0; margin-top:2px;"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
            <div><strong>Permissions Action Needed:</strong> ${escapeHtml(data.warning)}</div>
          </div>`;
        }

        resultDiv.className = (data.warning || data.node_warning) ? 'pve-test-warning' : 'pve-test-success';
        resultDiv.innerHTML = contentHtml;
      } else {
        resultDiv.className = 'pve-test-error';
        resultDiv.innerHTML = `<div style="display:flex; align-items:center; gap:6px;"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg><strong>Connection failed:</strong> ${escapeHtml(data.error || 'Unknown error')}</div>`;
      }
    }
  } catch (err) {
    if (resultDiv) {
      resultDiv.className = 'pve-test-error';
      resultDiv.innerHTML = `<div style="display:flex; align-items:center; gap:6px;"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg><strong>Request failed:</strong> ${escapeHtml(err.message)}</div>`;
    }
  } finally {
    if (btn) btn.disabled = false;
    if (btnText) btnText.textContent = 'Test Connection';
  }
}

async function saveProxmoxConfig() {
  if (!activePveNodeId) return;

  const btn = document.getElementById('btn-save-pve-config');
  const btnText = document.getElementById('btn-save-pve-config-text');
  let configId = (document.getElementById('pve-modal-endpoint-id')?.value || '').trim();
  const name = (document.getElementById('pve-modal-name')?.value || '').trim();
  const enabled = document.getElementById('pve-modal-enabled')?.checked ?? false;
  const url = (document.getElementById('pve-modal-url')?.value || '').trim();
  const node = (document.getElementById('pve-modal-target-node')?.value || '').trim();
  const tokenId = (document.getElementById('pve-modal-token-id')?.value || '').trim();
  const secret = (document.getElementById('pve-modal-token-secret')?.value || '').trim();
  const iface = (document.getElementById('pve-modal-interface')?.value || '').trim();
  const subnets = (document.getElementById('pve-modal-subnets')?.value || '').trim();
  const verifySSL = document.getElementById('pve-modal-verify-ssl')?.checked ?? false;

  if (enabled && !url) {
    showToast('Proxmox VE API URL is required to enable discovery', 'error');
    return;
  }

  if (activePveConfigId === 'new' || !configId) {
    configId = `pve-${Date.now()}`;
  }

  if (btn) btn.disabled = true;
  if (btnText) btnText.textContent = 'Saving...';

  try {
    const res = await fetch(`/api/cluster/nodes/${encodeURIComponent(activePveNodeId)}/proxmox`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        id: configId,
        name: name || url,
        enabled: enabled,
        url: url,
        token_id: tokenId,
        token_secret: secret,
        node: node,
        preferred_interface: iface || 'eth0',
        allowed_subnets: subnets,
        verify_ssl: verifySSL
      })
    });

    if (!res.ok) {
      const errText = await res.text();
      throw new Error(errText || `Server responded with ${res.status}`);
    }

    const data = await res.json();
    showToast(`Proxmox endpoint "${name || url || configId}" saved successfully!`, 'success');

    // Reload configs for this node
    const getRes = await fetch(`/api/cluster/nodes/${encodeURIComponent(activePveNodeId)}/proxmox`);
    if (getRes.ok) {
      const freshData = await getRes.json();
      nodeProxmoxConfigs = freshData.configs || [];
      activePveConfigId = configId;
      renderPveEndpointPills();
      selectPveEndpoint(configId);
    }

    // Refresh cluster nodes, status overview, and containers
    fetchClusterNodes();
    fetchStatus();
    fetchContainers(true);
    // Trigger immediate sync and scheduled container refreshes
    fetch('/api/sync', { method: 'POST' }).then(() => {
      setTimeout(() => fetchContainers(true), 1500);
      setTimeout(() => fetchContainers(true), 3500);
    }).catch(() => {});
  } catch (err) {
    showToast(`Failed to save Proxmox config: ${err.message}`, 'error');
  } finally {
    if (btn) btn.disabled = false;
    if (btnText) btnText.textContent = 'Save Endpoint';
  }
}

async function deleteCurrentPveEndpoint() {
  if (!activePveNodeId || !activePveConfigId || activePveConfigId === 'new') return;

  const cfg = nodeProxmoxConfigs.find(c => c.id === activePveConfigId);
  const displayName = cfg ? (cfg.name || cfg.url || cfg.id) : activePveConfigId;
  if (!confirm(`Are you sure you want to remove Proxmox endpoint "${displayName}" from node "${activePveNodeId}"?`)) {
    return;
  }

  const btn = document.getElementById('btn-delete-pve-endpoint');
  if (btn) btn.disabled = true;

  try {
    const res = await fetch(`/api/cluster/nodes/${encodeURIComponent(activePveNodeId)}/proxmox?id=${encodeURIComponent(activePveConfigId)}`, {
      method: 'DELETE'
    });

    if (!res.ok) {
      const errText = await res.text();
      throw new Error(errText || `Server responded with ${res.status}`);
    }

    showToast(`Removed Proxmox endpoint "${displayName}"`, 'info');

    // Reload configs
    const getRes = await fetch(`/api/cluster/nodes/${encodeURIComponent(activePveNodeId)}/proxmox`);
    if (getRes.ok) {
      const freshData = await getRes.json();
      if (freshData.configs && freshData.configs.length > 0) {
        nodeProxmoxConfigs = freshData.configs;
      } else if (freshData.url && freshData.enabled) {
        nodeProxmoxConfigs = [{
          id: freshData.id || 'default',
          name: freshData.name || 'Primary Proxmox',
          enabled: true,
          url: freshData.url,
          node: freshData.node || '',
          token_id: freshData.token_id || '',
          preferred_interface: freshData.preferred_interface || 'eth0',
          allowed_subnets: freshData.allowed_subnets || '',
          verify_ssl: !!freshData.verify_ssl,
          has_secret: !!freshData.has_secret
        }];
      } else {
        nodeProxmoxConfigs = [];
      }

      if (nodeProxmoxConfigs.length > 0) {
        selectPveEndpoint(nodeProxmoxConfigs[0].id);
      } else {
        addNewPveEndpoint();
      }
    }

    fetchClusterNodes();
    fetchStatus();
    fetch('/api/sync', { method: 'POST' }).catch(() => {});
  } catch (err) {
    showToast(`Failed to delete Proxmox endpoint: ${err.message}`, 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}

function closeProxmoxModal() {
  const modal = document.getElementById('proxmox-config-modal');
  if (modal) modal.style.display = 'none';
  activePveNodeId = null;
  activePveConfigId = null;
  nodeProxmoxConfigs = [];
}

window.openProxmoxModal = openProxmoxModal;
window.closeProxmoxModal = closeProxmoxModal;
window.addNewPveEndpoint = addNewPveEndpoint;
window.deleteCurrentPveEndpoint = deleteCurrentPveEndpoint;
window.selectPveEndpoint = selectPveEndpoint;
window.setDocsMode = setDocsMode;
window.filterDocsCategory = filterDocsCategory;
window.clearDocsSearch = clearDocsSearch;
window.fetchContainers = fetchContainers;

function initProxmoxModal() {
  const modal = document.getElementById('proxmox-config-modal');
  if (modal) {
    modal.addEventListener('click', (e) => {
      if (e.target === modal) closeProxmoxModal();
    });
  }

  const btnTest = document.getElementById('btn-test-pve-conn');
  if (btnTest) btnTest.addEventListener('click', testProxmoxConnection);

  const btnSave = document.getElementById('btn-save-pve-config');
  if (btnSave) btnSave.addEventListener('click', saveProxmoxConfig);

  const btnToggleSecret = document.getElementById('btn-toggle-pve-secret');
  if (btnToggleSecret) {
    btnToggleSecret.addEventListener('click', () => {
      const secretInput = document.getElementById('pve-modal-token-secret');
      if (!secretInput) return;
      if (secretInput.type === 'password') {
        secretInput.type = 'text';
        btnToggleSecret.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"></path><line x1="1" y1="1" x2="23" y2="23"></line></svg>';
      } else {
        secretInput.type = 'password';
        btnToggleSecret.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path><circle cx="12" cy="12" r="3"></circle></svg>';
      }
    });
  }
}

// ==============================================================================
// NPM Metrics, Traffic & Response Time Observability Engine
// ==============================================================================
let currentMetricsPeriod = '24h';
let currentMetricsHost = 'all';
let currentMetricsClassFilter = null; // '2xx' | '3xx' | '4xx' | '5xx' | null
let currentMetricsCodeFilter = null;  // number (e.g. 502) | null
let currentRequestFilterType = 'all'; // 'all' | 'errors' | '5xx' | '4xx'
let latestNPMMetricsData = null;
let latestRequestEvents = [];
let isFetchingMetrics = false;

function filterByStatusClass(cls) {
  if (currentMetricsClassFilter === cls && currentMetricsCodeFilter === null) {
    clearStatusDrillDown();
    return;
  }
  currentMetricsClassFilter = cls;
  currentMetricsCodeFilter = null;
  if (latestNPMMetricsData) {
    renderNPMMetricsDashboard(latestNPMMetricsData);
  }
}

function filterByStatusCode(code, cls) {
  if (currentMetricsCodeFilter === code) {
    clearStatusDrillDown();
    return;
  }
  currentMetricsCodeFilter = code;
  currentMetricsClassFilter = cls || (code ? Math.floor(code / 100) + 'xx' : null);
  if (latestNPMMetricsData) {
    renderNPMMetricsDashboard(latestNPMMetricsData);
  }
}

function clearStatusDrillDown() {
  if (!currentMetricsClassFilter && !currentMetricsCodeFilter) return;
  currentMetricsClassFilter = null;
  currentMetricsCodeFilter = null;
  if (latestNPMMetricsData) {
    renderNPMMetricsDashboard(latestNPMMetricsData);
  }
}

function formatBytes(bytes) {
  if (!bytes || bytes <= 0 || isNaN(bytes)) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(i > 0 ? 1 : 0)) + ' ' + sizes[i];
}

function initNPMMetricsTab() {
  // 1. Period segmented button pills (24h, 7d, 30d)
  document.querySelectorAll('.npm-period-pill').forEach(btn => {
    btn.addEventListener('click', () => {
      const period = btn.getAttribute('data-period');
      if (!period || period === currentMetricsPeriod) return;
      document.querySelectorAll('.npm-period-pill').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      currentMetricsPeriod = period;
      fetchNPMMetrics(false);
    });
  });

  // 2. Host dropdown filter
  const hostSelect = document.getElementById('npm-metrics-host-filter');
  if (hostSelect) {
    hostSelect.addEventListener('change', () => {
      currentMetricsHost = hostSelect.value || 'all';
      fetchNPMMetrics(false);
    });
  }

  // 3. Manual probe button
  const btnProbe = document.getElementById('btn-probe-npm-metrics');
  if (btnProbe) {
    btnProbe.addEventListener('click', probeNPMMetricsNow);
  }

  // 4. Live filter search in the hosts metrics table
  const searchInput = document.getElementById('npm-metrics-hosts-search');
  if (searchInput) {
    searchInput.addEventListener('input', () => {
      if (latestNPMMetricsData && Array.isArray(latestNPMMetricsData.hosts)) {
        renderNPMHostsTable(latestNPMMetricsData.hosts);
      }
    });
  }

  // 5. Clear drill-down button
  const btnClearDrill = document.getElementById('btn-clear-drilldown');
  if (btnClearDrill) {
    btnClearDrill.addEventListener('click', clearStatusDrillDown);
  }

  // 6. Request search filter input
  const reqSearchInput = document.getElementById('npm-requests-search');
  if (reqSearchInput) {
    reqSearchInput.addEventListener('input', () => {
      renderNPMRequestsTable(latestRequestEvents);
    });
  }

  // 7. Request detail modal backdrop close
  const reqModal = document.getElementById('request-detail-modal');
  if (reqModal) {
    reqModal.addEventListener('click', (e) => {
      if (e.target === reqModal) {
        closeRequestDetailModal();
      }
    });
  }
}

async function probeNPMMetricsNow() {
  const btn = document.getElementById('btn-probe-npm-metrics');
  const originalHTML = btn ? btn.innerHTML : '';
  if (btn) {
    btn.disabled = true;
    btn.classList.add('spinning');
    btn.innerHTML = `
      <svg class="spin" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="10" stroke-opacity="0.25"></circle>
        <path d="M12 2a10 10 0 0 1 10 10"></path>
      </svg>
      <span>Probing...</span>
    `;
  }

  try {
    const res = await fetch('/api/metrics/probe', { method: 'POST' });
    if (!res.ok) {
      throw new Error(`Server responded with ${res.status}`);
    }
    const data = await res.json();
    showToast(data.message || 'Probed NPM proxy hosts successfully', 'success');
    await fetchNPMMetrics(true);
  } catch (err) {
    console.error('NPM metrics probe failed:', err);
    showToast(`Probe failed: ${err.message}`, 'error');
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.classList.remove('spinning');
      btn.innerHTML = originalHTML;
    }
  }
}

async function fetchNPMMetrics(forceProbe = false) {
  if (isFetchingMetrics) return;
  isFetchingMetrics = true;

  try {
    const query = new URLSearchParams({
      period: currentMetricsPeriod,
      host: currentMetricsHost
    });
    if (forceProbe) {
      query.set('probe', 'true');
    }

    const res = await fetch(`/api/metrics/overview?${query.toString()}`);
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }

    const data = await res.json();
    latestNPMMetricsData = data;
    renderNPMMetricsDashboard(data);
  } catch (err) {
    console.error('Failed to fetch NPM metrics dashboard data:', err);
    if (activeTab === 'metrics-view') {
      const srcText = document.getElementById('npm-metrics-source-text');
      if (srcText) srcText.textContent = 'Telemetry Offline';
    }
  } finally {
    isFetchingMetrics = false;
  }
}

function renderNPMMetricsDashboard(data) {
  if (!data) return;

  // 1. Data Source & Telemetry Badge
  const srcText = document.getElementById('npm-metrics-source-text');
  if (srcText && data.data_source) {
    srcText.textContent = data.data_source;
  }

  // 2. Available Hosts Dropdown
  const hostSelect = document.getElementById('npm-metrics-host-filter');
  if (hostSelect && Array.isArray(data.available_hosts)) {
    const currentVal = currentMetricsHost;
    let html = '<option value="all">All hosts</option>';
    data.available_hosts.forEach(h => {
      const isSel = (h === currentVal) ? 'selected' : '';
      html += `<option value="${escapeHtml(h)}" ${isSel}>${escapeHtml(h)}</option>`;
    });
    hostSelect.innerHTML = html;
    if (currentVal && (currentVal === 'all' || data.available_hosts.includes(currentVal))) {
      hostSelect.value = currentVal;
    } else {
      hostSelect.value = 'all';
      currentMetricsHost = 'all';
    }
  }

  // 3. Summary KPI Cards
  const sum = data.summary || {};
  const elReq = document.getElementById('npm-kpi-requests');
  if (elReq) elReq.textContent = (sum.total_requests || 0).toLocaleString();
  const elReqSub = document.getElementById('npm-kpi-requests-sub');
  if (elReqSub) elReqSub.textContent = `${sum.active_hosts || 0} active NPM hosts`;

  const elTraffic = document.getElementById('npm-kpi-traffic');
  if (elTraffic) elTraffic.textContent = formatBytes(sum.total_traffic_bytes || 0);
  const elTrafficSub = document.getElementById('npm-kpi-traffic-sub');
  if (elTrafficSub) elTrafficSub.textContent = `${sum.ssl_hosts || 0} SSL/TLS hosts enabled`;

  const elErrPct = document.getElementById('npm-kpi-errors-pct');
  if (elErrPct) elErrPct.textContent = `${(sum.server_error_rate_pct || 0).toFixed(1)} %`;
  const elErrSub = document.getElementById('npm-kpi-errors-sub');
  if (elErrSub) elErrSub.textContent = `${(sum.server_errors_5xx || 0).toLocaleString()} requests`;

  const elLat = document.getElementById('npm-kpi-latency');
  if (elLat) elLat.textContent = `${sum.avg_response_time_ms || 0} ms`;
  const elLatSub = document.getElementById('npm-kpi-latency-sub');
  if (elLatSub) elLatSub.textContent = `max. ${sum.max_response_time_ms || 0} ms`;

  // Update Top Dashboard Overview Metrics Cards
  const elOverviewReq = document.getElementById('overview-metrics-requests');
  if (elOverviewReq) elOverviewReq.textContent = (sum.total_requests || 0).toLocaleString();

  const elOverviewTraffic = document.getElementById('overview-metrics-traffic');
  if (elOverviewTraffic) elOverviewTraffic.textContent = formatBytes(sum.total_traffic_bytes || 0);

  const elOverviewLat = document.getElementById('overview-metrics-latency');
  if (elOverviewLat) elOverviewLat.textContent = `${sum.avg_response_time_ms || 0} ms`;

  const elOverviewErrRate = document.getElementById('overview-metrics-error-rate');
  if (elOverviewErrRate) elOverviewErrRate.textContent = `${(sum.server_error_rate_pct || 0).toFixed(1)}%`;

  // 4. NPM Configuration & Security Posture Strip
  const post = data.npm_posture || {};
  const postHosts = document.getElementById('npm-posture-hosts');
  if (postHosts) postHosts.textContent = `${post.auto_discovered_hosts || 0} Auto · ${post.manual_hosts || 0} Manual`;

  const postOnline = document.getElementById('npm-posture-online');
  if (postOnline) postOnline.textContent = `${sum.online_hosts || 0} Online · ${sum.offline_hosts || 0} Offline`;

  const postSsl = document.getElementById('npm-posture-ssl');
  if (postSsl) postSsl.textContent = `${post.ssl_enabled_count || 0} SSL · ${post.ssl_forced_count || 0} Forced`;

  const postSec = document.getElementById('npm-posture-security');
  if (postSec) postSec.textContent = `${post.http2_count || 0} HTTP/2 · ${post.block_exploits_count || 0} Block Exploits`;

  const postCerts = document.getElementById('npm-posture-certs');
  if (postCerts) {
    const exp = sum.expiring_certificates || 0;
    postCerts.textContent = `${sum.total_certificates || 0} Active${exp > 0 ? ` · ${exp} Expiring` : ''}`;
  }

  // 5. Main Bar Chart (Requests per hour/day)
  const chartTitle = document.getElementById('npm-requests-chart-title');
  const chartSub = document.getElementById('npm-requests-chart-subtitle');
  const chartTag = document.getElementById('npm-chart-drilldown-tag');
  const chartTagText = document.getElementById('npm-chart-drilldown-text');

  const statusCodes = Array.isArray(data.status_codes) ? data.status_codes : [];
  const classes = data.status_classes || {};
  const c2xx = classes['2xx'] || { count: 0, percent: 0, peak: 0, series: [] };
  const c3xx = classes['3xx'] || { count: 0, percent: 0, peak: 0, series: [] };
  const c4xx = classes['4xx'] || { count: 0, percent: 0, peak: 0, series: [] };
  const c5xx = classes['5xx'] || { count: 0, percent: 0, peak: 0, series: [] };

  const timeline = Array.isArray(data.timeline) ? data.timeline : [];

  let isFiltered = false;
  let activeFilterLabel = '';
  let activeFilterClass = '';
  let filteredSeries = null;
  let filteredMax = 0;

  if (currentMetricsCodeFilter) {
    const codeItem = statusCodes.find(c => c.code === currentMetricsCodeFilter);
    isFiltered = true;
    activeFilterClass = (codeItem && codeItem.class) || currentMetricsClassFilter || '2xx';
    const codeTitle = codeItem ? codeItem.title : '';
    activeFilterLabel = `HTTP ${currentMetricsCodeFilter}${codeTitle ? ` · ${codeTitle}` : ''}`;
    if (codeItem && Array.isArray(codeItem.series)) {
      filteredSeries = codeItem.series;
      filteredMax = codeItem.peak || 0;
    }
  } else if (currentMetricsClassFilter) {
    const classData = classes[currentMetricsClassFilter] || {};
    isFiltered = true;
    activeFilterClass = currentMetricsClassFilter;
    const classTitles = {
      '2xx': '2xx Success',
      '3xx': '3xx Redirect',
      '4xx': '4xx Client error',
      '5xx': '5xx Server error'
    };
    activeFilterLabel = classTitles[currentMetricsClassFilter] || currentMetricsClassFilter;
    if (Array.isArray(classData.series)) {
      filteredSeries = classData.series;
      filteredMax = classData.peak || 0;
    }
  }

  // Update chart header and tags
  const baseTitle = currentMetricsPeriod === '24h' ? 'Requests per hour' : 'Requests per day';
  if (chartTitle) {
    if (isFiltered) {
      chartTitle.textContent = `${baseTitle} · ${activeFilterLabel}`;
    } else {
      chartTitle.textContent = baseTitle;
    }
  }
  if (chartSub) {
    const subMap = { '24h': 'Last 24 hours', '7d': 'Last 7 days', '30d': 'Last 30 days' };
    const baseSub = subMap[currentMetricsPeriod] || 'Selected period';
    if (isFiltered) {
      chartSub.textContent = `${baseSub} · Filtered time series`;
    } else {
      chartSub.textContent = baseSub;
    }
  }

  if (chartTag && chartTagText) {
    if (isFiltered) {
      chartTag.style.display = 'inline-flex';
      chartTag.className = `npm-chart-filter-tag tag-${activeFilterClass}`;
      chartTagText.textContent = activeFilterLabel;
    } else {
      chartTag.style.display = 'none';
    }
  }

  // Update Status codes section drill-down badge
  const drillWrap = document.getElementById('npm-drilldown-active-wrap');
  const drillLabel = document.getElementById('npm-drilldown-label');
  if (drillWrap && drillLabel) {
    if (isFiltered) {
      drillWrap.style.display = 'inline-flex';
      drillWrap.className = `npm-drilldown-badge badge-${activeFilterClass}`;
      drillLabel.textContent = activeFilterLabel;
    } else {
      drillWrap.style.display = 'none';
    }
  }

  // Calculate chart max scale
  let maxReq = 0;
  if (isFiltered) {
    if (filteredMax > 0) {
      maxReq = filteredMax;
    } else if (filteredSeries) {
      filteredSeries.forEach(v => { if (v > maxReq) maxReq = v; });
    }
  } else {
    timeline.forEach(slot => {
      if (slot.requests > maxReq) maxReq = slot.requests;
    });
  }

  // Y-Axis labels
  const yAxis = document.getElementById('npm-requests-y-axis');
  if (yAxis) {
    const topY = maxReq > 0 ? maxReq : 10;
    const midY = Math.round(topY / 2);
    yAxis.innerHTML = `
      <span>${topY >= 1000 ? (topY / 1000).toFixed(1) + 'k' : topY}</span>
      <span>${midY >= 1000 ? (midY / 1000).toFixed(1) + 'k' : midY}</span>
      <span>0</span>
    `;
  }

  // Bars Stage
  const barsStage = document.getElementById('npm-requests-bar-chart');
  if (barsStage) {
    if (timeline.length === 0) {
      barsStage.innerHTML = '<div style="width:100%; text-align:center; color:var(--text-muted); font-size:0.85rem; padding-top:60px;">No requests recorded in this period</div>';
    } else {
      let barsHtml = '';
      timeline.forEach((slot, i) => {
        let req = slot.requests || 0;
        let barClass = '';
        let tip = '';

        if (isFiltered) {
          barClass = `bar-filter-${activeFilterClass}`;
          if (filteredSeries && i < filteredSeries.length) {
            req = filteredSeries[i];
          } else if (currentMetricsClassFilter) {
            req = slot['status_' + currentMetricsClassFilter] || 0;
          }
          const totalSlotReq = slot.requests || 0;
          const sharePct = totalSlotReq > 0 ? ((req / totalSlotReq) * 100).toFixed(1) : '0.0';
          tip = `${slot.full_label || slot.label}: ${req.toLocaleString()} reqs (${sharePct}% of total ${totalSlotReq.toLocaleString()}) · ${activeFilterLabel}`;
        } else {
          tip = `${slot.full_label || slot.label}: ${req.toLocaleString()} reqs (${formatBytes(slot.traffic_bytes || 0)}) · ${slot.avg_time_ms || 0}ms avg`;
        }

        const pct = maxReq > 0 ? Math.max(3, Math.round((req / maxReq) * 100)) : 3;
        barsHtml += `
          <div class="npm-bar-col" title="${escapeHtml(tip)}">
            <div class="npm-bar-fill ${barClass}" style="height: ${pct}%;"></div>
          </div>
        `;
      });
      barsStage.innerHTML = barsHtml;
    }
  }

  // X-Axis tick labels
  const xAxis = document.getElementById('npm-requests-x-axis');
  if (xAxis) {
    let xHtml = '';
    const totalSlots = timeline.length;
    let step = 1;
    if (totalSlots > 20) step = 3;
    else if (totalSlots > 10) step = 2;

    timeline.forEach((slot, i) => {
      const showLabel = (i % step === 0) || (i === totalSlots - 1);
      const text = showLabel ? (slot.label || '') : '';
      xHtml += `<div class="npm-x-tick" title="${escapeHtml(slot.full_label || slot.label)}">${escapeHtml(text)}</div>`;
    });
    xAxis.innerHTML = xHtml;
  }

  // Timeline Table ("As table" disclosure)
  const timeTbody = document.getElementById('npm-timeline-table-tbody');
  if (timeTbody) {
    let tHtml = '';
    timeline.forEach(slot => {
      tHtml += `
        <tr>
          <td><strong>${escapeHtml(slot.full_label || slot.label)}</strong></td>
          <td style="text-align:right; font-family:var(--font-mono);">${(slot.requests || 0).toLocaleString()}</td>
          <td style="text-align:right; font-family:var(--font-mono);">${formatBytes(slot.traffic_bytes || 0)}</td>
          <td style="text-align:right; font-family:var(--font-mono); color:#22c55e;">${(slot.status_2xx || 0).toLocaleString()}</td>
          <td style="text-align:right; font-family:var(--font-mono); color:#94a3b8;">${(slot.status_3xx || 0).toLocaleString()}</td>
          <td style="text-align:right; font-family:var(--font-mono); color:#f59e0b;">${(slot.status_4xx || 0).toLocaleString()}</td>
          <td style="text-align:right; font-family:var(--font-mono); color:#ef4444;">${(slot.status_5xx || 0).toLocaleString()}</td>
          <td style="text-align:right; font-family:var(--font-mono);">${slot.avg_time_ms || 0} ms</td>
        </tr>
      `;
    });
    timeTbody.innerHTML = tHtml || '<tr><td colspan="8" style="text-align:center; color:var(--text-muted);">No timeline data</td></tr>';
  }

  // 6. Status Codes Section & Stacked Distribution Bar
  const stackedBar = document.getElementById('npm-status-stacked-bar');
  if (stackedBar) {
    const dim2 = (currentMetricsClassFilter && currentMetricsClassFilter !== '2xx') ? 'dimmed' : '';
    const dim3 = (currentMetricsClassFilter && currentMetricsClassFilter !== '3xx') ? 'dimmed' : '';
    const dim4 = (currentMetricsClassFilter && currentMetricsClassFilter !== '4xx') ? 'dimmed' : '';
    const dim5 = (currentMetricsClassFilter && currentMetricsClassFilter !== '5xx') ? 'dimmed' : '';

    stackedBar.innerHTML = `
      <div class="npm-stacked-seg seg-2xx ${dim2}" style="width: ${c2xx.percent}%;" title="2xx: ${c2xx.percent}% · Click to filter" onclick="filterByStatusClass('2xx')"></div>
      <div class="npm-stacked-seg seg-3xx ${dim3}" style="width: ${c3xx.percent}%;" title="3xx: ${c3xx.percent}% · Click to filter" onclick="filterByStatusClass('3xx')"></div>
      <div class="npm-stacked-seg seg-4xx ${dim4}" style="width: ${c4xx.percent}%;" title="4xx: ${c4xx.percent}% · Click to filter" onclick="filterByStatusClass('4xx')"></div>
      <div class="npm-stacked-seg seg-5xx ${dim5}" style="width: ${c5xx.percent}%;" title="5xx: ${c5xx.percent}% · Click to filter" onclick="filterByStatusClass('5xx')"></div>
    `;
  }

  const legendContainer = document.getElementById('npm-status-stacked-legend');
  if (legendContainer) {
    legendContainer.innerHTML = `
      <div class="npm-legend-item ${currentMetricsClassFilter === '2xx' ? 'selected-legend' : (currentMetricsClassFilter ? 'dimmed' : '')}" onclick="filterByStatusClass('2xx')" title="Click to filter by 2xx Success">
        <div class="npm-legend-title">
          <span class="npm-cls-icon cls-2xx">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="20 6 9 17 4 12"></polyline>
            </svg>
          </span>
          2xx · Success
        </div>
        <div class="npm-legend-val" id="npm-legend-2xx">${c2xx.count.toLocaleString()} · ${c2xx.percent} %</div>
      </div>
      <div class="npm-legend-item ${currentMetricsClassFilter === '3xx' ? 'selected-legend' : (currentMetricsClassFilter ? 'dimmed' : '')}" onclick="filterByStatusClass('3xx')" title="Click to filter by 3xx Redirect">
        <div class="npm-legend-title">
          <span class="npm-cls-icon cls-3xx">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
              <line x1="7" y1="17" x2="17" y2="7"></line>
              <polyline points="7 7 17 7 17 17"></polyline>
            </svg>
          </span>
          3xx · Redirect
        </div>
        <div class="npm-legend-val" id="npm-legend-3xx">${c3xx.count.toLocaleString()} · ${c3xx.percent} %</div>
      </div>
      <div class="npm-legend-item ${currentMetricsClassFilter === '4xx' ? 'selected-legend' : (currentMetricsClassFilter ? 'dimmed' : '')}" onclick="filterByStatusClass('4xx')" title="Click to filter by 4xx Client error">
        <div class="npm-legend-title">
          <span class="npm-cls-icon cls-4xx">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"></path>
              <line x1="12" y1="9" x2="12" y2="13"></line>
              <line x1="12" y1="17" x2="12.01" y2="17"></line>
            </svg>
          </span>
          4xx · Client error
        </div>
        <div class="npm-legend-val" id="npm-legend-4xx">${c4xx.count.toLocaleString()} · ${c4xx.percent} %</div>
      </div>
      <div class="npm-legend-item ${currentMetricsClassFilter === '5xx' ? 'selected-legend' : (currentMetricsClassFilter ? 'dimmed' : '')}" onclick="filterByStatusClass('5xx')" title="Click to filter by 5xx Server error">
        <div class="npm-legend-title">
          <span class="npm-cls-icon cls-5xx">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10"></circle>
              <line x1="15" y1="9" x2="9" y2="15"></line>
              <line x1="9" y1="9" x2="15" y2="15"></line>
            </svg>
          </span>
          5xx · Server error
        </div>
        <div class="npm-legend-val" id="npm-legend-5xx">${c5xx.count.toLocaleString()} · ${c5xx.percent} %</div>
      </div>
    `;
  }

  // 7. Status Class Cards (4 cards with sparklines)
  const classGrid = document.getElementById('npm-class-cards-grid');
  if (classGrid) {
    const classConfigs = [
      { key: '2xx', title: '2xx · Success', icon: '<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>', cls: '2xx', data: c2xx },
      { key: '3xx', title: '3xx · Redirect', icon: '<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><line x1="7" y1="17" x2="17" y2="7"></line><polyline points="7 7 17 7 17 17"></polyline></svg>', cls: '3xx', data: c3xx },
      { key: '4xx', title: '4xx · Client error', icon: '<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>', cls: '4xx', data: c4xx },
      { key: '5xx', title: '5xx · Server error', icon: '<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>', cls: '5xx', data: c5xx }
    ];

    let cHtml = '';
    classConfigs.forEach(cfg => {
      const cd = cfg.data;
      const peak = cd.peak || 0;
      const series = Array.isArray(cd.series) ? cd.series : [];
      let sparkHtml = '';
      series.forEach(val => {
        const hPct = peak > 0 ? Math.max(4, Math.round((val / peak) * 100)) : 4;
        sparkHtml += `<div class="npm-spark-bar bar-${cfg.cls}" style="height:${hPct}%;" title="${val.toLocaleString()}"></div>`;
      });

      let cardStateClass = '';
      let cardTooltip = `Click to drill down into ${cfg.title}`;
      if (currentMetricsClassFilter === cfg.cls) {
        cardStateClass = `selected-class-card selected-${cfg.cls}`;
        cardTooltip = `Active filter ${cfg.title} · Click to reset`;
      } else if (currentMetricsClassFilter) {
        cardStateClass = 'dimmed';
      }

      cHtml += `
        <div class="npm-class-card glass-panel ${cardStateClass}" onclick="filterByStatusClass('${cfg.cls}')" title="${cardTooltip}">
          <div class="npm-class-card-top">
            <span class="npm-class-card-title">
              <span class="npm-cls-icon cls-${cfg.cls}">${cfg.icon}</span>
              ${cfg.title}
            </span>
            <span class="npm-class-card-peak">Peak ${peak.toLocaleString()}</span>
          </div>
          <div class="npm-class-card-count">${(cd.count || 0).toLocaleString()}</div>
          <div class="npm-sparkline-bars">
            ${sparkHtml || '<div class="npm-spark-bar bar-' + cfg.cls + '" style="height:4%;"></div>'}
          </div>
        </div>
      `;
    });
    classGrid.innerHTML = cHtml;
  }

  // 8. Individual Status Code Cards & Table
  const codesGrid = document.getElementById('npm-status-codes-grid');
  if (codesGrid) {
    if (statusCodes.length === 0) {
      codesGrid.innerHTML = '<div style="grid-column: 1 / -1; padding: 1.5rem; text-align: center; color: var(--text-muted);">No HTTP status codes recorded in this time range</div>';
    } else {
      let sHtml = '';
      statusCodes.forEach(item => {
        const peak = item.peak || 0;
        const series = Array.isArray(item.series) ? item.series : [];
        let sparkHtml = '';
        series.forEach(val => {
          const hPct = peak > 0 ? Math.max(4, Math.round((val / peak) * 100)) : 4;
          sparkHtml += `<div class="npm-spark-bar bar-${item.class}" style="height:${hPct}%;" title="${val.toLocaleString()}"></div>`;
        });

        let codeStateClass = '';
        let codeTooltip = `Click to drill down into HTTP ${item.code} (${escapeHtml(item.title)})`;
        if (currentMetricsCodeFilter === item.code) {
          codeStateClass = `selected-code-card selected-${item.class}`;
          codeTooltip = `Active filter HTTP ${item.code} · Click to reset`;
        } else if (currentMetricsCodeFilter) {
          codeStateClass = 'dimmed';
        } else if (currentMetricsClassFilter) {
          if (item.class === currentMetricsClassFilter) {
            codeStateClass = `selected-${item.class}`;
          } else {
            codeStateClass = 'dimmed';
          }
        }

        sHtml += `
          <div class="npm-code-card ${codeStateClass}" onclick="filterByStatusCode(${item.code}, '${item.class}')" title="${codeTooltip}">
            <div class="npm-code-card-top">
              <span class="npm-code-num">${item.code}</span>
              <span class="npm-code-pct">${item.percent}%</span>
            </div>
            <div class="npm-code-desc">${escapeHtml(item.title)}</div>
            <div class="npm-sparkline-bars">
              ${sparkHtml || '<div class="npm-spark-bar bar-' + item.class + '" style="height:4%;"></div>'}
            </div>
            <div class="npm-code-total">${item.count.toLocaleString()} requests</div>
          </div>
        `;
      });
      codesGrid.innerHTML = sHtml;
    }
  }

  const codesTbody = document.getElementById('npm-status-codes-table-tbody');
  if (codesTbody) {
    let ctHtml = '';
    statusCodes.forEach(item => {
      let rowClass = '';
      if (currentMetricsCodeFilter === item.code) {
        rowClass = 'selected-status-row';
      } else if (currentMetricsCodeFilter) {
        rowClass = 'dimmed-row';
      } else if (currentMetricsClassFilter) {
        if (item.class !== currentMetricsClassFilter) {
          rowClass = 'dimmed-row';
        }
      }

      ctHtml += `
        <tr data-status-code="${item.code}" class="${rowClass}" onclick="filterByStatusCode(${item.code}, '${item.class}')" title="Click to filter by HTTP ${item.code}">
          <td><strong style="font-family:var(--font-mono);">${item.code}</strong></td>
          <td><span class="npm-feature-pill pill-${item.class}">${item.class}</span></td>
          <td>${escapeHtml(item.title)}</td>
          <td style="text-align:right; font-family:var(--font-mono);">${item.count.toLocaleString()}</td>
          <td style="text-align:right; font-family:var(--font-mono);">${item.percent}%</td>
          <td style="text-align:right; font-family:var(--font-mono);">${(item.peak || 0).toLocaleString()}</td>
        </tr>
      `;
    });
    codesTbody.innerHTML = ctHtml || '<tr><td colspan="6" style="text-align:center; color:var(--text-muted);">No status code records</td></tr>';
  }

  // 9. Hosts Metrics Table
  renderNPMHostsTable(data.hosts || []);

  // 10. Request Logs & Error Diagnostics Table
  latestRequestEvents = Array.isArray(data.recent_requests) ? data.recent_requests : [];
  renderNPMRequestsTable(latestRequestEvents);
}

function renderNPMHostsTable(hosts) {
  const tbody = document.getElementById('npm-metrics-hosts-tbody');
  if (!tbody) return;

  const searchInput = document.getElementById('npm-metrics-hosts-search');
  const term = (searchInput ? searchInput.value : '').toLowerCase().trim();

  // Dynamic subtitle feedback based on active drill-down
  const subtitleEl = document.getElementById('npm-hosts-table-subtitle');
  if (subtitleEl) {
    if (currentMetricsCodeFilter) {
      subtitleEl.innerHTML = `Showing hosts with HTTP ${currentMetricsCodeFilter} activity. <a href="javascript:void(0)" onclick="clearStatusDrillDown()" style="color:var(--accent-primary); text-decoration:underline; font-weight:600; margin-left:0.35rem;">Clear filter</a>`;
    } else if (currentMetricsClassFilter) {
      const clsName = { '2xx': '2xx Success', '3xx': '3xx Redirect', '4xx': '4xx Client error', '5xx': '5xx Server error' }[currentMetricsClassFilter] || currentMetricsClassFilter;
      subtitleEl.innerHTML = `Showing hosts with ${clsName} activity. <a href="javascript:void(0)" onclick="clearStatusDrillDown()" style="color:var(--accent-primary); text-decoration:underline; font-weight:600; margin-left:0.35rem;">Clear filter</a>`;
    } else {
      subtitleEl.textContent = 'Click any host row to filter metrics above, or click "All hosts" to reset.';
    }
  }

  const filtered = hosts.filter(h => {
    // 1. Text search filter
    if (term) {
      const match = (h.domain && h.domain.toLowerCase().includes(term)) ||
                    (h.target && h.target.toLowerCase().includes(term)) ||
                    (h.node_id && h.node_id.toLowerCase().includes(term)) ||
                    (h.source && h.source.toLowerCase().includes(term));
      if (!match) return false;
    }

    // 2. Status code or status class drill-down filter
    if (currentMetricsCodeFilter) {
      if (h.latest_status === currentMetricsCodeFilter) return true;
      if (currentMetricsClassFilter && (h['status_' + currentMetricsClassFilter] > 0)) return true;
      return false;
    } else if (currentMetricsClassFilter) {
      return (h['status_' + currentMetricsClassFilter] || 0) > 0;
    }

    return true;
  });

  if (filtered.length === 0) {
    const filterReason = currentMetricsCodeFilter ? `HTTP ${currentMetricsCodeFilter}` : (currentMetricsClassFilter || 'search term');
    tbody.innerHTML = `
      <tr>
        <td colspan="7" style="text-align:center; padding: 2rem; color:var(--text-muted);">
          No hosts match the active filter (${escapeHtml(filterReason)}).
          <button type="button" class="btn-clear-drilldown" onclick="clearStatusDrillDown()" style="margin-left:0.75rem;">Clear drill-down</button>
        </td>
      </tr>
    `;
    return;
  }

  let html = '';
  filtered.forEach(h => {
    const isSelected = (currentMetricsHost && currentMetricsHost.toLowerCase() === h.domain.toLowerCase());
    const selClass = isSelected ? 'selected-host-row' : '';

    let featurePills = '';
    if (h.ssl_enabled) featurePills += '<span class="npm-feature-pill pill-ssl" title="SSL/TLS Enabled">SSL</span>';
    if (h.ssl_forced) featurePills += '<span class="npm-feature-pill pill-ssl" title="HTTP to HTTPS Redirect">Force SSL</span>';
    if (h.http2_support) featurePills += '<span class="npm-feature-pill pill-http2" title="HTTP/2 Protocol Enabled">HTTP/2</span>';
    if (h.block_exploits) featurePills += '<span class="npm-feature-pill pill-waf" title="Block Common Exploits (WAF)">WAF</span>';
    if (h.websocket) featurePills += '<span class="npm-feature-pill" title="WebSockets Support">WS</span>';
    if (h.caching) featurePills += '<span class="npm-feature-pill" title="Assets Caching">Cache</span>';
    if (!featurePills) featurePills = '<span style="color:var(--text-muted); font-size:0.75rem;">—</span>';

    const statusDotClass = h.online ? 'online' : 'error';
    const errColor = (h.status_5xx > 0) ? '#ef4444' : 'var(--text-muted)';

    html += `
      <tr class="${selClass}" onclick="toggleHostMetricsFilter('${escapeHtml(h.domain)}')">
        <td>
          <div style="display:flex; align-items:center; gap:0.5rem;">
            <span class="status-dot ${statusDotClass}" title="${h.online ? 'Online' : 'Offline'}"></span>
            <div>
              <strong style="color:var(--text-primary); font-size:0.88rem;">${escapeHtml(h.domain)}</strong>
              <div style="font-size:0.72rem; color:var(--text-muted);">${escapeHtml(h.node_id || 'controller')} · ${escapeHtml(h.source || 'manual')}</div>
            </div>
          </div>
        </td>
        <td>
          <span style="font-family:var(--font-mono); font-size:0.78rem; color:var(--text-secondary);">${escapeHtml(h.target)}</span>
        </td>
        <td>
          <div style="display:flex; flex-wrap:wrap; gap:0.25rem;">
            ${featurePills}
          </div>
        </td>
        <td style="text-align:right; font-family:var(--font-mono); font-size:0.82rem;">
          ${(h.requests || 0).toLocaleString()}
        </td>
        <td style="text-align:right; font-family:var(--font-mono); font-size:0.82rem;">
          ${formatBytes(h.traffic_bytes || 0)}
        </td>
        <td style="text-align:right; font-family:var(--font-mono); font-size:0.82rem; color:${errColor};">
          ${(h.status_5xx || 0).toLocaleString()}
        </td>
        <td style="text-align:right; font-family:var(--font-mono); font-size:0.82rem;">
          ${h.avg_time_ms || 0} ms
        </td>
      </tr>
    `;
  });

  tbody.innerHTML = html;
}

function toggleHostMetricsFilter(domain) {
  if (!domain) return;
  if (currentMetricsHost.toLowerCase() === domain.toLowerCase()) {
    currentMetricsHost = 'all';
  } else {
    currentMetricsHost = domain;
  }
  const hostSelect = document.getElementById('npm-metrics-host-filter');
  if (hostSelect) {
    hostSelect.value = currentMetricsHost;
  }
  fetchNPMMetrics(false);
}

// Global exports for window/inline handlers
window.initNPMMetricsTab = initNPMMetricsTab;
window.fetchNPMMetrics = fetchNPMMetrics;
window.probeNPMMetricsNow = probeNPMMetricsNow;
window.toggleHostMetricsFilter = toggleHostMetricsFilter;
window.filterByStatusClass = filterByStatusClass;
window.filterByStatusCode = filterByStatusCode;
window.clearStatusDrillDown = clearStatusDrillDown;
window.formatBytes = formatBytes;
window.filterRequestEvents = filterRequestEvents;
window.showRequestDetailModal = showRequestDetailModal;
window.closeRequestDetailModal = closeRequestDetailModal;
window.toggleRequestsInspector = toggleRequestsInspector;

function toggleRequestsInspector() {
  const card = document.getElementById('npm-requests-inspector-card');
  const btn = document.getElementById('btn-toggle-requests-inspector');
  if (!card) return;
  const isCollapsed = card.classList.toggle('is-collapsed');
  if (btn) {
    btn.setAttribute('aria-expanded', String(!isCollapsed));
    btn.title = isCollapsed ? 'Expand Request Inspector' : 'Collapse Request Inspector';
  }
  try {
    localStorage.setItem('npm_requests_inspector_collapsed', isCollapsed ? '1' : '0');
  } catch (_) {}
}

function filterRequestEvents(type) {
  currentRequestFilterType = type || 'all';
  document.querySelectorAll('#npm-request-type-pills .npm-type-pill').forEach(btn => {
    if (btn.getAttribute('data-type') === currentRequestFilterType) {
      btn.classList.add('active');
    } else {
      btn.classList.remove('active');
    }
  });
  renderNPMRequestsTable(latestRequestEvents);
}

function renderNPMRequestsTable(events) {
  const tbody = document.getElementById('npm-requests-table-tbody');
  if (!tbody) return;

  const searchInput = document.getElementById('npm-requests-search');
  const term = (searchInput ? searchInput.value : '').toLowerCase().trim();

  // Dynamic subtitle feedback based on active drill-down
  const subtitleEl = document.getElementById('npm-requests-inspector-subtitle');
  if (subtitleEl) {
    if (currentMetricsCodeFilter) {
      subtitleEl.innerHTML = `Showing requests matching HTTP ${currentMetricsCodeFilter}. <a href="javascript:void(0)" onclick="clearStatusDrillDown()" style="color:var(--accent-primary); text-decoration:underline; font-weight:600; margin-left:0.35rem;">Clear code filter</a>`;
    } else if (currentMetricsClassFilter) {
      const clsName = { '2xx': '2xx Success', '3xx': '3xx Redirect', '4xx': '4xx Client error', '5xx': '5xx Server error' }[currentMetricsClassFilter] || currentMetricsClassFilter;
      subtitleEl.innerHTML = `Showing requests matching ${clsName}. <a href="javascript:void(0)" onclick="clearStatusDrillDown()" style="color:var(--accent-primary); text-decoration:underline; font-weight:600; margin-left:0.35rem;">Clear class filter</a>`;
    } else if (currentMetricsHost && currentMetricsHost !== 'all') {
      subtitleEl.innerHTML = `Showing requests for host <strong>${escapeHtml(currentMetricsHost)}</strong>. <a href="javascript:void(0)" onclick="toggleHostMetricsFilter('${escapeHtml(currentMetricsHost)}')" style="color:var(--accent-primary); text-decoration:underline; font-weight:600; margin-left:0.35rem;">Show all hosts</a>`;
    } else {
      subtitleEl.textContent = 'Inspect individual requests, status codes, and upstream error diagnostics across proxy hosts over time.';
    }
  }

  const allEvents = Array.isArray(events) ? events : [];

  const filtered = allEvents.filter(ev => {
    // 1. Request type pills filter ('all', 'errors', '5xx', '4xx')
    if (currentRequestFilterType === 'errors') {
      if ((ev.status_code < 400) && !ev.error) return false;
    } else if (currentRequestFilterType === '5xx') {
      if (ev.status_code < 500 || ev.status_code >= 600) return false;
    } else if (currentRequestFilterType === '4xx') {
      if (ev.status_code < 400 || ev.status_code >= 500) return false;
    }

    // 2. Status code drill-down filter
    if (currentMetricsCodeFilter) {
      if (ev.status_code !== currentMetricsCodeFilter) return false;
    }

    // 3. Status class drill-down filter
    if (currentMetricsClassFilter) {
      const cls = Math.floor(ev.status_code / 100) + 'xx';
      if (cls !== currentMetricsClassFilter) return false;
    }

    // 4. Host filter
    if (currentMetricsHost && currentMetricsHost !== 'all') {
      if (!ev.host || ev.host.toLowerCase() !== currentMetricsHost.toLowerCase()) return false;
    }

    // 5. Search query
    if (term) {
      const match = (ev.path && ev.path.toLowerCase().includes(term)) ||
                    (ev.host && ev.host.toLowerCase().includes(term)) ||
                    (ev.upstream && ev.upstream.toLowerCase().includes(term)) ||
                    (ev.client_ip && ev.client_ip.toLowerCase().includes(term)) ||
                    (ev.error && ev.error.toLowerCase().includes(term)) ||
                    (ev.method && ev.method.toLowerCase().includes(term)) ||
                    (String(ev.status_code).includes(term));
      if (!match) return false;
    }

    return true;
  });

  // Update badge counter
  const badge = document.getElementById('npm-requests-counter-badge');
  if (badge) {
    if (filtered.length !== allEvents.length) {
      badge.textContent = `${filtered.length} of ${allEvents.length} requests`;
    } else {
      badge.textContent = `${allEvents.length} requests`;
    }
  }

  if (filtered.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="8" style="text-align:center; padding: 2.5rem; color:var(--text-muted);">
          No requests match the active filter.
          <button type="button" class="btn-clear-drilldown" onclick="clearStatusDrillDown(); filterRequestEvents('all');" style="margin-left:0.75rem;">Reset filters</button>
        </td>
      </tr>
    `;
    return;
  }

  let html = '';
  filtered.forEach(ev => {
    const cls = Math.floor(ev.status_code / 100) + 'xx';
    const isError = ev.status_code >= 400 || !!ev.error;
    const rowClass = isError ? 'npm-req-row-error' : '';

    let timeFormatted = '';
    try {
      const d = new Date(ev.timestamp);
      timeFormatted = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    } catch {
      timeFormatted = ev.timestamp;
    }

    const methodUpper = (ev.method || 'GET').toUpperCase();

    let errorDetailHtml = '';
    if (ev.error) {
      errorDetailHtml = `
        <div class="npm-req-error-pill" title="${escapeHtml(ev.error)}">
          <span class="npm-error-icon">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"></path>
              <line x1="12" y1="9" x2="12" y2="13"></line>
              <line x1="12" y1="17" x2="12.01" y2="17"></line>
            </svg>
          </span>
          <span class="npm-error-msg">${escapeHtml(ev.error)}</span>
        </div>
      `;
    } else {
      errorDetailHtml = `<span style="color:var(--text-muted); font-size:0.78rem;">—</span>`;
    }

    const latencyColor = ev.latency_ms > 200 ? 'var(--accent-amber)' : (ev.latency_ms > 500 ? 'var(--accent-rose)' : 'inherit');

    html += `
      <tr class="${rowClass}">
        <td>
          <span style="font-family:var(--font-mono); font-size:0.78rem; color:var(--text-secondary);" title="${escapeHtml(ev.timestamp)}">${escapeHtml(timeFormatted)}</span>
        </td>
        <td>
          <span class="npm-feature-pill pill-${cls}" style="font-weight:700; cursor:pointer;" onclick="filterByStatusCode(${ev.status_code}, '${cls}')" title="Filter by HTTP ${ev.status_code}">
            ${ev.status_code}
          </span>
        </td>
        <td>
          <div style="display:flex; align-items:center; gap:0.4rem;">
            <span class="npm-method-badge method-${methodUpper.toLowerCase()}">${methodUpper}</span>
            <span class="npm-req-path" title="${escapeHtml(ev.path)}">${escapeHtml(ev.path)}</span>
          </div>
        </td>
        <td>
          <div>
            <strong style="color:var(--text-primary); font-size:0.82rem; cursor:pointer;" onclick="toggleHostMetricsFilter('${escapeHtml(ev.host)}')" title="Filter by host ${escapeHtml(ev.host)}">${escapeHtml(ev.host)}</strong>
            <div style="font-size:0.72rem; color:var(--text-muted); font-family:var(--font-mono);">${escapeHtml(ev.upstream || '—')}</div>
          </div>
        </td>
        <td>
          <span style="font-family:var(--font-mono); font-size:0.78rem; color:var(--text-secondary);">${escapeHtml(ev.client_ip || '—')}</span>
        </td>
        <td style="text-align:right; font-family:var(--font-mono); font-size:0.82rem; color:${latencyColor};">
          ${ev.latency_ms || 0} ms
        </td>
        <td>
          ${errorDetailHtml}
        </td>
        <td style="text-align:center;">
          <button type="button" class="btn btn-sm btn-outline-secondary" onclick="showRequestDetailModal('${escapeHtml(ev.id)}')" title="Inspect request diagnostics" style="padding:0.2rem 0.5rem; font-size:0.75rem;">
            Inspect
          </button>
        </td>
      </tr>
    `;
  });

  tbody.innerHTML = html;
}

function showRequestDetailModal(id) {
  const modal = document.getElementById('request-detail-modal');
  if (!modal) return;

  const ev = (latestRequestEvents || []).find(r => r.id === id);
  if (!ev) {
    showToast('Request record not found in active session cache', 'warning');
    return;
  }

  const badge = document.getElementById('req-modal-status-badge');
  if (badge) {
    const cls = Math.floor(ev.status_code / 100) + 'xx';
    badge.className = `npm-feature-pill pill-${cls}`;
    badge.textContent = `${ev.status_code} ${ev.status_text || ''}`;
  }

  const body = document.getElementById('req-modal-body');
  if (!body) return;

  let timeStr = ev.timestamp;
  try {
    timeStr = new Date(ev.timestamp).toLocaleString();
  } catch {}

  let adviceHtml = '';
  if (ev.status_code === 502) {
    adviceHtml = `
      <div class="npm-diagnostic-advice-box advice-error">
        <div class="advice-header">
          <span class="advice-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"></path>
              <line x1="12" y1="9" x2="12" y2="13"></line>
              <line x1="12" y1="17" x2="12.01" y2="17"></line>
            </svg>
          </span>
          <strong>Root Cause: 502 Bad Gateway (Upstream Unreachable)</strong>
        </div>
        <p>Nginx Proxy Manager attempted to proxy the request to <code>${escapeHtml(ev.upstream || 'upstream')}</code>, but the destination port was not listening, actively refused the TCP socket connection, or crashed during connection setup.</p>
        <ul class="advice-list">
          <li><strong>Verify container state:</strong> Ensure the upstream service container is running and healthy on the target host.</li>
          <li><strong>Verify bridge network:</strong> If NPM and the backend container are on Docker, verify both are attached to the same network (e.g. <code>npm-network</code> or <code>bridge</code>).</li>
          <li><strong>Verify container port:</strong> Check if the application is listening on <code>0.0.0.0</code> rather than <code>127.0.0.1</code> inside the container.</li>
        </ul>
      </div>
    `;
  } else if (ev.status_code === 504) {
    adviceHtml = `
      <div class="npm-diagnostic-advice-box advice-error">
        <div class="advice-header">
          <span class="advice-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10"></circle>
              <polyline points="12 6 12 12 16 14"></polyline>
            </svg>
          </span>
          <strong>Root Cause: 504 Gateway Timeout (Upstream Response Delayed)</strong>
        </div>
        <p>The upstream target <code>${escapeHtml(ev.upstream || 'upstream')}</code> accepted the TCP handshake but did not send a response within the proxy timeout window (${ev.latency_ms} ms elapsed).</p>
        <ul class="advice-list">
          <li>Check for high backend CPU/memory utilization or stuck database queries.</li>
          <li>For long-running AI streaming or large file uploads, configure custom Nginx <code>proxy_read_timeout 300s;</code> in NPM Advanced tab.</li>
        </ul>
      </div>
    `;
  } else if (ev.status_code === 503) {
    adviceHtml = `
      <div class="npm-diagnostic-advice-box advice-warn">
        <div class="advice-header">
          <span class="advice-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10"></circle>
              <line x1="10" y1="15" x2="10" y2="9"></line>
              <line x1="14" y1="15" x2="14" y2="9"></line>
            </svg>
          </span>
          <strong>Root Cause: 503 Service Unavailable</strong>
        </div>
        <p>The NPM proxy host is currently marked disabled or all upstream nodes in the pool are marked down.</p>
        <ul class="advice-list">
          <li>Enable the proxy host in Nginx Proxy Manager dashboard or verify worker node health.</li>
        </ul>
      </div>
    `;
  } else if (ev.status_code === 404) {
    adviceHtml = `
      <div class="npm-diagnostic-advice-box advice-warn">
        <div class="advice-header">
          <span class="advice-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8"></circle>
              <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
            </svg>
          </span>
          <strong>Root Cause: 404 Route Not Found</strong>
        </div>
        <p>Path <code>${escapeHtml(ev.path)}</code> does not match any registered endpoint on <code>${escapeHtml(ev.host)}</code>.</p>
        <ul class="advice-list">
          <li>Check application routing or base-path configurations.</li>
          <li>If this is a Single-Page Application (SPA), ensure NPM or upstream is configured with <code>try_files $uri /index.html;</code>.</li>
        </ul>
      </div>
    `;
  } else if (ev.status_code === 403) {
    adviceHtml = `
      <div class="npm-diagnostic-advice-box advice-warn">
        <div class="advice-header">
          <span class="advice-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect>
              <path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
            </svg>
          </span>
          <strong>Root Cause: 403 Access Forbidden</strong>
        </div>
        <p>The client IP <code>${escapeHtml(ev.client_ip)}</code> was rejected by upstream authorization or an NPM Access List.</p>
      </div>
    `;
  } else if (ev.status_code >= 200 && ev.status_code < 300) {
    adviceHtml = `
      <div class="npm-diagnostic-advice-box advice-success">
        <div class="advice-header">
          <span class="advice-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path>
              <polyline points="22 4 12 14.01 9 11.01"></polyline>
            </svg>
          </span>
          <strong>Request Succeeded (HTTP 200 OK)</strong>
        </div>
        <p>Proxied successfully to upstream <code>${escapeHtml(ev.upstream || 'backend')}</code> in ${ev.latency_ms} ms with no errors.</p>
      </div>
    `;
  } else {
    adviceHtml = `
      <div class="npm-diagnostic-advice-box advice-info">
        <div class="advice-header">
          <span class="advice-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10"></circle>
              <line x1="12" y1="16" x2="12" y2="12"></line>
              <line x1="12" y1="8" x2="12.01" y2="8"></line>
            </svg>
          </span>
          <strong>HTTP ${ev.status_code} Response</strong>
        </div>
        <p>${escapeHtml(ev.error || ev.status_text || 'Completed request')}</p>
      </div>
    `;
  }

  body.innerHTML = `
    <!-- Top Metadata Grid -->
    <div class="req-diag-grid">
      <div class="req-diag-item">
        <span class="req-diag-label">Timestamp</span>
        <span class="req-diag-val">${escapeHtml(timeStr)}</span>
      </div>
      <div class="req-diag-item">
        <span class="req-diag-label">Request ID</span>
        <span class="req-diag-val mono">${escapeHtml(ev.id)}</span>
      </div>
      <div class="req-diag-item">
        <span class="req-diag-label">HTTP Method & Path</span>
        <span class="req-diag-val mono"><span class="npm-method-badge method-${(ev.method||'GET').toLowerCase()}">${escapeHtml(ev.method||'GET')}</span> ${escapeHtml(ev.path)}</span>
      </div>
      <div class="req-diag-item">
        <span class="req-diag-label">Target Domain</span>
        <span class="req-diag-val">${escapeHtml(ev.host)}</span>
      </div>
      <div class="req-diag-item">
        <span class="req-diag-label">Upstream Proxy Destination</span>
        <span class="req-diag-val mono">${escapeHtml(ev.upstream || '—')}</span>
      </div>
      <div class="req-diag-item">
        <span class="req-diag-label">Client IP Address</span>
        <span class="req-diag-val mono">${escapeHtml(ev.client_ip || '—')}</span>
      </div>
      <div class="req-diag-item">
        <span class="req-diag-label">Round-Trip Latency</span>
        <span class="req-diag-val">${ev.latency_ms} ms</span>
      </div>
      <div class="req-diag-item">
        <span class="req-diag-label">Response Payload Size</span>
        <span class="req-diag-val">${formatBytes(ev.bytes_sent || 0)}</span>
      </div>
    </div>

    ${ev.error ? `
      <div class="npm-diag-error-highlight">
        <div style="font-size:0.75rem; text-transform:uppercase; letter-spacing:0.05em; color:#ef4444; font-weight:700; margin-bottom:0.25rem;">
          Recorded Error Diagnostic Message
        </div>
        <div style="font-family:var(--font-mono); font-size:0.85rem; color:#fca5a5; word-break:break-all;">
          ${escapeHtml(ev.error)}
        </div>
      </div>
    ` : ''}

    ${adviceHtml}

    <div style="margin-top: 1.25rem;">
      <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:0.4rem;">
        <span style="font-size:0.78rem; font-weight:600; color:var(--text-secondary); text-transform:uppercase; letter-spacing:0.04em;">Full Diagnostic Event JSON</span>
        <button type="button" class="btn btn-sm btn-outline-secondary" onclick="navigator.clipboard.writeText(JSON.stringify(latestRequestEvents.find(r => r.id === '${escapeHtml(ev.id)}'), null, 2)); showToast('Copied JSON record to clipboard', 'info');" style="padding:0.2rem 0.6rem; font-size:0.75rem;">
          Copy JSON
        </button>
      </div>
      <pre class="req-diag-raw-json">${escapeHtml(JSON.stringify(ev, null, 2))}</pre>
    </div>
  `;

  modal.style.display = 'flex';
}

function closeRequestDetailModal() {
  const modal = document.getElementById('request-detail-modal');
  if (modal) modal.style.display = 'none';
}

window.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') {
    closeRequestDetailModal();
  }
});
