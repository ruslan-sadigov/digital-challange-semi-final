package bl

import (
	"context"
	"math"

	"smartcity/internal/domain"
)

// Paths are road geometries stored as [lat, lng] points. Route paths are closed loops:
// the bus drives the whole path and starts again from the first point.

// toXY projects a point to local metres around refLat (fine at city scale).
func toXY(lat, lng, refLat float64) (float64, float64) {
	k := math.Cos(rad(refLat))
	return rad(lng) * k * earthRadiusM, rad(lat) * earthRadiusM
}

// PathLengthM is the total length of the polyline.
func PathLengthM(path [][2]float64) float64 {
	var d float64
	for i := 1; i < len(path); i++ {
		d += DistanceM(path[i-1][0], path[i-1][1], path[i][0], path[i][1])
	}
	return d
}

// ProjectOnPath returns the distance along the path of the point's closest projection (arc, metres)
// and how far the point is from the path (offset, metres).
func ProjectOnPath(path [][2]float64, lat, lng float64) (arc, offset float64) {
	if len(path) == 0 {
		return 0, math.Inf(1)
	}
	if len(path) == 1 {
		return 0, DistanceM(lat, lng, path[0][0], path[0][1])
	}
	best := math.Inf(1)
	var run float64
	px, py := toXY(lat, lng, lat)
	for i := 1; i < len(path); i++ {
		ax, ay := toXY(path[i-1][0], path[i-1][1], lat)
		bx, by := toXY(path[i][0], path[i][1], lat)
		dx, dy := bx-ax, by-ay
		l2 := dx*dx + dy*dy
		t := 0.0
		if l2 > 0 {
			t = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/l2))
		}
		cx, cy := ax+t*dx, ay+t*dy
		d := math.Hypot(px-cx, py-cy)
		segLen := math.Sqrt(l2)
		if d < best {
			best, arc = d, run+t*segLen
		}
		run += segLen
	}
	return arc, best
}

// PointAtArc returns the point at the given distance along the path (wrapping around a loop).
func PointAtArc(path [][2]float64, arc float64) (float64, float64) {
	if len(path) == 0 {
		return 0, 0
	}
	total := PathLengthM(path)
	if total <= 0 {
		return path[0][0], path[0][1]
	}
	arc = math.Mod(arc, total)
	if arc < 0 {
		arc += total
	}
	for i := 1; i < len(path); i++ {
		seg := DistanceM(path[i-1][0], path[i-1][1], path[i][0], path[i][1])
		if arc <= seg && seg > 0 {
			f := arc / seg
			return path[i-1][0] + (path[i][0]-path[i-1][0])*f, path[i-1][1] + (path[i][1]-path[i-1][1])*f
		}
		arc -= seg
	}
	last := path[len(path)-1]
	return last[0], last[1]
}

// PathDistanceM is how far a bus at (lat,lng) must drive along a looping path to reach the target point.
func PathDistanceM(path [][2]float64, lat, lng, tLat, tLng float64) float64 {
	total := PathLengthM(path)
	if total <= 0 {
		return DistanceM(lat, lng, tLat, tLng)
	}
	a, _ := ProjectOnPath(path, lat, lng)
	b, _ := ProjectOnPath(path, tLat, tLng)
	d := b - a
	if d < 0 {
		d += total
	}
	return d
}

// SimplifyPath removes points closer than tolM to the line between their neighbours (Douglas-Peucker).
func SimplifyPath(path [][2]float64, tolM float64) [][2]float64 {
	if len(path) < 3 {
		return path
	}
	keep := make([]bool, len(path))
	keep[0], keep[len(path)-1] = true, true
	var rec func(i, j int)
	rec = func(i, j int) {
		if j <= i+1 {
			return
		}
		idx, best := -1, tolM
		for k := i + 1; k < j; k++ {
			if d := DistanceToSegmentM(path[k][0], path[k][1], path[i][0], path[i][1], path[j][0], path[j][1]); d > best {
				idx, best = k, d
			}
		}
		if idx >= 0 {
			keep[idx] = true
			rec(i, idx)
			rec(idx, j)
		}
	}
	rec(0, len(path)-1)
	out := make([][2]float64, 0, len(path))
	for i, p := range path {
		if keep[i] {
			out = append(out, p)
		}
	}
	return out
}

// StraightPath is the fallback geometry: straight lines through the stops, closed back to the first.
func StraightPath(stops []domain.Station) [][2]float64 {
	out := make([][2]float64, 0, len(stops)+1)
	for _, s := range stops {
		out = append(out, [2]float64{s.Lat, s.Lng})
	}
	if len(stops) > 1 {
		out = append(out, [2]float64{stops[0].Lat, stops[0].Lng})
	}
	return out
}

// RoutePlanner computes road geometry through stops (implemented with OSRM in dl/osrm).
type RoutePlanner interface {
	// RoadPath is the closed driving loop through the stops.
	RoadPath(ctx context.Context, stops []domain.Station) ([][2]float64, error)
	// RoadBetween is the driving route from one point to another.
	RoadBetween(ctx context.Context, from, to [2]float64) ([][2]float64, error)
}

// EffectivePath is the route's stored road path, or straight lines between stops when it has none.
func EffectivePath(r *domain.Route) [][2]float64 {
	if len(r.Path) >= 2 {
		return r.Path
	}
	return StraightPath(r.Stops)
}

// projection is a candidate placement of a point on one path segment.
type projection struct {
	arc, offset float64
	dx, dy      float64 // segment direction in local metres
}

func projections(path [][2]float64, lat, lng float64) []projection {
	out := make([]projection, 0, len(path))
	var run float64
	px, py := toXY(lat, lng, lat)
	for i := 1; i < len(path); i++ {
		ax, ay := toXY(path[i-1][0], path[i-1][1], lat)
		bx, by := toXY(path[i][0], path[i][1], lat)
		dx, dy := bx-ax, by-ay
		l2 := dx*dx + dy*dy
		t := 0.0
		if l2 > 0 {
			t = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/l2))
		}
		segLen := math.Sqrt(l2)
		out = append(out, projection{arc: run + t*segLen, offset: math.Hypot(px-(ax+t*dx), py-(ay+t*dy)), dx: dx, dy: dy})
		run += segLen
	}
	return out
}

// sameRoadTolM: placements this much farther than the best one still count as "the same road"
// (outbound and return lanes of one street, or a stop on the opposite pavement).
const sameRoadTolM = 30

// StopArcs places each stop on the looping path in travel order: among the road segments near the stop,
// it takes the first one after the previous stop. This keeps stops on the right direction where the
// outbound and return roads run side by side.
func StopArcs(path [][2]float64, stops []domain.Station) []float64 {
	out := make([]float64, len(stops))
	prev := -1.0
	for i, s := range stops {
		cands := projections(path, s.Lat, s.Lng)
		best := math.Inf(1)
		for _, c := range cands {
			best = math.Min(best, c.offset)
		}
		pick, first := -1.0, math.Inf(1)
		for _, c := range cands {
			if c.offset > best+sameRoadTolM {
				continue
			}
			if c.arc >= prev && (pick < 0 || c.arc < pick) {
				pick = c.arc
			}
			first = math.Min(first, c.arc)
		}
		if pick < 0 { // wrapped past the end of the loop
			pick = first
		}
		out[i], prev = pick, pick
	}
	return out
}

// ProjectWithHeading places a moving vehicle on the path, preferring road segments that point the way
// it is travelling (from prev to the current point). Without a usable heading it takes the nearest segment.
func ProjectWithHeading(path [][2]float64, lat, lng float64, prev *[2]float64) float64 {
	cands := projections(path, lat, lng)
	if len(cands) == 0 {
		return 0
	}
	best := cands[0]
	for _, c := range cands {
		if c.offset < best.offset {
			best = c
		}
	}
	if prev == nil || DistanceM(prev[0], prev[1], lat, lng) < 5 {
		return best.arc
	}
	hx, hy := toXY(lat, lng, lat)
	px, py := toXY(prev[0], prev[1], lat)
	mx, my := hx-px, hy-py
	pick, pickOff := -1.0, math.Inf(1)
	for _, c := range cands {
		if c.offset > best.offset+sameRoadTolM || c.dx*mx+c.dy*my <= 0 {
			continue
		}
		if c.offset < pickOff {
			pick, pickOff = c.arc, c.offset
		}
	}
	if pick < 0 {
		return best.arc
	}
	return pick
}

// LoopDistance is how far ahead `to` is from `from` on a loop of the given length.
func LoopDistance(from, to, total float64) float64 {
	d := to - from
	if d < 0 {
		d += total
	}
	return d
}

// PointAtArcOpen is PointAtArc for a non-looping path: positions past the ends are clamped.
func PointAtArcOpen(path [][2]float64, arc float64) (float64, float64) {
	if len(path) == 0 {
		return 0, 0
	}
	if arc <= 0 {
		return path[0][0], path[0][1]
	}
	for i := 1; i < len(path); i++ {
		seg := DistanceM(path[i-1][0], path[i-1][1], path[i][0], path[i][1])
		if arc <= seg && seg > 0 {
			f := arc / seg
			return path[i-1][0] + (path[i][0]-path[i-1][0])*f, path[i-1][1] + (path[i][1]-path[i-1][1])*f
		}
		arc -= seg
	}
	last := path[len(path)-1]
	return last[0], last[1]
}

// JoinRadiusM: a rerouted bus counts as having joined its new line once it is this close to the line's road.
const JoinRadiusM = 60

// HasJoined reports whether a bus position is on the route's road (within JoinRadiusM).
func HasJoined(r *domain.Route, lat, lng float64) bool {
	_, off := ProjectOnPath(EffectivePath(r), lat, lng)
	return off <= JoinRadiusM
}

// ServingRouteID is the route a bus is actually serving right now. A rerouted bus keeps counting for its
// home route while it drives over the road network to the new line, and counts for the new line only once
// it has joined it. This keeps route density honest: approving a reroute does not relieve the crowded route
// until the extra bus is really there.
func ServingRouteID(b domain.Bus, routes map[int64]*domain.Route) *int64 {
	if b.ActiveRouteID == nil {
		return b.RouteID
	}
	r, ok := routes[*b.ActiveRouteID]
	if !ok || !b.HasPosition() || HasJoined(r, *b.LastLat, *b.LastLng) {
		return b.ActiveRouteID
	}
	return b.RouteID
}
