// Package worker runs the background jobs: anomaly checks, heatmap broadcasts, seeding and the demo simulator.
package worker

import (
	"context"
	"log"
	"time"

	"agilli/internal/bl"
	"agilli/internal/dl"
	"agilli/internal/domain"
)

// RunScheduler runs the anomaly check and the heatmap broadcast on their intervals until ctx is cancelled.
func RunScheduler(ctx context.Context, anomaly *bl.AnomalyService, heat *bl.HeatmapService, checkEvery, heatEvery time.Duration) {
	check := time.NewTicker(checkEvery)
	hm := time.NewTicker(heatEvery)
	defer check.Stop()
	defer hm.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-check.C:
			if err := anomaly.Tick(ctx); err != nil && ctx.Err() == nil {
				log.Printf("anomaly tick: %v", err)
			}
		case <-hm.C:
			if err := heat.Broadcast(ctx); err != nil && ctx.Err() == nil {
				log.Printf("heatmap broadcast: %v", err)
			}
		}
	}
}

type seedStation struct {
	name     string
	lat, lng float64
}

// Approximate positions of well-known Baku locations, used as demo bus stops.
var seedStations = []seedStation{
	{"Icherisheher", 40.3661, 49.8352}, // 0
	{"Sahil", 40.3716, 49.8432},        // 1
	{"28 May", 40.3795, 49.8486},       // 2
	{"Ganjlik", 40.4003, 49.8516},      // 3
	{"Narimanov", 40.4026, 49.8706},    // 4
	{"Koroglu", 40.4182, 49.9178},      // 5
	{"Elmlar Akademiyasi", 40.3753, 49.8122},
	{"Inshaatchilar", 40.3910, 49.8037}, // 7
	{"20 Yanvar", 40.4045, 49.8085},     // 8
	{"Memar Ajami", 40.4113, 49.8154},   // 9
	{"Nasimi", 40.4249, 49.8227},        // 10
	{"Azadliq", 40.4253, 49.8429},       // 11
	{"Nizami", 40.3793, 49.8302},        // 12
	{"Khatai", 40.3832, 49.8721},        // 13
	{"Darnagul", 40.4253, 49.8616},      // 14
}

type seedRoute struct {
	code, name string
	stops      []int
	buses      int
}

var seedRoutes = []seedRoute{
	{"65", "Icherisheher - Koroglu", []int{0, 1, 2, 3, 4, 5}, 2},
	{"13", "Elmlar - Azadliq", []int{6, 7, 8, 9, 10, 11}, 3},
	{"88", "Nizami - Darnagul", []int{12, 2, 13, 4, 14}, 3},
	{"5", "Sahil - 20 Yanvar", []int{1, 12, 6, 7, 8}, 3},
}

// Seed inserts demo stations, routes and buses when the database has no stations.
func Seed(ctx context.Context, store *dl.Store, fleet *bl.FleetService) error {
	existing, err := store.Stations.List(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	ids := make([]int64, len(seedStations))
	for i, s := range seedStations {
		st := &domain.Station{Name: s.name, Lat: s.lat, Lng: s.lng}
		if err := fleet.CreateStation(ctx, st); err != nil {
			return err
		}
		ids[i] = st.ID
	}
	for _, r := range seedRoutes {
		stationIDs := make([]int64, len(r.stops))
		for i, idx := range r.stops {
			stationIDs[i] = ids[idx]
		}
		rt := &domain.Route{Code: r.code, Name: r.name}
		if err := fleet.CreateRoute(ctx, rt, stationIDs); err != nil {
			return err
		}
		for i := 0; i < r.buses; i++ {
			rid := rt.ID
			b := &domain.Bus{Name: r.code + "-" + string(rune('A'+i)), RouteID: &rid, Capacity: 80}
			if err := fleet.CreateBus(ctx, b); err != nil {
				return err
			}
		}
	}
	log.Printf("seeded %d stations, %d routes", len(seedStations), len(seedRoutes))
	return nil
}
