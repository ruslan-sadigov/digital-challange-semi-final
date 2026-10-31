package osrm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agilli/internal/domain"
)

func TestRoadPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"code":"Ok","routes":[{"geometry":{"coordinates":[[49.85,40.40],[49.86,40.41],[49.85,40.40]]}}]}`))
	}))
	defer srv.Close()

	p := New(srv.URL + "/")
	stops := []domain.Station{{Lat: 40.40, Lng: 49.85}, {Lat: 40.41, Lng: 49.86}}
	path, err := p.RoadPath(context.Background(), stops)
	if err != nil {
		t.Fatal(err)
	}
	// request: lng,lat pairs, closed back to the first stop
	if want := "/route/v1/driving/49.850000,40.400000;49.860000,40.410000;49.850000,40.400000"; gotPath != want {
		t.Fatalf("request path %q", gotPath)
	}
	// response converted to [lat, lng]
	if len(path) != 3 || path[1] != [2]float64{40.41, 49.86} {
		t.Fatalf("path %v", path)
	}
}

func TestRoadPathErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "49.1") {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"code":"NoRoute","routes":[]}`))
	}))
	defer srv.Close()
	p := New(srv.URL)
	two := []domain.Station{{Lat: 40.4, Lng: 49.85}, {Lat: 40.41, Lng: 49.86}}
	if _, err := p.RoadPath(context.Background(), two); err == nil {
		t.Fatal("NoRoute must be an error")
	}
	if _, err := p.RoadPath(context.Background(), []domain.Station{{Lat: 40.4, Lng: 49.1}, {Lat: 40.41, Lng: 49.86}}); err == nil {
		t.Fatal("HTTP error must be an error")
	}
	if _, err := p.RoadPath(context.Background(), two[:1]); err == nil {
		t.Fatal("one stop must be an error")
	}
}
