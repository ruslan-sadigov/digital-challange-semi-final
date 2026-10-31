// Passenger panel: search buses, live positions, route, density, ETA, density reports, location sharing.
const map = makeMap();
const state = { buses: new Map(), markers: new Map(), routes: new Map(), selected: null, routeLayer: null, filter: null };

const $ = id => document.getElementById(id);

function routeCode(id) { const r = state.routes.get(id); return r ? r.code : '-'; }

function renderList() {
  const ids = state.filter || [...state.buses.keys()];
  $('bus-list').innerHTML = ids.map(id => state.buses.get(id)).filter(Boolean).map(b => `
    <div class="item ${state.selected === b.id ? 'active' : ''}" data-id="${b.id}">
      <div class="row"><strong>${esc(b.name)}</strong>${densityBadge(b.density)}</div>
      <div class="row"><span class="muted">Route ${esc(routeCode(b.activeRouteId || b.routeId))}</span>
      ${b.activeRouteId ? '<span class="badge temp">temporary route</span>' : ''}</div>
    </div>`).join('') || '<p class="muted">No buses found.</p>';
}

function upsertMarker(b) {
  if (b.lastLat == null) return;
  let m = state.markers.get(b.id);
  if (!m) {
    m = L.marker([b.lastLat, b.lastLng], { icon: busIcon(b) }).addTo(map).on('click', () => select(b.id));
    state.markers.set(b.id, m);
  } else {
    m.setLatLng([b.lastLat, b.lastLng]);
    m.setIcon(busIcon(b));
  }
  m.bindTooltip(`${esc(b.name)} · ${esc(b.density)}`);
}

async function select(id) {
  state.selected = id;
  renderList();
  const d = await api('GET', '/api/buses/' + id);
  const b = d.bus;
  state.buses.set(b.id, b);
  $('bus-detail').classList.remove('hidden');
  $('bus-detail').scrollIntoView({ block: 'start' });
  $('d-name').textContent = b.name;
  $('d-density').outerHTML = `<span id="d-density" class="badge d-${b.density}">${esc(b.density)}</span>`;
  $('d-route').textContent = d.route ? `Route ${d.route.code}: ${d.route.name}` : 'No route assigned';
  $('d-pass').textContent = `Estimated passengers: ${b.currentPassengerCount} / ${b.passengerCapacity}`;
  const temp = $('d-temp');
  if (d.temporaryRoute && d.homeRoute) {
    temp.textContent = `Temporarily serving route ${d.route.code} (normally ${d.homeRoute.code}) to relieve crowding` +
      (d.temporaryEndsAt ? ` until ${new Date(d.temporaryEndsAt).toLocaleTimeString()}` : '') + '.';
    temp.classList.remove('hidden');
  } else temp.classList.add('hidden');
  $('d-msg').textContent = '';
  const stops = d.route ? d.route.stops : [];
  $('d-stops').innerHTML = stops.map(s => `<li data-station="${s.id}">${esc(s.name)}<span class="eta"></span></li>`).join('');
  if (state.routeLayer) map.removeLayer(state.routeLayer);
  if (stops.length) {
    const pts = stops.map(s => [s.lat, s.lng]);
    state.routeLayer = L.layerGroup([
      L.polyline([...pts, pts[0]], { color: d.temporaryRoute ? '#1f6feb' : '#334', weight: 4, opacity: .7, dashArray: d.temporaryRoute ? '8 6' : null }),
      ...stops.map(s => L.circleMarker([s.lat, s.lng], { radius: 6, color: '#334', fillColor: '#fff', fillOpacity: 1 })
        .bindTooltip(s.name).on('click', () => showETA(id, s.id))),
    ]).addTo(map);
  }
  if (b.lastLat != null) map.panTo([b.lastLat, b.lastLng]);
}

async function showETA(busId, stationId) {
  const li = document.querySelector(`#d-stops li[data-station="${stationId}"] .eta`);
  try {
    const e = await api('GET', `/api/buses/${busId}/eta?station_id=${stationId}`);
    const text = `${fmtDuration(e.seconds)} (${(e.distanceM / 1000).toFixed(1)} km)`;
    if (li) li.textContent = text;
    toast(`ETA: ${text}`);
  } catch (err) { toast(err.message, 'alert'); }
}

$('d-stops').addEventListener('click', e => {
  const li = e.target.closest('li[data-station]');
  if (li && state.selected) showETA(state.selected, Number(li.dataset.station));
});

$('d-close').addEventListener('click', () => {
  state.selected = null;
  $('bus-detail').classList.add('hidden');
  if (state.routeLayer) { map.removeLayer(state.routeLayer); state.routeLayer = null; }
  renderList();
});

$('bus-list').addEventListener('click', e => {
  const it = e.target.closest('.item');
  if (it) select(Number(it.dataset.id));
});

document.querySelectorAll('[data-density]').forEach(btn => btn.addEventListener('click', async () => {
  if (!state.selected) return;
  try {
    await api('POST', `/api/buses/${state.selected}/density`, { density: btn.dataset.density });
    $('d-msg').textContent = 'Thanks! Your report was recorded.';
  } catch (err) { $('d-msg').textContent = err.message; }
}));

let searchTimer;
$('search').addEventListener('input', e => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(async () => {
    const q = e.target.value.trim();
    if (!q) { state.filter = null; renderList(); return; }
    const res = await api('GET', '/api/buses?q=' + encodeURIComponent(q));
    state.filter = res.map(b => b.id);
    renderList();
  }, 250);
});

// Location sharing (foreground, opt-in). Interval comes from the server (2 min by default).
let locTimer = null;
async function startSharing() {
  if (!navigator.geolocation) { toast('Geolocation is not available in this browser', 'alert'); return; }
  const cfg = await api('GET', '/api/config');
  const send = () => navigator.geolocation.getCurrentPosition(
    p => api('POST', '/api/locations', { lat: p.coords.latitude, lng: p.coords.longitude }).catch(() => {}),
    () => toast('Location permission denied', 'alert'), { enableHighAccuracy: true, timeout: 10000 });
  send();
  locTimer = setInterval(send, cfg.userSampleIntervalMs);
  $('share-loc').textContent = 'Stop sharing';
  try { localStorage.setItem('shareLoc', '1'); } catch (_) {}
}
function stopSharing() {
  clearInterval(locTimer); locTimer = null;
  $('share-loc').textContent = 'Share my location';
  try { localStorage.removeItem('shareLoc'); } catch (_) {}
}
$('share-loc').addEventListener('click', () => (locTimer ? stopSharing() : startSharing()));
try { if (localStorage.getItem('shareLoc')) startSharing(); } catch (_) {}

async function init() {
  const [routes, stations, buses] = await Promise.all([api('GET', '/api/routes'), api('GET', '/api/stations'), api('GET', '/api/buses')]);
  routes.forEach(r => state.routes.set(r.id, r));
  stations.forEach(s => L.circleMarker([s.lat, s.lng], { radius: 3, color: '#667085' }).addTo(map).bindTooltip(s.name));
  buses.forEach(b => { state.buses.set(b.id, b); upsertMarker(b); });
  renderList();
  connectWS(['buses'], msg => {
    if (msg.type === 'bus_location') {
      const b = state.buses.get(msg.data.busId);
      if (!b) return;
      b.lastLat = msg.data.lat; b.lastLng = msg.data.lng;
      upsertMarker(b);
    } else if (msg.type === 'densities') {
      msg.data.buses.forEach(nb => { state.buses.set(nb.id, nb); upsertMarker(nb); });
      renderList();
      if (state.selected) {
        const b = state.buses.get(state.selected);
        $('d-density').outerHTML = `<span id="d-density" class="badge d-${b.density}">${esc(b.density)}</span>`;
        $('d-pass').textContent = `Estimated passengers: ${b.currentPassengerCount} / ${b.passengerCapacity}`;
      }
    } else if (msg.type === 'override') {
      api('GET', '/api/buses').then(bs => { bs.forEach(b => { state.buses.set(b.id, b); upsertMarker(b); }); renderList(); });
      if (state.selected === msg.data.busId) select(state.selected);
    }
  });
}
init().catch(err => toast(err.message, 'alert'));
