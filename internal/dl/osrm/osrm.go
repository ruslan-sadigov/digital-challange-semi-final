// Package osrm computes road-following route geometry with an OSRM server (implements bl.RoutePlanner).
package osrm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"smartcity/internal/domain"
)

type Planner struct {
	BaseURL string // e.g. https://router.project-osrm.org
	Client  *http.Client
}

func New(baseURL string) *Planner {
	return &Planner{BaseURL: strings.TrimRight(baseURL, "/"), Client: &http.Client{Timeout: 15 * time.Second}}
}

// RoadPath returns the driving route through the stops and back to the first one, as [lat, lng] points.
func (p *Planner) RoadPath(ctx context.Context, stops []domain.Station) ([][2]float64, error) {
	if len(stops) < 2 {
		return nil, fmt.Errorf("need at least 2 stops")
	}
	pts := make([][2]float64, 0, len(stops)+1)
	for _, s := range append(stops, stops[0]) {
		pts = append(pts, [2]float64{s.Lat, s.Lng})
	}
	return p.route(ctx, pts)
}

// RoadBetween returns the driving route from one point to another (used when a bus moves to a new route).
func (p *Planner) RoadBetween(ctx context.Context, from, to [2]float64) ([][2]float64, error) {
	return p.route(ctx, [][2]float64{from, to})
}

func (p *Planner) route(ctx context.Context, pts [][2]float64) ([][2]float64, error) {
	parts := make([]string, 0, len(pts))
	for _, c := range pts {
		parts = append(parts, fmt.Sprintf("%.6f,%.6f", c[1], c[0]))
	}
	u := p.BaseURL + "/route/v1/driving/" + strings.Join(parts, ";") + "?overview=full&geometries=geojson"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "smart-city-transportation/1.0")
	res, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("osrm: %s", res.Status)
	}
	var body struct {
		Code   string `json:"code"`
		Routes []struct {
			Geometry struct {
				Coordinates [][2]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Code != "Ok" || len(body.Routes) == 0 {
		return nil, fmt.Errorf("osrm: %s", body.Code)
	}
	out := make([][2]float64, 0, len(body.Routes[0].Geometry.Coordinates))
	for _, c := range body.Routes[0].Geometry.Coordinates {
		out = append(out, [2]float64{c[1], c[0]})
	}
	return out, nil
}
