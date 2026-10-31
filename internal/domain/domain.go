// Package domain holds the entities shared by all layers (the MVC "model").
package domain

import (
	"errors"
	"time"
)

type Role string

const (
	RoleGuest     Role = "guest"
	RolePassenger Role = "passenger"
	RoleOperator  Role = "operator"
)

type Density string

const (
	DensityUnknown Density = "unknown"
	DensityLow     Density = "low"
	DensityMedium  Density = "medium"
	DensityHigh    Density = "high"
)

// ParseDensity accepts low/medium/high (case-sensitive, lower case).
func ParseDensity(s string) (Density, error) {
	switch Density(s) {
	case DensityLow, DensityMedium, DensityHigh:
		return Density(s), nil
	}
	return DensityUnknown, ErrInvalidInput("density must be low, medium or high")
}

// Value maps a density to its numeric value: low=0, medium=1, high=2.
func (d Density) Value() (float64, bool) {
	switch d {
	case DensityLow:
		return 0, true
	case DensityMedium:
		return 1, true
	case DensityHigh:
		return 2, true
	}
	return 0, false
}

type User struct {
	ID           int64     `json:"id"`
	Email        *string   `json:"email,omitempty"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Station struct {
	ID   int64   `json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

type Route struct {
	ID    int64     `json:"id"`
	Code  string    `json:"code"`
	Name  string    `json:"name"`
	Stops []Station `json:"stops"`
}

type Bus struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	RouteID          *int64     `json:"routeId"`
	ActiveRouteID    *int64     `json:"activeRouteId"`
	Capacity         int        `json:"passengerCapacity"`
	PassengerCount   int        `json:"currentPassengerCount"`
	LastLat          *float64   `json:"lastLat"`
	LastLng          *float64   `json:"lastLng"`
	LastSeenAt       *time.Time `json:"lastSeenAt"`
	Density          Density    `json:"density"`
	DensityScore     *float64   `json:"densityScore"`
	DensityUpdatedAt *time.Time `json:"densityUpdatedAt"`
}

// EffectiveRouteID is the route the bus currently serves: the temporary one if an override is active.
func (b Bus) EffectiveRouteID() *int64 {
	if b.ActiveRouteID != nil {
		return b.ActiveRouteID
	}
	return b.RouteID
}

func (b Bus) HasPosition() bool { return b.LastLat != nil && b.LastLng != nil }

type BusLocation struct {
	BusID     int64     `json:"busId"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	CreatedAt time.Time `json:"createdAt"`
}

type UserLocation struct {
	UserID    *int64    `json:"userId"`
	SessionID string    `json:"sessionId"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	CreatedAt time.Time `json:"createdAt"`
}

// DensityReport is a row of BusStatus: a user's report of how crowded a bus is.
type DensityReport struct {
	UserID    int64     `json:"userId"`
	BusID     int64     `json:"busId"`
	Density   Density   `json:"density"`
	Weight    float64   `json:"weight"`
	CreatedAt time.Time `json:"createdAt"`
}

type RouteDensityState struct {
	RouteID         int64      `json:"routeId"`
	Level           Density    `json:"level"`
	Score           *float64   `json:"score"`
	HighSince       *time.Time `json:"highSince"`
	BelowHighChecks int        `json:"belowHighChecks"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

const (
	AnomalyOpen     = "open"
	AnomalyResolved = "resolved"
)

type Anomaly struct {
	ID         int64      `json:"id"`
	RouteID    int64      `json:"routeId"`
	StartedAt  time.Time  `json:"startedAt"`
	DetectedAt time.Time  `json:"detectedAt"`
	ResolvedAt *time.Time `json:"resolvedAt"`
	Status     string     `json:"status"`
}

const (
	OverrideSuggested = "suggested"
	OverrideActive    = "active"
	OverrideRejected  = "rejected"
	OverrideEnded     = "ended"
	OverrideExpired   = "expired"
)

type RouteOverride struct {
	ID          int64      `json:"id"`
	AnomalyID   *int64     `json:"anomalyId"`
	BusID       int64      `json:"busId"`
	FromRouteID int64      `json:"fromRouteId"`
	ToRouteID   int64      `json:"toRouteId"`
	Status      string     `json:"status"`
	Score       float64    `json:"score"`
	DistanceKm  float64    `json:"distanceKm"`
	StartsAt    *time.Time `json:"startsAt"`
	EndsAt      *time.Time `json:"endsAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	DecidedBy   *int64     `json:"decidedBy"`
}

// Errors shared across layers. The PL maps them to HTTP status codes.
var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrRateLimited  = errors.New("rate limited")
)

// ValidationError is returned for bad user input.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func ErrInvalidInput(msg string) error { return &ValidationError{Msg: msg} }
