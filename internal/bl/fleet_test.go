package bl

import (
	"math"
	"testing"
	"time"

	"agilli/internal/domain"
)

func TestDistanceM(t *testing.T) {
	// 1 degree of latitude is ~111.2 km
	d := DistanceM(40, 49, 41, 49)
	if math.Abs(d-111195) > 100 {
		t.Fatalf("got %v", d)
	}
	if DistanceM(40.4, 49.8, 40.4, 49.8) != 0 {
		t.Fatal("same point should be 0")
	}
}

func TestDistanceToSegmentM(t *testing.T) {
	// point 100 m north of the middle of an east-west segment
	lat, lng := north(40.4, 49.85, 100)
	d := DistanceToSegmentM(lat, lng, 40.4, 49.84, 40.4, 49.86)
	if math.Abs(d-100) > 1 {
		t.Fatalf("got %v", d)
	}
	// beyond the end: distance to the endpoint
	d = DistanceToSegmentM(40.4, 49.87, 40.4, 49.84, 40.4, 49.86)
	if math.Abs(d-DistanceM(40.4, 49.87, 40.4, 49.86)) > 1 {
		t.Fatalf("got %v", d)
	}
}

// square route of 4 stops ~1.1 km apart: 0 (S) -> 1 (N) -> 2 (NE) -> 3 (E) -> back to 0
var square = []domain.Station{
	{ID: 10, Lat: 40.40, Lng: 49.85},
	{ID: 11, Lat: 40.41, Lng: 49.85},
	{ID: 12, Lat: 40.41, Lng: 49.863},
	{ID: 13, Lat: 40.40, Lng: 49.863},
}

func seg(i int) float64 {
	a, b := square[i], square[(i+1)%len(square)]
	return DistanceM(a.Lat, a.Lng, b.Lat, b.Lng)
}

func TestRouteDistanceM(t *testing.T) {
	mid := (square[0].Lat + square[1].Lat) / 2 // halfway along segment 0 -> 1
	got := RouteDistanceM(square, mid, 49.85, 1)
	if math.Abs(got-seg(0)/2) > 2 {
		t.Fatalf("to next stop: got %v want %v", got, seg(0)/2)
	}
	got = RouteDistanceM(square, mid, 49.85, 3)
	want := seg(0)/2 + seg(1) + seg(2)
	if math.Abs(got-want) > 2 {
		t.Fatalf("to stop 3: got %v want %v", got, want)
	}
	// stop 0 is behind the bus: it must go around the loop
	got = RouteDistanceM(square, mid, 49.85, 0)
	want = seg(0)/2 + seg(1) + seg(2) + seg(3)
	if math.Abs(got-want) > 2 {
		t.Fatalf("circular: got %v want %v", got, want)
	}
	if !math.IsNaN(RouteDistanceM(square, mid, 49.85, 9)) || !math.IsNaN(RouteDistanceM(nil, 0, 0, 0)) {
		t.Fatal("invalid target should be NaN")
	}
}

func TestAverageSpeedKmh(t *testing.T) {
	var track []domain.BusLocation
	for i := 0; i < 5; i++ { // 300 m per minute = 18 km/h
		lat, lng := north(40.4, 49.85, float64(i)*300)
		track = append(track, domain.BusLocation{Lat: lat, Lng: lng, CreatedAt: t0.Add(time.Duration(i) * time.Minute)})
	}
	v, ok := AverageSpeedKmh(track)
	if !ok || math.Abs(v-18) > 0.1 {
		t.Fatalf("got %v %v", v, ok)
	}
	if _, ok := AverageSpeedKmh(track[:1]); ok {
		t.Fatal("one point is not enough")
	}
	parked := []domain.BusLocation{{Lat: 40.4, Lng: 49.85, CreatedAt: t0}, {Lat: 40.4, Lng: 49.85, CreatedAt: t0.Add(time.Minute)}}
	if _, ok := AverageSpeedKmh(parked); ok {
		t.Fatal("a parked bus should fall back to the default speed")
	}
}

func TestAggregateHeat(t *testing.T) {
	locs := []domain.UserLocation{
		{Lat: 40.40001, Lng: 49.85001}, {Lat: 40.40009, Lng: 49.85009}, // same 0.001 cell
		{Lat: 40.4105, Lng: 49.8605},
	}
	pts := AggregateHeat(locs, 0.001)
	if len(pts) != 2 || pts[0].Weight != 2 || pts[1].Weight != 1 {
		t.Fatalf("unexpected heat points: %+v", pts)
	}
	// cell centre, not the raw point
	if math.Abs(pts[0].Lat-40.4005) > 1e-9 || math.Abs(pts[0].Lng-49.8505) > 1e-9 {
		t.Fatalf("expected the cell centre, got %+v", pts[0])
	}
	if len(AggregateHeat(nil, 0.001)) != 0 {
		t.Fatal("empty input")
	}
}

func TestValidation(t *testing.T) {
	if err := validateRoute(&domain.Route{Code: "1", Name: "x"}, []int64{1}); err == nil {
		t.Fatal("route with 1 stop must fail")
	}
	if err := validateRoute(&domain.Route{Code: "1", Name: "x"}, []int64{1, 1}); err == nil {
		t.Fatal("repeated consecutive stop must fail")
	}
	if err := validateRoute(&domain.Route{Code: " ", Name: "x"}, []int64{1, 2}); err == nil {
		t.Fatal("blank code must fail")
	}
	if err := validateRoute(&domain.Route{Code: "1", Name: "x"}, []int64{1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := validateBus(&domain.Bus{Name: "A", Capacity: 0}); err == nil {
		t.Fatal("capacity 0 must fail")
	}
	if err := validateStation(&domain.Station{Name: "S", Lat: 0, Lng: 0}); err == nil {
		t.Fatal("0,0 must fail")
	}
	if err := validateStation(&domain.Station{Name: "S", Lat: 95, Lng: 49}); err == nil {
		t.Fatal("lat 95 must fail")
	}
}
