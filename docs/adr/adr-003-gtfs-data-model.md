# ADR-003: GTFS as the Canonical Data Model, with Agency-Namespaced IDs

| | |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-09-19 |
| **Deciders** | Architecture team |

## Context

The platform must ingest schedule data from multiple independent transit agencies. Each agency publishes (or can be onboarded to publish) data in GTFS format, the de facto industry standard. A custom internal schema would require a translation layer per agency with no real benefit, since GTFS already models everything the platform needs: agencies, routes, trips, stops, stop times, and service calendars.

The multi-agency requirement introduces one specific risk GTFS doesn't solve out of the box: **ID collisions**. GTFS entity IDs (`stop_id`, `route_id`, `trip_id`) are only guaranteed unique *within* a single agency's feed. Two agencies could both use `stop_id = "1"` for entirely different physical stops.

## Decision

Adopt the GTFS static schema as the canonical internal data model, with one required extension: **all entity IDs are namespaced by agency at ingestion time** (e.g., `stop_id` becomes `{agency_code}:{original_stop_id}`). This namespacing is applied uniformly across all GTFS tables during the ingestion transform step, before data is loaded into the routing graph or Postgres.

Physically co-located stops from different agencies (e.g., two agencies serving the same physical bus stop) are **not** automatically merged; they are represented as distinct stops connected by a zero-or-near-zero-cost precomputed transfer, consistent with how ordinary walking transfers between nearby stops are modeled (see ADR-002).

## Consequences

**Positive**
- No custom schema to design, document, or keep in sync with an evolving domain — GTFS is a mature, well-understood spec
- Agencies can be onboarded with minimal transformation logic (namespace + load), keeping the ingestion pipeline uniform across agencies
- Namespacing eliminates an entire class of data-corruption bugs (silent cross-agency ID collisions) at the root, rather than requiring defensive checks throughout the codebase

**Negative**
- Every internal reference to a GTFS ID must consistently use the namespaced form; a missed spot in ingestion or a service reintroduces collision risk — this must be enforced via shared ingestion utilities, not ad hoc string concatenation per service
- Not merging co-located stops from different agencies means the UI must handle "these are technically two stops a few meters apart" gracefully (e.g., showing them as one cluster with both agencies' arrivals), which is a product/UX decision, not just a data one

## Alternatives Considered

| Alternative | Why rejected |
|---|---|
| Custom unified internal schema, GTFS as an import format only | Adds a maintenance burden (schema evolution, mapping logic) with no functional benefit over using GTFS directly, since GTFS already covers all required entities |
| Rely on agencies to guarantee globally unique IDs | Not enforceable — the platform doesn't control agency publishing systems (see SAD §2, organizational constraint) |
| Automatically merge co-located stops across agencies by proximity | Risks incorrectly merging genuinely distinct stops; better handled as an explicit, reviewable data curation step later rather than an automatic ingestion-time heuristic |
