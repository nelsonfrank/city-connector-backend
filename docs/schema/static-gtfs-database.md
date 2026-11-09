# Database Schema Contract: Static GTFS Data Store

| | |
|---|---|
| **Author** | Antigravity Routing Team |
| **Status** | Approved & Implemented |
| **Targets** | PostgreSQL 15 + PostGIS extension |
| **Related** | ADR-003, ADR-004, SAD §6.2, Roadmap 0.4 & 1.1 |

## 1. Overview
This document specifies the PostgreSQL static GTFS storage contract. It defines the tables populated by the Python **Ingestion Service** and queried by the Go **Routing Service** (for in-memory timetable compilation) and **Stop & Search Service** (for geospatial and text search).

## 2. Blue-Green Cutover Pattern (`feed_versions`)
Static schedule updates must not cause read downtime or inconsistent queries (NFR-5, SAD §6.2):
1. Ingestion downloads and validates a feed archive for agency `A`.
2. A row is inserted into `feed_versions` with `status = 'loading'`, `is_active = FALSE`, and a unique `id` (e.g. `mbta_20261109_001`).
3. Raw records are transformed (namespacing all IDs as `{agency_id}:{original_id}`) and loaded into the database tagging each row with `feed_version_id`.
4. Once all tables are committed and validated, the version is activated atomically:
   ```sql
   BEGIN;
   UPDATE feed_versions SET is_active = FALSE WHERE agency_id = 'mbta';
   UPDATE feed_versions SET is_active = TRUE, status = 'active' WHERE id = 'mbta_20261109_001';
   COMMIT;
   ```
5. Background watchers in the Routing Service observe the new active version, compile the in-memory timetable, and swap pointers without downtime.

## 3. Schema Definitions
See full initialization DDL in [`infra/docker/postgres-init/01-gtfs-schema.sql`](../../infra/docker/postgres-init/01-gtfs-schema.sql).

