# ADR-002: RAPTOR as the Journey Routing Algorithm

| | |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-09-19 |
| **Deciders** | Architecture team |

## Context

The core requirement is: given an origin, destination, and departure time, return multiple ranked journey options (fastest, and meaningful alternatives), potentially involving transfers between routes and agencies.

Public transit routing is not a simple shortest-path problem over a static weighted graph, because:
- Edge "cost" (wait time until the next departure) depends on *when* you arrive at a stop, not just the stop itself — this is time-dependent routing
- Users benefit from seeing multiple non-dominated options (fastest vs. fewest transfers vs. least walking), not a single shortest path
- Query latency must stay low (NFR-1: p95 < 800ms) even as the timetable grows across multiple agencies

Two broad approaches were considered: general-purpose time-expanded/time-dependent graph search (Dijkstra/A* variants), and RAPTOR, an algorithm designed specifically for public transit timetables.

## Decision

Use **RAPTOR (Round-based Public Transit Optimized Router)** as the core routing algorithm, operating over an in-memory representation of the active GTFS timetable.

Key implementation choices:
- The timetable (trips, stop_times, precomputed walking transfers) is loaded into memory as a RAPTOR-ready structure, rebuilt on each static data ingestion and hot-swapped
- RAPTOR's round-based structure (each round = one additional transfer) is used directly to produce a Pareto set of itineraries across transfer count and arrival time
- Walking transfers between nearby stops are precomputed at ingestion time, not calculated per query

## Consequences

**Positive**
- RAPTOR is purpose-built for exactly this problem and is the algorithm used by major transit routing systems; it is well-documented with known performance characteristics
- Naturally produces multiple non-dominated itineraries per query, directly satisfying the "show all available routes" and "shortest route" requirements without bolting on extra logic
- Operating on an in-memory structure (rather than live DB queries per round) is what makes the latency target achievable
- Scales well with number of transfers/rounds rather than graph size, which suits the many-small-hops nature of transit networks

**Negative**
- Requires building and maintaining a custom in-memory data structure and rebuild/hot-swap pipeline, rather than using an off-the-shelf general-purpose routing library
- Team must invest in understanding a less common algorithm (versus Dijkstra, which is broadly familiar)
- Multi-agency support requires care in how transfers *between* agencies (not just within one agency's network) are represented in the transfer precomputation step

## Alternatives Considered

| Alternative | Why rejected |
|---|---|
| Dijkstra/A* over a time-expanded graph | Correct but computationally heavier at scale (time-expanded graphs are large); doesn't naturally yield multiple Pareto-optimal itineraries without repeated runs |
| Third-party hosted transit routing API (e.g., a commercial directions API) | Removes control over multi-agency data handling and cost scales with query volume; also a lock-in risk given the product's core value is routing quality |
| Precomputed all-pairs shortest paths | Infeasible for time-dependent transit schedules — "shortest path between A and B" changes by time of day, so all-pairs precomputation doesn't apply directly |
