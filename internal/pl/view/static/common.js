// Shared helpers for both panels.
const BAKU = [40.395, 49.86];
const COLORS = { low: '#2e9e5b', medium: '#e0a100', high: '#d64545', unknown: '#98a2b3' };

async function api(method, url, body) {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data;
}

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function densityBadge(d) {
  d = d || 'unknown';
  return `<span class="badge d-${d}">${esc(d)}</span>`;
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
  const ring = bus.activeRouteId ? '#1f6feb' : '#fff';
  return L.divIcon({
    className: '',
    html: `<div style="background:${c};border:3px solid ${ring};border-radius:50%;width:22px;height:22px;color:#fff;font:600 10px/16px system-ui;text-align:center;box-shadow:0 1px 4px rgba(0,0,0,.4)">B</div>`,
    iconSize: [22, 22], iconAnchor: [11, 11],
  });
}

function toast(msg, cls) {
  const t = document.getElementById('toast');
  t.textContent = msg;
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
