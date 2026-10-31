package bl

import (
	"context"
	"math"

	"agilli/internal/domain"
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
	RoadPath(ctx context.Context, stops []domain.Station) ([][2]float64, error)
}

// EffectivePath is the route's stored road path, or straight lines between stops when it has none.
func EffectivePath(r *domain.Route) [][2]float64 {
	if len(r.Path) >= 2 {
		return r.Path
	}
	return StraightPath(r.Stops)
}
