package bl

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"agilli/internal/dl"
	"agilli/internal/domain"
)

// ---------- fleet (buses, routes, stations) ----------

type FleetService struct {
	store *dl.Store
}

func NewFleetService(store *dl.Store) *FleetService { return &FleetService{store: store} }

func (s *FleetService) Stations(ctx context.Context) ([]domain.Station, error) {
	return s.store.Stations.List(ctx)
}

func validateStation(st *domain.Station) error {
	st.Name = strings.TrimSpace(st.Name)
	if st.Name == "" {
		return domain.ErrInvalidInput("station name is required")
	}
	if !validCoord(st.Lat, st.Lng) {
		return domain.ErrInvalidInput("invalid coordinates")
	}
	return nil
}

func (s *FleetService) CreateStation(ctx context.Context, st *domain.Station) error {
	if err := validateStation(st); err != nil {
		return err
	}
	return s.store.Stations.Create(ctx, st)
}

func (s *FleetService) UpdateStation(ctx context.Context, st *domain.Station) error {
	if err := validateStation(st); err != nil {
		return err
	}
	return s.store.Stations.Update(ctx, st)
}

func (s *FleetService) DeleteStation(ctx context.Context, id int64) error {
	return s.store.Stations.Delete(ctx, id)
}

func (s *FleetService) Routes(ctx context.Context) ([]domain.Route, error) {
	return s.store.Routes.List(ctx)
}

func (s *FleetService) Route(ctx context.Context, id int64) (*domain.Route, error) {
	return s.store.Routes.Get(ctx, id)
}

func validateRoute(r *domain.Route, stationIDs []int64) error {
	r.Code = strings.TrimSpace(r.Code)
	r.Name = strings.TrimSpace(r.Name)
	if r.Code == "" || r.Name == "" {
		return domain.ErrInvalidInput("route code and name are required")
	}
	if len(stationIDs) < 2 {
		return domain.ErrInvalidInput("a route needs at least 2 stops")
	}
	for i := 1; i < len(stationIDs); i++ {
		if stationIDs[i] == stationIDs[i-1] {
			return domain.ErrInvalidInput("the same station cannot appear twice in a row")
		}
	}
	return nil
}

func (s *FleetService) CreateRoute(ctx context.Context, r *domain.Route, stationIDs []int64) error {
	if err := validateRoute(r, stationIDs); err != nil {
		return err
	}
	if err := s.store.Routes.Create(ctx, r, stationIDs); err != nil {
		return err
	}
	full, err := s.store.Routes.Get(ctx, r.ID)
	if err == nil {
		*r = *full
	}
	return err
}

func (s *FleetService) UpdateRoute(ctx context.Context, r *domain.Route, stationIDs []int64) error {
	if err := validateRoute(r, stationIDs); err != nil {
		return err
	}
	if err := s.store.Routes.Update(ctx, r, stationIDs); err != nil {
		return err
	}
	full, err := s.store.Routes.Get(ctx, r.ID)
	if err == nil {
		*r = *full
	}
	return err
}

func (s *FleetService) DeleteRoute(ctx context.Context, id int64) error {
	return s.store.Routes.Delete(ctx, id)
}

func (s *FleetService) Buses(ctx context.Context, q string) ([]domain.Bus, error) {
	if q = strings.TrimSpace(q); q != "" {
		return s.store.Buses.Search(ctx, q)
	}
	return s.store.Buses.List(ctx)
}

func validateBus(b *domain.Bus) error {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return domain.ErrInvalidInput("bus name is required")
	}
	if b.Capacity <= 0 {
		return domain.ErrInvalidInput("passenger capacity must be greater than 0")
	}
	return nil
}

func (s *FleetService) CreateBus(ctx context.Context, b *domain.Bus) error {
	if err := validateBus(b); err != nil {
		return err
	}
	return s.store.Buses.Create(ctx, b)
}

func (s *FleetService) UpdateBus(ctx context.Context, b *domain.Bus) error {
	if err := validateBus(b); err != nil {
		return err
	}
	return s.store.Buses.Update(ctx, b)
}

func (s *FleetService) DeleteBus(ctx context.Context, id int64) error {
	active, err := s.store.Overrides.ListByStatus(ctx, domain.OverrideActive)
	if err != nil {
		return err
	}
	for _, o := range active {
		if o.BusID == id {
			return fmt.Errorf("%w: end the bus's active reroute before deleting it", domain.ErrConflict)
		}
	}
	return s.store.Buses.Delete(ctx, id)
}

// BusDetail is what the passenger sees after selecting a bus.
type BusDetail struct {
	Bus             domain.Bus    `json:"bus"`
	Route           *domain.Route `json:"route"`     // route currently served (temporary one if rerouted)
	HomeRoute       *domain.Route `json:"homeRoute"` // permanent route
	TemporaryRoute  bool          `json:"temporaryRoute"`
	TemporaryEndsAt *time.Time    `json:"temporaryEndsAt"`
}

func (s *FleetService) BusDetail(ctx context.Context, id int64) (*BusDetail, error) {
	b, err := s.store.Buses.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &BusDetail{Bus: *b, TemporaryRoute: b.ActiveRouteID != nil}
	if b.RouteID != nil {
		if d.HomeRoute, err = s.store.Routes.Get(ctx, *b.RouteID); err != nil {
			return nil, err
		}
	}
	d.Route = d.HomeRoute
	if b.ActiveRouteID != nil {
		if d.Route, err = s.store.Routes.Get(ctx, *b.ActiveRouteID); err != nil {
			return nil, err
		}
		active, err := s.store.Overrides.ListByStatus(ctx, domain.OverrideActive)
		if err != nil {
			return nil, err
		}
		for _, o := range active {
			if o.BusID == b.ID {
				d.TemporaryEndsAt = o.EndsAt
			}
		}
	}
	return d, nil
}

// NearbyStations returns stations within radius metres, closest first.
func (s *FleetService) NearbyStations(ctx context.Context, lat, lng, radiusM float64) ([]domain.Station, error) {
	all, err := s.store.Stations.List(ctx)
	if err != nil {
		return nil, err
	}
	type sd struct {
		s domain.Station
		d float64
	}
	var tmp []sd
	for _, st := range all {
		if d := DistanceM(lat, lng, st.Lat, st.Lng); d <= radiusM {
			tmp = append(tmp, sd{st, d})
		}
	}
	for i := 1; i < len(tmp); i++ {
		for j := i; j > 0 && tmp[j].d < tmp[j-1].d; j-- {
			tmp[j], tmp[j-1] = tmp[j-1], tmp[j]
		}
	}
	out := make([]domain.Station, 0, len(tmp))
	for _, t := range tmp {
		out = append(out, t.s)
	}
	return out, nil
}

// ---------- locations ----------

type LocationService struct {
	store *dl.Store
	clock Clock
	pub   Publisher
}

func NewLocationService(store *dl.Store, clock Clock, pub Publisher) *LocationService {
	return &LocationService{store: store, clock: clock, pub: pub}
}

// RecordBus stores a bus GPS point (every ~1 min) and pushes it to live maps.
func (s *LocationService) RecordBus(ctx context.Context, busID int64, lat, lng float64) error {
	if !validCoord(lat, lng) {
		return domain.ErrInvalidInput("invalid coordinates")
	}
	now := s.clock.Now()
	if err := s.store.Buses.UpdatePosition(ctx, busID, lat, lng, now); err != nil {
		return err
	}
	if err := s.store.Locations.AddBusLocation(ctx, domain.BusLocation{BusID: busID, Lat: lat, Lng: lng, CreatedAt: now}); err != nil {
		return err
	}
	s.pub.Publish(ChannelBuses, "bus_location", domain.BusLocation{BusID: busID, Lat: lat, Lng: lng, CreatedAt: now})
	return nil
}

// RecordUser stores a user location sample (every ~2 min, foreground).
func (s *LocationService) RecordUser(ctx context.Context, sess Session, lat, lng float64) error {
	if !validCoord(lat, lng) {
		return domain.ErrInvalidInput("invalid coordinates")
	}
	if sess.SessionID == "" {
		return domain.ErrUnauthorized
	}
	var uid *int64
	if sess.UserID != 0 {
		uid = &sess.UserID
	}
	return s.store.Locations.AddUserLocation(ctx, domain.UserLocation{UserID: uid, SessionID: sess.SessionID, Lat: lat, Lng: lng, CreatedAt: s.clock.Now()})
}

// ---------- ETA ----------

// RouteDistanceM is the distance a bus at (lat,lng) must travel along a circular route to reach stop target.
// The bus is placed on the closest route segment (stop i -> stop i+1), then distance is summed stop by stop.
func RouteDistanceM(stops []domain.Station, lat, lng float64, target int) float64 {
	n := len(stops)
	if n == 0 || target < 0 || target >= n {
		return math.NaN()
	}
	if n == 1 {
		return DistanceM(lat, lng, stops[0].Lat, stops[0].Lng)
	}
	seg, best := 0, math.Inf(1)
	for i := 0; i < n; i++ {
		a, b := stops[i], stops[(i+1)%n]
		if d := DistanceToSegmentM(lat, lng, a.Lat, a.Lng, b.Lat, b.Lng); d < best {
			seg, best = i, d
		}
	}
	next := (seg + 1) % n
	dist := DistanceM(lat, lng, stops[next].Lat, stops[next].Lng)
	for i := next; i != target; i = (i + 1) % n {
		j := (i + 1) % n
		dist += DistanceM(stops[i].Lat, stops[i].Lng, stops[j].Lat, stops[j].Lng)
	}
	return dist
}

// AverageSpeedKmh from a GPS track; ok=false if the track is too short or implausible.
func AverageSpeedKmh(track []domain.BusLocation) (float64, bool) {
	if len(track) < 2 {
		return 0, false
	}
	var dist float64
	for i := 1; i < len(track); i++ {
		dist += DistanceM(track[i-1].Lat, track[i-1].Lng, track[i].Lat, track[i].Lng)
	}
	secs := track[len(track)-1].CreatedAt.Sub(track[0].CreatedAt).Seconds()
	if secs <= 0 || dist <= 0 {
		return 0, false
	}
	kmh := dist / secs * 3.6
	if kmh < 3 || kmh > 80 {
		return 0, false
	}
	return kmh, true
}

type ETAResult struct {
	BusID     int64   `json:"busId"`
	StationID int64   `json:"stationId"`
	DistanceM float64 `json:"distanceM"`
	SpeedKmh  float64 `json:"speedKmh"`
	Seconds   float64 `json:"seconds"`
}

type ETAService struct {
	store *dl.Store
	clock Clock
	cfg   Settings
}

func NewETAService(store *dl.Store, clock Clock, cfg Settings) *ETAService {
	return &ETAService{store: store, clock: clock, cfg: cfg}
}

func (s *ETAService) ETA(ctx context.Context, busID, stationID int64) (*ETAResult, error) {
	b, err := s.store.Buses.Get(ctx, busID)
	if err != nil {
		return nil, err
	}
	rid := b.EffectiveRouteID()
	if rid == nil || !b.HasPosition() {
		return nil, domain.ErrInvalidInput("bus has no route or position yet")
	}
	r, err := s.store.Routes.Get(ctx, *rid)
	if err != nil {
		return nil, err
	}
	target := -1
	for i, st := range r.Stops {
		if st.ID == stationID {
			target = i
			break
		}
	}
	if target < 0 {
		return nil, domain.ErrInvalidInput("station is not on this bus's route")
	}
	track, err := s.store.Locations.BusTrack(ctx, busID, s.clock.Now().Add(-10*time.Minute))
	if err != nil {
		return nil, err
	}
	speed, ok := AverageSpeedKmh(track)
	if !ok {
		speed = s.cfg.FallbackSpeedKmh
	}
	dist := RouteDistanceM(r.Stops, *b.LastLat, *b.LastLng, target)
	return &ETAResult{BusID: busID, StationID: stationID, DistanceM: math.Round(dist), SpeedKmh: math.Round(speed*10) / 10,
		Seconds: math.Round(dist / (speed / 3.6))}, nil
}

// ---------- heatmap ----------

type HeatPoint struct {
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
	Weight int     `json:"weight"`
}

// AggregateHeat snaps each location to a grid cell and counts sessions per cell, so raw points never leave the server.
func AggregateHeat(locs []domain.UserLocation, cellDeg float64) []HeatPoint {
	type key struct{ x, y int64 }
	counts := map[key]int{}
	var order []key
	for _, l := range locs {
		k := key{int64(math.Floor(l.Lat / cellDeg)), int64(math.Floor(l.Lng / cellDeg))}
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}
	out := make([]HeatPoint, 0, len(order))
	for _, k := range order {
		out = append(out, HeatPoint{
			Lat:    (float64(k.x) + 0.5) * cellDeg,
			Lng:    (float64(k.y) + 0.5) * cellDeg,
			Weight: counts[k],
		})
	}
	return out
}

type HeatmapService struct {
	store *dl.Store
	clock Clock
	cfg   Settings
	pub   Publisher
}

func NewHeatmapService(store *dl.Store, clock Clock, cfg Settings, pub Publisher) *HeatmapService {
	return &HeatmapService{store: store, clock: clock, cfg: cfg, pub: pub}
}

// Snapshot returns the heatmap of users active in the last HeatmapWindow (one point per session).
func (s *HeatmapService) Snapshot(ctx context.Context) ([]HeatPoint, error) {
	locs, err := s.store.Locations.LatestPerSessionSince(ctx, s.clock.Now().Add(-s.cfg.HeatmapWindow))
	if err != nil {
		return nil, err
	}
	return AggregateHeat(locs, s.cfg.HeatmapCellDeg), nil
}

// Broadcast pushes the current snapshot to operators.
func (s *HeatmapService) Broadcast(ctx context.Context) error {
	pts, err := s.Snapshot(ctx)
	if err != nil {
		return err
	}
	s.pub.Publish(ChannelOperatorHeat, "heatmap", pts)
	return nil
}
