package bl

import (
	"math"
	"testing"

	"agilli/internal/domain"
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
