//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"agilli/internal/bl"
	"agilli/internal/domain"
)

// ---------- repositories ----------

func TestRepositories(t *testing.T) {
	e := setup(t)
	ctx := e.ctx

	routes, err := e.store.Routes.List(ctx)
	if err != nil || len(routes) != 4 {
		t.Fatalf("routes: %d %v", len(routes), err)
	}
	r65 := routeByCode(t, e, "65")
	if len(r65.Stops) != 6 || r65.Stops[0].Name != "Icherisheher" || r65.Stops[5].Name != "Koroglu" {
		t.Fatalf("route stops not in order: %+v", r65.Stops)
	}

	found, err := e.store.Buses.Search(ctx, "65")
	if err != nil || len(found) != 2 {
		t.Fatalf("search by route code: %d %v", len(found), err)
	}
	found, _ = e.store.Buses.Search(ctx, "koroglu")
	if len(found) != 2 {
		t.Fatalf("search by route name (case-insensitive): %d", len(found))
	}
	found, _ = e.store.Buses.Search(ctx, "%")
	if len(found) != 0 {
		t.Fatalf("LIKE wildcards must be escaped, got %d", len(found))
	}

	bus := e.mustBus(t, "13-A")
	now := e.clock.Now()
	if err := e.store.Buses.UpdatePosition(ctx, bus.ID, 40.39, 49.80, now); err != nil {
		t.Fatal(err)
	}
	if err := e.store.Buses.UpdateDensity(ctx, bus.ID, domain.DensityHigh, ptr(1.8), 70, now); err != nil {
		t.Fatal(err)
	}
	got, _ := e.store.Buses.Get(ctx, bus.ID)
	if *got.LastLat != 40.39 || got.Density != domain.DensityHigh || got.PassengerCount != 70 {
		t.Fatalf("bus not updated: %+v", got)
	}
	if _, err := e.store.Buses.Get(ctx, 99999); err != domain.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	// latest location per session
	for i, s := range []string{"s1", "s1", "s2"} {
		lats := []float64{40.400, 40.401, 40.402}
		_ = e.store.Locations.AddUserLocation(ctx, domain.UserLocation{SessionID: s, Lat: lats[i], Lng: 49.8, CreatedAt: now.Add(time.Duration(i) * time.Second)})
	}
	latest, err := e.store.Locations.LatestPerSessionSince(ctx, now.Add(-time.Minute))
	if err != nil || len(latest) != 2 {
		t.Fatalf("latest per session: %d %v", len(latest), err)
	}
	for _, l := range latest {
		if l.SessionID == "s1" && l.Lat != 40.401 {
			t.Fatalf("s1 should be its newest point, got %v", l.Lat)
		}
	}

	// one open anomaly per route (partial unique index)
	a := &domain.Anomaly{RouteID: r65.ID, StartedAt: now, DetectedAt: now}
	if err := e.store.Anomalies.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := e.store.Anomalies.Create(ctx, &domain.Anomaly{RouteID: r65.ID, StartedAt: now, DetectedAt: now}); err == nil {
		t.Fatal("second open anomaly for the same route must be rejected")
	}
}

// ---------- auth + roles over HTTP ----------

func TestAuthAndRoles(t *testing.T) {
	e := setup(t)
	anon := e.newClient(t)
	expect(t, "anon report", anon.do("POST", "/api/buses/1/density", map[string]string{"density": "high"}, nil), 401)
	expect(t, "anon heatmap", anon.do("GET", "/api/operator/heatmap", nil, nil), 401)
	expect(t, "anon search is public", anon.do("GET", "/api/buses?q=65", nil, nil), 200)

	p := e.newClient(t)
	var reg struct{ Session bl.Session }
	expect(t, "register", p.do("POST", "/api/auth/register", map[string]string{"email": "rider@agilli.az", "password": "secret1"}, &reg), 200)
	if reg.Session.Role != domain.RolePassenger {
		t.Fatalf("role %s", reg.Session.Role)
	}
	expect(t, "duplicate register", e.newClient(t).do("POST", "/api/auth/register", map[string]string{"email": "rider@agilli.az", "password": "secret1"}, nil), 409)
	expect(t, "bad register", e.newClient(t).do("POST", "/api/auth/register", map[string]string{"email": "x", "password": "1"}, nil), 400)
	expect(t, "passenger heatmap", p.do("GET", "/api/operator/heatmap", nil, nil), 403)
	expect(t, "passenger create bus", p.do("POST", "/api/operator/buses", map[string]any{"name": "X", "passengerCapacity": 10}, nil), 403)
	expect(t, "wrong password", e.newClient(t).do("POST", "/api/auth/login", map[string]string{"email": "rider@agilli.az", "password": "nope00"}, nil), 401)

	op := e.newClient(t)
	op.login("op@agilli.az", "operator123")
	expect(t, "operator heatmap", op.do("GET", "/api/operator/heatmap", nil, nil), 200)
	var me bl.Session
	expect(t, "me", op.do("GET", "/api/auth/me", nil, &me), 200)
	if me.Role != domain.RoleOperator {
		t.Fatalf("me role %s", me.Role)
	}
	expect(t, "logout", op.do("POST", "/api/auth/logout", nil, nil), 200)
	expect(t, "after logout", op.do("GET", "/api/operator/heatmap", nil, nil), 401)

	// device endpoint uses its API key, not a session
	expect(t, "device no key", anon.do("POST", "/api/device/buses/1/location", map[string]float64{"lat": 40.4, "lng": 49.8}, nil), 401)
	expect(t, "device key", anon.do("POST", "/api/device/buses/1/location", map[string]float64{"lat": 40.4, "lng": 49.8}, nil, "X-Device-Key", deviceKey), 201)
	expect(t, "device bad coords", anon.do("POST", "/api/device/buses/1/location", map[string]float64{"lat": 140, "lng": 49.8}, nil, "X-Device-Key", deviceKey), 400)
	expect(t, "device unknown bus", anon.do("POST", "/api/device/buses/999/location", map[string]float64{"lat": 40.4, "lng": 49.8}, nil, "X-Device-Key", deviceKey), 404)

	// passenger page issues a guest cookie
	g := e.newClient(t)
	expect(t, "passenger page", g.do("GET", "/", nil, nil), 200)
	expect(t, "guest can share location", g.do("POST", "/api/locations", map[string]float64{"lat": 40.4, "lng": 49.85}, nil), 201)
	// operator page redirects non-operators to login
	res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Get(e.server.URL + "/operator")
	if err != nil || res.StatusCode != http.StatusSeeOther {
		t.Fatalf("operator page should redirect, got %v %v", res.StatusCode, err)
	}
}

// ---------- operator CRUD ----------

func TestOperatorCRUD(t *testing.T) {
	e := setup(t)
	op := e.newClient(t)
	op.login("op@agilli.az", "operator123")

	var s1, s2 domain.Station
	expect(t, "create station", op.do("POST", "/api/operator/stations", map[string]any{"name": "Bayil", "lat": 40.345, "lng": 49.835}, &s1), 201)
	expect(t, "create station 2", op.do("POST", "/api/operator/stations", map[string]any{"name": "Badamdar", "lat": 40.335, "lng": 49.81}, &s2), 201)
	expect(t, "invalid station", op.do("POST", "/api/operator/stations", map[string]any{"name": "", "lat": 40.3, "lng": 49.8}, nil), 400)

	var r domain.Route
	expect(t, "route with one stop", op.do("POST", "/api/operator/routes", map[string]any{"code": "100", "name": "Bayil loop", "stationIds": []int64{s1.ID}}, nil), 400)
	expect(t, "create route", op.do("POST", "/api/operator/routes", map[string]any{"code": "100", "name": "Bayil loop", "stationIds": []int64{s1.ID, s2.ID}}, &r), 201)
	if len(r.Stops) != 2 || r.Stops[0].ID != s1.ID {
		t.Fatalf("route stops: %+v", r.Stops)
	}
	expect(t, "duplicate code", op.do("POST", "/api/operator/routes", map[string]any{"code": "100", "name": "dup", "stationIds": []int64{s1.ID, s2.ID}}, nil), 409)
	expect(t, "reorder stops", op.do("PUT", fmt.Sprintf("/api/operator/routes/%d", r.ID), map[string]any{"code": "100", "name": "Bayil loop", "stationIds": []int64{s2.ID, s1.ID}}, &r), 200)
	if r.Stops[0].ID != s2.ID {
		t.Fatalf("reorder failed: %+v", r.Stops)
	}
	expect(t, "station used by a route", op.do("DELETE", fmt.Sprintf("/api/operator/stations/%d", s1.ID), nil, nil), 409)

	var b domain.Bus
	expect(t, "create bus", op.do("POST", "/api/operator/buses", map[string]any{"name": "100-A", "passengerCapacity": 60, "routeId": r.ID}, &b), 201)
	expect(t, "invalid bus", op.do("POST", "/api/operator/buses", map[string]any{"name": "Z", "passengerCapacity": 0}, nil), 400)
	expect(t, "update bus", op.do("PUT", fmt.Sprintf("/api/operator/buses/%d", b.ID), map[string]any{"name": "100-A", "passengerCapacity": 90, "routeId": r.ID}, &b), 200)
	var detail bl.BusDetail
	expect(t, "bus detail", op.do("GET", fmt.Sprintf("/api/buses/%d", b.ID), nil, &detail), 200)
	if detail.Bus.Capacity != 90 || detail.Route == nil || detail.Route.Code != "100" {
		t.Fatalf("detail: %+v", detail)
	}
	expect(t, "delete bus", op.do("DELETE", fmt.Sprintf("/api/operator/buses/%d", b.ID), nil, nil), 204)
	expect(t, "deleted bus", op.do("GET", fmt.Sprintf("/api/buses/%d", b.ID), nil, nil), 404)
	expect(t, "delete route", op.do("DELETE", fmt.Sprintf("/api/operator/routes/%d", r.ID), nil, nil), 204)
	expect(t, "delete station after route", op.do("DELETE", fmt.Sprintf("/api/operator/stations/%d", s1.ID), nil, nil), 204)
}

// ---------- passenger features ----------

func TestPassengerFeatures(t *testing.T) {
	e := setup(t)
	ctx := e.ctx
	bus := e.mustBus(t, "65-A")
	r65 := routeByCode(t, e, "65")
	// bus at the first stop, then 300 m further every minute
	for i := 0; i < 4; i++ {
		a, b := r65.Stops[0], r65.Stops[1]
		f := float64(i) * 0.15
		if err := e.location.RecordBus(ctx, bus.ID, a.Lat+(b.Lat-a.Lat)*f, a.Lng+(b.Lng-a.Lng)*f); err != nil {
			t.Fatal(err)
		}
		e.clock.Advance(time.Minute)
	}

	c := e.newClient(t)
	c.guest()
	var eta bl.ETAResult
	expect(t, "eta", c.do("GET", fmt.Sprintf("/api/buses/%d/eta?station_id=%d", bus.ID, r65.Stops[2].ID), nil, &eta), 200)
	if eta.DistanceM <= 0 || eta.Seconds <= 0 || eta.SpeedKmh <= 3 {
		t.Fatalf("eta: %+v", eta)
	}
	expect(t, "eta station not on route", c.do("GET", fmt.Sprintf("/api/buses/%d/eta?station_id=%d", bus.ID, routeByCode(t, e, "13").Stops[0].ID), nil, nil), 400)

	var near []domain.Station
	expect(t, "nearby stations", c.do("GET", fmt.Sprintf("/api/stations/near?lat=%f&lng=%f&radius=300", r65.Stops[0].Lat, r65.Stops[0].Lng), nil, &near), 200)
	if len(near) == 0 || near[0].ID != r65.Stops[0].ID {
		t.Fatalf("nearby: %+v", near)
	}

	// density report: accepted, then rate-limited
	path := fmt.Sprintf("/api/buses/%d/density", bus.ID)
	expect(t, "invalid density", c.do("POST", path, map[string]string{"density": "packed"}, nil), 400)
	expect(t, "report", c.do("POST", path, map[string]string{"density": "high"}, nil), 201)
	expect(t, "rate limited", c.do("POST", path, map[string]string{"density": "high"}, nil), 429)
	got, _ := e.store.Buses.Get(ctx, bus.ID)
	if got.Density != domain.DensityHigh {
		t.Fatalf("report should update bus density immediately, got %s", got.Density)
	}

	// a user whose last known location is far from the bus cannot report it
	far := e.newClient(t)
	far.guest()
	expect(t, "far location", far.do("POST", "/api/locations", map[string]float64{"lat": 40.45, "lng": 49.95}, nil), 201)
	expect(t, "far report", far.do("POST", path, map[string]string{"density": "low"}, nil), 400)
}

// Riders sharing their location raise the bus's estimated passenger count.
func TestPassengerCountFromUserLocations(t *testing.T) {
	e := setup(t)
	ctx := e.ctx
	bus := e.mustBus(t, "13-A")
	r := routeByCode(t, e, "13")
	a, b := r.Stops[1], r.Stops[2]
	var riders []bl.Session
	for i := 0; i < 25; i++ {
		u, _ := e.auth.Guest(ctx)
		_, s := e.auth.IssueToken(u)
		riders = append(riders, s)
	}
	for step := 0; step < 2; step++ {
		f := 0.3 + 0.2*float64(step) // mid-segment, away from stations
		lat, lng := a.Lat+(b.Lat-a.Lat)*f, a.Lng+(b.Lng-a.Lng)*f
		if err := e.location.RecordBus(ctx, bus.ID, lat, lng); err != nil {
			t.Fatal(err)
		}
		for _, s := range riders {
			if err := e.location.RecordUser(ctx, s, lat+0.00005, lng); err != nil {
				t.Fatal(err)
			}
		}
		e.clock.Advance(time.Minute)
	}
	levels, err := e.density.Recompute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := e.store.Buses.Get(ctx, bus.ID)
	if got.PassengerCount != 75 || got.Density != domain.DensityHigh {
		t.Fatalf("25 riders x3 = 75 of 80 => High; got count %d density %s", got.PassengerCount, got.Density)
	}
	if levels[r.ID].BusCount != 3 {
		t.Fatalf("route bus count %d", levels[r.ID].BusCount)
	}
}

// ---------- WebSocket ----------

func TestWebSocket(t *testing.T) {
	e := setup(t)
	u, _ := e.auth.Guest(e.ctx)
	token, _ := e.auth.IssueToken(u)
	wsURL := "ws" + strings.TrimPrefix(e.server.URL, "http") + "/ws"
	hdr := http.Header{"Cookie": {"agilli_session=" + token}}

	if _, res, err := websocket.DefaultDialer.Dial(wsURL, nil); err == nil || res.StatusCode != 401 {
		t.Fatal("websocket without a session must be rejected")
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	type msg struct {
		Channel string          `json:"channel"`
		Type    string          `json:"type"`
		Data    json.RawMessage `json:"data"`
	}
	read := func() msg {
		var m msg
		if err := conn.ReadJSON(&m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	_ = conn.WriteJSON(map[string]string{"action": "subscribe", "channel": "operator:heatmap"})
	if m := read(); m.Type != "subscribed" || string(m.Data) != `{"ok":false}` {
		t.Fatalf("guest must not join operator channels: %+v %s", m, m.Data)
	}
	_ = conn.WriteJSON(map[string]string{"action": "subscribe", "channel": "buses"})
	if m := read(); string(m.Data) != `{"ok":true}` {
		t.Fatalf("subscribe buses: %s", m.Data)
	}

	bus := e.mustBus(t, "5-A")
	c := e.newClient(t)
	expect(t, "device", c.do("POST", fmt.Sprintf("/api/device/buses/%d/location", bus.ID), map[string]float64{"lat": 40.38, "lng": 49.83}, nil, "X-Device-Key", deviceKey), 201)
	m := read()
	var loc domain.BusLocation
	_ = json.Unmarshal(m.Data, &loc)
	if m.Channel != "buses" || m.Type != "bus_location" || loc.BusID != bus.ID || loc.Lat != 40.38 {
		t.Fatalf("unexpected message %+v %s", m, m.Data)
	}
}

// ---------- anomaly end-to-end ----------

// Route 65 is reported High for 10 minutes => one anomaly, one alert, reroute suggestions from
// low-density routes; the operator approves one; reports turn low => anomaly resolves, the
// reroute keeps running and ends when its duration is over.
func TestAnomalyEndToEnd(t *testing.T) {
	e := setup(t)
	ctx := e.ctx
	r65 := routeByCode(t, e, "65")
	buses, _ := e.store.Buses.List(ctx)

	// Give every bus a position on its route.
	routes, _ := e.store.Routes.List(ctx)
	stops := map[int64][]domain.Station{}
	for _, r := range routes {
		stops[r.ID] = r.Stops
	}
	place := func() {
		for i, b := range buses {
			s := stops[*b.RouteID][i%len(stops[*b.RouteID])]
			if err := e.location.RecordBus(ctx, b.ID, s.Lat+0.0005, s.Lng); err != nil {
				t.Fatal(err)
			}
		}
	}
	reporter := func() bl.Session { u, _ := e.auth.Guest(ctx); _, s := e.auth.IssueToken(u); return s }
	report := func(level string, onRoute65 bool) {
		for _, b := range buses {
			if (*b.RouteID == r65.ID) == onRoute65 {
				if err := e.density.Report(ctx, reporter(), b.ID, level); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	tick := func() {
		if err := e.anomaly.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// minute 0..10: route 65 High, all other routes Low
	for minute := 0; minute <= 10; minute++ {
		place()
		report("high", true)
		report("low", false)
		tick()
		alerts := e.notifier.all()
		if minute < 10 && len(alerts) != 0 {
			t.Fatalf("alert before 10 minutes (minute %d)", minute)
		}
		if minute < 10 {
			e.clock.Advance(time.Minute)
		}
	}
	alerts := e.notifier.all()
	if len(alerts) != 1 || alerts[0].Type != "anomaly_opened" || alerts[0].RouteCode != "65" {
		t.Fatalf("want one anomaly_opened alert for 65, got %+v", alerts)
	}

	op := e.newClient(t)
	op.login("op@agilli.az", "operator123")
	var anoms []bl.AnomalyView
	expect(t, "anomalies", op.do("GET", "/api/operator/anomalies", nil, &anoms), 200)
	if len(anoms) != 1 || anoms[0].Status != "open" || len(anoms[0].Overrides) == 0 {
		t.Fatalf("anomaly with suggestions expected: %+v", anoms)
	}
	for _, o := range anoms[0].Overrides {
		if o.FromRouteCode == "65" || o.Status != "suggested" {
			t.Fatalf("bad suggestion %+v", o)
		}
	}
	// Still High for more minutes: no second alert.
	e.clock.Advance(time.Minute)
	place()
	report("high", true)
	tick()
	if n := len(e.notifier.all()); n != 1 {
		t.Fatalf("duplicate alerts: %d", n)
	}

	// Manual approval.
	top := anoms[0].Overrides[0]
	passenger := e.newClient(t)
	passenger.guest()
	expect(t, "passenger cannot approve", passenger.do("POST", fmt.Sprintf("/api/operator/overrides/%d/approve", top.ID), nil, nil), 403)
	var approved domain.RouteOverride
	expect(t, "approve", op.do("POST", fmt.Sprintf("/api/operator/overrides/%d/approve", top.ID), nil, &approved), 200)
	if approved.Status != "active" || approved.EndsAt == nil {
		t.Fatalf("approved: %+v", approved)
	}
	expect(t, "approve twice", op.do("POST", fmt.Sprintf("/api/operator/overrides/%d/approve", top.ID), nil, nil), 409)
	var detail bl.BusDetail
	expect(t, "rerouted bus", op.do("GET", fmt.Sprintf("/api/buses/%d", top.BusID), nil, &detail), 200)
	if !detail.TemporaryRoute || detail.Route.ID != r65.ID || detail.HomeRoute.ID != top.FromRouteID {
		t.Fatalf("bus should temporarily serve route 65: %+v", detail)
	}
	if len(anoms[0].Overrides) > 1 {
		expect(t, "reject another", op.do("POST", fmt.Sprintf("/api/operator/overrides/%d/reject", anoms[0].Overrides[1].ID), nil, nil), 200)
	}

	// Density drops: after the old High reports age out, two non-High checks resolve the anomaly.
	e.clock.Advance(e.cfg.ReportWindow + time.Minute)
	for i := 0; i < 2; i++ {
		place()
		report("low", true)
		tick()
		e.clock.Advance(time.Minute)
	}
	alerts = e.notifier.all()
	if len(alerts) != 2 || alerts[1].Type != "anomaly_resolved" {
		t.Fatalf("want resolved alert, got %+v", alerts)
	}
	expect(t, "anomalies", op.do("GET", "/api/operator/anomalies", nil, &anoms), 200)
	if anoms[0].Status != "resolved" || anoms[0].ResolvedAt == nil {
		t.Fatalf("anomaly not resolved: %+v", anoms[0].Anomaly)
	}
	statuses := map[string]int{}
	for _, o := range anoms[0].Overrides {
		statuses[o.Status]++
	}
	// The approved reroute keeps running (it is what relieved the route); unused suggestions expire.
	if statuses["active"] != 1 || statuses["suggested"] != 0 {
		t.Fatalf("overrides after resolve: %v", statuses)
	}
	still, _ := e.store.Buses.Get(ctx, top.BusID)
	if still.ActiveRouteID == nil || *still.ActiveRouteID != r65.ID {
		t.Fatal("rerouted bus should stay on route 65 after the anomaly resolves")
	}
	// It returns home when the reroute's duration ends.
	e.clock.Advance(e.cfg.OverrideDuration)
	tick()
	back, _ := e.store.Buses.Get(ctx, top.BusID)
	if back.ActiveRouteID != nil {
		t.Fatal("bus should be back on its home route after the reroute expires")
	}
}

// An approved reroute ends by itself after OverrideDuration, and can be ended early.
func TestOverrideExpiryAndEnd(t *testing.T) {
	e := setup(t)
	ctx := e.ctx
	r65 := routeByCode(t, e, "65")
	b1, b2 := e.mustBus(t, "13-A"), e.mustBus(t, "5-A")
	mk := func(b *domain.Bus) *domain.RouteOverride {
		o := &domain.RouteOverride{BusID: b.ID, FromRouteID: *b.RouteID, ToRouteID: r65.ID, Status: domain.OverrideSuggested, CreatedAt: e.clock.Now()}
		if err := e.store.Overrides.Create(ctx, o); err != nil {
			t.Fatal(err)
		}
		return o
	}
	o1, o2 := mk(b1), mk(b2)
	if _, err := e.anomaly.Approve(ctx, o1.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.anomaly.Approve(ctx, o2.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.anomaly.End(ctx, o2.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.store.Buses.Get(ctx, b2.ID); got.ActiveRouteID != nil {
		t.Fatal("ended override must restore the home route")
	}
	e.clock.Advance(e.cfg.OverrideDuration)
	if err := e.anomaly.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := e.store.Overrides.Get(ctx, o1.ID)
	if got.Status != domain.OverrideEnded {
		t.Fatalf("override should expire after its duration, got %s", got.Status)
	}
	if bus, _ := e.store.Buses.Get(ctx, b1.ID); bus.ActiveRouteID != nil {
		t.Fatal("expired override must restore the home route")
	}
	if err := e.fleet.DeleteBus(ctx, b1.ID); err != nil {
		t.Fatalf("bus without active override can be deleted: %v", err)
	}
}

// ---------- helpers ----------

func ptr[T any](v T) *T { return &v }

func routeByCode(t *testing.T, e *env, code string) domain.Route {
	t.Helper()
	routes, err := e.store.Routes.List(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range routes {
		if r.Code == code {
			return r
		}
	}
	t.Fatalf("route %s not found", code)
	return domain.Route{}
}

func (e *env) mustBus(t *testing.T, name string) *domain.Bus {
	t.Helper()
	buses, err := e.store.Buses.List(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range buses {
		if b.Name == name {
			return &b
		}
	}
	t.Fatalf("bus %s not found", name)
	return nil
}
