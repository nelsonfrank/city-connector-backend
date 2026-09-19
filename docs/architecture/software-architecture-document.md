# Software Architecture Document (SAD)
## Public Transport Route Discovery Platform

| | |
|---|---|
| **Document version** | 1.0 |
| **Status** | Draft |
| **Date** | 2026-09-19 |
| **Scope** | Single city, multiple transit agencies |

---

## 1. Introduction and Goals

### 1.1 Purpose
This document describes the software architecture of a public transport application that allows users to:
- Discover all available routes between an origin and a destination
- Identify the shortest (fastest) and alternative (including longest) route options
- Browse and search bus stops and routes
- View real-time bus positions and live arrival estimates

The document follows an arc42-inspired structure: goals and constraints, context, solution strategy, building blocks, runtime behavior, deployment, cross-cutting concerns, and risks. It should be read alongside the Architecture Decision Records (ADRs) in `/docs/adr/`, which capture the *why* behind specific choices referenced here.

### 1.2 Requirements Overview

**Functional requirements**

| ID | Requirement |
|---|---|
| FR-1 | User can search for journey options between two points (address, GPS coordinate, or named stop) |
| FR-2 | System returns multiple itinerary options, not a single answer |
| FR-3 | System highlights the fastest itinerary and surfaces alternative itineraries (fewer transfers, less walking, etc.) |
| FR-4 | User can browse stops near a location (map and list view) |
| FR-5 | User can view a stop's details: served routes, upcoming scheduled and real-time arrivals |
| FR-6 | User can view a route's full path and ordered stop list |
| FR-7 | User can see live vehicle positions and real-time delay-adjusted ETAs |
| FR-8 | System supports multiple transit agencies operating within the same city |

**Non-functional requirements**

| ID | Requirement | Target |
|---|---|---|
| NFR-1 | Journey query latency | p95 < 800ms |
| NFR-2 | Nearby-stop query latency | p95 < 150ms |
| NFR-3 | Realtime position freshness | ≤ 30s staleness |
| NFR-4 | Availability | 99.9% for read paths |
| NFR-5 | Static schedule update | Daily ingestion with zero read downtime |
| NFR-6 | Horizontal scalability | Stateless services scale independently under load |
| NFR-7 | Agency isolation | One agency's feed failure must not degrade other agencies' data |
| NFR-8 | Mobile resilience | Usable in degraded network conditions (cached last-known schedule) |

### 1.3 Quality Goals (ranked)

1. **Correctness of itineraries** — wrong routing information is worse than no information
2. **Latency** — journey search must feel instant to retain users
3. **Data freshness** — realtime ETAs must reflect current conditions
4. **Extensibility** — adding a new agency should not require architectural change
5. **Operability** — the system must be debuggable and observable in production

### 1.4 Stakeholders

| Role | Concern |
|---|---|
| End users (riders) | Fast, accurate, trustworthy journey planning |
| Transit agencies | Their schedule/realtime data is represented accurately; low integration burden |
| Mobile/web engineering teams | Stable APIs, predictable latency |
| Platform/infra team | Operability, cost, scalability |
| Product | Feature velocity (new cities, new modes later) |

---

## 2. Constraints

| Type | Constraint |
|---|---|
| Technical | Must ingest data in **GTFS** (static) and **GTFS-Realtime** (protobuf) formats, since this is the de facto standard agencies publish in |
| Organizational | Agencies control feed quality and update cadence; the platform cannot mandate changes to their publishing systems |
| Data | Multiple agencies may have overlapping or adjacent coverage in the same city — stop/route de-duplication is not guaranteed by the data itself |
| Platform | Must serve both a mobile app and a web app from the same backend APIs |
| Operational | Static feed updates must not cause downtime or incorrect responses during the cutover window |

---

## 3. Context and Scope

### 3.1 Business Context

```
                         ┌───────────────────────┐
   Agency A (GTFS) ───▶  │                       │
   Agency B (GTFS) ───▶  │   Transport Platform   │ ───▶  Mobile app users
   Agency C (GTFS-RT)─▶  │                       │ ───▶  Web app users
                         └───────────────────────┘
```

The platform sits between transit agencies (data producers) and end users (data consumers). It does not operate any vehicles or schedules itself — it is purely an aggregation, routing, and presentation layer.

### 3.2 Technical Context

| External system | Interface | Direction | Notes |
|---|---|---|---|
| Agency GTFS static feeds | HTTP download (zip) | Inbound | Polled on a schedule (daily/weekly per agency) |
| Agency GTFS-RT feeds | HTTP protobuf | Inbound | Polled every 15–30s |
| Geocoding provider | REST API | Outbound | Resolves free-text addresses to coordinates |
| Push notification service | REST API | Outbound | Optional: service alerts, arrival reminders |
| Mobile/web clients | REST/JSON API | Inbound/Outbound | Primary consumer interface |

---

## 4. Solution Strategy

| Concern | Strategy | Rationale (see ADR) |
|---|---|---|
| Route/service decomposition | Domain-oriented services (Routing, Stop & Search, Realtime) behind an API gateway | ADR-001 |
| Journey routing algorithm | RAPTOR (Round-based Public Transit Routing) over an in-memory timetable | ADR-002 |
| Canonical data model | GTFS static schema, extended minimally for cross-agency needs | ADR-003 |
| Static data storage | PostgreSQL + PostGIS | ADR-004 |
| Realtime data storage | Redis, TTL-based | ADR-004 |
| Realtime ingestion | Per-agency polling workers, isolated failure domains | ADR-005 |

The overarching strategy is: **keep the write/ingestion path separate from the read/query path**, and make the read path as fast as possible by precomputing everything that can be precomputed (routing graph, transfers, geospatial indexes) ahead of query time.

---

## 5. Building Block View

### 5.1 Level 1 — System Overview

```
Clients (Mobile, Web)
        │
   API Gateway  (auth, rate limiting, routing, response cache)
        │
   ┌────┼─────────────────┐
Routing   Stop & Search   Realtime
Service   Service         Service
   │           │               │
Static GTFS   Geospatial    Redis
DB (Postgres  Index         (vehicle
+ PostGIS)   (PostGIS)      positions,
   ▲           ▲            delays)
   │           │               ▲
   └─────┬─────┘               │
   Ingestion Pipeline ─────────┘
   (GTFS static + GTFS-RT)
        ▲
   Agency feeds
```

### 5.2 Level 2 — Component Responsibilities

**API Gateway**
- Authentication, per-client rate limiting
- Path-based routing to backend services
- Caches cacheable GET responses (e.g., route shapes, popular nearby-stop queries)
- No transit-domain logic

**Routing Service**
- Holds an in-memory RAPTOR-ready timetable structure per active data version
- Answers `journey` queries: returns a Pareto set of itineraries (fastest, fewest transfers, least walking)
- Applies realtime delay adjustments to itinerary ETAs by calling the Realtime Service
- Hot-swaps its in-memory timetable when a new static data version is published (blue-green)

**Stop & Search Service**
- Geospatial "nearby stops" queries (PostGIS `ST_DWithin`/KNN, or grid-index-backed)
- Text/autocomplete search over stop names and addresses
- Stop detail and route detail lookups (served routes, ordered stop list, shape)

**Realtime Service**
- Maintains current vehicle positions and per-trip delay deltas in Redis
- Serves "next arrivals at stop X" by merging static schedule with live delay
- Exposes vehicle position queries for map rendering

**Ingestion Pipeline**
- Static: scheduled pull per agency → validate → transform → build routing graph + load DB tables → atomic version swap
- Realtime: per-agency polling workers → parse GTFS-RT protobuf → normalize → write to Redis

---

## 6. Runtime View

### 6.1 Scenario: Journey Search

1. Client sends `GET /journey?from&to&time` to the API Gateway
2. Gateway authenticates, rate-limits, forwards to Routing Service
3. Routing Service runs RAPTOR against the current in-memory timetable, producing candidate itineraries
4. Routing Service calls Realtime Service to adjust ETAs for trips currently in progress
5. Routing Service returns a ranked list of itineraries (fastest, alternatives) to the client

### 6.2 Scenario: Static GTFS Update

1. Ingestion Pipeline polls an agency's GTFS static feed on schedule
2. Feed is validated (structural + referential integrity checks)
3. If valid: transformed into routing-graph format and loaded into a new, inactive set of DB tables/graph version
4. Once fully loaded, the active version pointer is atomically swapped
5. Routing Service and Stop & Search Service pick up the new version on next reference (in-flight requests complete against the prior version)
6. If invalid: feed is rejected, alert raised, previous valid version remains active

### 6.3 Scenario: Realtime Vehicle Update

1. Per-agency polling worker fetches GTFS-RT feed (vehicle positions + trip updates)
2. Worker parses and normalizes data into a common internal shape
3. Worker writes vehicle position and delay data into Redis with a short TTL
4. Realtime Service reads from Redis to serve live ETA and vehicle position queries
5. If a worker's feed fails, only that agency's realtime data goes stale/unavailable — other agencies are unaffected

---

## 7. Deployment View

```
                    ┌───────────────────────────┐
                    │        Load Balancer        │
                    └───────────────┬───────────┘
                                     │
                    ┌───────────────▼───────────┐
                    │        API Gateway          │
                    │   (horizontally scaled)     │
                    └──┬──────────┬─────────┬────┘
                       │          │         │
              ┌────────▼──┐ ┌─────▼─────┐ ┌─▼──────────┐
              │ Routing    │ │ Stop &    │ │ Realtime    │
              │ Service    │ │ Search    │ │ Service     │
              │ (N pods,   │ │ Service   │ │ (N pods)    │
              │ in-mem     │ │ (N pods)  │ │             │
              │ timetable) │ │           │ │             │
              └────────┬───┘ └─────┬─────┘ └─┬───────────┘
                       │           │            │
              ┌────────▼───────────▼──┐   ┌────▼──────┐
              │  Postgres + PostGIS    │   │  Redis     │
              │  (primary + replicas)  │   │  Cluster   │
              └────────────────────────┘   └────────────┘
                       ▲
              ┌────────┴───────────────┐
              │  Ingestion Workers      │
              │  (static + per-agency   │
              │   realtime pollers)     │
              └─────────────────────────┘
```

- Services are containerized and deployed on an orchestrator (Kubernetes or equivalent), each independently scalable
- Routing Service pods are stateful *in memory only* (the timetable) — they hold no persistent state and can be recreated freely; new pods load the current active timetable version on startup
- Postgres uses read replicas to isolate the ingestion write path from the query read path
- Redis is deployed as a cluster for realtime data, sized for write-heavy, low-latency, ephemeral data

---

## 8. Cross-Cutting Concepts

### 8.1 Data Model
Canonical schema follows GTFS: `agency`, `routes`, `trips`, `stop_times`, `stops`, `calendar`/`calendar_dates`. See ADR-003 for cross-agency extensions (e.g., a synthetic `platform_agency_id`, agency-prefixed IDs to avoid collisions).

### 8.2 Caching Strategy
- API Gateway caches idempotent, low-cardinality GET responses (route shapes, stop details) with short TTLs
- Routing Service's timetable is itself a cache of the static DB, rebuilt on ingestion
- Realtime data lives entirely in Redis with TTL-based expiry — no cold-store fallback, since stale realtime data is worse than none

### 8.3 Security
- All external APIs behind TLS
- API keys/OAuth tokens for client authentication
- No direct client access to Postgres/Redis; all access mediated through services

### 8.4 Observability
- Structured logging per service, correlation ID propagated from Gateway through all downstream calls
- Key metrics: journey query latency (p50/p95/p99), RAPTOR round count, feed ingestion success/failure per agency, Redis staleness per agency
- Alerting on: feed validation failures, ingestion pipeline failures, realtime feed staleness exceeding NFR-3

### 8.5 Scalability Approach
- Stateless services (Gateway, Stop & Search, Realtime) scale horizontally behind the load balancer
- Routing Service scales horizontally; each pod holds a full copy of the timetable in memory (feasible at single-city scale; would need partitioning at multi-region scale — noted as a risk below)

---

## 9. Architecture Decisions

See `/docs/adr/`:
- ADR-001: Service decomposition strategy
- ADR-002: Routing algorithm selection (RAPTOR)
- ADR-003: Canonical data model (GTFS-based)
- ADR-004: Storage technology selection (Postgres/PostGIS + Redis)
- ADR-005: Realtime ingestion strategy (per-agency isolation)

---

## 10. Risks and Technical Debt

| Risk | Impact | Mitigation |
|---|---|---|
| In-memory timetable size grows beyond single-pod memory as city/agency count grows | Routing Service can't scale further this way | Revisit partitioning (e.g., by geography) if timetable exceeds a defined memory budget; tracked as a future ADR |
| Agencies publish malformed or delayed GTFS feeds | Stale or broken schedule data | Strict validation gate before activating any new static version; alerting; automatic fallback to last-known-good |
| Cross-agency stop/route ID collisions | Incorrect data merging | Namespacing IDs by agency at ingestion time (see ADR-003) |
| GTFS-RT feed inconsistency across agencies (field support varies) | Inconsistent realtime quality per agency | Realtime Service degrades gracefully per agency; UI must indicate when realtime is unavailable for a given trip |
| "Longest route" semantics are inherently fuzzy | Feature may not produce intuitive user value | Product to confirm definition (time/distance/stops); may be deprioritized versus fastest + alternatives |

---

## 11. Glossary

| Term | Definition |
|---|---|
| GTFS | General Transit Feed Specification — static schedule data format |
| GTFS-RT | GTFS-Realtime — protobuf format for live vehicle positions/trip updates |
| RAPTOR | Round-based Public Transit Optimized Router — a transit routing algorithm |
| Pareto set (itineraries) | A set of itineraries where none is strictly better than another across all criteria (time, transfers, walking) |
| Itinerary | One complete journey option from origin to destination |
| Trip | One scheduled run of a route (e.g., the 8:03am run of Route 12) |
