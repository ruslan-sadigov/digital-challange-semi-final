// Operator panel: live heatmap, buses, route densities, anomaly alerts, reroute approval, CRUD.
const map = makeMap();
const $ = id => document.getElementById(id);
const st = {
  buses: new Map(), markers: new Map(), routes: new Map(), stations: new Map(), states: new Map(),
  heat: L.heatLayer([], { radius: 28, blur: 18, maxZoom: 16, minOpacity: 0.45 }).addTo(map), stationLayer: L.layerGroup().addTo(map),
  routeLayer: null, editStops: [], tab: 'alerts',
};

// ---------- tabs ----------
document.querySelectorAll('.tabs button').forEach(b => b.addEventListener('click', () => {
  st.tab = b.dataset.tab;
  document.querySelectorAll('.tabs button').forEach(x => x.classList.toggle('active', x === b));
  document.querySelectorAll('[data-pane]').forEach(p => p.classList.toggle('hidden', p.dataset.pane !== st.tab));
}));

$('toggle-heat').addEventListener('change', e => (e.target.checked ? st.heat.addTo(map) : map.removeLayer(st.heat)));
$('logout').addEventListener('click', async () => { await api('POST', '/api/auth/logout'); location.href = '/operator/login'; });

// ---------- notifications ----------
$('enable-push').addEventListener('click', async () => {
  if (!('Notification' in window)) return toast('This browser does not support notifications', 'alert');
  const p = await Notification.requestPermission();
  toast(p === 'granted' ? 'Notifications enabled' : 'Notifications blocked');
});
function notify(alert) {
  toast(alert.message, alert.type === 'anomaly_opened' ? 'alert' : '');
  if ('Notification' in window && Notification.permission === 'granted') {
    new Notification(alert.type === 'anomaly_opened' ? `Route ${alert.routeCode}: high density` : `Route ${alert.routeCode}: resolved`, { body: alert.message });
  }
}

// ---------- map layers ----------
function routeCode(id) { const r = st.routes.get(id); return r ? r.code : '-'; }
function upsertBus(b) {
  st.buses.set(b.id, b);
  if (b.lastLat == null) return;
  let m = st.markers.get(b.id);
  if (!m) { m = L.marker([b.lastLat, b.lastLng], { icon: busIcon(b) }).addTo(map); st.markers.set(b.id, m); }
  else { m.setLatLng([b.lastLat, b.lastLng]); m.setIcon(busIcon(b)); }
  m.bindTooltip(`${esc(b.name)} · route ${esc(routeCode(b.activeRouteId || b.routeId))} · ${esc(b.density)} · ~${b.currentPassengerCount}/${b.passengerCapacity}`);
}
function drawStations() {
  st.stationLayer.clearLayers();
  st.stations.forEach(s => {
    L.circleMarker([s.lat, s.lng], { radius: 6, color: '#334', fillColor: '#fff', fillOpacity: 1 })
      .bindTooltip(s.name).on('click', () => stationClicked(s)).addTo(st.stationLayer);
  });
}
function showRoute(r) {
  if (st.routeLayer) map.removeLayer(st.routeLayer);
  if (!r || !r.stops.length) return;
  const pts = r.path && r.path.length > 1 ? r.path : [...r.stops.map(s => [s.lat, s.lng]), [r.stops[0].lat, r.stops[0].lng]];
  st.routeLayer = L.polyline(pts, { color: '#1f6feb', weight: 4, opacity: .7 }).addTo(map);
}

// ---------- alerts ----------
async function loadAlerts() {
  const [anoms, active] = await Promise.all([api('GET', '/api/operator/anomalies'), api('GET', '/api/operator/overrides')]);
  const open = anoms.filter(a => a.status === 'open');
  $('alert-count').textContent = open.length;
  $('alert-count').classList.toggle('hidden', !open.length);
  $('active-overrides').innerHTML = active.map(o => `
    <div class="item"><div class="row"><span><strong>${esc(o.busName)}</strong> ${esc(o.fromRouteCode)} → ${esc(o.toRouteCode)}</span>
    <button class="btn small danger" data-end="${o.id}">End</button></div>
    <div class="muted small-text">until ${o.endsAt ? new Date(o.endsAt).toLocaleTimeString() : '-'}</div></div>`).join('') || '<p class="muted">None.</p>';
  $('anomalies').innerHTML = anoms.map(a => `
    <div class="item">
      <div class="row"><strong>Route ${esc(a.routeCode)}</strong><span class="badge ${a.status === 'open' ? 'd-high' : 'd-low'}">${esc(a.status)}</span></div>
      <div class="muted small-text">High since ${new Date(a.startedAt).toLocaleTimeString()} · detected ${new Date(a.detectedAt).toLocaleTimeString()}
      ${a.resolvedAt ? ' · resolved ' + new Date(a.resolvedAt).toLocaleTimeString() : ''}</div>
      ${a.overrides.map(o => `<div class="suggestion">
        <div class="row"><span>Move <strong>${esc(o.busName)}</strong> from ${esc(o.fromRouteCode)}</span><span class="muted small-text">${o.distanceKm.toFixed(1)} km</span></div>
        <div class="row gap">${o.status === 'suggested'
          ? `<button class="btn small" data-approve="${o.id}">Approve</button><button class="btn small ghost" data-reject="${o.id}">Reject</button>`
          : `<span class="muted small-text">${esc(o.status)}</span>`}</div></div>`).join('')
        || (a.status === 'open' ? '<p class="muted small-text">No low-density bus available to move.</p>' : '')}
    </div>`).join('') || '<p class="muted">No anomalies yet.</p>';
}
document.querySelector('[data-pane="alerts"]').addEventListener('click', async e => {
  const t = e.target;
  const act = t.dataset.approve ? ['approve', t.dataset.approve] : t.dataset.reject ? ['reject', t.dataset.reject] : t.dataset.end ? ['end', t.dataset.end] : null;
  if (!act) return;
  try { await api('POST', `/api/operator/overrides/${act[1]}/${act[0]}`); toast('Reroute ' + { approve: 'approved', reject: 'rejected', end: 'ended' }[act[0]]); }
  catch (err) { toast(err.message, 'alert'); }
  loadAlerts(); loadBuses();
});

// ---------- routes ----------
function renderRoutes() {
  $('route-list').innerHTML = [...st.routes.values()].map(r => {
    const s = st.states.get(r.id) || {};
    const high = s.highSince ? ` · High for ${fmtDuration((Date.now() - new Date(s.highSince)) / 1000)}` : '';
    return `<div class="item" data-route="${r.id}">
      <div class="row"><strong>${esc(r.code)} · ${esc(r.name)}</strong>${densityBadge(s.level)}</div>
      <div class="row"><span class="muted small-text">${r.stops.length} stops${high}</span>
      <span><button class="btn small ghost" data-edit-route="${r.id}">Edit</button>
      <button class="btn small danger" data-del-route="${r.id}">Delete</button></span></div></div>`;
  }).join('');
  const sel = $('bus-route-select');
  const cur = sel.value;
  sel.innerHTML = '<option value="">No route</option>' + [...st.routes.values()].map(r => `<option value="${r.id}">${esc(r.code)} · ${esc(r.name)}</option>`).join('');
  sel.value = cur;
}
function renderEditStops() {
  $('route-stops').innerHTML = st.editStops.map(s => `<li>${esc(s.name)}</li>`).join('');
  showRoute({ stops: st.editStops });
}
function stationClicked(s) {
  if (st.tab !== 'routes') return;
  st.editStops.push(s);
  renderEditStops();
}
$('route-list').addEventListener('click', async e => {
  const t = e.target;
  if (t.dataset.editRoute) {
    const r = st.routes.get(Number(t.dataset.editRoute));
    const f = $('route-form');
    f.id.value = r.id; f.code.value = r.code; f.name.value = r.name;
    st.editStops = [...r.stops];
    $('route-form-title').textContent = 'Edit route ' + r.code;
    renderEditStops();
  } else if (t.dataset.delRoute) {
    if (!confirm('Delete this route?')) return;
    try { await api('DELETE', '/api/operator/routes/' + t.dataset.delRoute); await loadRoutes(); } catch (err) { toast(err.message, 'alert'); }
  } else {
    const it = t.closest('[data-route]');
    if (it) showRoute(st.routes.get(Number(it.dataset.route)));
  }
});
$('route-undo').addEventListener('click', () => { st.editStops.pop(); renderEditStops(); });
$('route-reset').addEventListener('click', () => { st.editStops = []; $('route-form-title').textContent = 'New route'; $('route-form').id.value = ''; renderEditStops(); });
$('route-form').addEventListener('submit', async e => {
  e.preventDefault();
  const f = e.target;
  const body = { code: f.code.value, name: f.name.value, stationIds: st.editStops.map(s => s.id) };
  try {
    if (f.id.value) await api('PUT', '/api/operator/routes/' + f.id.value, body);
    else await api('POST', '/api/operator/routes', body);
    toast('Route saved'); f.reset(); $('route-reset').click(); await loadRoutes();
  } catch (err) { toast(err.message, 'alert'); }
});
async function loadRoutes() {
  const [routes, states] = await Promise.all([api('GET', '/api/routes'), api('GET', '/api/routes/density')]);
  st.routes = new Map(routes.map(r => [r.id, r]));
  st.states = new Map(states.map(s => [s.routeId, s]));
  renderRoutes();
}

// ---------- buses ----------
function renderBuses() {
  $('op-bus-list').innerHTML = [...st.buses.values()].map(b => `
    <div class="item"><div class="row"><strong>${esc(b.name)}</strong>${densityBadge(b.density)}</div>
    <div class="row"><span class="muted small-text">Route ${esc(routeCode(b.activeRouteId || b.routeId))}${b.activeRouteId ? ' (temporary)' : ''} · ~${b.currentPassengerCount}/${b.passengerCapacity}</span>
    <span><button class="btn small ghost" data-edit-bus="${b.id}">Edit</button><button class="btn small danger" data-del-bus="${b.id}">Delete</button></span></div></div>`).join('');
}
$('op-bus-list').addEventListener('click', async e => {
  const t = e.target;
  if (t.dataset.editBus) {
    const b = st.buses.get(Number(t.dataset.editBus));
    const f = $('bus-form');
    f.id.value = b.id; f.name.value = b.name; f.passengerCapacity.value = b.passengerCapacity; f.routeId.value = b.routeId || '';
    $('bus-form-title').textContent = 'Edit bus ' + b.name;
  } else if (t.dataset.delBus) {
    if (!confirm('Delete this bus?')) return;
    try {
      await api('DELETE', '/api/operator/buses/' + t.dataset.delBus);
      const m = st.markers.get(Number(t.dataset.delBus)); if (m) map.removeLayer(m);
      st.markers.delete(Number(t.dataset.delBus));
      await loadBuses();
    } catch (err) { toast(err.message, 'alert'); }
  }
});
$('bus-reset').addEventListener('click', () => { $('bus-form').id.value = ''; $('bus-form-title').textContent = 'New bus'; });
$('bus-form').addEventListener('submit', async e => {
  e.preventDefault();
  const f = e.target;
  const body = { name: f.name.value, passengerCapacity: Number(f.passengerCapacity.value), routeId: f.routeId.value ? Number(f.routeId.value) : null };
  try {
    if (f.id.value) await api('PUT', '/api/operator/buses/' + f.id.value, body);
    else await api('POST', '/api/operator/buses', body);
    toast('Bus saved'); f.reset(); f.id.value = ''; await loadBuses();
  } catch (err) { toast(err.message, 'alert'); }
});
async function loadBuses() {
  const buses = await api('GET', '/api/buses');
  st.buses = new Map();
  buses.forEach(upsertBus);
  renderBuses();
}

// ---------- stations ----------
function renderStations() {
  $('station-list').innerHTML = [...st.stations.values()].map(s => `
    <div class="item"><div class="row"><span>${esc(s.name)}</span><button class="btn small danger" data-del-station="${s.id}">Delete</button></div></div>`).join('');
}
map.on('click', e => {
  if (st.tab !== 'stations') return;
  const f = $('station-form');
  f.lat.value = e.latlng.lat.toFixed(6); f.lng.value = e.latlng.lng.toFixed(6);
});
$('station-form').addEventListener('submit', async e => {
  e.preventDefault();
  const f = e.target;
  try {
    await api('POST', '/api/operator/stations', { name: f.name.value, lat: Number(f.lat.value), lng: Number(f.lng.value) });
    f.reset(); toast('Station added'); await loadStations();
  } catch (err) { toast(err.message, 'alert'); }
});
$('station-list').addEventListener('click', async e => {
  const id = e.target.dataset.delStation;
  if (!id || !confirm('Delete this station?')) return;
  try { await api('DELETE', '/api/operator/stations/' + id); await loadStations(); } catch (err) { toast(err.message, 'alert'); }
});
async function loadStations() {
  const stations = await api('GET', '/api/stations');
  st.stations = new Map(stations.map(s => [s.id, s]));
  drawStations(); renderStations();
}

// ---------- init + live updates ----------
async function init() {
  await Promise.all([loadRoutes(), loadStations()]);
  await Promise.all([loadBuses(), loadAlerts()]);
  st.heat.setLatLngs((await api('GET', '/api/operator/heatmap')).map(p => [p.lat, p.lng, p.weight]));
  connectWS(['buses', 'operator:heatmap', 'operator:alerts'], msg => {
    if (msg.type === 'heatmap') {
      const max = Math.max(1, ...msg.data.map(p => p.weight));
      st.heat.setOptions({ max });
      st.heat.setLatLngs(msg.data.map(p => [p.lat, p.lng, p.weight]));
    } else if (msg.type === 'bus_location') {
      const b = st.buses.get(msg.data.busId);
      if (b) { b.lastLat = msg.data.lat; b.lastLng = msg.data.lng; upsertBus(b); }
    } else if (msg.type === 'densities') {
      msg.data.buses.forEach(upsertBus);
      msg.data.routes.forEach(r => { const s = st.states.get(r.routeId) || { routeId: r.routeId }; s.level = r.level; st.states.set(r.routeId, s); });
      renderBuses(); renderRoutes();
    } else if (msg.type === 'alert') {
      notify(msg.data); loadAlerts(); loadRoutes();
    } else if (msg.type === 'override') {
      loadAlerts(); loadBuses();
    }
  });
  // Route "High for" timers come from the persisted state; refresh it periodically.
  setInterval(loadRoutes, 15000);
}
init().catch(err => toast(err.message, 'alert'));
