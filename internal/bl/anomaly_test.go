package bl

import (
	"testing"
	"time"

	"smartcity/internal/domain"
)

const threshold = 10 * time.Minute

// run feeds a sequence of per-minute levels to Evaluate and returns the events per minute.
func run(levels []domain.Density, open bool) ([]DetectorEvent, domain.RouteDensityState) {
	st := domain.RouteDensityState{RouteID: 1}
	var evs []DetectorEvent
	for i, l := range levels {
		var ev DetectorEvent
		st, ev = Evaluate(st, l, t0.Add(time.Duration(i)*time.Minute), open, threshold, 2)
		if ev == EventOpen {
			open = true
		}
		if ev == EventResolve {
			open = false
		}
		evs = append(evs, ev)
	}
	return evs, st
}

func repeat(d domain.Density, n int) []domain.Density {
	out := make([]domain.Density, n)
	for i := range out {
		out[i] = d
	}
	return out
}

const H, M, L = domain.DensityHigh, domain.DensityMedium, domain.DensityLow

func TestDetector_OpensAtExactlyThreshold(t *testing.T) {
	evs, st := run(repeat(H, 11), false) // minutes 0..10
	for i := 0; i < 10; i++ {
		if evs[i] != EventNone {
			t.Fatalf("minute %d: got event %v before threshold", i, evs[i])
		}
	}
	if evs[10] != EventOpen {
		t.Fatalf("minute 10: want EventOpen, got %v", evs[10])
	}
	if st.HighSince == nil || !st.HighSince.Equal(t0) {
		t.Fatalf("HighSince should stay at the first High check, got %v", st.HighSince)
	}
}

func TestDetector_JustBelowThreshold(t *testing.T) {
	st := domain.RouteDensityState{RouteID: 1}
	st, _ = Evaluate(st, H, t0, false, threshold, 2)
	_, ev := Evaluate(st, H, t0.Add(threshold-time.Second), false, threshold, 2)
	if ev != EventNone {
		t.Fatalf("9:59 should not alert, got %v", ev)
	}
}

func TestDetector_OneAlertPerEpisode(t *testing.T) {
	evs, _ := run(repeat(H, 30), false)
	n := 0
	for _, e := range evs {
		if e == EventOpen {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want exactly 1 open event, got %d", n)
	}
}

func TestDetector_DipBeforeThresholdResetsClock(t *testing.T) {
	levels := append(repeat(H, 9), M)         // minutes 0..9: High x9 then Medium
	levels = append(levels, repeat(H, 10)...) // minutes 10..19: High again
	evs, _ := run(levels, false)
	for i, e := range evs {
		if e == EventOpen {
			t.Fatalf("opened at minute %d although High was never continuous for 10 minutes", i)
		}
	}
	evs, _ = run(append(levels, H), false) // minute 20: 10 continuous minutes since minute 10
	if evs[20] != EventOpen {
		t.Fatalf("want open at minute 20, got %v", evs[20])
	}
}

func TestDetector_ResolveNeedsTwoChecks(t *testing.T) {
	levels := append(repeat(H, 11), M, H, M, L) // open at 10; dip at 11; High at 12; then 2 non-High
	evs, st := run(levels, false)
	if evs[10] != EventOpen {
		t.Fatalf("expected open at 10")
	}
	if evs[11] != EventNone || evs[12] != EventNone || evs[13] != EventNone {
		t.Fatalf("single dips must not resolve: %v", evs[11:14])
	}
	if evs[14] != EventResolve {
		t.Fatalf("expected resolve after 2 consecutive non-High checks, got %v", evs[14])
	}
	if st.HighSince != nil || st.BelowHighChecks != 0 {
		t.Fatalf("state should reset after resolve: %+v", st)
	}
}

func TestDetector_UnknownIsNotHigh(t *testing.T) {
	evs, st := run(repeat(domain.DensityUnknown, 15), false)
	for _, e := range evs {
		if e != EventNone {
			t.Fatal("unknown density must never open an anomaly")
		}
	}
	if st.HighSince != nil {
		t.Fatal("unknown must not start the clock")
	}
}

func TestDetector_RestartKeepsClock(t *testing.T) {
	// The state is persisted; a "restart" is just evaluating with the saved state later.
	saved := domain.RouteDensityState{RouteID: 1, HighSince: ptr(t0)}
	_, ev := Evaluate(saved, H, t0.Add(10*time.Minute), false, threshold, 2)
	if ev != EventOpen {
		t.Fatalf("persisted HighSince should be honoured, got %v", ev)
	}
}

// ---------- suggester ----------

var targetStops = []domain.Station{{ID: 1, Lat: 40.38, Lng: 49.85}, {ID: 2, Lat: 40.40, Lng: 49.85}}

func cand(id, route int64, d domain.Density, kmNorth float64) Candidate {
	lat, lng := north(40.40, 49.85, kmNorth*1000)
	return Candidate{BusID: id, RouteID: route, Lat: lat, Lng: lng, Density: d, RouteBusCount: 3}
}

func TestSuggest_PrefersLowAndNearest(t *testing.T) {
	cands := []Candidate{
		cand(1, 2, L, 3),
		cand(2, 3, L, 1),
		cand(3, 4, M, 0.1), // medium, closer, but Low exists
		cand(4, 5, H, 0),   // never High
		cand(5, 1, L, 0),   // same route as target
	}
	got := Suggest(1, targetStops, cands, 3, 1)
	if len(got) != 2 || got[0].BusID != 2 || got[1].BusID != 1 {
		t.Fatalf("unexpected suggestions: %+v", got)
	}
	if got[0].DistanceKm < 0.99 || got[0].DistanceKm > 1.01 {
		t.Fatalf("distance should be ~1 km, got %v", got[0].DistanceKm)
	}
}

func TestSuggest_FallsBackToMedium(t *testing.T) {
	got := Suggest(1, targetStops, []Candidate{cand(3, 4, M, 0.5), cand(4, 5, H, 0)}, 3, 1)
	if len(got) != 1 || got[0].BusID != 3 {
		t.Fatalf("want the medium bus, got %+v", got)
	}
}

func TestSuggest_KeepsMinimumBusesOnDonorRoute(t *testing.T) {
	c := cand(1, 2, L, 1)
	c.RouteBusCount = 1 // the only bus of its route
	if got := Suggest(1, targetStops, []Candidate{c}, 3, 1); len(got) != 0 {
		t.Fatalf("must not strip the last bus from a route: %+v", got)
	}
}

func TestSuggest_ScoreUsesDonorDensityAndOverlap(t *testing.T) {
	a := cand(1, 2, L, 1)
	a.RouteScore = 1.0 // busier donor route: +0.5
	b := cand(2, 3, L, 1)
	b.SharedStations = 2 // overlapping route: -0.6
	got := Suggest(1, targetStops, []Candidate{a, b}, 3, 1)
	if got[0].BusID != 2 {
		t.Fatalf("overlapping, quieter route should rank first: %+v", got)
	}
}

func TestSuggest_LimitsResultsAndHandlesEmpty(t *testing.T) {
	var cs []Candidate
	for i := int64(1); i <= 6; i++ {
		cs = append(cs, cand(i, 10+i, L, float64(i)))
	}
	if got := Suggest(1, targetStops, cs, 3, 1); len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	if got := Suggest(1, nil, cs, 3, 1); got != nil {
		t.Fatal("no target stops => no suggestions")
	}
	if got := Suggest(1, targetStops, nil, 3, 1); len(got) != 0 {
		t.Fatal("no candidates => empty")
	}
}

func TestSharedStations(t *testing.T) {
	a := []domain.Station{{ID: 1}, {ID: 2}, {ID: 3}}
	b := []domain.Station{{ID: 3}, {ID: 4}, {ID: 1}, {ID: 1}}
	if got := SharedStations(a, b); got != 2 {
		t.Fatalf("got %d", got)
	}
}

func TestHumanDuration(t *testing.T) {
	if humanDuration(45) != "45 s" || humanDuration(600) != "10 min" || humanDuration(659) != "10 min" {
		t.Fatal("unexpected format")
	}
}
