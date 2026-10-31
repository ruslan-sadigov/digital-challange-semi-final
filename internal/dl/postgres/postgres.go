// Package postgres implements the dl repositories on PostgreSQL using pgx.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"agilli/internal/dl"
	"agilli/internal/domain"
)

// NewStore wires every repository on one connection pool.
func NewStore(pool *pgxpool.Pool) *dl.Store {
	return &dl.Store{
		Users:     &UserRepo{pool},
		Stations:  &StationRepo{pool},
		Routes:    &RouteRepo{pool},
		Buses:     &BusRepo{pool},
		Locations: &LocationRepo{pool},
		Reports:   &ReportRepo{pool},
		Anomalies: &AnomalyRepo{pool},
		Overrides: &OverrideRepo{pool},
	}
}

// Connect opens a pool and pings it.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", domain.ErrConflict, pgErr.Detail)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%w: %s", domain.ErrConflict, pgErr.Detail)
		}
	}
	return err
}

func affected(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ---------- users ----------

type UserRepo struct{ pool *pgxpool.Pool }

func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO app_user(email, password_hash, role, created_at) VALUES ($1,$2,$3,$4) RETURNING id`,
		u.Email, u.PasswordHash, string(u.Role), u.CreatedAt).Scan(&u.ID)
	return mapErr(err)
}

const userCols = `id, email, password_hash, role, created_at`

func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	var role string
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &role, &u.CreatedAt); err != nil {
		return nil, mapErr(err)
	}
	u.Role = domain.Role(role)
	return &u, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	return scanUser(r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM app_user WHERE id=$1`, id))
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return scanUser(r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM app_user WHERE lower(email)=lower($1)`, email))
}

// ---------- stations ----------

type StationRepo struct{ pool *pgxpool.Pool }

func (r *StationRepo) List(ctx context.Context) ([]domain.Station, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, lat, lng FROM station ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Station, error) {
		var s domain.Station
		err := row.Scan(&s.ID, &s.Name, &s.Lat, &s.Lng)
		return s, err
	})
}

func (r *StationRepo) Get(ctx context.Context, id int64) (*domain.Station, error) {
	var s domain.Station
	err := r.pool.QueryRow(ctx, `SELECT id, name, lat, lng FROM station WHERE id=$1`, id).Scan(&s.ID, &s.Name, &s.Lat, &s.Lng)
	if err != nil {
		return nil, mapErr(err)
	}
	return &s, nil
}

func (r *StationRepo) Create(ctx context.Context, s *domain.Station) error {
	return mapErr(r.pool.QueryRow(ctx, `INSERT INTO station(name, lat, lng) VALUES ($1,$2,$3) RETURNING id`, s.Name, s.Lat, s.Lng).Scan(&s.ID))
}

func (r *StationRepo) Update(ctx context.Context, s *domain.Station) error {
	return affected(r.pool.Exec(ctx, `UPDATE station SET name=$2, lat=$3, lng=$4 WHERE id=$1`, s.ID, s.Name, s.Lat, s.Lng))
}

func (r *StationRepo) Delete(ctx context.Context, id int64) error {
	return affected(r.pool.Exec(ctx, `DELETE FROM station WHERE id=$1`, id))
}

// ---------- routes ----------

type RouteRepo struct{ pool *pgxpool.Pool }

func (r *RouteRepo) List(ctx context.Context) ([]domain.Route, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, code, name, path FROM route ORDER BY code`)
	if err != nil {
		return nil, err
	}
	routes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Route, error) {
		var rt domain.Route
		err := row.Scan(&rt.ID, &rt.Code, &rt.Name, &rt.Path)
		rt.Stops = []domain.Station{}
		return rt, err
	})
	if err != nil {
		return nil, err
	}
	stops, err := r.pool.Query(ctx, `SELECT rs.route_id, s.id, s.name, s.lat, s.lng FROM route_stop rs JOIN station s ON s.id=rs.station_id ORDER BY rs.route_id, rs.seq`)
	if err != nil {
		return nil, err
	}
	defer stops.Close()
	idx := map[int64]int{}
	for i, rt := range routes {
		idx[rt.ID] = i
	}
	for stops.Next() {
		var routeID int64
		var s domain.Station
		if err := stops.Scan(&routeID, &s.ID, &s.Name, &s.Lat, &s.Lng); err != nil {
			return nil, err
		}
		if i, ok := idx[routeID]; ok {
			routes[i].Stops = append(routes[i].Stops, s)
		}
	}
	return routes, stops.Err()
}

func (r *RouteRepo) Get(ctx context.Context, id int64) (*domain.Route, error) {
	var rt domain.Route
	if err := r.pool.QueryRow(ctx, `SELECT id, code, name, path FROM route WHERE id=$1`, id).Scan(&rt.ID, &rt.Code, &rt.Name, &rt.Path); err != nil {
		return nil, mapErr(err)
	}
	rows, err := r.pool.Query(ctx, `SELECT s.id, s.name, s.lat, s.lng FROM route_stop rs JOIN station s ON s.id=rs.station_id WHERE rs.route_id=$1 ORDER BY rs.seq`, id)
	if err != nil {
		return nil, err
	}
	rt.Stops, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Station, error) {
		var s domain.Station
		err := row.Scan(&s.ID, &s.Name, &s.Lat, &s.Lng)
		return s, err
	})
	if rt.Stops == nil {
		rt.Stops = []domain.Station{}
	}
	return &rt, err
}

// pathArg stores an empty path as NULL.
func pathArg(p [][2]float64) any {
	if len(p) == 0 {
		return nil
	}
	return p
}

func writeStops(ctx context.Context, tx pgx.Tx, routeID int64, stationIDs []int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM route_stop WHERE route_id=$1`, routeID); err != nil {
		return err
	}
	for i, sid := range stationIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO route_stop(route_id, seq, station_id) VALUES ($1,$2,$3)`, routeID, i, sid); err != nil {
			return mapErr(err)
		}
	}
	return nil
}

func (r *RouteRepo) Create(ctx context.Context, rt *domain.Route, stationIDs []int64) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO route(code, name, path) VALUES ($1,$2,$3) RETURNING id`, rt.Code, rt.Name, pathArg(rt.Path)).Scan(&rt.ID); err != nil {
			return mapErr(err)
		}
		return writeStops(ctx, tx, rt.ID, stationIDs)
	})
}

func (r *RouteRepo) Update(ctx context.Context, rt *domain.Route, stationIDs []int64) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := affected(tx.Exec(ctx, `UPDATE route SET code=$2, name=$3, path=$4 WHERE id=$1`, rt.ID, rt.Code, rt.Name, pathArg(rt.Path))); err != nil {
			return err
		}
		return writeStops(ctx, tx, rt.ID, stationIDs)
	})
}

func (r *RouteRepo) Delete(ctx context.Context, id int64) error {
	return affected(r.pool.Exec(ctx, `DELETE FROM route WHERE id=$1`, id))
}

// ---------- buses ----------

type BusRepo struct{ pool *pgxpool.Pool }

const busCols = `b.id, b.name, b.route_id, b.active_route_id, b.passenger_capacity, b.current_passenger_count,
	b.last_lat, b.last_lng, b.last_seen_at, b.density, b.density_score, b.density_updated_at`

func scanBus(row pgx.Row) (domain.Bus, error) {
	var b domain.Bus
	var d string
	err := row.Scan(&b.ID, &b.Name, &b.RouteID, &b.ActiveRouteID, &b.Capacity, &b.PassengerCount,
		&b.LastLat, &b.LastLng, &b.LastSeenAt, &d, &b.DensityScore, &b.DensityUpdatedAt)
	b.Density = domain.Density(d)
	return b, err
}

func (r *BusRepo) collect(ctx context.Context, sql string, args ...any) ([]domain.Bus, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Bus, error) { return scanBus(row) })
}

func (r *BusRepo) List(ctx context.Context) ([]domain.Bus, error) {
	return r.collect(ctx, `SELECT `+busCols+` FROM bus b ORDER BY b.id`)
}

func (r *BusRepo) Get(ctx context.Context, id int64) (*domain.Bus, error) {
	b, err := scanBus(r.pool.QueryRow(ctx, `SELECT `+busCols+` FROM bus b WHERE b.id=$1`, id))
	if err != nil {
		return nil, mapErr(err)
	}
	return &b, nil
}

func (r *BusRepo) Search(ctx context.Context, q string) ([]domain.Bus, error) {
	pattern := "%" + strings.NewReplacer(`%`, `\%`, `_`, `\_`).Replace(q) + "%"
	return r.collect(ctx, `SELECT `+busCols+` FROM bus b
		LEFT JOIN route r ON r.id = COALESCE(b.active_route_id, b.route_id)
		WHERE b.name ILIKE $1 OR r.code ILIKE $1 OR r.name ILIKE $1
		ORDER BY b.id`, pattern)
}

func (r *BusRepo) Create(ctx context.Context, b *domain.Bus) error {
	if b.Density == "" {
		b.Density = domain.DensityUnknown
	}
	return mapErr(r.pool.QueryRow(ctx,
		`INSERT INTO bus(name, route_id, passenger_capacity, current_passenger_count, last_lat, last_lng)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		b.Name, b.RouteID, b.Capacity, b.PassengerCount, b.LastLat, b.LastLng).Scan(&b.ID))
}

func (r *BusRepo) Update(ctx context.Context, b *domain.Bus) error {
	return affected(r.pool.Exec(ctx, `UPDATE bus SET name=$2, route_id=$3, passenger_capacity=$4 WHERE id=$1`,
		b.ID, b.Name, b.RouteID, b.Capacity))
}

func (r *BusRepo) Delete(ctx context.Context, id int64) error {
	return affected(r.pool.Exec(ctx, `DELETE FROM bus WHERE id=$1`, id))
}

func (r *BusRepo) UpdatePosition(ctx context.Context, id int64, lat, lng float64, at time.Time) error {
	return affected(r.pool.Exec(ctx, `UPDATE bus SET last_lat=$2, last_lng=$3, last_seen_at=$4 WHERE id=$1`, id, lat, lng, at))
}

func (r *BusRepo) UpdateDensity(ctx context.Context, id int64, d domain.Density, score *float64, count int, at time.Time) error {
	return affected(r.pool.Exec(ctx,
		`UPDATE bus SET density=$2, density_score=$3, current_passenger_count=$4, density_updated_at=$5 WHERE id=$1`,
		id, string(d), score, count, at))
}

func (r *BusRepo) SetActiveRoute(ctx context.Context, id int64, routeID *int64) error {
	return affected(r.pool.Exec(ctx, `UPDATE bus SET active_route_id=$2 WHERE id=$1`, id, routeID))
}

// ---------- locations ----------

type LocationRepo struct{ pool *pgxpool.Pool }

func (r *LocationRepo) AddBusLocation(ctx context.Context, l domain.BusLocation) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO bus_location(bus_id, lat, lng, created_at) VALUES ($1,$2,$3,$4)`, l.BusID, l.Lat, l.Lng, l.CreatedAt)
	return mapErr(err)
}

func (r *LocationRepo) AddUserLocation(ctx context.Context, l domain.UserLocation) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO user_location(user_id, session_id, lat, lng, created_at) VALUES ($1,$2,$3,$4,$5)`,
		l.UserID, l.SessionID, l.Lat, l.Lng, l.CreatedAt)
	return mapErr(err)
}

func (r *LocationRepo) BusLocationsSince(ctx context.Context, since time.Time) (map[int64][]domain.BusLocation, error) {
	rows, err := r.pool.Query(ctx, `SELECT bus_id, lat, lng, created_at FROM bus_location WHERE created_at > $1 ORDER BY bus_id, created_at`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]domain.BusLocation{}
	for rows.Next() {
		var l domain.BusLocation
		if err := rows.Scan(&l.BusID, &l.Lat, &l.Lng, &l.CreatedAt); err != nil {
			return nil, err
		}
		out[l.BusID] = append(out[l.BusID], l)
	}
	return out, rows.Err()
}

func (r *LocationRepo) BusTrack(ctx context.Context, busID int64, since time.Time) ([]domain.BusLocation, error) {
	rows, err := r.pool.Query(ctx, `SELECT bus_id, lat, lng, created_at FROM bus_location WHERE bus_id=$1 AND created_at > $2 ORDER BY created_at`, busID, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.BusLocation, error) {
		var l domain.BusLocation
		err := row.Scan(&l.BusID, &l.Lat, &l.Lng, &l.CreatedAt)
		return l, err
	})
}

func scanUserLoc(rows pgx.Rows) (domain.UserLocation, error) {
	var l domain.UserLocation
	err := rows.Scan(&l.UserID, &l.SessionID, &l.Lat, &l.Lng, &l.CreatedAt)
	return l, err
}

func (r *LocationRepo) UserLocationsSince(ctx context.Context, since time.Time) (map[string][]domain.UserLocation, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id, session_id, lat, lng, created_at FROM user_location WHERE created_at > $1 ORDER BY session_id, created_at`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]domain.UserLocation{}
	for rows.Next() {
		l, err := scanUserLoc(rows)
		if err != nil {
			return nil, err
		}
		out[l.SessionID] = append(out[l.SessionID], l)
	}
	return out, rows.Err()
}

func (r *LocationRepo) LatestPerSessionSince(ctx context.Context, since time.Time) ([]domain.UserLocation, error) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT ON (session_id) user_id, session_id, lat, lng, created_at
		FROM user_location WHERE created_at > $1 ORDER BY session_id, created_at DESC`, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.UserLocation, error) {
		var l domain.UserLocation
		err := row.Scan(&l.UserID, &l.SessionID, &l.Lat, &l.Lng, &l.CreatedAt)
		return l, err
	})
}

func (r *LocationRepo) LatestForUserSince(ctx context.Context, userID int64, since time.Time) (*domain.UserLocation, error) {
	var l domain.UserLocation
	err := r.pool.QueryRow(ctx, `SELECT user_id, session_id, lat, lng, created_at FROM user_location
		WHERE user_id=$1 AND created_at > $2 ORDER BY created_at DESC LIMIT 1`, userID, since).
		Scan(&l.UserID, &l.SessionID, &l.Lat, &l.Lng, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// ---------- density reports ----------

type ReportRepo struct{ pool *pgxpool.Pool }

func (r *ReportRepo) Add(ctx context.Context, rep domain.DensityReport) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO bus_status(user_id, bus_id, density, weight, created_at) VALUES ($1,$2,$3,$4,$5)`,
		rep.UserID, rep.BusID, string(rep.Density), rep.Weight, rep.CreatedAt)
	return mapErr(err)
}

func (r *ReportRepo) LastReportAt(ctx context.Context, userID, busID int64) (*time.Time, error) {
	var t *time.Time
	err := r.pool.QueryRow(ctx, `SELECT max(created_at) FROM bus_status WHERE user_id=$1 AND bus_id=$2`, userID, busID).Scan(&t)
	return t, err
}

func (r *ReportRepo) Since(ctx context.Context, since time.Time) (map[int64][]domain.DensityReport, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id, bus_id, density, weight, created_at FROM bus_status WHERE created_at > $1 ORDER BY created_at`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]domain.DensityReport{}
	for rows.Next() {
		var rep domain.DensityReport
		var d string
		if err := rows.Scan(&rep.UserID, &rep.BusID, &d, &rep.Weight, &rep.CreatedAt); err != nil {
			return nil, err
		}
		rep.Density = domain.Density(d)
		out[rep.BusID] = append(out[rep.BusID], rep)
	}
	return out, rows.Err()
}

// ---------- anomalies + route density state ----------

type AnomalyRepo struct{ pool *pgxpool.Pool }

func (r *AnomalyRepo) ListStates(ctx context.Context) ([]domain.RouteDensityState, error) {
	rows, err := r.pool.Query(ctx, `SELECT route_id, level, score, high_since, below_high_checks, updated_at FROM route_density_state ORDER BY route_id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.RouteDensityState, error) {
		var s domain.RouteDensityState
		var lvl string
		err := row.Scan(&s.RouteID, &lvl, &s.Score, &s.HighSince, &s.BelowHighChecks, &s.UpdatedAt)
		s.Level = domain.Density(lvl)
		return s, err
	})
}

func (r *AnomalyRepo) SaveState(ctx context.Context, s domain.RouteDensityState) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO route_density_state(route_id, level, score, high_since, below_high_checks, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (route_id) DO UPDATE SET level=EXCLUDED.level, score=EXCLUDED.score, high_since=EXCLUDED.high_since,
			below_high_checks=EXCLUDED.below_high_checks, updated_at=EXCLUDED.updated_at`,
		s.RouteID, string(s.Level), s.Score, s.HighSince, s.BelowHighChecks, s.UpdatedAt)
	return mapErr(err)
}

const anomalyCols = `id, route_id, started_at, detected_at, resolved_at, status`

func scanAnomaly(row pgx.Row) (domain.Anomaly, error) {
	var a domain.Anomaly
	err := row.Scan(&a.ID, &a.RouteID, &a.StartedAt, &a.DetectedAt, &a.ResolvedAt, &a.Status)
	return a, err
}

func (r *AnomalyRepo) OpenForRoute(ctx context.Context, routeID int64) (*domain.Anomaly, error) {
	a, err := scanAnomaly(r.pool.QueryRow(ctx, `SELECT `+anomalyCols+` FROM density_anomaly WHERE route_id=$1 AND status='open'`, routeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AnomalyRepo) Create(ctx context.Context, a *domain.Anomaly) error {
	if a.Status == "" {
		a.Status = domain.AnomalyOpen
	}
	return mapErr(r.pool.QueryRow(ctx, `INSERT INTO density_anomaly(route_id, started_at, detected_at, status) VALUES ($1,$2,$3,$4) RETURNING id`,
		a.RouteID, a.StartedAt, a.DetectedAt, a.Status).Scan(&a.ID))
}

func (r *AnomalyRepo) Resolve(ctx context.Context, id int64, at time.Time) error {
	return affected(r.pool.Exec(ctx, `UPDATE density_anomaly SET status='resolved', resolved_at=$2 WHERE id=$1 AND status='open'`, id, at))
}

func (r *AnomalyRepo) Get(ctx context.Context, id int64) (*domain.Anomaly, error) {
	a, err := scanAnomaly(r.pool.QueryRow(ctx, `SELECT `+anomalyCols+` FROM density_anomaly WHERE id=$1`, id))
	if err != nil {
		return nil, mapErr(err)
	}
	return &a, nil
}

func (r *AnomalyRepo) List(ctx context.Context, limit int) ([]domain.Anomaly, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+anomalyCols+` FROM density_anomaly ORDER BY (status='open') DESC, detected_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Anomaly, error) { return scanAnomaly(row) })
}

// ---------- overrides ----------

type OverrideRepo struct{ pool *pgxpool.Pool }

const overrideCols = `id, anomaly_id, bus_id, from_route_id, to_route_id, status, score, distance_km, starts_at, ends_at, created_at, decided_by`

func scanOverride(row pgx.Row) (domain.RouteOverride, error) {
	var o domain.RouteOverride
	err := row.Scan(&o.ID, &o.AnomalyID, &o.BusID, &o.FromRouteID, &o.ToRouteID, &o.Status, &o.Score, &o.DistanceKm,
		&o.StartsAt, &o.EndsAt, &o.CreatedAt, &o.DecidedBy)
	return o, err
}

func (r *OverrideRepo) Create(ctx context.Context, o *domain.RouteOverride) error {
	return mapErr(r.pool.QueryRow(ctx, `INSERT INTO route_override(anomaly_id, bus_id, from_route_id, to_route_id, status, score, distance_km, starts_at, ends_at, created_at, decided_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		o.AnomalyID, o.BusID, o.FromRouteID, o.ToRouteID, o.Status, o.Score, o.DistanceKm, o.StartsAt, o.EndsAt, o.CreatedAt, o.DecidedBy).Scan(&o.ID))
}

func (r *OverrideRepo) Get(ctx context.Context, id int64) (*domain.RouteOverride, error) {
	o, err := scanOverride(r.pool.QueryRow(ctx, `SELECT `+overrideCols+` FROM route_override WHERE id=$1`, id))
	if err != nil {
		return nil, mapErr(err)
	}
	return &o, nil
}

func (r *OverrideRepo) list(ctx context.Context, sql string, args ...any) ([]domain.RouteOverride, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.RouteOverride, error) { return scanOverride(row) })
}

func (r *OverrideRepo) ListByStatus(ctx context.Context, statuses ...string) ([]domain.RouteOverride, error) {
	return r.list(ctx, `SELECT `+overrideCols+` FROM route_override WHERE status = ANY($1) ORDER BY created_at DESC, id DESC`, statuses)
}

func (r *OverrideRepo) ListByAnomaly(ctx context.Context, anomalyID int64) ([]domain.RouteOverride, error) {
	return r.list(ctx, `SELECT `+overrideCols+` FROM route_override WHERE anomaly_id=$1 ORDER BY score, id`, anomalyID)
}

func (r *OverrideRepo) Update(ctx context.Context, o *domain.RouteOverride) error {
	return affected(r.pool.Exec(ctx, `UPDATE route_override SET status=$2, starts_at=$3, ends_at=$4, decided_by=$5 WHERE id=$1`,
		o.ID, o.Status, o.StartsAt, o.EndsAt, o.DecidedBy))
}
