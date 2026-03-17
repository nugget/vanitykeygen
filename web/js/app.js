// VKG Web UI

const API = '';

// --- State ---
let currentTab = 'dashboard';

// --- Tabs ---
document.querySelectorAll('.tab').forEach(btn => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('.tab').forEach(b => b.classList.remove('active'));
    document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
    btn.classList.add('active');
    document.getElementById(btn.dataset.tab).classList.add('active');
    currentTab = btn.dataset.tab;
    refreshTab(currentTab);
  });
});

// --- API helpers ---
async function api(path, opts = {}) {
  const res = await fetch(API + path, {
    headers: { 'Content-Type': 'application/json' },
    ...opts,
  });
  if (res.status === 204) return null;
  return res.json();
}

function formatTime(ts) {
  if (!ts) return '-';
  const d = new Date(ts);
  const now = new Date();
  const diffMs = now - d;
  if (diffMs < 60000) return Math.floor(diffMs / 1000) + 's ago';
  if (diffMs < 3600000) return Math.floor(diffMs / 60000) + 'm ago';
  if (diffMs < 86400000) return Math.floor(diffMs / 3600000) + 'h ago';
  return d.toLocaleDateString();
}

function formatNumber(n) {
  if (n == null) return '-';
  if (n >= 1e9) return (n / 1e9).toFixed(1) + 'B';
  if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
  if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
  return n.toLocaleString();
}

function escapeHtml(s) {
  const div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

// --- Dashboard ---
async function refreshDashboard() {
  const [statsRes, targetRes, matchesRes] = await Promise.all([
    api('/api/stats'),
    api('/api/targets/active'),
    api('/api/matches?limit=10'),
  ]);

  if (statsRes && statsRes.data) {
    const s = statsRes.data;
    document.getElementById('stat-clients').textContent = s.activeClients;
    document.getElementById('stat-keyrate').textContent = formatNumber(Math.round(s.totalKeyRate));
    document.getElementById('stat-keycount').textContent = formatNumber(s.totalKeyCount);
    document.getElementById('stat-matches').textContent = s.totalMatches;
  }

  const atEl = document.getElementById('active-target');
  if (targetRes && targetRes.data) {
    const t = targetRes.data;
    atEl.innerHTML = `<strong>${escapeHtml(t.label || 'Unnamed')}</strong><br><code>${escapeHtml(t.pattern)}</code>`;
  } else {
    atEl.textContent = 'No active target';
  }

  renderRecentMatches(matchesRes && matchesRes.data ? matchesRes.data : []);
}

function renderRecentMatches(matches) {
  const tbody = document.querySelector('#recent-matches-table tbody');
  const empty = document.getElementById('recent-matches-empty');
  if (!matches.length) {
    tbody.innerHTML = '';
    empty.style.display = '';
    return;
  }
  empty.style.display = 'none';
  tbody.innerHTML = matches.map(m => `
    <tr>
      <td>${formatTime(m.timestamp)}</td>
      <td class="mono">${escapeHtml(m.matchString)}</td>
      <td>${matchTypeBadges(m)}</td>
      <td>${escapeHtml(m.hostname || m.clientId || '-')}</td>
    </tr>
  `).join('');
}

function matchTypeBadges(m) {
  let s = '';
  if (m.matchedFingerprint) s += '<span class="badge badge-fp">FP</span> ';
  if (m.matchedAuthorizedKey) s += '<span class="badge badge-auth">Auth</span>';
  return s || '-';
}

// --- Targets ---
async function refreshTargets() {
  const res = await api('/api/targets');
  const targets = res && res.data ? res.data : [];
  const tbody = document.querySelector('#targets-table tbody');
  const empty = document.getElementById('targets-empty');

  if (!targets.length) {
    tbody.innerHTML = '';
    empty.style.display = '';
    return;
  }
  empty.style.display = 'none';
  tbody.innerHTML = targets.map(t => `
    <tr>
      <td>${t.active
        ? '<span class="badge badge-active">Active</span>'
        : '<span class="badge badge-inactive">Inactive</span>'}</td>
      <td>${escapeHtml(t.label || '-')}</td>
      <td class="mono">${escapeHtml(t.pattern)}</td>
      <td>${t.priority}</td>
      <td>
        <button class="btn btn-sm" onclick="editTarget('${t.id}')">Edit</button>
        <button class="btn btn-sm btn-danger" onclick="deleteTarget('${t.id}')">Delete</button>
      </td>
    </tr>
  `).join('');
}

document.getElementById('btn-new-target').addEventListener('click', () => {
  document.getElementById('target-dialog-title').textContent = 'New Target';
  document.getElementById('target-id').value = '';
  document.getElementById('target-pattern').value = '';
  document.getElementById('target-label').value = '';
  document.getElementById('target-priority').value = '0';
  document.getElementById('target-active').checked = false;
  document.getElementById('target-dialog').showModal();
});

document.getElementById('btn-cancel-target').addEventListener('click', () => {
  document.getElementById('target-dialog').close();
});

document.getElementById('target-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const id = document.getElementById('target-id').value;
  const body = {
    pattern: document.getElementById('target-pattern').value,
    label: document.getElementById('target-label').value,
    priority: parseInt(document.getElementById('target-priority').value) || 0,
    active: document.getElementById('target-active').checked,
  };

  if (id) {
    await api(`/api/targets/${id}`, { method: 'PUT', body: JSON.stringify(body) });
  } else {
    await api('/api/targets', { method: 'POST', body: JSON.stringify(body) });
  }
  document.getElementById('target-dialog').close();
  refreshTargets();
});

window.editTarget = async function(id) {
  const res = await api(`/api/targets/${id}`);
  if (!res || !res.data) return;
  const t = res.data;
  document.getElementById('target-dialog-title').textContent = 'Edit Target';
  document.getElementById('target-id').value = t.id;
  document.getElementById('target-pattern').value = t.pattern;
  document.getElementById('target-label').value = t.label;
  document.getElementById('target-priority').value = t.priority;
  document.getElementById('target-active').checked = t.active;
  document.getElementById('target-dialog').showModal();
};

window.deleteTarget = async function(id) {
  if (!confirm('Delete this target?')) return;
  await api(`/api/targets/${id}`, { method: 'DELETE' });
  refreshTargets();
};

// --- Matches ---
async function refreshMatches() {
  const res = await api('/api/matches?limit=100');
  const matches = res && res.data ? res.data : [];
  const tbody = document.querySelector('#matches-table tbody');
  const empty = document.getElementById('matches-empty');

  if (!matches.length) {
    tbody.innerHTML = '';
    empty.style.display = '';
    return;
  }
  empty.style.display = 'none';
  tbody.innerHTML = matches.map(m => `
    <tr>
      <td>${formatTime(m.timestamp)}</td>
      <td class="mono">${escapeHtml(m.matchString)}</td>
      <td class="mono">${escapeHtml((m.key && m.key.fingerprint) || '-')}</td>
      <td class="mono" title="${escapeHtml((m.key && m.key.authorizedString) || '')}">${escapeHtml(((m.key && m.key.authorizedString) || '').slice(0, 40))}...</td>
      <td>${escapeHtml(m.hostname || m.clientId || '-')}</td>
      <td><button class="btn btn-sm" onclick="viewMatch('${m.id}')">View</button></td>
    </tr>
  `).join('');
}

window.viewMatch = async function(id) {
  const res = await api(`/api/matches/${id}`);
  if (!res || !res.data) return;
  const m = res.data;
  const c = document.getElementById('match-detail-content');
  c.innerHTML = `
    <div class="detail-row"><span class="detail-label">ID</span><span class="mono">${escapeHtml(m.id)}</span></div>
    <div class="detail-row"><span class="detail-label">Time</span><span>${new Date(m.timestamp).toLocaleString()}</span></div>
    <div class="detail-row"><span class="detail-label">Match</span><span class="mono">${escapeHtml(m.matchString)}</span></div>
    <div class="detail-row"><span class="detail-label">Type</span><span>${matchTypeBadges(m)}</span></div>
    <div class="detail-row"><span class="detail-label">Client</span><span>${escapeHtml(m.hostname || '-')} (${escapeHtml(m.clientId || '-')})</span></div>
    <div class="detail-row"><span class="detail-label">Fingerprint</span><span class="mono">${escapeHtml((m.key && m.key.fingerprint) || '-')}</span></div>
    <div class="detail-row"><span class="detail-label">Auth Key</span></div>
    <pre>${escapeHtml((m.key && m.key.authorizedString) || '-')}</pre>
    <div class="detail-row"><span class="detail-label">Private Key</span></div>
    <pre>${escapeHtml((m.key && m.key.privateString) || '-')}</pre>
  `;
  document.getElementById('match-detail-dialog').showModal();
};

document.getElementById('btn-close-match-detail').addEventListener('click', () => {
  document.getElementById('match-detail-dialog').close();
});

// --- Fleet ---
async function refreshFleet() {
  const res = await api('/api/clients');
  const clients = res && res.data ? res.data : [];
  const tbody = document.querySelector('#fleet-table tbody');
  const empty = document.getElementById('fleet-empty');

  if (!clients.length) {
    tbody.innerHTML = '';
    empty.style.display = '';
    return;
  }
  empty.style.display = 'none';
  tbody.innerHTML = clients.map(c => {
    const statusClass = c.status === 'active' ? 'badge-active'
      : c.status === 'offline' ? 'badge-offline' : 'badge-idle';
    return `
      <tr>
        <td><span class="badge ${statusClass}">${escapeHtml(c.status)}</span></td>
        <td>${escapeHtml(c.hostname)}</td>
        <td class="mono">${escapeHtml(c.id)}</td>
        <td>${c.seekers}</td>
        <td class="mono">${formatNumber(Math.round(c.keyRate))}/s</td>
        <td class="mono">${formatNumber(c.keyCount)}</td>
        <td>${formatTime(c.lastSeen)}</td>
      </tr>
    `;
  }).join('');
}

// --- Tab refresh ---
function refreshTab(tab) {
  switch (tab) {
    case 'dashboard': refreshDashboard(); break;
    case 'targets': refreshTargets(); break;
    case 'matches': refreshMatches(); break;
    case 'fleet': refreshFleet(); break;
  }
}

// --- SSE ---
function connectSSE() {
  const statusEl = document.getElementById('sse-status');
  const evtSource = new EventSource(API + '/api/events');

  evtSource.onopen = () => {
    statusEl.textContent = 'Connected';
    statusEl.className = 'sse-connected';
  };

  evtSource.onerror = () => {
    statusEl.textContent = 'Disconnected';
    statusEl.className = 'sse-disconnected';
    evtSource.close();
    // Reconnect after 3s
    setTimeout(connectSSE, 3000);
  };

  evtSource.addEventListener('match', () => {
    if (currentTab === 'dashboard') refreshDashboard();
    if (currentTab === 'matches') refreshMatches();
  });

  evtSource.addEventListener('client_update', () => {
    if (currentTab === 'dashboard') refreshDashboard();
    if (currentTab === 'fleet') refreshFleet();
  });

  evtSource.addEventListener('target_update', () => {
    if (currentTab === 'dashboard') refreshDashboard();
    if (currentTab === 'targets') refreshTargets();
  });
}

// --- Version ---
async function fetchVersion() {
  const res = await api('/api/version');
  if (res && res.data && res.data.version) {
    document.getElementById('app-version').textContent = res.data.version;
  }
}

// --- Init ---
refreshDashboard();
fetchVersion();
connectSSE();

// Periodic refresh for time-based displays
setInterval(() => refreshTab(currentTab), 30000);
