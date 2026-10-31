-- Smart City Transportation: initial schema

CREATE TYPE user_role AS ENUM ('guest', 'passenger', 'operator');
CREATE TYPE density_level AS ENUM ('unknown', 'low', 'medium', 'high');

CREATE TABLE app_user (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT UNIQUE,              -- NULL for guests
    password_hash TEXT NOT NULL DEFAULT '', -- bcrypt, empty for guests
    role          user_role NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE station (
    id   BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    lat  DOUBLE PRECISION NOT NULL,
    lng  DOUBLE PRECISION NOT NULL
);

CREATE TABLE route (
    id   BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);

-- Ordered stops of a route. Routes are circular: after the last stop the bus returns to the first.
CREATE TABLE route_stop (
    route_id   BIGINT NOT NULL REFERENCES route(id) ON DELETE CASCADE,
    seq        INT    NOT NULL,
    station_id BIGINT NOT NULL REFERENCES station(id) ON DELETE RESTRICT,
    PRIMARY KEY (route_id, seq)
);

CREATE TABLE bus (
    id                      BIGSERIAL PRIMARY KEY,
    name                    TEXT NOT NULL,
    route_id                BIGINT REFERENCES route(id) ON DELETE SET NULL,  -- home route
    active_route_id         BIGINT REFERENCES route(id) ON DELETE SET NULL,  -- temporary route (approved override)
    passenger_capacity      INT NOT NULL CHECK (passenger_capacity > 0),
    current_passenger_count INT NOT NULL DEFAULT 0,
    last_lat                DOUBLE PRECISION,
    last_lng                DOUBLE PRECISION,
    last_seen_at            TIMESTAMPTZ,
    density                 density_level NOT NULL DEFAULT 'unknown',
    density_score           DOUBLE PRECISION,
    density_updated_at      TIMESTAMPTZ
);

CREATE TABLE bus_location (
    id         BIGSERIAL PRIMARY KEY,
    bus_id     BIGINT NOT NULL REFERENCES bus(id) ON DELETE CASCADE,
    lat        DOUBLE PRECISION NOT NULL,
    lng        DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX bus_location_bus_time ON bus_location (bus_id, created_at DESC);
CREATE INDEX bus_location_time ON bus_location (created_at DESC);

CREATE TABLE user_location (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES app_user(id) ON DELETE SET NULL,
    session_id TEXT NOT NULL,
    lat        DOUBLE PRECISION NOT NULL,
    lng        DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX user_location_time ON user_location (created_at DESC);
CREATE INDEX user_location_session_time ON user_location (session_id, created_at DESC);

-- Density reports given by users (BusStatus in the original design).
CREATE TABLE bus_status (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    bus_id     BIGINT NOT NULL REFERENCES bus(id) ON DELETE CASCADE,
    density    density_level NOT NULL CHECK (density <> 'unknown'),
    weight     DOUBLE PRECISION NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX bus_status_bus_time ON bus_status (bus_id, created_at DESC);
CREATE INDEX bus_status_user_bus_time ON bus_status (user_id, bus_id, created_at DESC);

-- Live density per route + the clock for the 10-minute rule (survives restarts).
CREATE TABLE route_density_state (
    route_id          BIGINT PRIMARY KEY REFERENCES route(id) ON DELETE CASCADE,
    level             density_level NOT NULL DEFAULT 'unknown',
    score             DOUBLE PRECISION,
    high_since        TIMESTAMPTZ,
    below_high_checks INT NOT NULL DEFAULT 0,
    updated_at        TIMESTAMPTZ NOT NULL
);

CREATE TABLE density_anomaly (
    id          BIGSERIAL PRIMARY KEY,
    route_id    BIGINT NOT NULL REFERENCES route(id) ON DELETE CASCADE,
    started_at  TIMESTAMPTZ NOT NULL,  -- first moment the route was High
    detected_at TIMESTAMPTZ NOT NULL,  -- when the threshold was crossed
    resolved_at TIMESTAMPTZ,
    status      TEXT NOT NULL CHECK (status IN ('open', 'resolved'))
);
-- At most one open anomaly per route.
CREATE UNIQUE INDEX density_anomaly_one_open ON density_anomaly (route_id) WHERE status = 'open';

-- Temporary reroute: suggested by the system, approved manually by an operator.
CREATE TABLE route_override (
    id            BIGSERIAL PRIMARY KEY,
    anomaly_id    BIGINT REFERENCES density_anomaly(id) ON DELETE CASCADE,
    bus_id        BIGINT NOT NULL REFERENCES bus(id) ON DELETE CASCADE,
    from_route_id BIGINT NOT NULL REFERENCES route(id) ON DELETE CASCADE,
    to_route_id   BIGINT NOT NULL REFERENCES route(id) ON DELETE CASCADE,
    status        TEXT NOT NULL CHECK (status IN ('suggested', 'active', 'rejected', 'ended', 'expired')),
    score         DOUBLE PRECISION NOT NULL DEFAULT 0,
    distance_km   DOUBLE PRECISION NOT NULL DEFAULT 0,
    starts_at     TIMESTAMPTZ,
    ends_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL,
    decided_by    BIGINT REFERENCES app_user(id) ON DELETE SET NULL
);
CREATE INDEX route_override_status ON route_override (status);
