# ADR-005: Per-Agency Isolated Realtime Ingestion Workers

| | |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-09-19 |
| **Deciders** | Architecture team |

## Context

With multiple independent agencies, each publishing its own GTFS-RT feed, feed quality and reliability will vary: one agency's feed may go down, return malformed protobuf, or lag, independent of the others. NFR-7 requires that one agency's feed failure must not degrade other agencies' data.

A single shared ingestion process pulling all agency feeds in sequence (or in one batch) creates a coupling risk: a slow or hanging feed from one agency could delay processing for all others, and an unhandled parsing error could crash the shared process entirely.

## Decision

Run realtime ingestion as **one independent polling worker per agency feed**. Each worker:
- Polls only its assigned agency's GTFS-RT endpoint on its own schedule
- Parses and normalizes that agency's data independently
- Writes to Redis under that agency's namespaced keys (consistent with ID namespacing in ADR-003)
- Fails independently — a crash, timeout, or malformed feed in one worker does not affect any other worker's process or schedule

Workers are deployed as independently restartable units (e.g., separate processes/pods managed by the orchestrator), so an unhealthy worker can be restarted without affecting others.

## Consequences

**Positive**
- Directly satisfies NFR-7 (agency isolation) — failure domains are cleanly separated at the process level, not just logically
- New agencies can be onboarded by adding a new worker instance without touching existing workers' code or configuration
- Per-agency observability is natural: metrics and alerts (feed staleness, parse error rate) are scoped per worker/agency out of the box

**Negative**
- More running processes to deploy and monitor than a single shared ingestion job, though each is simple and independently disposable
- Common parsing/normalization logic must be shared as a library across workers to avoid duplicated, drifting implementations of GTFS-RT parsing

## Alternatives Considered

| Alternative | Why rejected |
|---|---|
| Single shared ingestion process polling all feeds in a loop | Violates agency isolation (NFR-7) — a hanging or crashing feed handler affects all agencies sharing the process |
| Single process with per-agency async tasks (isolation via concurrency, not separate processes) | Reduces but doesn't eliminate coupling — an unhandled exception or resource exhaustion (e.g., memory) in one task can still affect the shared process; separate processes give a harder isolation boundary |
