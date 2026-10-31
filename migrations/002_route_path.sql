-- Road geometry of a route: JSON array of [lat, lng] points forming a closed loop.
-- NULL means "straight lines between stops" (fallback when no road router is available).
ALTER TABLE route ADD COLUMN path JSONB;
