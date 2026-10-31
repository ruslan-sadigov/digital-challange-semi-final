// Command seedgen builds the demo network from real OpenStreetMap bus routes in Baku.
//
// For each configured bus line it downloads the OSM route relations (both directions, linked by
// their route_master) from the OSM API, stitches the member ways into the exact road path the bus
// drives, and takes the stops (platform members, real names) in their official order. The forward
// and return directions are joined into one loop.
//
// The result is written to internal/worker/seeddata/baku.json and embedded in the server, so the
// app runs offline. Re-run only to refresh the data:
//
//	go run ./cmd/seedgen
//
// Data (c) OpenStreetMap contributors, ODbL.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"smartcity/internal/bl"
)

const (
	userAgent = "smart-city-transportation/1.0 (hackathon seed generator)"
	osmAPI    = "https://api.openstreetmap.org/api/0.6"
	outFile   = "internal/worker/seeddata/baku.json"
)

// Real Baku bus lines (one OSM route relation per line; the other direction is found through its route_master).
// Relation ids were discovered by asking the OSM API which bus routes serve real bus stops in central Baku.
var lines = []struct {
	ref      string
	relation int64
	reverse  int64 // return direction when OSM has no route_master linking them
	buses    int
}{
	{"65", 21213015, 0, 2},        // Azneft meydanı - 9-cu mikrorayon (overloaded in the demo)
	{"88", 2393325, 0, 3},         // 20-ci sahə - Dərnəgül m/st
	{"13", 2835830, 0, 3},         // Avtovağzal - Koroğlu m/st
	{"3", 21212095, 0, 3},         // Dərnəgül m/st - Badamdar qəs.
	{"85", 21204983, 0, 3},        // 28 Mall - 9-cu mikrorayon
	{"10", 2439658, 0, 3},         // MİDA Yasamal - Ağ Şəhər
	{"93", 21233102, 21233065, 2}, // Dərnəgül m/st - Sabunçu stansiyası
	{"2", 21214695, 0, 3},         // Dərnəgül m/st - Avtovağzal
}

type station struct {
	OSMID int64   `json:"osmId"`
	Name  string  `json:"name"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
}

type routeOut struct {
	Code      string       `json:"code"`
	Name      string       `json:"name"`
	Buses     int          `json:"buses"`
	OSMRoutes []int64      `json:"osmRelations"`
	Stops     []int        `json:"stops"` // indexes into stations, in travel order around the loop
	Path      [][2]float64 `json:"path"`  // [lat, lng] along the real roads, closed loop
}

type dataset struct {
	Source   string     `json:"source"`
	Stations []station  `json:"stations"`
	Routes   []routeOut `json:"routes"`
}

// ---------- OSM API ----------

type element struct {
	Type    string            `json:"type"`
	ID      int64             `json:"id"`
	Lat     float64           `json:"lat"`
	Lon     float64           `json:"lon"`
	Nodes   []int64           `json:"nodes"`
	Tags    map[string]string `json:"tags"`
	Members []struct {
		Type string `json:"type"`
		Ref  int64  `json:"ref"`
		Role string `json:"role"`
	} `json:"members"`
}

var client = &http.Client{Timeout: 60 * time.Second}

func getElements(path string) ([]element, error) {
	for attempt := 0; ; attempt++ {
		req, _ := http.NewRequest("GET", osmAPI+path, nil)
		req.Header.Set("User-Agent", userAgent)
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if (res.StatusCode == 429 || res.StatusCode >= 500) && attempt < 4 {
			res.Body.Close()
			time.Sleep(time.Duration(5*(attempt+1)) * time.Second)
			continue
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("GET %s: %s", path, res.Status)
		}
		var body struct {
			Elements []element `json:"elements"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			return nil, err
		}
		time.Sleep(400 * time.Millisecond) // be gentle with the OSM API
		return body.Elements, nil
	}
}

// directions returns the route relation ids of both directions of a line (via route_master).
func directions(rel int64) ([]int64, error) {
	parents, err := getElements(fmt.Sprintf("/relation/%d/relations.json", rel))
	if err != nil {
		return nil, err
	}
	for _, p := range parents {
		if p.Tags["type"] != "route_master" {
			continue
		}
		var out []int64
		for _, m := range p.Members {
			if m.Type == "relation" {
				out = append(out, m.Ref)
			}
		}
		for i, id := range out { // start with the configured direction
			if id == rel {
				out = append(out[i:], out[:i]...)
				break
			}
		}
		if len(out) > 2 {
			out = out[:2]
		}
		return out, nil
	}
	return []int64{rel}, nil
}

type direction struct {
	rel   element
	path  [][2]float64
	stops []station
	gaps  int
}

func isPlatform(role string) bool { return strings.HasPrefix(role, "platform") }

// loadDirection downloads a route relation with its ways and nodes and builds its road path and stops.
func loadDirection(id int64) (*direction, error) {
	els, err := getElements(fmt.Sprintf("/relation/%d/full.json", id))
	if err != nil {
		return nil, err
	}
	nodes := map[int64]element{}
	ways := map[int64]element{}
	var rel element
	for _, e := range els {
		switch e.Type {
		case "node":
			nodes[e.ID] = e
		case "way":
			ways[e.ID] = e
		case "relation":
			if e.ID == id {
				rel = e
			}
		}
	}
	d := &direction{rel: rel}
	// Stops: platform members in order (real bus stop nodes with names).
	for _, m := range rel.Members {
		if m.Type != "node" || !isPlatform(m.Role) {
			continue
		}
		n, ok := nodes[m.Ref]
		if !ok {
			continue
		}
		name := n.Tags["name"]
		if name == "" {
			name = n.Tags["name:az"]
		}
		if name == "" {
			name = fmt.Sprintf("Bus stop %d", n.ID)
		}
		d.stops = append(d.stops, station{OSMID: n.ID, Name: name, Lat: n.Lat, Lng: n.Lon})
	}
	// Path: stitch the road ways (members without a stop/platform role) in order.
	var seq [][]int64
	for _, m := range rel.Members {
		if m.Type == "way" && (m.Role == "" || m.Role == "forward" || m.Role == "backward") {
			if w, ok := ways[m.Ref]; ok && len(w.Nodes) > 1 {
				seq = append(seq, w.Nodes)
			}
		}
	}
	if len(seq) == 0 {
		return nil, fmt.Errorf("relation %d has no road ways", id)
	}
	coord := func(n int64) [2]float64 { e := nodes[n]; return [2]float64{e.Lat, e.Lon} }
	reverse := func(a []int64) []int64 {
		b := make([]int64, len(a))
		for i := range a {
			b[i] = a[len(a)-1-i]
		}
		return b
	}
	first := seq[0] // orient the first way towards the second
	if len(seq) > 1 {
		nx := seq[1]
		if first[0] == nx[0] || first[0] == nx[len(nx)-1] {
			first = reverse(first)
		}
	}
	chain := append([]int64{}, first...)
	for _, w := range seq[1:] {
		end := chain[len(chain)-1]
		switch {
		case w[0] == end:
			chain = append(chain, w[1:]...)
		case w[len(w)-1] == end:
			chain = append(chain, reverse(w)[1:]...)
		default:
			// not connected (incomplete mapping): take the orientation whose start is closer
			e := coord(end)
			a, b := coord(w[0]), coord(w[len(w)-1])
			if bl.DistanceM(e[0], e[1], b[0], b[1]) < bl.DistanceM(e[0], e[1], a[0], a[1]) {
				w = reverse(w)
			}
			d.gaps++
			chain = append(chain, w...)
		}
	}
	for _, n := range chain {
		if _, ok := nodes[n]; ok {
			d.path = append(d.path, coord(n))
		}
	}
	return d, nil
}

func round6(p [][2]float64) [][2]float64 {
	out := make([][2]float64, len(p))
	for i, c := range p {
		out[i] = [2]float64{math.Round(c[0]*1e6) / 1e6, math.Round(c[1]*1e6) / 1e6}
	}
	return out
}

func main() {
	ds := dataset{Source: "OpenStreetMap contributors (ODbL): real Baku bus route relations via the OSM API"}
	stationIdx := map[int64]int{}
	addStation := func(s station) int {
		if i, ok := stationIdx[s.OSMID]; ok {
			return i
		}
		ds.Stations = append(ds.Stations, s)
		stationIdx[s.OSMID] = len(ds.Stations) - 1
		return len(ds.Stations) - 1
	}

	for _, ln := range lines {
		dirIDs, err := directions(ln.relation)
		if err != nil {
			log.Fatalf("line %s: %v", ln.ref, err)
		}
		if len(dirIDs) == 1 && ln.reverse != 0 {
			dirIDs = append(dirIDs, ln.reverse)
		}
		var dirs []*direction
		for _, id := range dirIDs {
			d, err := loadDirection(id)
			if err != nil {
				log.Fatalf("line %s: %v", ln.ref, err)
			}
			dirs = append(dirs, d)
		}
		// One loop: forward path + return path (or back along the same roads if only one direction is mapped).
		var path [][2]float64
		var stops []station
		gaps := 0
		for _, d := range dirs {
			path = append(path, d.path...)
			stops = append(stops, d.stops...)
			gaps += d.gaps
		}
		if len(dirs) == 1 {
			for i := len(dirs[0].path) - 2; i >= 0; i-- {
				path = append(path, dirs[0].path[i])
			}
		}
		if len(path) > 0 {
			path = append(path, path[0]) // close the loop
		}
		path = round6(bl.SimplifyPath(path, 2))

		name := dirs[0].rel.Tags["from"] + " - " + dirs[0].rel.Tags["to"]
		if strings.TrimSpace(name) == "-" {
			name = dirs[0].rel.Tags["name"]
		}
		ro := routeOut{Code: ln.ref, Name: name, Buses: ln.buses, Path: path}
		for _, d := range dirs {
			ro.OSMRoutes = append(ro.OSMRoutes, d.rel.ID)
		}
		for _, s := range stops { // a terminal shared by both directions appears once
			idx := addStation(s)
			if n := len(ro.Stops); n > 0 && ro.Stops[n-1] == idx {
				continue
			}
			ro.Stops = append(ro.Stops, idx)
		}
		if n := len(ro.Stops); n > 1 && ro.Stops[0] == ro.Stops[n-1] {
			ro.Stops = ro.Stops[:n-1]
		}
		ds.Routes = append(ds.Routes, ro)
		log.Printf("line %-3s %-45s %d dir, %3d stops, %4d points, %.1f km loop, %d gap(s)",
			ln.ref, name, len(dirs), len(ro.Stops), len(path), bl.PathLengthM(path)/1000, gaps)
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
