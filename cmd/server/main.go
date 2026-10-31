// Command server runs the Agilli Mobillik ve Neqliyyat web app: HTTP + WebSocket, background workers and the optional simulator.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"agilli/internal/bl"
	"agilli/internal/dl/postgres"
	"agilli/internal/pl"
	"agilli/internal/pl/controller"
	"agilli/internal/pl/ws"
	"agilli/internal/worker"
	"agilli/migrations"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			log.Fatalf("%s: %v", key, err)
		}
		return d
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			log.Fatalf("%s: %v", key, err)
		}
		return b
	}
	return def
}

func main() {
	demo := envBool("DEMO", false)
	cfg := bl.DefaultSettings()
	checkEvery, heatEvery := 60*time.Second, 30*time.Second
	if demo {
		// Faster clocks so the full anomaly -> reroute -> resolve flow shows up in a few minutes.
		cfg.AnomalyThreshold = 2 * time.Minute
		cfg.ReportWindow = 3 * time.Minute
		cfg.ReportCooldown = time.Minute
		cfg.UserSampleInterval = 10 * time.Second
		cfg.BusMatchTolerance = 10 * time.Second
		cfg.PositionFreshness = time.Minute
		cfg.OverrideDuration = 15 * time.Minute
		checkEvery, heatEvery = 10*time.Second, 5*time.Second
	}
	cfg.AnomalyThreshold = envDur("ANOMALY_THRESHOLD", cfg.AnomalyThreshold)
	cfg.ReportWindow = envDur("REPORT_WINDOW", cfg.ReportWindow)
	cfg.UserSampleInterval = envDur("USER_SAMPLE_INTERVAL", cfg.UserSampleInterval)
	cfg.OverrideDuration = envDur("OVERRIDE_DURATION", cfg.OverrideDuration)
	checkEvery = envDur("CHECK_INTERVAL", checkEvery)
	heatEvery = envDur("HEATMAP_INTERVAL", heatEvery)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/agilli?sslmode=disable"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := migrations.Up(ctx, pool); err != nil {
		log.Fatal(err)
	}

	store := postgres.NewStore(pool)
	clock := bl.RealClock{}
	hub := ws.NewHub()

	auth := bl.NewAuthService(store.Users, env("SESSION_SECRET", "dev-secret-change-me"), clock)
	fleet := bl.NewFleetService(store)
	location := bl.NewLocationService(store, clock, hub)
	density := bl.NewDensityService(store, clock, cfg, hub)
	eta := bl.NewETAService(store, clock, cfg)
	heat := bl.NewHeatmapService(store, clock, cfg, hub)
	anomaly := bl.NewAnomalyService(store, clock, cfg, density, hub, hub)

	if err := auth.EnsureOperator(ctx, env("OPERATOR_EMAIL", "operator@agilli.az"), env("OPERATOR_PASSWORD", "operator123")); err != nil {
		log.Fatalf("seed operator: %v", err)
	}
	if envBool("SEED", true) {
		if err := worker.Seed(ctx, store, fleet); err != nil {
			log.Fatalf("seed: %v", err)
		}
	}

	handler, err := pl.NewRouter(&controller.Services{
		Auth: auth, Fleet: fleet, Location: location, Density: density, ETA: eta, Heatmap: heat, Anomaly: anomaly, Hub: hub, Settings: cfg,
	}, env("DEVICE_API_KEY", "dev-device-key"))
	if err != nil {
		log.Fatal(err)
	}

	go worker.RunScheduler(ctx, anomaly, heat, checkEvery, heatEvery)
	if envBool("SIMULATE", true) {
		sim := &worker.Simulator{Store: store, Location: location, Density: density, Settings: cfg,
			Tick: envDur("SIM_TICK", 5*time.Second), CrowdedRoute: env("SIM_CROWDED_ROUTE", "65"), Walkers: 80}
		go sim.Run(ctx)
	}

	srv := &http.Server{Addr: env("ADDR", ":8080"), Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("listening on %s (demo=%v, anomaly threshold %s, check every %s)", srv.Addr, demo, cfg.AnomalyThreshold, checkEvery)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
