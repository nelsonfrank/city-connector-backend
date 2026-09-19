# ADR-001: Domain-Oriented Service Decomposition

| | |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-09-19 |
| **Deciders** | Architecture team |

## Context

The platform must support journey search, stop/route browsing, and real-time vehicle tracking across multiple transit agencies in one city. These capabilities have very different performance profiles and scaling needs:

- Journey search is CPU-intensive (graph search over a timetable) and read-heavy but computationally expensive per request
- Stop/route browsing is geospatial/text search, latency-sensitive, high query volume, computationally cheap per request
- Realtime tracking is write-heavy on ingestion (constant feed polling) and read-heavy on serving, with strict freshness requirements and no tolerance for stale data beyond a TTL

A single monolithic service would couple these very different scaling and deployment needs together, making it hard to scale the expensive routing computation independently from cheap, high-volume stop lookups.

## Decision

Decompose the backend into three domain-oriented services behind a shared API gateway:

1. **Routing Service** — journey search and itinerary computation
2. **Stop & Search Service** — geospatial and text search over stops/routes
3. **Realtime Service** — live vehicle positions and delay-adjusted ETAs

Each service is independently deployable and independently scalable. They do not share a database; the Routing and Stop & Search services both read from the static GTFS store (Postgres), while the Realtime Service owns the Redis-backed realtime store.

## Consequences

**Positive**
- Each service can be scaled according to its own load profile (e.g., more Routing Service replicas during commute peaks without over-provisioning Realtime Service)
- Independent deployability — updating the routing algorithm doesn't require redeploying stop search
- Clear ownership boundaries for future team growth

**Negative**
- Additional operational complexity versus a monolith (service discovery, inter-service calls, distributed tracing)
- The Routing Service must call the Realtime Service synchronously to adjust ETAs, introducing a network hop and a dependency that must be handled gracefully on failure (degrade to static ETA rather than failing the whole request)

## Alternatives Considered

| Alternative | Why rejected |
|---|---|
| Single monolithic backend service | Couples scaling of cheap and expensive query types; harder to reason about failure isolation (a routing computation spike could degrade stop search latency) |
| Fully separate microservice per GTFS entity (routes, stops, trips, etc.) | Over-decomposition for current scale; would multiply inter-service calls for what are naturally joined queries (a journey touches trips, stop_times, and stops together) |
