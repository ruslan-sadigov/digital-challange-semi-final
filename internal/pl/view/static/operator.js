// Operator panel: live heatmap, buses, route densities, anomaly alerts, reroute approval, CRUD.
const map = makeMap();
const $ = id => document.getElementById(id);
const st = {
  buses: new Map(), markers: new Map(), routes: new Map(), stations: new Map(), states: new Map(),
  heat: L.heatLayer([], { radius: 28, blur: 18, maxZoom: 16, minOpacity: 0.45 }).addTo(map), stationLayer: L.layerGroup().addTo(map),
  routeLayer: null, shownRoute: null, editStops: [], editRoute: null, editBus: null, tab: 'alerts', anoms: [], active: [],
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
  if (!('Notification' in window)) return toast(t('op.notify.unsupported'), 'alert');
  const p = await Notification.requestPermission();
  toast(t(p === 'granted' ? 'op.notify.on' : 'op.notify.off'), p === 'granted' ? 'success' : 'alert');
});
// Alerts are rendered in the operator's language from their data fields.
function alertText(a) {
  return a.type === 'anomaly_opened'
    ? t('alert.opened', { code: a.routeCode, d: fmtDuration(a.highForSeconds || 0), n: a.suggestions || 0 })
    : t('alert.resolved', { code: a.routeCode });
}
function notify(a) {
  const body = alertText(a);
  toast(body, a.type === 'anomaly_opened' ? 'alert' : '');
  if ('Notification' in window && Notification.permission === 'granted') {
    new Notification(t(a.type === 'anomaly_opened' ? 'alert.opened.title' : 'alert.resolved.title', { code: a.routeCode }), { body });
  }
}

// ---------- map layers ----------
function routeCode(id) { const r = st.routes.get(id); return r ? r.code : '-'; }
function busCode(b) { return routeCode(b.activeRouteId || b.routeId); }
function upsertBus(b) {
  st.buses.set(b.id, b);
  if (b.lastLat == null) return;
  let m = st.markers.get(b.id);
  if (!m) { m = L.marker([b.lastLat, b.lastLng], { icon: busIcon(b, busCode(b)) }).addTo(map); st.markers.set(b.id, m); }
  else { m.setLatLng([b.lastLat, b.lastLng]); m.setIcon(busIcon(b, busCode(b))); }
  m.bindTooltip(`${t('bus.tooltip', { name: b.name, code: busCode(b), density: levelLabel(b.density) })} · ~${b.currentPassengerCount}/${b.passengerCapacity}`);
}
function drawStations() {
  st.stationLayer.clearLayers();
  st.stations.forEach(s => {
    L.circleMarker([s.lat, s.lng], { radius: 5, weight: 2, color: mapColor.ink(), fillColor: mapColor.fill(), fillOpacity: 1 })
      .bindTooltip(s.name).on('click', () => stationClicked(s)).addTo(st.stationLayer);
  });
}
function showRoute(r) {
  st.shownRoute = r;
  if (st.routeLayer) { map.removeLayer(st.routeLayer); st.routeLayer = null; }
  if (!r || !r.stops.length) return;
  const pts = r.path && r.path.length > 1 ? r.path : [...r.stops.map(s => [s.lat, s.lng]), [r.stops[0].lat, r.stops[0].lng]];
  st.routeLayer = L.polyline(pts, { color: mapColor.ink(), weight: 5, opacity: .8, lineCap: 'round' }).addTo(map);
}

// ---------- alerts ----------
// The reroute interval is chosen by the operator; the default keeps it until ended manually.
const DURATIONS = [0, 15, 30, 60, 120, 240];
function durationOptions() { return DURATIONS.map(v => `<option value="${v}">${esc(t('dur.' + v))}</option>`).join(''); }

function renderAlerts() {
  const anoms = st.anoms, active = st.active;
  const open = anoms.filter(a => a.status === 'open');
  $('alert-count').textContent = open.length;
  $('alert-count').classList.toggle('hidden', !open.length);
  $('active-overrides').innerHTML = active.map(o => `
    <div class="item static"><div class="row"><span><strong>${esc(o.busName)}</strong> ${esc(o.fromRouteCode)} → ${esc(o.toRouteCode)}</span>
    <button class="btn small secondary" data-end="${o.id}">${esc(t('btn.end'))}</button></div>
    <div class="muted small-text">${esc(o.endsAt ? t('alerts.ends', { t: fmtTime(o.endsAt) }) : t('alerts.manual'))}</div></div>`).join('')
    || emptyState(t('alerts.active.empty.title'), t('alerts.active.empty.text'));
  $('anomalies').innerHTML = anoms.map(a => `
    <div class="item static">
      <div class="row"><strong>${esc(t('route.label', { code: a.routeCode }))}</strong><span class="badge ${a.status === 'open' ? 'd-high' : 'd-low'}">${esc(t('status.' + a.status))}</span></div>
      <div class="muted small-text">${esc(t('alerts.since', { t: fmtTime(a.startedAt), d: fmtTime(a.detectedAt) }))}${a.resolvedAt ? esc(t('alerts.resolved', { t: fmtTime(a.resolvedAt) })) : ''}</div>
      ${a.overrides.map(o => `<div class="suggestion">
        <div class="row"><span>${esc(t('alerts.move', { bus: o.busName, route: o.fromRouteCode }))}</span><span class="muted small-text nowrap">${o.distanceKm.toFixed(1)} km</span></div>
        <div class="row gap">${o.status === 'suggested'
          ? `<select class="input compact" data-duration="${o.id}" aria-label="${esc(t('alerts.interval'))}">${durationOptions()}</select>
             <button class="btn small" data-approve="${o.id}">${esc(t('btn.approve'))}</button><button class="btn small secondary" data-reject="${o.id}">${esc(t('btn.reject'))}</button>`
          : `<span class="muted small-text">${esc(t('status.' + o.status))}</span>`}</div></div>`).join('')
        || (a.status === 'open' ? `<p class="muted small-text">${esc(t('alerts.nobus'))}</p>` : '')}
    </div>`).join('') || emptyState(t('alerts.empty.title'), t('alerts.empty.text'));
}
async function loadAlerts() {
  // keep the operator's interval choices while the list refreshes
  const picked = {};
  document.querySelectorAll('[data-duration]').forEach(s => { picked[s.dataset.duration] = s.value; });
  [st.anoms, st.active] = await Promise.all([api('GET', '/api/operator/anomalies'), api('GET', '/api/operator/overrides')]);
  renderAlerts();
  document.querySelectorAll('[data-duration]').forEach(s => { if (picked[s.dataset.duration]) s.value = picked[s.dataset.duration]; });
}
document.querySelector('[data-pane="alerts"]').addEventListener('click', async e => {
  const el = e.target;
  const act = el.dataset.approve ? ['approve', el.dataset.approve] : el.dataset.reject ? ['reject', el.dataset.reject] : el.dataset.end ? ['end', el.dataset.end] : null;
  if (!act) return;
  const body = act[0] === 'approve'
    ? { durationMinutes: Number(document.querySelector(`[data-duration="${act[1]}"]`)?.value || 0) } : undefined;
  try {
    await api('POST', `/api/operator/overrides/${act[1]}/${act[0]}`, body);
    toast(t({ approve: 'reroute.approved', reject: 'reroute.rejected', end: 'reroute.ended' }[act[0]]), 'success');
  } catch (err) { toast(err.message, 'alert'); }
  loadAlerts(); loadBuses();
});

// ---------- routes ----------
function renderRoutes() {
  $('route-list').innerHTML = [...st.routes.values()].map(r => {
    const s = st.states.get(r.id) || {};
    const high = s.highSince ? ' · ' + t('routes.highfor', { d: fmtDuration((Date.now() - new Date(s.highSince)) / 1000) }) : '';
    return `<div class="item" data-route="${r.id}">
      <div class="row"><strong>${esc(r.code)} · ${esc(r.name)}</strong>${densityBadge(s.level)}</div>
      <div class="row"><span class="muted small-text">${esc(t('routes.stops', { n: r.stops.length }) + high)}</span>
      <span class="actions"><button class="btn small ghost" data-edit-route="${r.id}">${esc(t('btn.edit'))}</button>
      <button class="btn small ghost danger" data-del-route="${r.id}">${esc(t('btn.delete'))}</button></span></div></div>`;
  }).join('');
  const sel = $('bus-route-select');
  const cur = sel.value;
  sel.innerHTML = `<option value="">${esc(t('busf.noroute'))}</option>` + [...st.routes.values()].map(r => `<option value="${r.id}">${esc(r.code)} · ${esc(r.name)}</option>`).join('');
  sel.value = cur;
}
function renderFormTitles() {
  $('route-form-title').textContent = st.editRoute ? t('routes.edit', { code: st.editRoute.code }) : t('routes.new');
  $('bus-form-title').textContent = st.editBus ? t('busf.edit', { name: st.editBus.name }) : t('busf.new');
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
  const el = e.target;
  if (el.dataset.editRoute) {
    const r = st.routes.get(Number(el.dataset.editRoute));
    const f = $('route-form');
    f.id.value = r.id; f.code.value = r.code; f.name.value = r.name;
    st.editStops = [...r.stops];
    st.editRoute = r;
    renderFormTitles();
    renderEditStops();
  } else if (el.dataset.delRoute) {
    if (!confirm(t('routes.confirm'))) return;
    try { await api('DELETE', '/api/operator/routes/' + el.dataset.delRoute); await loadRoutes(); } catch (err) { toast(err.message, 'alert'); }
  } else {
    const it = el.closest('[data-route]');
    if (it) showRoute(st.routes.get(Number(it.dataset.route)));
  }
});
$('route-undo').addEventListener('click', () => { st.editStops.pop(); renderEditStops(); });
$('route-reset').addEventListener('click', () => { st.editStops = []; st.editRoute = null; renderFormTitles(); $('route-form').id.value = ''; renderEditStops(); });
$('route-form').addEventListener('submit', async e => {
  e.preventDefault();
  const f = e.target;
  const body = { code: f.code.value, name: f.name.value, stationIds: st.editStops.map(s => s.id) };
  try {
    if (f.id.value) await api('PUT', '/api/operator/routes/' + f.id.value, body);
    else await api('POST', '/api/operator/routes', body);
    toast(t('routes.saved'), 'success'); f.reset(); $('route-reset').click(); await loadRoutes();
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
    <div class="item static"><div class="row"><strong>${esc(b.name)}</strong>${densityBadge(b.density)}</div>
    <div class="row"><span class="muted small-text">${esc(t('route.label', { code: busCode(b) }))}${b.activeRouteId ? ' ' + esc(t('busf.temporary')) : ''} · ~${b.currentPassengerCount}/${b.passengerCapacity}</span>
    <span class="actions"><button class="btn small ghost" data-edit-bus="${b.id}">${esc(t('btn.edit'))}</button><button class="btn small ghost danger" data-del-bus="${b.id}">${esc(t('btn.delete'))}</button></span></div></div>`).join('')
    || emptyState(t('busf.empty.title'), t('busf.empty.text'));
}
$('op-bus-list').addEventListener('click', async e => {
  const el = e.target;
  if (el.dataset.editBus) {
    const b = st.buses.get(Number(el.dataset.editBus));
    const f = $('bus-form');
    f.id.value = b.id; f.name.value = b.name; f.passengerCapacity.value = b.passengerCapacity; f.routeId.value = b.routeId || '';
    st.editBus = b;
    renderFormTitles();
  } else if (el.dataset.delBus) {
    if (!confirm(t('busf.confirm'))) return;
    try {
      await api('DELETE', '/api/operator/buses/' + el.dataset.delBus);
      const m = st.markers.get(Number(el.dataset.delBus)); if (m) map.removeLayer(m);
      st.markers.delete(Number(el.dataset.delBus));
      await loadBuses();
    } catch (err) { toast(err.message, 'alert'); }
  }
});
$('bus-reset').addEventListener('click', () => { $('bus-form').id.value = ''; st.editBus = null; renderFormTitles(); });
$('bus-form').addEventListener('submit', async e => {
  e.preventDefault();
  const f = e.target;
  const body = { name: f.name.value, passengerCapacity: Number(f.passengerCapacity.value), routeId: f.routeId.value ? Number(f.routeId.value) : null };
  try {
    if (f.id.value) await api('PUT', '/api/operator/buses/' + f.id.value, body);
    else await api('POST', '/api/operator/buses', body);
    toast(t('busf.saved'), 'success'); f.reset(); f.id.value = ''; st.editBus = null; renderFormTitles(); await loadBuses();
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
    <div class="item static"><div class="row"><span>${esc(s.name)}</span><button class="btn small ghost danger" data-del-station="${s.id}">${esc(t('btn.delete'))}</button></div></div>`).join('')
    || emptyState(t('st.empty.title'), t('st.empty.text'));
  $('station-count').textContent = st.stations.size ? t('st.count', { n: st.stations.size }) : '';
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
    f.reset(); toast(t('st.added'), 'success'); await loadStations();
  } catch (err) { toast(err.message, 'alert'); }
});
$('station-list').addEventListener('click', async e => {
  const id = e.target.dataset.delStation;
  if (!id || !confirm(t('st.confirm'))) return;
  try { await api('DELETE', '/api/operator/stations/' + id); await loadStations(); } catch (err) { toast(err.message, 'alert'); }
});
async function loadStations() {
  const stations = await api('GET', '/api/stations');
  st.stations = new Map(stations.map(s => [s.id, s]));
  drawStations(); renderStations();
}

// ---------- language + theme changes ----------
window.addEventListener('langchange', () => {
  renderAlerts(); renderRoutes(); renderBuses(); renderStations(); renderFormTitles();
  st.buses.forEach(upsertBus);
});
window.addEventListener('themechange', () => { drawStations(); showRoute(st.shownRoute); });

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
      if (b) {
        b.lastLat = msg.data.lat; b.lastLng = msg.data.lng;
        if (msg.data.heading != null) b.heading = msg.data.heading;
        upsertBus(b);
      }
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
