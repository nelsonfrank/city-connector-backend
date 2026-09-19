# C4 Level 3 — Component: Realtime Service

Zooms into the Realtime Service container from Level 2. Reflects the per-agency isolation decision in ADR-005: each agency's feed is handled by its own poller component, not a shared loop.

```mermaid
C4Component
    title Component Diagram — Realtime Service

    Container_Boundary(realtime, "Realtime Service") {
        Component(pollerA, "Feed Poller — Agency A", "Worker process", "Polls Agency A's GTFS-RT feed every 15-30s")
        Component(pollerB, "Feed Poller — Agency B", "Worker process", "Polls Agency B's GTFS-RT feed every 15-30s")
        Component(normalizer, "GTFS-RT Normalizer", "Shared library/component", "Parses protobuf, applies agency ID namespacing, normalizes into a common shape")
        Component(etaApi, "Position & ETA API", "REST endpoint", "Serves live vehicle positions and delay-adjusted arrival estimates")
    }

    ContainerDb(redis, "Position Store", "Redis")
    System_Ext(agencyA, "Transit Agency A")
    System_Ext(agencyB, "Transit Agency B")
    Container(routingSvc, "Routing Service", "Backend service")
    Container(searchSvc, "Stop & Search Service", "Backend service")

    Rel(pollerA, agencyA, "Polls GTFS-RT feed from", "HTTP/protobuf")
    Rel(pollerB, agencyB, "Polls GTFS-RT feed from", "HTTP/protobuf")
    Rel(pollerA, normalizer, "Sends raw feed data to")
    Rel(pollerB, normalizer, "Sends raw feed data to")
    Rel(normalizer, redis, "Writes normalized positions/delays into", "Redis protocol, TTL-based")
    Rel(etaApi, redis, "Reads positions/delays from", "Redis protocol")
    Rel(routingSvc, etaApi, "Requests delay deltas from", "HTTPS/JSON")
    Rel(searchSvc, etaApi, "Requests live arrivals for a stop from", "HTTPS/JSON")

    UpdateLayoutConfig($c4ShapeInRow="4", $c4BoundaryInRow="1")
```

## Notes

- **One poller component per agency** is the direct implementation of ADR-005: a crash or hang in `Feed Poller — Agency A` cannot block or slow down `Feed Poller — Agency B`. Onboarding a new agency means adding a new poller component, not modifying existing ones.
- **GTFS-RT Normalizer** is shared *code*, not a shared *process* — each poller uses it as a library so parsing/namespacing logic doesn't drift between agencies, but each poller still runs and fails independently.
- **Position & ETA API** is the single read surface other services depend on, so that the internal poller/normalizer structure can change without affecting Routing Service or Stop & Search Service's integration.
