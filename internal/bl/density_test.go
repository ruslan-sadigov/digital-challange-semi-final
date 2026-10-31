package bl

import (
	"math"
	"testing"
	"time"

	"agilli/internal/domain"
)

var t0 = time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)

func approx(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestReportScore(t *testing.T) {
	w := 10 * time.Minute
	rep := func(d domain.Density, ago time.Duration, weight float64) domain.DensityReport {
		return domain.DensityReport{Density: d, CreatedAt: t0.Add(-ago), Weight: weight}
	}
	tests := []struct {
		name    string
		reports []domain.DensityReport
		want    float64
		ok      bool
	}{
		{"no reports", nil, 0, false},
		{"single fresh high", []domain.DensityReport{rep(domain.DensityHigh, 0, 1)}, 2, true},
		{"all too old", []domain.DensityReport{rep(domain.DensityHigh, 11*time.Minute, 1)}, 0, false},
		// fresh low (w=1) + 10-min-old high (w=0.2) => 0.4/1.2
		{"recency weighting", []domain.DensityReport{rep(domain.DensityLow, 0, 1), rep(domain.DensityHigh, w, 1)}, 0.4 / 1.2, true},
		// guest weight 0.5 on high, registered 1 on low => 1/1.5
		{"report weights", []domain.DensityReport{rep(domain.DensityLow, 0, 1), rep(domain.DensityHigh, 0, 0.5)}, 1 / 1.5, true},
		{"zero weight counts as 1", []domain.DensityReport{rep(domain.DensityMedium, 0, 0)}, 1, true},
		{"unknown ignored", []domain.DensityReport{rep(domain.DensityUnknown, 0, 1)}, 0, false},
		{"future timestamp treated as fresh", []domain.DensityReport{rep(domain.DensityMedium, -time.Minute, 1)}, 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ReportScore(tc.reports, t0, w)
			if ok != tc.ok {
				t.Fatalf("ok=%v, want %v", ok, tc.ok)
			}
			approx(t, got, tc.want)
		})
	}
}

func TestOccupancyScore(t *testing.T) {
	cases := []struct {
		count, capacity int
		want            float64
	}{
		{0, 80, 0}, {39, 80, 0}, {40, 80, 1}, {68, 80, 1}, {69, 80, 2}, {80, 80, 2}, {5, 0, 0},
	}
	for _, c := range cases {
		if got := OccupancyScore(c.count, c.capacity); got != c.want {
			t.Errorf("OccupancyScore(%d,%d)=%v want %v", c.count, c.capacity, got, c.want)
		}
	}
}

func TestCombineAndLevel(t *testing.T) {
	if CombineScores(nil, nil) != nil {
		t.Fatal("expected nil when no data")
	}
	approx(t, *CombineScores(ptr(2.0), nil), 2)
	approx(t, *CombineScores(nil, ptr(1.0)), 1)
	approx(t, *CombineScores(ptr(2.0), ptr(1.0)), 1.5)

	levels := []struct {
		score *float64
		want  domain.Density
	}{
		{nil, domain.DensityUnknown}, {ptr(0.0), domain.DensityLow}, {ptr(0.7499), domain.DensityLow},
		{ptr(0.75), domain.DensityMedium}, {ptr(1.4999), domain.DensityMedium}, {ptr(1.5), domain.DensityHigh}, {ptr(2.0), domain.DensityHigh},
	}
	for _, l := range levels {
		if got := LevelFromScore(l.score); got != l.want {
			t.Errorf("LevelFromScore(%v)=%s want %s", l.score, got, l.want)
		}
	}
	if AverageScore(nil) != nil {
		t.Fatal("expected nil average")
	}
	approx(t, *AverageScore([]float64{2, 1, 0}), 1)
}

func TestEstimatePassengers(t *testing.T) {
	if got := EstimatePassengers(10, 3, 80); got != 30 {
		t.Fatalf("got %d", got)
	}
	if got := EstimatePassengers(40, 3, 80); got != 80 {
		t.Fatalf("expected cap at capacity, got %d", got)
	}
	if got := EstimatePassengers(0, 3, 80); got != 0 {
		t.Fatalf("got %d", got)
	}
}

// offset returns lat/lng shifted north by m metres.
func north(lat, lng, m float64) (float64, float64) { return lat + m/111320, lng }

func TestEstimateOnboard(t *testing.T) {
	s := DefaultSettings()
	// bus drives north from (40.40, 49.86), far from any station, one point per minute
	var track []domain.BusLocation
	for i := 0; i <= 4; i++ {
		lat, lng := north(40.40, 49.86, float64(i)*400)
		track = append(track, domain.BusLocation{BusID: 1, Lat: lat, Lng: lng, CreatedAt: t0.Add(time.Duration(i) * time.Minute)})
	}
	stationAtStart := []domain.Station{{ID: 1, Lat: 40.40, Lng: 49.86}}
	at := func(i int, metresOff float64) domain.UserLocation {
		lat, lng := north(track[i].Lat, track[i].Lng, metresOff)
		return domain.UserLocation{Lat: lat, Lng: lng, CreatedAt: track[i].CreatedAt.Add(10 * time.Second)}
	}

	users := map[string][]domain.UserLocation{
		"rider":         {at(2, 5), at(4, 10)},                                                                                                                                      // moves with the bus => counted
		"rider3samples": {at(0, 900), at(2, 5), at(4, 5)},                                                                                                                           // only the last two samples matter => counted
		"oneSample":     {at(4, 5)},                                                                                                                                                 // not enough samples
		"walkerFar":     {at(2, 200), at(4, 200)},                                                                                                                                   // too far
		"leftBus":       {at(2, 5), at(4, 60)},                                                                                                                                      // second sample too far
		"stale":         {{Lat: track[4].Lat, Lng: track[4].Lng, CreatedAt: t0.Add(10 * time.Minute)}, {Lat: track[4].Lat, Lng: track[4].Lng, CreatedAt: t0.Add(12 * time.Minute)}}, // no bus point near in time
	}
	if got := EstimateOnboard(track, users, nil, s); got != 2 {
		t.Fatalf("EstimateOnboard = %d, want 2", got)
	}
	if got := EstimateOnboard(nil, users, nil, s); got != 0 {
		t.Fatalf("no track => 0, got %d", got)
	}

	// Someone standing at a station while the bus waits there for two samples is not counted.
	waiting := map[string][]domain.UserLocation{
		"waiting": {{Lat: 40.40, Lng: 49.86, CreatedAt: t0}, {Lat: 40.40, Lng: 49.86, CreatedAt: t0.Add(time.Minute)}},
	}
	parked := []domain.BusLocation{{BusID: 1, Lat: 40.40, Lng: 49.86, CreatedAt: t0}, {BusID: 1, Lat: 40.40, Lng: 49.86, CreatedAt: t0.Add(time.Minute)}}
	if got := EstimateOnboard(parked, waiting, stationAtStart, s); got != 0 {
		t.Fatalf("waiting passenger counted: %d", got)
	}
	// ...but a rider whose second sample is away from the station is counted.
	rider := map[string][]domain.UserLocation{"r": {at(0, 3), at(1, 3)}}
	if got := EstimateOnboard(track, rider, stationAtStart, s); got != 1 {
		t.Fatalf("rider leaving station not counted: %d", got)
	}
}
