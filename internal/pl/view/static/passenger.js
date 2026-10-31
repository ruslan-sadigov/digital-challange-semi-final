// Passenger panel: search buses, live positions, route, density, ETA, density reports, location sharing.
const map = makeMap();
const state = {
  buses: new Map(), markers: new Map(), routes: new Map(), stations: [],
  selected: null, selectedRoute: null, detail: null, routeLayer: null, stationLayer: L.layerGroup().addTo(map), filter: null,
  testPoint: null, testMarker: null, testLine: null, picking: false,
};

const $ = id => document.getElementById(id);

function routeCode(id) { const r = state.routes.get(id); return r ? r.code : '-'; }
function busCode(b) { return routeCode(b.activeRouteId || b.routeId); }

function renderList() {
  const ids = state.filter || [...state.buses.keys()];
  const shown = ids.map(id => state.buses.get(id)).filter(Boolean);
  $('bus-count').textContent = shown.length ? (shown.length === 1 ? t('buses.count1') : t('buses.count', { n: shown.length })) : '';
  $('bus-list').setAttribute('aria-busy', 'false');
  $('bus-list').innerHTML = shown.map(b => `
    <div class="item ${state.selected === b.id ? 'active' : ''}" data-id="${b.id}">
      <div class="row"><strong>${esc(b.name)}</strong>${densityBadge(b.density)}</div>
      <div class="row"><span class="muted">${esc(t('route.label', { code: busCode(b) }))}</span>
      ${b.activeRouteId ? `<span class="badge temp">${esc(t('bus.temporary'))}</span>` : ''}</div>
    </div>`).join('') || emptyState(t('buses.empty.title'), t('buses.empty.text'));
}

function upsertMarker(b) {
  if (b.lastLat == null) return;
  let m = state.markers.get(b.id);
  if (!m) {
    m = L.marker([b.lastLat, b.lastLng], { icon: busIcon(b, busCode(b)) }).addTo(map).on('click', () => select(b.id));
    state.markers.set(b.id, m);
  } else {
    m.setLatLng([b.lastLat, b.lastLng]);
    m.setIcon(busIcon(b, busCode(b)));
  }
  m.bindTooltip(t('bus.tooltip', { name: b.name, code: busCode(b), density: levelLabel(b.density) }));
}

function drawStations() {
  state.stationLayer.clearLayers();
  state.stations.forEach(s => L.circleMarker([s.lat, s.lng], { radius: 3, weight: 1.5, color: mapColor.muted(), fillColor: mapColor.fill(), fillOpacity: 1 })
    .bindTooltip(s.name).addTo(state.stationLayer));
}

function drawRoute(d) {
  if (state.routeLayer) { map.removeLayer(state.routeLayer); state.routeLayer = null; }
  const stops = d && d.route ? d.route.stops : [];
  if (!stops.length) return;
  // Real road geometry when the route has one, otherwise straight lines between stops.
  const pts = d.route.path && d.route.path.length > 1 ? d.route.path : [...stops.map(s => [s.lat, s.lng]), [stops[0].lat, stops[0].lng]];
  state.routeLayer = L.layerGroup([
    L.polyline(pts, { color: d.temporaryRoute ? mapColor.accent() : mapColor.ink(), weight: 5, opacity: .8, dashArray: d.temporaryRoute ? '8 8' : null, lineCap: 'round' }),
    ...stops.map(s => L.circleMarker([s.lat, s.lng], { radius: 6, color: mapColor.ink(), weight: 2, fillColor: mapColor.fill(), fillOpacity: 1 })
      .bindTooltip(s.name).on('click', () => showETA(d.bus.id, s.id))),
  ]).addTo(map);
}

function renderDetail(d) {
  const b = state.buses.get(d.bus.id) || d.bus;
  $('d-name').textContent = b.name;
  $('d-density').outerHTML = densityBadge(b.density).replace('<span ', '<span id="d-density" ');
  $('d-route').textContent = d.route ? t('route.full', { code: d.route.code, name: d.route.name }) : t('route.none');
  $('d-pass').textContent = t('bus.passengers', { n: b.currentPassengerCount, cap: b.passengerCapacity });
  const temp = $('d-temp');
  if (d.temporaryRoute && d.homeRoute) {
    let text = d.joining
      ? t('bus.joining', { to: d.route.code, home: d.homeRoute.code, km: (d.joinDistanceM / 1000).toFixed(1) })
      : t('bus.rerouted', { to: d.route.code, home: d.homeRoute.code });
    if (d.temporaryEndsAt) text += ' ' + t('bus.until', { t: fmtTime(d.temporaryEndsAt) });
    temp.textContent = text;
    temp.classList.remove('hidden');
  } else temp.classList.add('hidden');
}

async function select(id) {
  state.selected = id;
  renderList();
  const d = await api('GET', '/api/buses/' + id);
  state.detail = d;
  state.buses.set(d.bus.id, d.bus);
  $('bus-detail').classList.remove('hidden');
  document.querySelector('.panel').scrollTop = 0; // test-location box, search and detail sit at the top
  renderDetail(d);
  $('d-msg').textContent = '';
  $('d-msg').className = 'help';
  const stops = d.route ? d.route.stops : [];
  state.selectedRoute = d.route;
  $('d-stops').innerHTML = stops.map(s => `<li data-station="${s.id}">${esc(s.name)}<span class="eta"></span></li>`).join('');
  drawRoute(d);
  if (d.bus.lastLat != null) map.panTo([d.bus.lastLat, d.bus.lastLng]);
  updateTestInfo();
}

async function showETA(busId, stationId) {
  const li = document.querySelector(`#d-stops li[data-station="${stationId}"] .eta`);
  try {
    const e = await api('GET', `/api/buses/${busId}/eta?station_id=${stationId}`);
    const time = fmtDuration(e.seconds), km = (e.distanceM / 1000).toFixed(1);
    if (li) li.textContent = `${time} (${km} km)`;
    toast(t('eta.toast', { t: time, km }));
  } catch (err) { toast(err.message, 'alert'); }
}

$('d-stops').addEventListener('click', e => {
  const li = e.target.closest('li[data-station]');
  if (li && state.selected) showETA(state.selected, Number(li.dataset.station));
});

$('d-close').addEventListener('click', () => {
  state.selected = null;
  state.selectedRoute = null;
  state.detail = null;
  $('bus-detail').classList.add('hidden');
  drawRoute(null);
  renderList();
  updateTestInfo();
});

$('bus-list').addEventListener('click', e => {
  const it = e.target.closest('.item');
  if (it) select(Number(it.dataset.id));
});

document.querySelectorAll('[data-density]').forEach(btn => btn.addEventListener('click', async () => {
  if (!state.selected) return;
  try {
    await api('POST', `/api/buses/${state.selected}/density`, { density: btn.dataset.density });
    $('d-msg').className = 'help';
    $('d-msg').textContent = t('density.thanks');
    toast(t('density.sent'), 'success');
  } catch (err) { $('d-msg').className = 'error'; $('d-msg').textContent = err.message; }
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

// ---------- location sharing (foreground, opt-in; interval from the server, 2 min by default) ----------
let locTimer = null;
function syncShareLabel() { $('share-loc').textContent = t(locTimer ? 'loc.stop' : 'loc.share'); }
async function startSharing() {
  if (!navigator.geolocation && !state.testPoint) { toast(t('loc.unavailable'), 'alert'); return; }
  const cfg = await api('GET', '/api/config');
  // While a test location is set, it is sent instead of the real GPS position.
  const send = () => {
    if (state.testPoint) { sendTestPoint(); return; }
    navigator.geolocation.getCurrentPosition(
      p => api('POST', '/api/locations', { lat: p.coords.latitude, lng: p.coords.longitude }).catch(() => {}),
      () => toast(t('loc.denied'), 'alert'), { enableHighAccuracy: true, timeout: 10000 });
  };
  send();
  locTimer = setInterval(send, cfg.userSampleIntervalMs);
  syncShareLabel();
  try { localStorage.setItem('shareLoc', '1'); } catch (_) {}
}
function stopSharing() {
  clearInterval(locTimer); locTimer = null;
  syncShareLabel();
  try { localStorage.removeItem('shareLoc'); } catch (_) {}
}
$('share-loc').addEventListener('click', () => (locTimer ? stopSharing() : startSharing()));
try { if (localStorage.getItem('shareLoc')) startSharing(); } catch (_) {}

// ---------- test location (simulated position for testing ETA, reports and sharing) ----------
const WALK_M_PER_MIN = 80; // ~4.8 km/h
const youIcon = L.divIcon({ className: '', html: '<div class="test-pin"></div>', iconSize: [26, 26], iconAnchor: [13, 26] });

function sendTestPoint() {
  if (!state.testPoint) return;
  api('POST', '/api/locations', { lat: state.testPoint.lat, lng: state.testPoint.lng }).catch(err => toast(err.message, 'alert'));
}

function setPicking(on) {
  state.picking = on;
  $('test-loc').classList.toggle('picking', on);
  $('test-loc').textContent = t(on ? 'loc.picking' : 'loc.test');
  map.getContainer().style.cursor = on ? 'crosshair' : '';
}

function setTestPoint(latlng, save = true) {
  state.testPoint = L.latLng(latlng.lat, latlng.lng);
  if (!state.testMarker) {
    state.testMarker = L.marker(state.testPoint, { icon: youIcon, draggable: true, zIndexOffset: 1000 })
      .bindTooltip(t('test.pin')).addTo(map)
      .on('dragend', e => setTestPoint(e.target.getLatLng()));
  } else state.testMarker.setLatLng(state.testPoint);
  $('testloc').classList.remove('hidden');
  if (save) { try { localStorage.setItem('testPoint', JSON.stringify(state.testPoint)); } catch (_) {} }
  sendTestPoint();
  updateTestInfo();
}

function clearTestPoint() {
  state.testPoint = null;
  if (state.testMarker) { map.removeLayer(state.testMarker); state.testMarker = null; }
  if (state.testLine) { map.removeLayer(state.testLine); state.testLine = null; }
  $('testloc').classList.add('hidden');
  try { localStorage.removeItem('testPoint'); } catch (_) {}
}

let testInfoSeq = 0;
async function updateTestInfo() {
  if (!state.testPoint) return;
  const seq = ++testInfoSeq;
  const p = state.testPoint;
  const box = $('testloc-info');
  if (state.testLine) { map.removeLayer(state.testLine); state.testLine = null; }
  const route = state.selectedRoute;
  if (state.selected && route && route.stops.length) {
    // Nearest stop of the selected bus's route, the bus's ETA there, and the walk to it.
    let best = null;
    route.stops.forEach(s => { const d = distM(p.lat, p.lng, s.lat, s.lng); if (!best || d < best.d) best = { s, d }; });
    const walkMin = best.d / WALK_M_PER_MIN;
    state.testLine = L.polyline([[p.lat, p.lng], [best.s.lat, best.s.lng]], { color: mapColor.muted(), weight: 3, dashArray: '2 8', lineCap: 'round' }).addTo(map);
    try {
      // ETA of every bus currently serving this route to that stop: the selected one plus the soonest you can catch.
      const onRoute = [...state.buses.values()].filter(b => (b.activeRouteId || b.routeId) === route.id && b.lastLat != null);
      const etas = (await Promise.all(onRoute.map(b => api('GET', `/api/buses/${b.id}/eta?station_id=${best.s.id}`)
        .then(e => ({ bus: b, e })).catch(() => null)))).filter(Boolean);
      if (seq !== testInfoSeq) return;
      const mine = etas.find(x => x.bus.id === state.selected);
      if (!mine) throw new Error(t('test.noeta'));
      const catchable = etas.filter(x => x.e.seconds / 60 >= walkMin).sort((a, b) => a.e.seconds - b.e.seconds)[0];
      const busMin = mine.e.seconds / 60;
      const verdict = walkMin <= busMin
        ? `<span class="badge d-low">${esc(t('test.make'))}</span><span class="muted">${esc(t('test.leave', { t: fmtDuration((busMin - walkMin) * 60) }))}</span>`
        : `<span class="badge d-high">${esc(t('test.miss'))}</span><span class="muted">${esc(t('test.miss.why'))}</span>`;
      const better = catchable && catchable.bus.id !== state.selected
        ? `<div class="banner">${esc(t('test.soonest', { bus: catchable.bus.name, t: fmtDuration(catchable.e.seconds) }))}</div>` : '';
      box.innerHTML = `
        <div class="inset kv">
          <span class="k">${esc(t('test.nearest'))}</span><span class="v">${esc(best.s.name)}</span>
          <span class="k">${esc(t('test.walk'))}</span><span class="v">${Math.round(best.d)} m · ${fmtDuration(walkMin * 60)}</span>
          <span class="k">${esc(t('test.arrives', { bus: mine.bus.name }))}</span><span class="v">${fmtDuration(mine.e.seconds)}</span>
          <span class="k">${esc(t('test.road'))}</span><span class="v">${(mine.e.distanceM / 1000).toFixed(1)} km</span>
        </div>
        <div class="verdict">${verdict}</div>${better}`;
    } catch (err) {
      if (seq === testInfoSeq) box.innerHTML = `<p class="error">${esc(err.message)}</p>`;
    }
    return;
  }
  // No bus selected: show the closest real stops.
  try {
    const near = await api('GET', `/api/stations/near?lat=${p.lat}&lng=${p.lng}&radius=800`);
    if (seq !== testInfoSeq) return;
    box.innerHTML = near.length
      ? `<p class="help">${esc(t('test.closest'))}</p><div class="inset kv">` + near.slice(0, 4).map(s => {
          const d = distM(p.lat, p.lng, s.lat, s.lng);
          return `<span class="k">${esc(s.name)}</span><span class="v">${Math.round(d)} m · ${fmtDuration(d / WALK_M_PER_MIN * 60)}</span>`;
        }).join('') + '</div>'
      : emptyState(t('test.none.title'), t('test.none.text'));
  } catch (err) { box.innerHTML = `<p class="error">${esc(err.message)}</p>`; }
}

$('test-loc').addEventListener('click', () => {
  setPicking(!state.picking);
  if (state.picking) toast(t('loc.pick.toast'));
});
$('testloc-clear').addEventListener('click', clearTestPoint);
map.on('click', e => {
  if (!state.picking) return;
  setPicking(false);
  setTestPoint(e.latlng);
});
// The bus keeps moving: refresh the ETA comparison every 15 s.
setInterval(() => { if (state.testPoint && state.selected) updateTestInfo(); }, 15000);
try {
  const saved = JSON.parse(localStorage.getItem('testPoint') || 'null');
  if (saved) setTestPoint(saved, false);
} catch (_) {}

if ($('logout')) $('logout').addEventListener('click', async () => { await api('POST', '/api/auth/logout'); location.href = '/'; });

// ---------- language + theme changes re-render dynamic content ----------
window.addEventListener('langchange', () => {
  renderList();
  state.buses.forEach(upsertMarker);
  if (state.detail) renderDetail(state.detail);
  syncShareLabel();
  setPicking(state.picking);
  if (state.testMarker) state.testMarker.setTooltipContent(t('test.pin'));
  updateTestInfo();
});
window.addEventListener('themechange', () => {
  drawStations();
  if (state.detail) drawRoute(state.detail);
  updateTestInfo();
});

async function init() {
  const [routes, stations, buses] = await Promise.all([api('GET', '/api/routes'), api('GET', '/api/stations'), api('GET', '/api/buses')]);
  routes.forEach(r => state.routes.set(r.id, r));
  state.stations = stations;
  drawStations();
  buses.forEach(b => { state.buses.set(b.id, b); upsertMarker(b); });
  renderList();
  syncShareLabel();
  connectWS(['buses'], msg => {
    if (msg.type === 'bus_location') {
      const b = state.buses.get(msg.data.busId);
      if (!b) return;
      b.lastLat = msg.data.lat; b.lastLng = msg.data.lng;
      if (msg.data.heading != null) b.heading = msg.data.heading;
      upsertMarker(b);
    } else if (msg.type === 'densities') {
      msg.data.buses.forEach(nb => { state.buses.set(nb.id, nb); upsertMarker(nb); });
      renderList();
      if (state.detail) renderDetail(state.detail);
    } else if (msg.type === 'override') {
      api('GET', '/api/buses').then(bs => { bs.forEach(b => { state.buses.set(b.id, b); upsertMarker(b); }); renderList(); });
      if (state.selected === msg.data.busId) select(state.selected);
    }
  });
}
init().catch(err => toast(err.message, 'alert'));
