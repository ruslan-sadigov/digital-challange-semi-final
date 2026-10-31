-- Key/value metadata, e.g. which version of the embedded seed network is loaded.
CREATE TABLE app_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
