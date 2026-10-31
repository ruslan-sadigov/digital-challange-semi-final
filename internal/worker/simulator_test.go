package worker

import (
	"context"
	"errors"
	"testing"

	"smartcity/internal/bl"
	"smartcity/internal/domain"
)

type roadPlanner struct{ fail bool }

func (p roadPlanner) RoadPath(context.Context, []domain.Station) ([][2]float64, error) {
	return nil, nil
}

func (p roadPlanner) RoadBetween(_ context.Context, from, to [2]float64) ([][2]float64, error) {
	if p.fail {
		return nil, errors.New("down")
	}
	corner := [2]float64{from[0], to[1]} // an L-shaped street route
	return [][2]float64{from, corner, to}, nil
}

func TestTransferPathFollowsRoads(t *testing.T) {
	from, to := [2]float64{40.40, 49.85}, [2]float64{40.41, 49.86}
	s := &Simulator{Planner: roadPlanner{}}
	p := s.transferPath(context.Background(), from, to)
	if p[0] != from || p[len(p)-1] != to {
		t.Fatalf("transfer must start at the bus and end at the join point: %v", p)
	}
	road := bl.PathLengthM(p)
	straight := bl.DistanceM(from[0], from[1], to[0], to[1])
	if road <= straight {
		t.Fatalf("road transfer (%.0f m) should follow the streets, not the straight line (%.0f m)", road, straight)
	}
	// halfway along, the bus is on the road (at the corner leg), not on the straight line
	lat, lng := bl.PointAtArcOpen(p, road/2)
	if d := bl.DistanceToSegmentM(lat, lng, from[0], from[1], to[0], to[1]); d < 50 {
		t.Fatalf("midpoint is on the straight line, not the road (%.0f m off)", d)
	}
}

func TestTransferPathFallsBackToGradualStraightLine(t *testing.T) {
	from, to := [2]float64{40.40, 49.85}, [2]float64{40.41, 49.86}
	for _, s := range []*Simulator{{}, {Planner: roadPlanner{fail: true}}} {
		p := s.transferPath(context.Background(), from, to)
		if len(p) != 2 || p[0] != from || p[1] != to {
			t.Fatalf("fallback should be a straight segment driven gradually: %v", p)
		}
	}
	if p := (&Simulator{}).transferPath(context.Background(), from, from); p != nil {
		t.Fatal("no transfer needed when already on the line")
	}
}
