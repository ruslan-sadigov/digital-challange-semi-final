# Agilli Mobillik ve Neqliyyat

Real-time bus tracking and crowd management for Baku, with two map panels on OpenStreetMap:

- **Passenger panel** (`/`): search buses, see live positions, routes on real roads, ETA to any stop and live density; report density; share location; **test location** (click the map to simulate where you are and see the nearest stop, walk time and bus ETA).
- **Sign in** (`/login`), **Register** (`/register`) and **Operator sign in** (`/operator/login`) are separate tabs with separate forms; passenger and operator accounts can only use their own sign-in.
- **Operator panel** (`/operator`): live heatmap of app users, bus / route / station management, density anomaly alerts, reroute suggestions with manual approval, browser notifications.

Go + PostgreSQL, MVC, N-layered (PL / BL / DL). The UI follows the **Essential** design system: light-gray surfaces, one blue accent, pill controls, 20px cards and soft layered shadows, all as CSS tokens in `internal/pl/view/static/app.css`.

## Quick start

```bash
docker compose up --build
```

Open http://localhost:8080 (passenger, auto guest session) and http://localhost:8080/operator
(operator sign-in `operator@agilli.az` / `operator123`). Compose runs in **demo mode**: a simulator drives
11 buses along real Baku roads on 4 routes with ~85 riders and 80 pedestrians, and route **65** is overloaded, so
within about 3 minutes you see an anomaly, reroute suggestions, and can approve one.

Without Docker for the app (PostgreSQL 16 still required):

```bash
make db      # or point DATABASE_URL at your own PostgreSQL
make demo    # fast timings; `make run` uses the real 10-minute rule
```

## Real map data

The seed network is built from OpenStreetMap: **107 real bus stops** (OSM `highway=bus_stop`, found via
Nominatim) and **road-following route geometry** (via OSRM). It is generated once by `go run ./cmd/seedgen`
and embedded in the binary (`internal/worker/seeddata/baku.json`), so the app runs offline. Each route
anchors on well-known places and includes every real stop the bus passes within 20 m of its road, in
travel order, at least 300 m apart.

Routes created or edited by an operator are routed along roads at save time through OSRM (`OSRM_URL`,
default `https://router.project-osrm.org`, `off` to disable); if OSRM is unreachable the route falls back to
straight lines between stops. Buses in the simulator drive the road path, and ETA is measured along it.

## How it works

| Rule | Implementation |
| --- | --- |
| Bus density | Recency-weighted user reports (last 10 min; guests weigh 0.7) blended 50/50 with occupancy estimated from user locations. Score >= 1.5 High, >= 0.75 Medium, else Low |
| Passenger count from locations | A user counts as on board when their last 2 location samples were both within 30 m of the bus at the same time, and not both at a station. Count x 3 (configurable) capped at capacity. Zero detected users = no information (only reports count) |
| Route density | Average score of the buses currently serving the route (including temporarily rerouted ones), recomputed every check |
| Anomaly | Route High continuously for >= 10 min opens one anomaly (one alert per episode). A dip before 10 min resets the clock; 2 consecutive non-High checks resolve it. The clock is persisted, so restarts do not reset it |
| Reroute suggestion | Up to 3 buses from other routes: Low density first (Medium only if no Low), donor route keeps >= 1 bus. Ranked by distance to the crowded route + donor route density - shared stations |
| Approval | Manual. An approved reroute lasts 60 min (or until an operator ends it). It is **not** ended when the anomaly resolves, because the reroute is usually what resolved it |
| Notifications | WebSocket alert to every open operator panel + browser Notification API |
| Location sampling | Users every 2 min (foreground, opt-in), buses every 1 min via `POST /api/device/buses/{id}/location` with `X-Device-Key` |

## Layout

```
cmd/server/          wiring, config, workers
internal/domain/     entities (MVC model) + domain errors
internal/bl/         business layer: auth, fleet, location, density, ETA, heatmap, anomaly + reroute
internal/dl/         repository interfaces; dl/postgres = pgx implementation
internal/pl/         presentation: router, controllers, middleware, WebSocket hub, views (templates + static JS)
internal/worker/     scheduler, seed data, simulator
migrations/          SQL schema (embedded, applied on start)
test/integration/    end-to-end tests against PostgreSQL
```

## Tests

```bash
make test               # unit tests, no database
make test-integration   # needs PostgreSQL (TEST_DATABASE_URL, database agilli_test)
make cover              # combined coverage (71% of internal/ at the time of writing)
```

Unit tests cover the pure business rules (density scoring, on-board estimation, the 10-minute
detector incl. boundaries, the reroute suggester, ETA geometry, heatmap aggregation, auth tokens),
middleware, error mapping and the WebSocket hub. Integration tests reset the schema and cover
repositories, auth and role enforcement, operator CRUD, passenger features, the location-based
passenger count, WebSocket delivery, and the full anomaly -> suggestion -> approval -> resolve ->
expiry scenario with a fake clock.

## Configuration

| Variable | Default | Notes |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/agilli?sslmode=disable` | |
| `ADDR` | `:8080` | |
| `SESSION_SECRET` | `dev-secret-change-me` | HMAC key for session cookies |
| `OPERATOR_EMAIL` / `OPERATOR_PASSWORD` | `operator@agilli.az` / `operator123` | Created on first start |
| `DEVICE_API_KEY` | `dev-device-key` | For bus GPS devices |
| `OSRM_URL` | `https://router.project-osrm.org` | Road routing for operator-created routes; `off` = straight lines |
| `DEMO` | `false` | Fast timings (2 min anomaly, 10 s checks) |
| `SIMULATE` / `SEED` | `true` | Demo simulator and Baku seed network |
| `ANOMALY_THRESHOLD` | `10m` (`2m` in demo) | |
| `CHECK_INTERVAL` / `HEATMAP_INTERVAL` | `60s` / `30s` | |
| `USER_SAMPLE_INTERVAL` | `2m` | |
| `REPORT_WINDOW`, `OVERRIDE_DURATION` | `10m`, `60m` | |
| `SIM_TICK`, `SIM_CROWDED_ROUTE` | `5s`, `65` | |

## Known limits (competition scope)

- Location is collected only while the web app is open; true background tracking needs a native app.
- ETA is road distance along the route divided by the bus's recent speed (GPS jumps ignored); it does not model traffic or dwell time at stops.
- Moving a station does not re-route the routes that use it until the route is saved again.
- Distances are computed in Go (no PostGIS) and location tables are not partitioned.
- Push is in-app + browser notifications while the operator panel is open (no Web Push service worker).
- Station coordinates in the seed data are approximate.

## Screenshots

![Passenger](docs/screenshots/passenger.png)
![Operator alerts](docs/screenshots/operator-alerts.png)
![Operator routes and heatmap](docs/screenshots/operator-routes.png)
![Register](docs/screenshots/register.png)
