// Package dl is the data layer: repository interfaces consumed by the business layer.
// The PostgreSQL implementations live in dl/postgres.
package dl

import (
	"context"
	"time"

	"agilli/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, u *domain.User) error
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
}

type StationRepository interface {
	List(ctx context.Context) ([]domain.Station, error)
	Get(ctx context.Context, id int64) (*domain.Station, error)
	Create(ctx context.Context, s *domain.Station) error
	Update(ctx context.Context, s *domain.Station) error
	Delete(ctx context.Context, id int64) error
}

type RouteRepository interface {
	// List returns all routes with their ordered stops.
	List(ctx context.Context) ([]domain.Route, error)
	Get(ctx context.Context, id int64) (*domain.Route, error)
	// Create inserts the route and its ordered stops (stationIDs in order).
	Create(ctx context.Context, r *domain.Route, stationIDs []int64) error
	Update(ctx context.Context, r *domain.Route, stationIDs []int64) error
	Delete(ctx context.Context, id int64) error
}

type BusRepository interface {
	List(ctx context.Context) ([]domain.Bus, error)
	Get(ctx context.Context, id int64) (*domain.Bus, error)
	// Search matches bus name, route code or route name (case-insensitive).
	Search(ctx context.Context, q string) ([]domain.Bus, error)
	Create(ctx context.Context, b *domain.Bus) error
	Update(ctx context.Context, b *domain.Bus) error
	Delete(ctx context.Context, id int64) error
	UpdatePosition(ctx context.Context, id int64, lat, lng float64, at time.Time) error
	UpdateDensity(ctx context.Context, id int64, d domain.Density, score *float64, passengerCount int, at time.Time) error
	SetActiveRoute(ctx context.Context, id int64, routeID *int64) error
}

type LocationRepository interface {
	AddBusLocation(ctx context.Context, l domain.BusLocation) error
	AddUserLocation(ctx context.Context, l domain.UserLocation) error
	// BusLocationsSince returns bus locations newer than since, grouped by bus, oldest first.
	BusLocationsSince(ctx context.Context, since time.Time) (map[int64][]domain.BusLocation, error)
	// BusTrack returns one bus's locations newer than since, oldest first.
	BusTrack(ctx context.Context, busID int64, since time.Time) ([]domain.BusLocation, error)
	// UserLocationsSince returns user locations newer than since, grouped by session, oldest first.
	UserLocationsSince(ctx context.Context, since time.Time) (map[string][]domain.UserLocation, error)
	// LatestPerSessionSince returns the newest location of every session seen since the given time.
	LatestPerSessionSince(ctx context.Context, since time.Time) ([]domain.UserLocation, error)
	// LatestForUserSince returns the newest location of a user, or nil.
	LatestForUserSince(ctx context.Context, userID int64, since time.Time) (*domain.UserLocation, error)
}

type ReportRepository interface {
	Add(ctx context.Context, r domain.DensityReport) error
	// LastReportAt returns when the user last reported on the bus, or nil.
	LastReportAt(ctx context.Context, userID, busID int64) (*time.Time, error)
	// Since returns reports newer than since, grouped by bus.
	Since(ctx context.Context, since time.Time) (map[int64][]domain.DensityReport, error)
}

type AnomalyRepository interface {
	ListStates(ctx context.Context) ([]domain.RouteDensityState, error)
	SaveState(ctx context.Context, s domain.RouteDensityState) error
	OpenForRoute(ctx context.Context, routeID int64) (*domain.Anomaly, error)
	Create(ctx context.Context, a *domain.Anomaly) error
	Resolve(ctx context.Context, id int64, at time.Time) error
	Get(ctx context.Context, id int64) (*domain.Anomaly, error)
	List(ctx context.Context, limit int) ([]domain.Anomaly, error)
}

type OverrideRepository interface {
	Create(ctx context.Context, o *domain.RouteOverride) error
	Get(ctx context.Context, id int64) (*domain.RouteOverride, error)
	// ListByStatus returns overrides having any of the given statuses, newest first.
	ListByStatus(ctx context.Context, statuses ...string) ([]domain.RouteOverride, error)
	ListByAnomaly(ctx context.Context, anomalyID int64) ([]domain.RouteOverride, error)
	Update(ctx context.Context, o *domain.RouteOverride) error
}

// Store groups all repositories for dependency injection.
type Store struct {
	Users     UserRepository
	Stations  StationRepository
	Routes    RouteRepository
	Buses     BusRepository
	Locations LocationRepository
	Reports   ReportRepository
	Anomalies AnomalyRepository
	Overrides OverrideRepository
}
