-- PostgreSQL + PostGIS Static GTFS Schema Contract
-- Shared between Ingestion Service (producer) and Routing / Search Services (consumers)
-- Reference: ADR-003, ADR-004, SAD §6.2

CREATE EXTENSION IF NOT EXISTS postgis;

-- Feed Versions track static ingestion versions per transit agency.
-- Active version pointer flip is atomic (blue-green cutover, SAD §6.2).
CREATE TABLE IF NOT EXISTS feed_versions (
    id VARCHAR(64) PRIMARY KEY,
    agency_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    status VARCHAR(32) NOT NULL DEFAULT 'loading', -- 'loading', 'active', 'archived', 'failed'
    is_active BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_feed_versions_active ON feed_versions (agency_id, is_active);

-- Agencies
CREATE TABLE IF NOT EXISTS gtfs_agencies (
    feed_version_id VARCHAR(64) REFERENCES feed_versions(id) ON DELETE CASCADE,
    agency_id VARCHAR(128) NOT NULL, -- Namespaced: {agency_code}:{id}
    agency_name TEXT NOT NULL,
    agency_url TEXT,
    agency_timezone VARCHAR(64) NOT NULL,
    PRIMARY KEY (feed_version_id, agency_id)
);

-- Stops
CREATE TABLE IF NOT EXISTS gtfs_stops (
    feed_version_id VARCHAR(64) REFERENCES feed_versions(id) ON DELETE CASCADE,
    stop_id VARCHAR(128) NOT NULL, -- Namespaced
    stop_name TEXT NOT NULL,
    stop_lat DOUBLE PRECISION NOT NULL,
    stop_lon DOUBLE PRECISION NOT NULL,
    geom GEOMETRY(Point, 4326),
    PRIMARY KEY (feed_version_id, stop_id)
);

CREATE INDEX IF NOT EXISTS idx_gtfs_stops_geom ON gtfs_stops USING GIST (geom);

-- Routes
CREATE TABLE IF NOT EXISTS gtfs_routes (
    feed_version_id VARCHAR(64) REFERENCES feed_versions(id) ON DELETE CASCADE,
    route_id VARCHAR(128) NOT NULL, -- Namespaced
    agency_id VARCHAR(128) NOT NULL,
    route_short_name TEXT,
    route_long_name TEXT,
    route_type INTEGER NOT NULL,
    PRIMARY KEY (feed_version_id, route_id)
);

-- Service Calendars
CREATE TABLE IF NOT EXISTS gtfs_calendar (
    feed_version_id VARCHAR(64) REFERENCES feed_versions(id) ON DELETE CASCADE,
    service_id VARCHAR(128) NOT NULL, -- Namespaced
    monday BOOLEAN NOT NULL,
    tuesday BOOLEAN NOT NULL,
    wednesday BOOLEAN NOT NULL,
    thursday BOOLEAN NOT NULL,
    friday BOOLEAN NOT NULL,
    saturday BOOLEAN NOT NULL,
    sunday BOOLEAN NOT NULL,
    start_date VARCHAR(8) NOT NULL, -- YYYYMMDD
    end_date VARCHAR(8) NOT NULL,   -- YYYYMMDD
    PRIMARY KEY (feed_version_id, service_id)
);

-- Calendar Exceptions
CREATE TABLE IF NOT EXISTS gtfs_calendar_dates (
    feed_version_id VARCHAR(64) REFERENCES feed_versions(id) ON DELETE CASCADE,
    service_id VARCHAR(128) NOT NULL, -- Namespaced
    date VARCHAR(8) NOT NULL,         -- YYYYMMDD
    exception_type INTEGER NOT NULL,  -- 1 = added, 2 = removed
    PRIMARY KEY (feed_version_id, service_id, date)
);

-- Trips
CREATE TABLE IF NOT EXISTS gtfs_trips (
    feed_version_id VARCHAR(64) REFERENCES feed_versions(id) ON DELETE CASCADE,
    trip_id VARCHAR(128) NOT NULL, -- Namespaced
    route_id VARCHAR(128) NOT NULL,
    service_id VARCHAR(128) NOT NULL,
    trip_headsign TEXT,
    direction_id INTEGER DEFAULT 0,
    PRIMARY KEY (feed_version_id, trip_id)
);

CREATE INDEX IF NOT EXISTS idx_gtfs_trips_route ON gtfs_trips (feed_version_id, route_id);

-- Stop Times
CREATE TABLE IF NOT EXISTS gtfs_stop_times (
    feed_version_id VARCHAR(64) REFERENCES feed_versions(id) ON DELETE CASCADE,
    trip_id VARCHAR(128) NOT NULL,
    stop_id VARCHAR(128) NOT NULL,
    stop_sequence INTEGER NOT NULL,
    arrival_time VARCHAR(10) NOT NULL,   -- H:MM:SS or HH:MM:SS (may exceed 24:00:00)
    departure_time VARCHAR(10) NOT NULL, -- H:MM:SS or HH:MM:SS
    PRIMARY KEY (feed_version_id, trip_id, stop_sequence)
);

CREATE INDEX IF NOT EXISTS idx_gtfs_stop_times_trip ON gtfs_stop_times (feed_version_id, trip_id);
CREATE INDEX IF NOT EXISTS idx_gtfs_stop_times_stop ON gtfs_stop_times (feed_version_id, stop_id);

-- Transfers
CREATE TABLE IF NOT EXISTS gtfs_transfers (
    feed_version_id VARCHAR(64) REFERENCES feed_versions(id) ON DELETE CASCADE,
    from_stop_id VARCHAR(128) NOT NULL,
    to_stop_id VARCHAR(128) NOT NULL,
    transfer_type INTEGER NOT NULL,
    min_transfer_time INTEGER,
    PRIMARY KEY (feed_version_id, from_stop_id, to_stop_id)
);

-- Frequencies
CREATE TABLE IF NOT EXISTS gtfs_frequencies (
    feed_version_id VARCHAR(64) REFERENCES feed_versions(id) ON DELETE CASCADE,
    trip_id VARCHAR(128) NOT NULL,
    start_time VARCHAR(10) NOT NULL,
    end_time VARCHAR(10) NOT NULL,
    headway_secs INTEGER NOT NULL,
    exact_times INTEGER DEFAULT 0,
    PRIMARY KEY (feed_version_id, trip_id, start_time)
);

