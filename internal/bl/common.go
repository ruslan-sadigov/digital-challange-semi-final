// Package bl is the business layer: services that implement the rules of the system.
// Services depend on dl repository interfaces and on the small ports below, never on HTTP or SQL.
package bl

import (
	"context"
	"math"
	"sync"
	"time"

	"smartcity/internal/domain"
)

// ---------- clock ----------

// Clock lets time-based rules (10-minute anomaly, rate limits) be tested without sleeping.
type Clock interface{ Now() time.Time }

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// FakeClock is a manually advanced clock for tests.
type FakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func NewFakeClock(t time.Time) *FakeClock { return &FakeClock{t: t} }

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

// ---------- ports ----------

// Publisher pushes real-time messages to subscribers of a channel (implemented by the PL WebSocket hub).
type Publisher interface {
	Publish(channel, msgType string, data any)
}

// Alert is an operator notification about a density anomaly.
type Alert struct {
	Type      string  `json:"type"` // anomaly_opened | anomaly_resolved
	AnomalyID int64   `json:"anomalyId"`
	RouteID   int64   `json:"routeId"`
	RouteCode string  `json:"routeCode"`
	Message   string  `json:"message"`
	HighForS  float64 `json:"highForSeconds"`
	// Suggestions is the number of reroute suggestions created (anomaly_opened only).
	// Clients render the message in the user's language from these fields; Message is the English fallback.
	Suggestions int `json:"suggestions"`
}

// Notifier delivers anomaly alerts to operators (push, in-app, ...).
type Notifier interface {
	Notify(ctx context.Context, a Alert) error
}

type NopPublisher struct{}

func (NopPublisher) Publish(string, string, any) {}

type NopNotifier struct{}

func (NopNotifier) Notify(context.Context, Alert) error { return nil }

// WebSocket channel names.
const (
	ChannelBuses          = "buses"
	ChannelOperatorHeat   = "operator:heatmap"
	ChannelOperatorAlerts = "operator:alerts"
)

// ---------- settings ----------

// Settings holds every tunable business rule. Defaults follow the design doc.
type Settings struct {
	ReportWindow       time.Duration // density reports older than this are ignored
	ReportCooldown     time.Duration // one report per user per bus per cooldown
	ReportMaxDistanceM float64       // reporter must be this close to the bus (when we know both positions)
	GuestReportWeight  float64

	OnboardRadiusM      float64       // user within this distance of the bus counts as on board...
	StationRadiusM      float64       // ...unless both samples are this close to a station (waiting people)
	UserSampleInterval  time.Duration // how often clients send their location
	BusMatchTolerance   time.Duration // max time gap between a user sample and the bus point it is compared with
	PassengerMultiplier float64       // app users seen on board x multiplier = estimated passengers
	PositionFreshness   time.Duration // a bus position older than this is stale

	AnomalyThreshold time.Duration // route High continuously this long => anomaly
	ResolveChecks    int           // consecutive non-High checks to resolve an anomaly
	OverrideDuration time.Duration // interval pre-selected in the operator's approve dialog (0 = until ended manually)
	MaxSuggestions   int
	MinBusesLeft     int // a donor route must keep at least this many buses

	HeatmapWindow  time.Duration
	HeatmapCellDeg float64

	FallbackSpeedKmh float64
}

func DefaultSettings() Settings {
	return Settings{
		ReportWindow:        10 * time.Minute,
		ReportCooldown:      5 * time.Minute,
		ReportMaxDistanceM:  300,
		GuestReportWeight:   0.7,
		OnboardRadiusM:      30,
		StationRadiusM:      40,
		UserSampleInterval:  2 * time.Minute,
		BusMatchTolerance:   90 * time.Second,
		PassengerMultiplier: 3,
		PositionFreshness:   5 * time.Minute,
		AnomalyThreshold:    10 * time.Minute,
		ResolveChecks:       2,
		OverrideDuration:    0,
		MaxSuggestions:      3,
		MinBusesLeft:        1,
		HeatmapWindow:       5 * time.Minute,
		HeatmapCellDeg:      0.001, // about 110 m north-south
		FallbackSpeedKmh:    18,
	}
}

// ---------- geo ----------

const earthRadiusM = 6371000.0

func rad(d float64) float64 { return d * math.Pi / 180 }

// DistanceM is the great-circle distance between two points in metres.
func DistanceM(lat1, lng1, lat2, lng2 float64) float64 {
	dLat := rad(lat2 - lat1)
	dLng := rad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusM * math.Asin(math.Min(1, math.Sqrt(a)))
}

// DistanceToSegmentM is the distance from point p to segment a-b (local flat projection, fine at city scale).
func DistanceToSegmentM(pLat, pLng, aLat, aLng, bLat, bLng float64) float64 {
	k := math.Cos(rad(pLat))
	ax, ay := rad(aLng)*k*earthRadiusM, rad(aLat)*earthRadiusM
	bx, by := rad(bLng)*k*earthRadiusM, rad(bLat)*earthRadiusM
	px, py := rad(pLng)*k*earthRadiusM, rad(pLat)*earthRadiusM
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	t := 0.0
	if l2 > 0 {
		t = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/l2))
	}
	cx, cy := ax+t*dx, ay+t*dy
	return math.Hypot(px-cx, py-cy)
}

// NearestStationM returns the distance to the closest station (+Inf when there are none).
func NearestStationM(lat, lng float64, stations []domain.Station) float64 {
	best := math.Inf(1)
	for _, s := range stations {
		if d := DistanceM(lat, lng, s.Lat, s.Lng); d < best {
			best = d
		}
	}
	return best
}

func validCoord(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 && !(lat == 0 && lng == 0)
}

func ptr[T any](v T) *T { return &v }

// MinHeadingMoveM: GPS movements shorter than this do not change the bus's heading (noise while standing).
const MinHeadingMoveM = 3

// Bearing is the initial compass bearing from point 1 to point 2, in degrees [0, 360), 0 = north, 90 = east.
func Bearing(lat1, lng1, lat2, lng2 float64) float64 {
	p1, p2 := rad(lat1), rad(lat2)
	dl := rad(lng2 - lng1)
	y := math.Sin(dl) * math.Cos(p2)
	x := math.Cos(p1)*math.Sin(p2) - math.Sin(p1)*math.Cos(p2)*math.Cos(dl)
	b := math.Atan2(y, x) * 180 / math.Pi
	return math.Mod(b+360, 360)
}
