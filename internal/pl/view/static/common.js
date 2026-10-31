// Shared helpers for both panels.
const BAKU = [40.395, 49.86];
// Essential tokens used on the map. Status colors only express density.
const COLORS = { low: '#16A34A', medium: '#D97706', high: '#DC2626', unknown: '#8A929E' };
const INK = '#111827', ACCENT = '#2563EB', MUTED = '#5B6472';

async function api(method, url, body) {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new Error(cap((data && data.error) || res.statusText));
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
  return `<span class="badge d-${d}">${esc(cap(d))}</span>`;
}

function makeMap() {
  const map = L.map('map').setView(BAKU, 13);
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19, attribution: '&copy; OpenStreetMap contributors',
  }).addTo(map);
  return map;
}

function busIcon(bus) {
  const c = COLORS[bus.density || 'unknown'];
  const ring = bus.activeRouteId ? ACCENT : '#fff'; // blue ring = temporarily rerouted (active state)
  return L.divIcon({
    className: '',
    html: `<div style="background:${c};border:3px solid ${ring};border-radius:50%;width:24px;height:24px;display:grid;place-items:center;box-shadow:0 2px 4px rgba(17,24,39,.08),0 8px 20px rgba(17,24,39,.12)"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="3" stroke-linecap="round"><rect x="5" y="4" width="14" height="12" rx="3"/><path d="M8 20v-4M16 20v-4"/></svg></div>`,
    iconSize: [24, 24], iconAnchor: [12, 12],
  });
}

// Toast: surface card, bottom-right, status icon, auto-dismiss after 6 s. cls: '' (info) | 'alert' | 'success'.
function toast(msg, cls) {
  const t = document.getElementById('toast');
  const icon = cls === 'alert' ? '!' : cls === 'success' ? '&#10003;' : 'i';
  t.innerHTML = `<span class="icon" aria-hidden="true">${icon}</span><span>${esc(msg)}</span>`;
  t.className = 'toast ' + (cls || '');
  clearTimeout(toast._t);
  toast._t = setTimeout(() => t.classList.add('hidden'), 6000);
}

// Live WebSocket with auto-reconnect. handlers: {channel: fn(msg)}.
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
  if (sec < 60) return sec + ' s';
  const m = Math.round(sec / 60);
  return m < 60 ? m + ' min' : Math.floor(m / 60) + ' h ' + (m % 60) + ' min';
}

// Great-circle distance in metres.
function distM(lat1, lng1, lat2, lng2) {
  const r = Math.PI / 180, dLat = (lat2 - lat1) * r, dLng = (lng2 - lng1) * r;
  const a = Math.sin(dLat / 2) ** 2 + Math.cos(lat1 * r) * Math.cos(lat2 * r) * Math.sin(dLng / 2) ** 2;
  return 2 * 6371000 * Math.asin(Math.min(1, Math.sqrt(a)));
}
