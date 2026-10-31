//go:build integration

// Package integration runs the system against a real PostgreSQL database.
//
//	TEST_DATABASE_URL=postgres://postgres@localhost:5432/agilli_test?sslmode=disable go test -tags=integration ./test/integration/...
//
// Each test resets the public schema, applies the migrations and seeds the demo network.
package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"agilli/internal/bl"
	"agilli/internal/dl"
	"agilli/internal/dl/postgres"
	"agilli/internal/pl"
	"agilli/internal/pl/controller"
	"agilli/internal/pl/ws"
	"agilli/internal/worker"
	"agilli/migrations"
)

const deviceKey = "test-device-key"

type recordingNotifier struct {
	mu     sync.Mutex
	alerts []bl.Alert
}

func (r *recordingNotifier) Notify(_ context.Context, a bl.Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.alerts = append(r.alerts, a)
	return nil
}

func (r *recordingNotifier) all() []bl.Alert {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]bl.Alert(nil), r.alerts...)
}

type env struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	store    *dl.Store
	clock    *bl.FakeClock
	cfg      bl.Settings
	hub      *ws.Hub
	notifier *recordingNotifier
	auth     *bl.AuthService
	fleet    *bl.FleetService
	location *bl.LocationService
	density  *bl.DensityService
	anomaly  *bl.AnomalyService
	server   *httptest.Server
}

func setup(t *testing.T) *env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	e := &env{ctx: ctx, pool: pool, store: postgres.NewStore(pool), cfg: bl.DefaultSettings(), hub: ws.NewHub(), notifier: &recordingNotifier{}}
	// Real wall-clock "now" so cookies/tokens behave normally; tests move it forward explicitly.
	e.clock = bl.NewFakeClock(time.Now().UTC().Truncate(time.Second))
	e.auth = bl.NewAuthService(e.store.Users, "test-secret", e.clock)
	e.fleet = bl.NewFleetService(e.store)
	e.location = bl.NewLocationService(e.store, e.clock, e.hub)
	e.density = bl.NewDensityService(e.store, e.clock, e.cfg, e.hub)
	e.anomaly = bl.NewAnomalyService(e.store, e.clock, e.cfg, e.density, e.notifier, e.hub)
	if err := worker.Seed(ctx, e.store, e.fleet); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := e.auth.EnsureOperator(ctx, "op@agilli.az", "operator123"); err != nil {
		t.Fatal(err)
	}
	handler, err := pl.NewRouter(&controller.Services{
		Auth: e.auth, Fleet: e.fleet, Location: e.location, Density: e.density,
		ETA: bl.NewETAService(e.store, e.clock, e.cfg), Heatmap: bl.NewHeatmapService(e.store, e.clock, e.cfg, e.hub),
		Anomaly: e.anomaly, Hub: e.hub, Settings: e.cfg,
	}, deviceKey)
	if err != nil {
		t.Fatal(err)
	}
	e.server = httptest.NewServer(handler)
	t.Cleanup(e.server.Close)
	return e
}

// client is an HTTP client with its own cookie jar (one browser).
type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func (e *env) newClient(t *testing.T) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: e.server.URL, http: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

// do sends a JSON request and decodes the JSON response into out (if non-nil). It returns the status code.
func (c *client) do(method, path string, body any, out any, headers ...string) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: decode %q: %v", method, path, raw, err)
		}
	}
	return res.StatusCode
}

func (c *client) login(email, pw string) {
	c.t.Helper()
	if code := c.do("POST", "/api/auth/login", map[string]string{"email": email, "password": pw}, nil); code != 200 {
		c.t.Fatalf("login %s: %d", email, code)
	}
}

func (c *client) guest() {
	c.t.Helper()
	if code := c.do("POST", "/api/auth/guest", nil, nil); code != 200 {
		c.t.Fatalf("guest: %d", code)
	}
}

func expect(t *testing.T, what string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: status %d, want %d", what, got, want)
	}
}
