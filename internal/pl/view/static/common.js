// Shared helpers for both panels. Needs i18n.js (t, tError, levelLabel) and theme.js (cssVar) loaded first.
const BAKU = [40.395, 49.86];

// Map colors come from the theme tokens, so lines and dots stay readable in light and dark.
const mapColor = {
  ink: () => cssVar('--map-ink'),
  muted: () => cssVar('--map-muted'),
  fill: () => cssVar('--map-fill'),
  accent: () => cssVar('--accent'),
};

async function api(method, url, body) {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new Error(cap(tError((data && data.error) || res.statusText)));
  return data;
}

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function emptyState(title, text) {
  return `<div class="empty"><strong>${esc(title)}</strong><p>${esc(text)}</p></div>`;
}

function cap(s) { s = String(s ?? ''); return s.charAt(0).toUpperCase() + s.slice(1); }

function densityBadge(d) {
  d = d || 'unknown';
  return `<span class="badge d-${d}">${esc(levelLabel(d))}</span>`;
}

function makeMap() {
  const map = L.map('map').setView(BAKU, 13);
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19, attribution: '&copy; OpenStreetMap contributors',
  }).addTo(map);
  return map;
}

// Bus marker: the line number in a circle colored by density, plus a small triangle pointing the
// direction of travel (heading in degrees from north). A blue ring marks a temporarily rerouted bus.
function busIcon(bus, code) {
  const label = String(code ?? '').slice(0, 4) || '•';
  const cls = ['bus-marker', 'd-' + (bus.density || 'unknown')];
  if (bus.activeRouteId) cls.push('rerouted');
  if (bus.heading == null) cls.push('no-dir');
  if (label.length > 2) cls.push('long');
  return L.divIcon({
    className: '',
    html: `<div class="${cls.join(' ')}" style="--rot:${Math.round(bus.heading || 0)}deg"><span class="bus-dir"></span><span class="bus-code">${esc(label)}</span></div>`,
    iconSize: [34, 34], iconAnchor: [17, 17], tooltipAnchor: [0, -20],
  });
}

// Toast: surface card, bottom-right, status icon, auto-dismiss after 6 s. cls: '' (info) | 'alert' | 'success'.
function toast(msg, cls) {
  const el = document.getElementById('toast');
  const icon = cls === 'alert' ? '!' : cls === 'success' ? '&#10003;' : 'i';
  el.innerHTML = `<span class="icon" aria-hidden="true">${icon}</span><span>${esc(msg)}</span>`;
  el.className = 'toast ' + (cls || '');
  clearTimeout(toast._t);
  toast._t = setTimeout(() => el.classList.add('hidden'), 6000);
}

// Live WebSocket with auto-reconnect.
function connectWS(channels, onMessage) {
  let delay = 1000;
  function open() {
    const ws = new WebSocket((location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws');
    ws.onopen = () => { delay = 1000; channels.forEach(ch => ws.send(JSON.stringify({ action: 'subscribe', channel: ch }))); };
    ws.onmessage = e => { try { onMessage(JSON.parse(e.data)); } catch (err) { console.error(err); } };
    ws.onclose = () => { setTimeout(open, delay); delay = Math.min(delay * 2, 15000); };
  }
  open();
}

function fmtDuration(sec) {
  sec = Math.max(0, Math.round(sec));
  if (sec < 60) return `${sec} ${t('unit.s')}`;
  const m = Math.round(sec / 60);
  return m < 60 ? `${m} ${t('unit.min')}` : `${Math.floor(m / 60)} ${t('unit.h')} ${m % 60} ${t('unit.min')}`;
}

function fmtTime(iso) {
  return new Date(iso).toLocaleTimeString(LANG === 'az' ? 'az-Latn-AZ' : 'en-GB', { hour: '2-digit', minute: '2-digit' });
}

// Great-circle distance in metres.
function distM(lat1, lng1, lat2, lng2) {
  const r = Math.PI / 180, dLat = (lat2 - lat1) * r, dLng = (lng2 - lng1) * r;
  const a = Math.sin(dLat / 2) ** 2 + Math.cos(lat1 * r) * Math.cos(lat2 * r) * Math.sin(dLng / 2) ** 2;
  return 2 * 6371000 * Math.asin(Math.min(1, Math.sqrt(a)));
}
