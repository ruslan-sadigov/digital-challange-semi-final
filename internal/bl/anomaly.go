package bl

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"smartcity/internal/dl"
	"smartcity/internal/domain"
)

// ---------- detector (pure) ----------

type DetectorEvent int

const (
	EventNone DetectorEvent = iota
	EventOpen
	EventResolve
)

// Evaluate applies the anomaly rule to one route for one check:
//   - High starts (or continues) the clock; once High has lasted >= threshold and no anomaly is open => EventOpen.
//   - Without an open anomaly, any non-High check resets the clock (the 10 minutes must be continuous).
//   - With an open anomaly, ResolveChecks consecutive non-High checks => EventResolve (avoids flapping).
func Evaluate(st domain.RouteDensityState, level domain.Density, now time.Time, hasOpen bool, threshold time.Duration, resolveChecks int) (domain.RouteDensityState, DetectorEvent) {
	st.Level = level
	st.UpdatedAt = now
	if level == domain.DensityHigh {
		st.BelowHighChecks = 0
		if st.HighSince == nil {
			st.HighSince = ptr(now)
		}
		if !hasOpen && now.Sub(*st.HighSince) >= threshold {
			return st, EventOpen
		}
		return st, EventNone
	}
	if !hasOpen {
		st.HighSince = nil
		st.BelowHighChecks = 0
		return st, EventNone
	}
	st.BelowHighChecks++
	if st.BelowHighChecks >= resolveChecks {
		st.HighSince = nil
		st.BelowHighChecks = 0
		return st, EventResolve
	}
	return st, EventNone
}

// humanDuration formats seconds as "45 s" or "12 min".
func humanDuration(sec float64) string {
	if sec < 60 {
		return fmt.Sprintf("%d s", int(sec))
	}
	return fmt.Sprintf("%d min", int(sec/60))
}

// ---------- suggester (pure) ----------

// Candidate is a bus that might be moved temporarily to the overloaded route.
type Candidate struct {
	BusID          int64
	RouteID        int64 // the bus's current (effective) route
	Lat, Lng       float64
	Density        domain.Density
	RouteBusCount  int     // buses currently serving the candidate's route
	RouteScore     float64 // candidate route's density score (0..2)
	SharedStations int     // stations the candidate route shares with the target route
}

type Suggestion struct {
	BusID       int64
	FromRouteID int64
	DistanceKm  float64
	Score       float64 // lower is better
}

// Suggest ranks low-density buses from other routes for a temporary move to the target route.
// Low buses are preferred; Medium buses are used only if no Low bus qualifies; High/unknown never.
// A bus is skipped if its route would keep fewer than minBusesLeft buses.
// Score = distance to the target route's nearest stop (km) + 0.5 x donor route score - 0.3 x shared stations.
func Suggest(targetRouteID int64, targetStops []domain.Station, cands []Candidate, maxResults, minBusesLeft int) []Suggestion {
	if len(targetStops) == 0 {
		return nil
	}
	pick := func(level domain.Density) []Suggestion {
		var out []Suggestion
		for _, c := range cands {
			if c.RouteID == targetRouteID || c.Density != level || c.RouteBusCount-1 < minBusesLeft {
				continue
			}
			distKm := NearestStationM(c.Lat, c.Lng, targetStops) / 1000
			score := distKm + 0.5*c.RouteScore - 0.3*float64(c.SharedStations)
			out = append(out, Suggestion{BusID: c.BusID, FromRouteID: c.RouteID, DistanceKm: distKm, Score: score})
		}
		return out
	}
	out := pick(domain.DensityLow)
	if len(out) == 0 {
		out = pick(domain.DensityMedium)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score < out[j].Score
		}
		return out[i].BusID < out[j].BusID
	})
	if len(out) > maxResults {
		out = out[:maxResults]
	}
	return out
}

// SharedStations counts stations present on both routes.
func SharedStations(a, b []domain.Station) int {
	set := map[int64]bool{}
	for _, s := range a {
		set[s.ID] = true
	}
	n := 0
	seen := map[int64]bool{}
	for _, s := range b {
		if set[s.ID] && !seen[s.ID] {
			n++
			seen[s.ID] = true
		}
	}
	return n
}

// ---------- service ----------

type AnomalyService struct {
	store    *dl.Store
	clock    Clock
	cfg      Settings
	density  *DensityService
	notifier Notifier
	pub      Publisher
}

func NewAnomalyService(store *dl.Store, clock Clock, cfg Settings, density *DensityService, notifier Notifier, pub Publisher) *AnomalyService {
	return &AnomalyService{store: store, clock: clock, cfg: cfg, density: density, notifier: notifier, pub: pub}
}

// Tick runs one detection cycle: recompute densities, apply the rule per route, open/resolve anomalies,
// create reroute suggestions, notify operators, and expire finished overrides.
func (s *AnomalyService) Tick(ctx context.Context) error {
	levels, err := s.density.Recompute(ctx)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	states, err := s.store.Anomalies.ListStates(ctx)
	if err != nil {
		return err
	}
	byRoute := map[int64]domain.RouteDensityState{}
	for _, st := range states {
		byRoute[st.RouteID] = st
	}
	routes, err := s.store.Routes.List(ctx)
	if err != nil {
		return err
	}
	for _, r := range routes {
		rd := levels[r.ID]
		open, err := s.store.Anomalies.OpenForRoute(ctx, r.ID)
		if err != nil {
			return err
		}
		st := byRoute[r.ID]
		st.RouteID = r.ID
		st, ev := Evaluate(st, rd.Level, now, open != nil, s.cfg.AnomalyThreshold, s.cfg.ResolveChecks)
		st.Score = rd.Score
		highFor := 0.0
		if st.HighSince != nil {
			highFor = now.Sub(*st.HighSince).Seconds()
		}
		switch ev {
		case EventOpen:
			a := &domain.Anomaly{RouteID: r.ID, StartedAt: *st.HighSince, DetectedAt: now, Status: domain.AnomalyOpen}
			if err := s.store.Anomalies.Create(ctx, a); err != nil {
				return err
			}
			n, err := s.createSuggestions(ctx, a, r, routes, levels)
			if err != nil {
				return err
			}
			_ = s.notifier.Notify(ctx, Alert{Type: "anomaly_opened", AnomalyID: a.ID, RouteID: r.ID, RouteCode: r.Code, HighForS: highFor,
				Message: fmt.Sprintf("Route %s has been High for %s. %d reroute suggestion(s) ready.", r.Code, humanDuration(highFor), n)})
		case EventResolve:
			if err := s.resolve(ctx, open, now); err != nil {
				return err
			}
			_ = s.notifier.Notify(ctx, Alert{Type: "anomaly_resolved", AnomalyID: open.ID, RouteID: r.ID, RouteCode: r.Code,
				Message: fmt.Sprintf("Route %s density is back below High. Active reroutes continue until they expire or you end them.", r.Code)})
		}
		if err := s.store.Anomalies.SaveState(ctx, st); err != nil {
			return err
		}
	}
	return s.expireOverrides(ctx, now)
}

func (s *AnomalyService) createSuggestions(ctx context.Context, a *domain.Anomaly, target domain.Route, routes []domain.Route, levels map[int64]RouteDensity) (int, error) {
	buses, err := s.store.Buses.List(ctx)
	if err != nil {
		return 0, err
	}
	busy, err := s.store.Overrides.ListByStatus(ctx, domain.OverrideSuggested, domain.OverrideActive)
	if err != nil {
		return 0, err
	}
	taken := map[int64]bool{}
	for _, o := range busy {
		taken[o.BusID] = true
	}
	stopsByRoute := map[int64][]domain.Station{}
	for _, r := range routes {
		stopsByRoute[r.ID] = r.Stops
	}
	var cands []Candidate
	for _, b := range buses {
		rid := b.EffectiveRouteID()
		if rid == nil || taken[b.ID] || !b.HasPosition() {
			continue
		}
		rd := levels[*rid]
		rs := 0.0
		if rd.Score != nil {
			rs = *rd.Score
		}
		cands = append(cands, Candidate{BusID: b.ID, RouteID: *rid, Lat: *b.LastLat, Lng: *b.LastLng, Density: b.Density,
			RouteBusCount: rd.BusCount, RouteScore: rs, SharedStations: SharedStations(stopsByRoute[*rid], target.Stops)})
	}
	sugg := Suggest(target.ID, target.Stops, cands, s.cfg.MaxSuggestions, s.cfg.MinBusesLeft)
	now := s.clock.Now()
	for _, sg := range sugg {
		o := &domain.RouteOverride{AnomalyID: &a.ID, BusID: sg.BusID, FromRouteID: sg.FromRouteID, ToRouteID: target.ID,
			Status: domain.OverrideSuggested, Score: sg.Score, DistanceKm: sg.DistanceKm, CreatedAt: now}
		if err := s.store.Overrides.Create(ctx, o); err != nil {
			return 0, err
		}
	}
	return len(sugg), nil
}

func (s *AnomalyService) resolve(ctx context.Context, a *domain.Anomaly, now time.Time) error {
	if err := s.store.Anomalies.Resolve(ctx, a.ID, now); err != nil {
		return err
	}
	ovs, err := s.store.Overrides.ListByAnomaly(ctx, a.ID)
	if err != nil {
		return err
	}
	// Unused suggestions expire. Active reroutes keep running until their duration ends or an operator
	// ends them: the reroute is usually what brought density down, so ending it here would make the
	// route crowded again and the anomaly would flap.
	for i := range ovs {
		o := &ovs[i]
		if o.Status == domain.OverrideSuggested {
			o.Status = domain.OverrideExpired
			if err := s.store.Overrides.Update(ctx, o); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *AnomalyService) endOverride(ctx context.Context, o *domain.RouteOverride, now time.Time, status string) error {
	o.Status = status
	o.EndsAt = ptr(now)
	if err := s.store.Overrides.Update(ctx, o); err != nil {
		return err
	}
	if err := s.store.Buses.SetActiveRoute(ctx, o.BusID, nil); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	s.pub.Publish(ChannelBuses, "override", o)
	return nil
}

func (s *AnomalyService) expireOverrides(ctx context.Context, now time.Time) error {
	active, err := s.store.Overrides.ListByStatus(ctx, domain.OverrideActive)
	if err != nil {
		return err
	}
	for i := range active {
		o := &active[i]
		if o.EndsAt != nil && !now.Before(*o.EndsAt) {
			if err := s.endOverride(ctx, o, *o.EndsAt, domain.OverrideEnded); err != nil {
				return err
			}
		}
	}
	return nil
}

// MaxOverrideDuration caps the interval an operator can set for a reroute.
const MaxOverrideDuration = 24 * time.Hour

// Approve activates a suggested reroute. It is a manual operator decision, and so is its interval:
// duration > 0 ends the reroute automatically after that time; duration == 0 keeps it until an operator ends it.
func (s *AnomalyService) Approve(ctx context.Context, overrideID, operatorID int64, duration time.Duration) (*domain.RouteOverride, error) {
	if duration < 0 || duration > MaxOverrideDuration {
		return nil, domain.ErrInvalidInput("reroute duration must be between 0 (until ended) and 24 hours")
	}
	o, err := s.store.Overrides.Get(ctx, overrideID)
	if err != nil {
		return nil, err
	}
	if o.Status != domain.OverrideSuggested {
		return nil, fmt.Errorf("%w: override is %s, only suggested overrides can be approved", domain.ErrConflict, o.Status)
	}
	active, err := s.store.Overrides.ListByStatus(ctx, domain.OverrideActive)
	if err != nil {
		return nil, err
	}
	for _, a := range active {
		if a.BusID == o.BusID {
			return nil, fmt.Errorf("%w: bus already has an active reroute", domain.ErrConflict)
		}
	}
	// Approving several suggestions from the same donor route must not empty that route.
	buses, err := s.store.Buses.List(ctx)
	if err != nil {
		return nil, err
	}
	left := 0
	for _, b := range buses {
		if rid := b.EffectiveRouteID(); rid != nil && *rid == o.FromRouteID && b.ID != o.BusID {
			left++
		}
	}
	if left < s.cfg.MinBusesLeft {
		return nil, fmt.Errorf("%w: route would be left with fewer than %d bus(es)", domain.ErrConflict, s.cfg.MinBusesLeft)
	}
	now := s.clock.Now()
	o.Status = domain.OverrideActive
	o.StartsAt = ptr(now)
	o.EndsAt = nil
	if duration > 0 {
		o.EndsAt = ptr(now.Add(duration))
	}
	o.DecidedBy = &operatorID
	if err := s.store.Overrides.Update(ctx, o); err != nil {
		return nil, err
	}
	if err := s.store.Buses.SetActiveRoute(ctx, o.BusID, &o.ToRouteID); err != nil {
		return nil, err
	}
	s.pub.Publish(ChannelBuses, "override", o)
	s.pub.Publish(ChannelOperatorAlerts, "override", o)
	return o, nil
}

func (s *AnomalyService) Reject(ctx context.Context, overrideID, operatorID int64) (*domain.RouteOverride, error) {
	o, err := s.store.Overrides.Get(ctx, overrideID)
	if err != nil {
		return nil, err
	}
	if o.Status != domain.OverrideSuggested {
		return nil, fmt.Errorf("%w: override is %s", domain.ErrConflict, o.Status)
	}
	o.Status = domain.OverrideRejected
	o.DecidedBy = &operatorID
	if err := s.store.Overrides.Update(ctx, o); err != nil {
		return nil, err
	}
	s.pub.Publish(ChannelOperatorAlerts, "override", o)
	return o, nil
}

// End stops an active reroute early; the bus returns to its home route.
func (s *AnomalyService) End(ctx context.Context, overrideID int64) (*domain.RouteOverride, error) {
	o, err := s.store.Overrides.Get(ctx, overrideID)
	if err != nil {
		return nil, err
	}
	if o.Status != domain.OverrideActive {
		return nil, fmt.Errorf("%w: override is not active", domain.ErrConflict)
	}
	if err := s.endOverride(ctx, o, s.clock.Now(), domain.OverrideEnded); err != nil {
		return nil, err
	}
	s.pub.Publish(ChannelOperatorAlerts, "override", o)
	return o, nil
}

// AnomalyView is an anomaly with its route code and suggestions, for the operator panel.
type AnomalyView struct {
	domain.Anomaly
	RouteCode string         `json:"routeCode"`
	Overrides []OverrideView `json:"overrides"`
}

type OverrideView struct {
	domain.RouteOverride
	BusName       string `json:"busName"`
	FromRouteCode string `json:"fromRouteCode"`
	ToRouteCode   string `json:"toRouteCode"`
}

func (s *AnomalyService) lookups(ctx context.Context) (map[int64]string, map[int64]string, error) {
	routes, err := s.store.Routes.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	buses, err := s.store.Buses.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	rc := map[int64]string{}
	for _, r := range routes {
		rc[r.ID] = r.Code
	}
	bn := map[int64]string{}
	for _, b := range buses {
		bn[b.ID] = b.Name
	}
	return rc, bn, nil
}

func view(o domain.RouteOverride, rc, bn map[int64]string) OverrideView {
	return OverrideView{RouteOverride: o, BusName: bn[o.BusID], FromRouteCode: rc[o.FromRouteID], ToRouteCode: rc[o.ToRouteID]}
}

func (s *AnomalyService) ListAnomalies(ctx context.Context, limit int) ([]AnomalyView, error) {
	rc, bn, err := s.lookups(ctx)
	if err != nil {
		return nil, err
	}
	as, err := s.store.Anomalies.List(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]AnomalyView, 0, len(as))
	for _, a := range as {
		ovs, err := s.store.Overrides.ListByAnomaly(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		v := AnomalyView{Anomaly: a, RouteCode: rc[a.RouteID], Overrides: []OverrideView{}}
		for _, o := range ovs {
			v.Overrides = append(v.Overrides, view(o, rc, bn))
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *AnomalyService) ListActiveOverrides(ctx context.Context) ([]OverrideView, error) {
	rc, bn, err := s.lookups(ctx)
	if err != nil {
		return nil, err
	}
	ovs, err := s.store.Overrides.ListByStatus(ctx, domain.OverrideActive)
	if err != nil {
		return nil, err
	}
	out := make([]OverrideView, 0, len(ovs))
	for _, o := range ovs {
		out = append(out, view(o, rc, bn))
	}
	return out, nil
}

// RouteStates returns the live density state of every route.
func (s *AnomalyService) RouteStates(ctx context.Context) ([]domain.RouteDensityState, error) {
	return s.store.Anomalies.ListStates(ctx)
}
