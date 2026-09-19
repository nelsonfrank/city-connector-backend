# ADR-006: Technology Stack Selection

| | |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-09-19 |
| **Deciders** | Architecture team |

## Context

With the service decomposition (ADR-001), routing algorithm (ADR-002), data model (ADR-003), and storage technology (ADR-004, ADR-005) decided, each service still needs a concrete implementation language and each layer needs specific tooling. The choice should follow from what those prior decisions already demand:

- The **Routing Service** runs RAPTOR over a large in-memory timetable on every request, on the platform's tightest latency budget (NFR-1, p95 < 800ms) — this rules out interpreted languages without strong JIT/compiled performance for the hot path
- The **Realtime Service** runs continuous, independent per-agency polling workers (ADR-005) — a natural fit for lightweight concurrency primitives
- The **Ingestion Pipeline**'s static side is a scheduled, not latency-sensitive, data-transformation job, where iteration speed and data-tooling maturity matter more than raw throughput
- Both mobile and web clients are required (per project scope), and the team has not indicated platform-specific performance requirements that would justify separate native codebases
- The team benefits from minimizing the number of distinct languages/runtimes in production, for hiring, code review, and operational simplicity — but not at the cost of forcing a poor-fit language onto the routing hot path

## Decision

Adopt the following stack:

**Backend services**
- **Go** for the Routing Service, Realtime Service, and Stop & Search Service — one language across all backend services, chosen because it satisfies the two performance-sensitive services (Routing, Realtime) well and is entirely adequate for the third
- **Python** for the static Ingestion Pipeline specifically — data-wrangling maturity (pandas) and access to existing GTFS validation tooling outweigh raw performance here, since this runs on a schedule, not per user request

**API Gateway**
- **Kong** (self-hosted) for auth, rate limiting, routing, and response caching — chosen over building a custom gateway (see ADR-001) and over a cloud-managed gateway to avoid premature cloud lock-in

**Data layer**
- **PostgreSQL + PostGIS**, managed (e.g., AWS RDS/Aurora or GCP Cloud SQL), per ADR-004
- **Redis**, managed (e.g., Elasticache or Memorystore), per ADR-004
- Deferred: a dedicated search engine (Typesense/Elasticsearch) for autocomplete — only introduced if PostGIS full-text search proves insufficient at real usage scale

**Clients**
- **React Native** for mobile (iOS + Android from one codebase), with Mapbox GL or a platform map SDK
- **React + TypeScript** (Next.js if SSR is wanted) for web

**Infrastructure**
- Managed container platform (AWS Fargate/ECS or GCP Cloud Run) rather than self-managed Kubernetes at this stage
- GitHub Actions for CI/CD
- Prometheus + Grafana for metrics, OpenTelemetry for tracing, Sentry for error tracking

## Consequences

**Positive**
- A single primary backend language (Go) across three of four backend components reduces context-switching, simplifies hiring/onboarding, and lets shared libraries (e.g., GTFS ID namespacing utilities referenced in ADR-003, the GTFS-RT Normalizer in the C4 Level 3 Realtime diagram) be written once
- Go's concurrency model maps directly onto the per-agency isolated worker design mandated by ADR-005, without needing a separate framework for that isolation
- Managed data stores and a managed container platform reduce early operational burden, letting the team focus on the routing/data logic that is this product's actual differentiator
- React Native avoids maintaining two separate native mobile codebases for a product whose core interactions (search, lists, maps) don't inherently require native-only capabilities

**Negative**
- Python as a second backend language for ingestion means the team maintains two toolchains, dependency ecosystems, and CI configurations instead of one; this is accepted because forcing ingestion into Go would sacrifice data-tooling velocity for no latency benefit (ingestion is not on the user-facing critical path)
- Starting on a managed container platform (Fargate/Cloud Run) rather than Kubernetes may require a migration later if the team needs finer-grained scheduling control, custom operators, or multi-cloud portability — treated as an acceptable deferred cost, not a permanent constraint
- React Native introduces a dependency on its native-bridge ecosystem for any future feature requiring deep native integration; acceptable given no such requirement exists today

## Alternatives Considered

| Alternative | Why rejected |
|---|---|
| Java/Kotlin for Routing Service | A legitimate alternative — mature GTFS/RAPTOR prior art exists (OpenTripPlanner). Not chosen as the default here because it would introduce a second backend language for no clear benefit over Go, given no existing Java codebase or team expertise was specified as a constraint. Worth revisiting if the team has strong existing JVM expertise. |
| Node.js/TypeScript across all backend services (one language including frontend) | Rejected for the Routing Service specifically — single-threaded event loop is a worse fit for CPU-bound RAPTOR computation than Go's goroutines/compiled performance; acceptable for Stop & Search but not worth splitting the "one primary language" benefit for that alone |
| Native iOS (Swift) + Android (Kotlin) instead of React Native | Rejected at this stage — doubles mobile engineering effort for a UI (lists, maps, search) that doesn't need native-only capabilities; revisit only if a specific feature demands it |
| Self-managed Kubernetes from day one | Rejected as premature operational overhead before the team has validated real scale; ADR does not rule out migrating to Kubernetes later, and no code changes are required to do so since services are already containerized |
| Cloud-managed API Gateway (AWS/GCP native) instead of Kong | Rejected to avoid coupling gateway configuration to a single cloud provider this early; revisit if the team commits fully to one cloud and wants tighter native integration (e.g., IAM-based auth) |
