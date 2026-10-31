package bl

import (
	"context"
	"errors"
	"math"
	"time"

	"smartcity/internal/dl"
	"smartcity/internal/domain"
)

// ---------- pure scoring functions ----------

// ReportScore is the recency-weighted average of density reports (low=0, medium=1, high=2).
// A fresh report weighs 1.0 and a report as old as the window weighs 0.2, times the report's own weight.
func ReportScore(reports []domain.DensityReport, now time.Time, window time.Duration) (float64, bool) {
	var sum, wsum float64
	for _, r := range reports {
		v, ok := r.Density.Value()
		if !ok {
			continue
		}
		age := now.Sub(r.CreatedAt)
		if age < 0 {
			age = 0
		}
		if age > window {
			continue
		}
		w := r.Weight
		if w <= 0 {
			w = 1
		}
		w *= 1 - 0.8*float64(age)/float64(window)
		sum += v * w
		wsum += w
	}
	if wsum == 0 {
		return 0, false
	}
	return sum / wsum, true
}

// OccupancyScore maps estimated passengers / capacity to 0 (<50%), 1 (50-85%) or 2 (>85%).
func OccupancyScore(count, capacity int) float64 {
	if capacity <= 0 {
		return 0
	}
	occ := float64(count) / float64(capacity)
	switch {
	case occ > 0.85:
		return 2
	case occ >= 0.5:
		return 1
	}
	return 0
}

// CombineScores blends the report score and the occupancy score 50/50; either may be missing.
func CombineScores(report, occupancy *float64) *float64 {
	switch {
	case report != nil && occupancy != nil:
		return ptr((*report + *occupancy) / 2)
	case report != nil:
		return ptr(*report)
	case occupancy != nil:
		return ptr(*occupancy)
	}
	return nil
}

// LevelFromScore: >=1.5 high, >=0.75 medium, else low; nil = unknown.
func LevelFromScore(score *float64) domain.Density {
	if score == nil {
		return domain.DensityUnknown
	}
	switch {
	case *score >= 1.5:
		return domain.DensityHigh
	case *score >= 0.75:
		return domain.DensityMedium
	}
	return domain.DensityLow
}

// AverageScore averages bus scores into a route score; nil when there are none.
func AverageScore(scores []float64) *float64 {
	if len(scores) == 0 {
		return nil
	}
	var s float64
	for _, v := range scores {
		s += v
	}
	return ptr(s / float64(len(scores)))
}

// nearestInTime returns the bus point closest in time to t, if within tolerance.
func nearestInTime(track []domain.BusLocation, t time.Time, tol time.Duration) (domain.BusLocation, bool) {
	var best domain.BusLocation
	bestGap := time.Duration(math.MaxInt64)
	for _, p := range track {
		gap := p.CreatedAt.Sub(t)
		if gap < 0 {
			gap = -gap
		}
		if gap < bestGap {
			best, bestGap = p, gap
		}
	}
	return best, bestGap <= tol
}

// EstimateOnboard counts app users riding a bus: a session counts when its last two location samples
// were both within OnboardRadiusM of the bus at (nearly) the same time, and at least one of them was
// away from every station (so people waiting at a stop as the bus passes are not counted).
func EstimateOnboard(track []domain.BusLocation, users map[string][]domain.UserLocation, stations []domain.Station, s Settings) int {
	if len(track) == 0 {
		return 0
	}
	count := 0
	for _, samples := range users {
		if len(samples) < 2 {
			continue
		}
		a, b := samples[len(samples)-2], samples[len(samples)-1]
		pa, okA := nearestInTime(track, a.CreatedAt, s.BusMatchTolerance)
		pb, okB := nearestInTime(track, b.CreatedAt, s.BusMatchTolerance)
		if !okA || !okB {
			continue
		}
		if DistanceM(a.Lat, a.Lng, pa.Lat, pa.Lng) > s.OnboardRadiusM || DistanceM(b.Lat, b.Lng, pb.Lat, pb.Lng) > s.OnboardRadiusM {
			continue
		}
		if NearestStationM(a.Lat, a.Lng, stations) <= s.StationRadiusM && NearestStationM(b.Lat, b.Lng, stations) <= s.StationRadiusM {
			continue
		}
		count++
	}
	return count
}

// EstimatePassengers scales the on-board app users by the multiplier, capped at capacity.
func EstimatePassengers(onboard int, multiplier float64, capacity int) int {
	n := int(math.Round(float64(onboard) * multiplier))
	if n > capacity {
		n = capacity
	}
	if n < 0 {
		n = 0
	}
	return n
}

// ---------- service ----------

// RouteDensity is the live density of a route.
type RouteDensity struct {
	RouteID  int64          `json:"routeId"`
	Level    domain.Density `json:"level"`
	Score    *float64       `json:"score"`
	BusCount int            `json:"busCount"`
}

type DensityService struct {
	store *dl.Store
	clock Clock
	cfg   Settings
	pub   Publisher
}

func NewDensityService(store *dl.Store, clock Clock, cfg Settings, pub Publisher) *DensityService {
	return &DensityService{store: store, clock: clock, cfg: cfg, pub: pub}
}

// Report stores a user's density report after rate-limit and proximity checks, then recomputes densities.
func (s *DensityService) Report(ctx context.Context, sess Session, busID int64, densityStr string) error {
	d, err := domain.ParseDensity(densityStr)
	if err != nil {
		return err
	}
	bus, err := s.store.Buses.Get(ctx, busID)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	last, err := s.store.Reports.LastReportAt(ctx, sess.UserID, busID)
	if err != nil {
		return err
	}
	if last != nil && now.Sub(*last) < s.cfg.ReportCooldown {
		return domain.ErrRateLimited
	}
	if bus.HasPosition() {
		loc, err := s.store.Locations.LatestForUserSince(ctx, sess.UserID, now.Add(-s.cfg.PositionFreshness))
		if err != nil {
			return err
		}
		if loc != nil && DistanceM(loc.Lat, loc.Lng, *bus.LastLat, *bus.LastLng) > s.cfg.ReportMaxDistanceM {
			return domain.ErrInvalidInput("you are too far from this bus to report its density")
		}
	}
	weight := 1.0
	if sess.Role == domain.RoleGuest {
		weight = s.cfg.GuestReportWeight
	}
	if err := s.store.Reports.Add(ctx, domain.DensityReport{UserID: sess.UserID, BusID: busID, Density: d, Weight: weight, CreatedAt: now}); err != nil {
		return err
	}
	_, err = s.Recompute(ctx)
	return err
}

// Recompute updates every bus's density and passenger estimate and returns each route's live density.
func (s *DensityService) Recompute(ctx context.Context) (map[int64]RouteDensity, error) {
	now := s.clock.Now()
	buses, err := s.store.Buses.List(ctx)
	if err != nil {
		return nil, err
	}
	routes, err := s.store.Routes.List(ctx)
	if err != nil {
		return nil, err
	}
	routeByID := make(map[int64]*domain.Route, len(routes))
	for i := range routes {
		routeByID[routes[i].ID] = &routes[i]
	}
	stations, err := s.store.Stations.List(ctx)
	if err != nil {
		return nil, err
	}
	reports, err := s.store.Reports.Since(ctx, now.Add(-s.cfg.ReportWindow))
	if err != nil {
		return nil, err
	}
	userWindow := 3 * s.cfg.UserSampleInterval
	users, err := s.store.Locations.UserLocationsSince(ctx, now.Add(-userWindow))
	if err != nil {
		return nil, err
	}
	tracks, err := s.store.Locations.BusLocationsSince(ctx, now.Add(-userWindow-s.cfg.BusMatchTolerance))
	if err != nil {
		return nil, err
	}

	routeScores := map[int64][]float64{}
	routeBuses := map[int64]int{}
	updated := make([]domain.Bus, 0, len(buses))
	for _, b := range buses {
		var rep, occ *float64
		if v, ok := ReportScore(reports[b.ID], now, s.cfg.ReportWindow); ok {
			rep = &v
		}
		count := 0
		if b.LastSeenAt != nil && now.Sub(*b.LastSeenAt) <= s.cfg.PositionFreshness {
			// Zero app users detected means "no information", not "empty bus":
			// most passengers do not share their location, so only a positive count is a signal.
			if onboard := EstimateOnboard(tracks[b.ID], users, stations, s.cfg); onboard > 0 {
				count = EstimatePassengers(onboard, s.cfg.PassengerMultiplier, b.Capacity)
				occ = ptr(OccupancyScore(count, b.Capacity))
			}
		}
		score := CombineScores(rep, occ)
		level := LevelFromScore(score)
		if err := s.store.Buses.UpdateDensity(ctx, b.ID, level, score, count, now); err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		b.Density, b.DensityScore, b.PassengerCount, b.DensityUpdatedAt = level, score, count, &now
		updated = append(updated, b)
		if rid := ServingRouteID(b, routeByID); rid != nil {
			routeBuses[*rid]++
			if score != nil {
				routeScores[*rid] = append(routeScores[*rid], *score)
			}
		}
	}

	out := make(map[int64]RouteDensity, len(routes))
	list := make([]RouteDensity, 0, len(routes))
	for _, r := range routes {
		score := AverageScore(routeScores[r.ID])
		rd := RouteDensity{RouteID: r.ID, Level: LevelFromScore(score), Score: score, BusCount: routeBuses[r.ID]}
		out[r.ID] = rd
		list = append(list, rd)
	}
	s.pub.Publish(ChannelBuses, "densities", map[string]any{"buses": updated, "routes": list})
	return out, nil
}
