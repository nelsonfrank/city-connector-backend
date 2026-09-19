# ADR-004: PostgreSQL + PostGIS for Static Data, Redis for Realtime Data

| | |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-09-19 |
| **Deciders** | Architecture team |

## Context

The platform has two data stores with fundamentally different characteristics:

1. **Static GTFS data**: relational by nature (agencies → routes → trips → stop_times → stops), updated in batch (daily/weekly), read constantly, and requires geospatial queries ("stops within 500m of this point")
2. **Realtime data**: high write frequency (every 15–30s per agency feed), ephemeral (a stale vehicle position is worse than none), read-heavy for live ETA/position queries, and naturally expires

A single storage technology optimized for one pattern would be a poor fit for the other: a relational store is not designed for high-frequency ephemeral writes with automatic expiry, and a pure key-value store is not designed for relational joins and geospatial queries.

## Decision

Use **two separate storage technologies**, matched to their access pattern:

- **PostgreSQL with the PostGIS extension** for all static GTFS data. PostGIS provides mature geospatial query support (`ST_DWithin`, KNN indexes) directly alongside the relational schema, avoiding a separate geospatial system for "nearby stops" queries.
- **Redis** for realtime vehicle positions and delay data, keyed by trip/vehicle ID with TTL-based expiry, giving low-latency reads/writes without needing to manage expiry logic in application code.

## Consequences

**Positive**
- Each store is used for what it's good at: Postgres/PostGIS for relational + geospatial static data, Redis for high-throughput ephemeral data
- TTL-based expiry in Redis means stale realtime data disappears automatically rather than requiring cleanup jobs
- PostGIS avoids introducing a third specialized geospatial system (e.g., Elasticsearch purely for geo) when the relational store already covers the need at this scale

**Negative**
- Two storage technologies to operate, monitor, and back up (though Redis realtime data is inherently disposable and doesn't need durable backups)
- Realtime data becoming unavailable (Redis outage) must be handled gracefully by the Realtime Service — falling back to static-schedule-only ETAs rather than failing the request

## Alternatives Considered

| Alternative | Why rejected |
|---|---|
| Single PostgreSQL instance for both static and realtime data | Poor fit for high-frequency ephemeral writes; would require manual expiry logic and risks realtime write load impacting static query performance on the same instance |
| Dedicated geospatial engine (e.g., Elasticsearch) instead of PostGIS | Adds an additional system to operate for a capability PostGIS already provides at the required scale (single city); revisit if search/geo needs grow substantially (e.g., full-text ranking at large scale) |
| In-memory-only realtime store without Redis (custom in-process cache per service instance) | Would not be shareable across service replicas, causing inconsistent realtime views depending on which replica served a request |
