// Command seedgen builds the demo network from real OpenStreetMap data:
//   - real bus stops (highway=bus_stop) in central Baku, via Nominatim
//   - road-following route geometry between those stops, via OSRM
//
// The result is written to internal/worker/seeddata/baku.json and embedded in the server,
// so running the app needs no internet access. Re-run only to refresh the data:
//
//	go run ./cmd/seedgen
//
// It respects Nominatim's usage policy (1 request per second, identifying User-Agent).
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"time"

	"agilli/internal/bl"
)

const (
	userAgent = "agilli-mobillik-hackathon/1.0 (seed generator)"
	osrmBase  = "https://router.project-osrm.org"
	outFile   = "internal/worker/seeddata/baku.json"
)

type stop struct {
	OSMID int64   `json:"osmId"`
	Name  string  `json:"name"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
}

type routeOut struct {
	Code  string       `json:"code"`
	Name  string       `json:"name"`
	Buses int          `json:"buses"`
	Stops []int        `json:"stops"` // indexes into stations, in travel order
	Path  [][2]float64 `json:"path"`  // [lat, lng] along real roads, closed loop
}

type dataset struct {
	Source   string     `json:"source"`
	Stations []stop     `json:"stations"`
	Routes   []routeOut `json:"routes"`
}

// Route definitions: anchor points near well-known places; each anchor is snapped to the nearest
// real bus stop, and real stops lying on the road path in between are added automatically.
type anchor struct {
	label    string
	lat, lng float64
}

var routes = []struct {
	code, name string
	buses      int
	anchors    []anchor
}{
	{"65", "Icherisheher - Koroglu", 2, []anchor{
		{"Icherisheher", 40.3661, 49.8352}, {"Sahil", 40.3716, 49.8432}, {"28 May", 40.3795, 49.8486},
		{"Ganjlik", 40.4003, 49.8516}, {"Narimanov", 40.4026, 49.8706}, {"Koroglu", 40.4182, 49.9178}}},
	{"13", "Elmlar - Azadliq", 3, []anchor{
		{"Elmlar", 40.3753, 49.8122}, {"Inshaatchilar", 40.3910, 49.8037}, {"20 Yanvar", 40.4045, 49.8085},
		{"Memar Ajami", 40.4113, 49.8154}, {"Nasimi", 40.4249, 49.8227}, {"Azadliq", 40.4253, 49.8429}}},
	{"88", "Nizami - Darnagul", 3, []anchor{
		{"Nizami", 40.3793, 49.8302}, {"28 May", 40.3795, 49.8486}, {"Khatai", 40.3832, 49.8721},
		{"Narimanov", 40.4026, 49.8706}, {"Darnagul", 40.4253, 49.8616}}},
	{"5", "Sahil - 20 Yanvar", 3, []anchor{
		{"Sahil", 40.3716, 49.8432}, {"Nizami", 40.3793, 49.8302}, {"Elmlar", 40.3753, 49.8122},
		{"Inshaatchilar", 40.3910, 49.8037}, {"20 Yanvar", 40.4045, 49.8085}}},
}

var client = &http.Client{Timeout: 60 * time.Second}

// getJSON fetches JSON, backing off on HTTP 429 (rate limit).
func getJSON(u string, out any) error {
	for attempt := 0; ; attempt++ {
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("User-Agent", userAgent)
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 5 {
			res.Body.Close()
			wait := time.Duration(15*(attempt+1)) * time.Second
			log.Printf("rate limited, waiting %s", wait)
			time.Sleep(wait)
			continue
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("GET %s: %s", u, res.Status)
		}
		return json.NewDecoder(res.Body).Decode(out)
	}
}

const cacheFile = ".cache/osm_bus_stops.json"

// loadStops returns the cached stop list, or fetches it from Nominatim and caches it.
func loadStops() ([]stop, error) {
	if b, err := os.ReadFile(cacheFile); err == nil {
		var cached []stop
		if json.Unmarshal(b, &cached) == nil && len(cached) > 0 {
			log.Printf("using cached bus stops from %s (delete it to refresh)", cacheFile)
			return cached, nil
		}
	}
	log.Println("fetching real bus stops from Nominatim (about 40 s)...")
	stops, err := fetchStops()
	if err != nil {
		return nil, err
	}
	_ = os.MkdirAll(".cache", 0o755)
	b, _ := json.Marshal(stops)
	_ = os.WriteFile(cacheFile, b, 0o644)
	return stops, nil
}

// fetchStops tiles the area and asks Nominatim for bus stops in each tile.
func fetchStops() ([]stop, error) {
	seen := map[int64]bool{}
	var out []stop
	const step = 0.02
	for lat := 40.355; lat < 40.44; lat += step {
		for lng := 49.79; lng < 49.935; lng += step {
			q := url.Values{
				"q": {"bus stop"}, "format": {"jsonv2"}, "limit": {"50"}, "bounded": {"1"},
				"viewbox": {fmt.Sprintf("%f,%f,%f,%f", lng, lat+step, lng+step, lat)},
			}
			var res []struct {
				OSMID    int64  `json:"osm_id"`
				OSMType  string `json:"osm_type"`
				Category string `json:"category"`
				Type     string `json:"type"`
				Name     string `json:"name"`
				Lat      string `json:"lat"`
				Lon      string `json:"lon"`
			}
			if err := getJSON("https://nominatim.openstreetmap.org/search?"+q.Encode(), &res); err != nil {
				return nil, err
			}
			for _, r := range res {
				if r.Category != "highway" || r.Type != "bus_stop" || r.OSMType != "node" || seen[r.OSMID] {
					continue
				}
				la, _ := strconv.ParseFloat(r.Lat, 64)
				lo, _ := strconv.ParseFloat(r.Lon, 64)
				name := r.Name
				if name == "" {
					name = fmt.Sprintf("Bus stop %d", r.OSMID)
				}
				seen[r.OSMID] = true
				out = append(out, stop{OSMID: r.OSMID, Name: name, Lat: la, Lng: lo})
			}
			time.Sleep(1100 * time.Millisecond) // Nominatim policy: max 1 req/s
		}
	}
	return out, nil
}

// roadPath asks OSRM for the driving route through the points and back to the first one.
func roadPath(pts [][2]float64) ([][2]float64, error) {
	coords := ""
	for i, p := range append(pts, pts[0]) {
		if i > 0 {
			coords += ";"
		}
		coords += fmt.Sprintf("%.6f,%.6f", p[1], p[0])
	}
	var res struct {
		Code   string `json:"code"`
		Routes []struct {
			Geometry struct {
				Coordinates [][2]float64 `json:"coordinates"` // [lng, lat]
			} `json:"geometry"`
		} `json:"routes"`
	}
	if err := getJSON(osrmBase+"/route/v1/driving/"+coords+"?overview=full&geometries=geojson", &res); err != nil {
		return nil, err
	}
	if res.Code != "Ok" || len(res.Routes) == 0 {
		return nil, fmt.Errorf("osrm: %s", res.Code)
	}
	var out [][2]float64
	for _, c := range res.Routes[0].Geometry.Coordinates {
		out = append(out, [2]float64{round6(c[1]), round6(c[0])})
	}
	return out, nil
}

func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }

func nearestStop(stops []stop, lat, lng float64) (int, float64) {
	bi, bd := -1, math.Inf(1)
	for i, s := range stops {
		if d := bl.DistanceM(lat, lng, s.Lat, s.Lng); d < bd {
			bi, bd = i, d
		}
	}
	return bi, bd
}

func main() {
	pool, err := loadStops()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%d bus stops found", len(pool))

	ds := dataset{Source: "OpenStreetMap contributors (ODbL): stops via Nominatim, road geometry via OSRM"}
	stationIdx := map[int64]int{}
	addStation := func(s stop) int {
		if i, ok := stationIdx[s.OSMID]; ok {
			return i
		}
		ds.Stations = append(ds.Stations, s)
		stationIdx[s.OSMID] = len(ds.Stations) - 1
		return len(ds.Stations) - 1
	}

	for _, r := range routes {
		// 1. snap anchors to real stops
		var anchorStops []stop
		for _, a := range r.anchors {
			i, d := nearestStop(pool, a.lat, a.lng)
			if i < 0 || d > 600 {
				log.Fatalf("route %s: no real bus stop within 600 m of %s", r.code, a.label)
			}
			anchorStops = append(anchorStops, pool[i])
		}
		pts := make([][2]float64, len(anchorStops))
		for i, s := range anchorStops {
			pts[i] = [2]float64{s.Lat, s.Lng}
		}
		// 2. real road geometry through them
		path, err := roadPath(pts)
		if err != nil {
			log.Fatalf("route %s: %v", r.code, err)
		}
		path = bl.SimplifyPath(path, 3)
		time.Sleep(500 * time.Millisecond)

		// 3. every real stop within 20 m of the road becomes a stop, in travel order, at least 300 m apart
		type onPath struct {
			s   stop
			arc float64
		}
		var cand []onPath
		total := bl.PathLengthM(path)
		for _, s := range pool {
			arc, off := bl.ProjectOnPath(path, s.Lat, s.Lng)
			if off <= 20 && arc < total-50 {
				cand = append(cand, onPath{s, arc})
			}
		}
		for _, a := range anchorStops { // anchors are preferred, but only if the bus really passes them
			arc, off := bl.ProjectOnPath(path, a.Lat, a.Lng)
			if off <= 20 {
				cand = append(cand, onPath{a, arc})
			} else {
				log.Printf("route %s: anchor stop %q is %.0f m off the road, skipped", r.code, a.Name, off)
			}
		}
		sort.Slice(cand, func(i, j int) bool { return cand[i].arc < cand[j].arc })
		isAnchor := map[int64]bool{}
		for _, a := range anchorStops {
			isAnchor[a.OSMID] = true
		}
		var chosen []onPath
		used := map[int64]bool{}
		for _, c := range cand {
			if used[c.s.OSMID] {
				continue
			}
			if n := len(chosen); n > 0 && c.arc-chosen[n-1].arc < 300 {
				if !isAnchor[c.s.OSMID] {
					continue
				}
				if !isAnchor[chosen[n-1].s.OSMID] { // anchor replaces a too-close plain stop
					used[chosen[n-1].s.OSMID] = false
					chosen = chosen[:n-1]
				}
			}
			chosen = append(chosen, c)
			used[c.s.OSMID] = true
		}
		ro := routeOut{Code: r.code, Name: r.name, Buses: r.buses, Path: path}
		for _, c := range chosen {
			ro.Stops = append(ro.Stops, addStation(c.s))
		}
		ds.Routes = append(ds.Routes, ro)
		log.Printf("route %s: %d stops, %d path points, %.1f km loop", r.code, len(ro.Stops), len(path), total/1000)
	}

	if err := os.MkdirAll("internal/worker/seeddata", 0o755); err != nil {
		log.Fatal(err)
	}
	b, _ := json.MarshalIndent(ds, "", " ")
	if err := os.WriteFile(outFile, b, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s: %d stations, %d routes", outFile, len(ds.Stations), len(ds.Routes))
}
