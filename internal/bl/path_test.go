package bl

import (
	"math"
	"testing"

	"smartcity/internal/domain"
)

// An L-shaped closed loop: 1 km north, 1 km east, then straight back (~1.41 km).
func lLoop() [][2]float64 {
	aLat, aLng := 40.40, 49.85
	bLat, _ := north(aLat, aLng, 1000)
	cLng := aLng + 1000/(111320*math.Cos(rad(bLat)))
	return [][2]float64{{aLat, aLng}, {bLat, aLng}, {bLat, cLng}, {aLat, aLng}}
}

func TestPathLengthAndProjection(t *testing.T) {
	p := lLoop()
	total := PathLengthM(p)
	if math.Abs(total-(2000+math.Sqrt2*1000)) > 15 {
		t.Fatalf("length %v", total)
	}
	// a point 50 m east of the middle of the first leg
	mLat, mLng := north(40.40, 49.85, 500)
	mLng += 50 / (111320 * math.Cos(rad(mLat)))
	arc, off := ProjectOnPath(p, mLat, mLng)
	if math.Abs(arc-500) > 2 || math.Abs(off-50) > 2 {
		t.Fatalf("arc %v off %v", arc, off)
	}
	if _, off := ProjectOnPath(nil, 1, 1); !math.IsInf(off, 1) {
		t.Fatal("empty path should give +Inf offset")
	}
}

func TestPointAtArcWraps(t *testing.T) {
	p := lLoop()
	total := PathLengthM(p)
	lat, lng := PointAtArc(p, 1500) // half way along the second leg
	arc, off := ProjectOnPath(p, lat, lng)
	if math.Abs(arc-1500) > 2 || off > 1 {
		t.Fatalf("arc %v off %v", arc, off)
	}
	lat2, lng2 := PointAtArc(p, 1500+3*total) // laps are ignored
	if DistanceM(lat, lng, lat2, lng2) > 1 {
		t.Fatal("PointAtArc should wrap around the loop")
	}
	lat3, lng3 := PointAtArc(p, -10) // negative = just before the end
	if a, _ := ProjectOnPath(p, lat3, lng3); math.Abs(a-(total-10)) > 2 {
		t.Fatalf("negative arc: %v", a)
	}
}

func TestPathDistanceIsCircular(t *testing.T) {
	p := lLoop()
	total := PathLengthM(p)
	aLat, aLng := PointAtArc(p, 200)
	bLat, bLng := PointAtArc(p, 1200)
	if d := PathDistanceM(p, aLat, aLng, bLat, bLng); math.Abs(d-1000) > 3 {
		t.Fatalf("ahead: %v", d)
	}
	// the target is behind the bus: it must complete the loop
	if d := PathDistanceM(p, bLat, bLng, aLat, aLng); math.Abs(d-(total-1000)) > 3 {
		t.Fatalf("behind: %v", d)
	}
}

func TestSimplifyPath(t *testing.T) {
	// 101 points on a straight line with 1 m wiggles collapse to the two ends
	var line [][2]float64
	for i := 0; i <= 100; i++ {
		lat, lng := north(40.40, 49.85, float64(i)*10)
		if i%2 == 1 {
			lng += 1 / (111320 * math.Cos(rad(lat)))
		}
		line = append(line, [2]float64{lat, lng})
	}
	if got := SimplifyPath(line, 3); len(got) != 2 {
		t.Fatalf("want 2 points, got %d", len(got))
	}
	// corners are kept
	if got := SimplifyPath(lLoop(), 3); len(got) != 4 {
		t.Fatalf("corners lost: %d", len(got))
	}
}

func TestEffectivePath(t *testing.T) {
	stops := []domain.Station{{Lat: 40.40, Lng: 49.85}, {Lat: 40.41, Lng: 49.85}}
	r := &domain.Route{Stops: stops}
	if got := EffectivePath(r); len(got) != 3 || got[2] != got[0] {
		t.Fatalf("fallback should be a closed straight loop: %v", got)
	}
	r.Path = lLoop()
	if got := EffectivePath(r); len(got) != 4 {
		t.Fatal("stored road path should be used")
	}
}

func TestRouteDistanceUsesRoadNotStraightLine(t *testing.T) {
	// Two stops at the start and corner of the L: by road the bus drives 2 km, not the 1.41 km diagonal.
	p := lLoop()
	cLat, cLng := p[2][0], p[2][1]
	d := PathDistanceM(p, p[0][0], p[0][1], cLat, cLng)
	if math.Abs(d-2000) > 10 {
		t.Fatalf("road distance %v, want ~2000", d)
	}
	if straight := DistanceM(p[0][0], p[0][1], cLat, cLng); d <= straight {
		t.Fatal("road distance must exceed straight-line distance here")
	}
}

// An out-and-back street: east along the north lane, back west along the south lane 12 m away.
func twoLaneLoop() [][2]float64 {
	lat, lng := 40.40, 49.85
	k := 111320 * math.Cos(rad(lat))
	south := lat - 12.0/111320
	end := lng + 2000/k
	return [][2]float64{{lat, lng}, {lat, end}, {south, end}, {south, lng}, {lat, lng}}
}

func TestStopArcsFollowTravelOrderOnParallelLanes(t *testing.T) {
	p := twoLaneLoop()
	k := 111320 * math.Cos(rad(40.40))
	mid := 49.85 + 1000/k
	// Outbound stop at 1 km, terminal at the far end, then a return stop 300 m back west.
	// The return stop is closer to the outbound lane, but it comes after the terminal in travel order,
	// so it must land on the return lane (~3.3 km along the loop), not the outbound lane (~0.7 km).
	stops := []domain.Station{
		{Lat: 40.40 + 6.0/111320, Lng: mid},            // outbound pavement
		{Lat: 40.40 - 6.0/111320, Lng: 49.85 + 2000/k}, // terminal at the turnaround
		{Lat: 40.40 - 3.0/111320, Lng: 49.85 + 700/k},  // return stop, nearer the outbound lane
	}
	arcs := StopArcs(p, stops)
	if math.Abs(arcs[0]-1000) > 5 {
		t.Fatalf("outbound stop arc %v, want ~1000", arcs[0])
	}
	if math.Abs(arcs[2]-3312) > 10 {
		t.Fatalf("return stop arc %v, want ~3312 (return lane)", arcs[2])
	}
	// plain nearest-road projection gets it wrong, which is why StopArcs exists
	if a, _ := ProjectOnPath(p, stops[2].Lat, stops[2].Lng); math.Abs(a-700) > 10 {
		t.Fatalf("expected nearest projection on the outbound lane, got %v", a)
	}
}

func TestProjectWithHeadingPicksTheLaneOfTravel(t *testing.T) {
	p := twoLaneLoop()
	k := 111320 * math.Cos(rad(40.40))
	lat := 40.40 - 5.0/111320 // between the lanes, nearer the outbound one
	lng := 49.85 + 1000/k
	westward := &[2]float64{lat, lng + 30/k} // came from the east => driving west => return lane
	if a := ProjectWithHeading(p, lat, lng, westward); math.Abs(a-3012) > 10 {
		t.Fatalf("westbound bus arc %v, want ~3012", a)
	}
	eastward := &[2]float64{lat, lng - 30/k}
	if a := ProjectWithHeading(p, lat, lng, eastward); math.Abs(a-1000) > 10 {
		t.Fatalf("eastbound bus arc %v, want ~1000", a)
	}
	if a := ProjectWithHeading(p, lat, lng, nil); math.Abs(a-1000) > 10 {
		t.Fatalf("no heading: nearest lane expected, got %v", a)
	}
	// ETA distance: westbound bus to the return-side stop just ahead vs. behind
	if d := LoopDistance(3012, 3112, PathLengthM(p)); math.Abs(d-100) > 1 {
		t.Fatalf("loop distance %v", d)
	}
	if d := LoopDistance(3112, 3012, 4024); math.Abs(d-3924) > 1 {
		t.Fatalf("wrapped loop distance %v", d)
	}
}

func TestPointAtArcOpenClamps(t *testing.T) {
	p := [][2]float64{{40.40, 49.85}, {40.41, 49.85}}
	if lat, _ := PointAtArcOpen(p, -5); lat != 40.40 {
		t.Fatal("before start should clamp to start")
	}
	if lat, _ := PointAtArcOpen(p, 1e9); lat != 40.41 {
		t.Fatal("past end should clamp to end (no wrap)")
	}
	lat, _ := PointAtArcOpen(p, PathLengthM(p)/2)
	if math.Abs(lat-40.405) > 1e-6 {
		t.Fatalf("middle: %v", lat)
	}
}
