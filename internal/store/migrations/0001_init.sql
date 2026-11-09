-- 0001: app metadata. Domain tables arrive in 0002 (plan item 8).
CREATE TABLE app_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT;

INSERT INTO app_meta(key, value) VALUES ('created_by', 'LeGaJ 2');
