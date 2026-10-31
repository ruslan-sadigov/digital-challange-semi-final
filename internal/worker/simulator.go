package worker

import (
	"context"
	"errors"
	"log"
	"math"
	"math/rand"
	"time"

	"agilli/internal/bl"
	"agilli/internal/dl"
	"agilli/internal/domain"
)

// Simulator feeds the system with realistic demo data: buses driving their routes, app users riding
// them (sending locations and density reports), and pedestrians around stations for the heatmap.
// One route (CrowdedRoute) is overloaded so the anomaly + reroute flow can be demonstrated.
type Simulator struct {
	Store        *dl.Store
	Location     *bl.LocationService
	Density      *bl.DensityService
	Settings     bl.Settings
	Tick         time.Duration
	CrowdedRoute string
	Walkers      int

	rng     *rand.Rand
	buses   map[int64]*simBus
	riders  []*simUser
	walkers []*simUser
	tickN   int
}

type simBus struct {
	routeID int64
	seg     int
	frac    float64
	speed   float64 // m/s
	lat     float64
	lng     float64
}

type simUser struct {
	sess   bl.Session
	busID  int64 // riders only
	dLat   float64
	dLng   float64
	lat    float64 // walkers only
	lng    float64
	phase  int
	hasPos bool
}

func (s *Simulator) newUser(ctx context.Context) (*simUser, error) {
	u := &domain.User{Role: domain.RoleGuest, CreatedAt: time.Now()}
	if err := s.Store.Users.Create(ctx, u); err != nil {
		return nil, err
	}
	return &simUser{sess: bl.Session{UserID: u.ID, Role: domain.RoleGuest, SessionID: bl.NewSessionID()}, phase: s.rng.Intn(2)}, nil
}

// Run drives the simulation until ctx is cancelled.
func (s *Simulator) Run(ctx context.Context) {
	s.rng = rand.New(rand.NewSource(42))
	s.buses = map[int64]*simBus{}
	if err := s.setup(ctx); err != nil {
		log.Printf("simulator setup: %v", err)
		return
	}
	log.Printf("simulator: %d riders, %d walkers, tick %s, crowded route %q", len(s.riders), len(s.walkers), s.Tick, s.CrowdedRoute)
	t := time.NewTicker(s.Tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.step(ctx); err != nil && ctx.Err() == nil {
				log.Printf("simulator step: %v", err)
			}
		}
	}
}

func (s *Simulator) setup(ctx context.Context) error {
	buses, err := s.Store.Buses.List(ctx)
	if err != nil {
		return err
	}
	routes, err := s.Store.Routes.List(ctx)
	if err != nil {
		return err
	}
	stations, err := s.Store.Stations.List(ctx)
	if err != nil {
		return err
	}
	crowded := map[int64]bool{}
	for _, r := range routes {
		if r.Code == s.CrowdedRoute {
			crowded[r.ID] = true
		}
	}
	for _, b := range buses {
		rid := b.EffectiveRouteID()
		if rid == nil {
			continue
		}
		n := 2 + s.rng.Intn(4) // quiet bus: 2-5 app users => ~6-15 passengers
		if crowded[*rid] {
			// enough riders that riders x multiplier is ~95% of capacity
			n = int(math.Ceil(float64(b.Capacity) * 0.95 / s.Settings.PassengerMultiplier))
		}
		for i := 0; i < n; i++ {
			u, err := s.newUser(ctx)
			if err != nil {
				return err
			}
			u.busID = b.ID
			u.dLat, u.dLng = jitterDeg(s.rng, 8)
			s.riders = append(s.riders, u)
		}
	}
	for i := 0; i < s.Walkers && len(stations) > 0; i++ {
		u, err := s.newUser(ctx)
		if err != nil {
			return err
		}
		st := stations[s.rng.Intn(len(stations))]
		dl, dg := jitterDeg(s.rng, 350)
		u.lat, u.lng = st.Lat+dl, st.Lng+dg
		s.walkers = append(s.walkers, u)
	}
	return nil
}

// jitterDeg returns a random offset of at most maxM metres, in degrees (Baku latitude).
func jitterDeg(rng *rand.Rand, maxM float64) (float64, float64) {
	r := maxM * math.Sqrt(rng.Float64())
	a := rng.Float64() * 2 * math.Pi
	return r * math.Sin(a) / 111320, r * math.Cos(a) / (111320 * math.Cos(40.4*math.Pi/180))
}

func (s *Simulator) step(ctx context.Context) error {
	s.tickN++
	buses, err := s.Store.Buses.List(ctx)
	if err != nil {
		return err
	}
	routes, err := s.Store.Routes.List(ctx)
	if err != nil {
		return err
	}
	routeByID := map[int64]domain.Route{}
	for _, r := range routes {
		routeByID[r.ID] = r
	}
	busByID := map[int64]domain.Bus{}
	for _, b := range buses {
		busByID[b.ID] = b
		rid := b.EffectiveRouteID()
		if rid == nil {
			continue
		}
		r, ok := routeByID[*rid]
		if !ok || len(r.Stops) < 2 {
			continue
		}
		sb := s.buses[b.ID]
		if sb == nil {
			sb = &simBus{routeID: r.ID, seg: s.rng.Intn(len(r.Stops)), frac: s.rng.Float64(), speed: 6 + s.rng.Float64()*3}
			s.buses[b.ID] = sb
		} else if sb.routeID != r.ID {
			// rerouted (or back home): continue from the stop of the new route closest to the bus
			sb.routeID, sb.seg, sb.frac = r.ID, nearestStop(r.Stops, sb.lat, sb.lng), 0
		}
		advance(sb, r.Stops, sb.speed*s.Tick.Seconds())
		if err := s.Location.RecordBus(ctx, b.ID, sb.lat, sb.lng); err != nil {
			return err
		}
	}

	s.rebalance(busByID)

	// Users send a location every 2 ticks (half of them on each tick).
	for _, u := range s.riders {
		sb := s.buses[u.busID]
		if sb == nil || (s.tickN+u.phase)%2 != 0 {
			continue
		}
		if err := s.Location.RecordUser(ctx, u.sess, sb.lat+u.dLat, sb.lng+u.dLng); err != nil {
			return err
		}
		u.hasPos = true
	}
	for _, u := range s.walkers {
		if (s.tickN+u.phase)%2 != 0 {
			continue
		}
		dl, dg := jitterDeg(s.rng, 15)
		u.lat, u.lng = u.lat+dl, u.lng+dg
		if err := s.Location.RecordUser(ctx, u.sess, u.lat, u.lng); err != nil {
			return err
		}
	}

	// A few riders report how crowded their bus feels.
	onBus := map[int64]int{}
	for _, u := range s.riders {
		onBus[u.busID]++
	}
	for _, u := range s.riders {
		if !u.hasPos || s.rng.Float64() > 0.03 {
			continue
		}
		b, ok := busByID[u.busID]
		if !ok {
			continue
		}
		occ := float64(onBus[u.busID]) * s.Settings.PassengerMultiplier / float64(b.Capacity)
		d := "low"
		switch {
		case occ > 0.85:
			d = "high"
		case occ >= 0.5:
			d = "medium"
		}
		err := s.Density.Report(ctx, u.sess, u.busID, d)
		var ve *domain.ValidationError
		if err != nil && !errors.Is(err, domain.ErrRateLimited) && !errors.As(err, &ve) {
			return err
		}
	}
	return nil
}

// rebalance models passengers moving onto a bus that was temporarily sent to their route:
// each tick a few riders move from the fullest bus on the route to the rerouted bus.
func (s *Simulator) rebalance(buses map[int64]domain.Bus) {
	count := map[int64]int{}
	for _, u := range s.riders {
		count[u.busID]++
	}
	for _, helper := range buses {
		if helper.ActiveRouteID == nil {
			continue
		}
		for moved := 0; moved < 3; moved++ {
			var fullest int64
			for _, b := range buses {
				rid := b.EffectiveRouteID()
				if b.ID == helper.ID || rid == nil || *rid != *helper.ActiveRouteID {
					continue
				}
				if fullest == 0 || count[b.ID] > count[fullest] {
					fullest = b.ID
				}
			}
			if fullest == 0 || count[fullest]-count[helper.ID] <= 1 {
				break
			}
			for _, u := range s.riders {
				if u.busID == fullest {
					u.busID = helper.ID
					u.hasPos = false
					count[fullest]--
					count[helper.ID]++
					break
				}
			}
		}
	}
}

func nearestStop(stops []domain.Station, lat, lng float64) int {
	best, bi := math.Inf(1), 0
	for i, st := range stops {
		if d := bl.DistanceM(lat, lng, st.Lat, st.Lng); d < best {
			best, bi = d, i
		}
	}
	return bi
}

// advance moves the bus metres along its circular route and updates its position.
func advance(b *simBus, stops []domain.Station, metres float64) {
	n := len(stops)
	if b.seg >= n {
		b.seg = 0
	}
	for i := 0; i < 4*n; i++ {
		a, c := stops[b.seg], stops[(b.seg+1)%n]
		segLen := bl.DistanceM(a.Lat, a.Lng, c.Lat, c.Lng)
		if segLen < 1 {
			b.seg, b.frac = (b.seg+1)%n, 0
			continue
		}
		left := (1 - b.frac) * segLen
		if metres < left {
			b.frac += metres / segLen
			break
		}
		metres -= left
		b.seg, b.frac = (b.seg+1)%n, 0
	}
	a, c := stops[b.seg], stops[(b.seg+1)%n]
	b.lat = a.Lat + (c.Lat-a.Lat)*b.frac
	b.lng = a.Lng + (c.Lng-a.Lng)*b.frac
}
