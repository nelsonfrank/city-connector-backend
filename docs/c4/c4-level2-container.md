# C4 Level 2 — Container

Zooms into the platform from Level 1 and shows its deployable/runnable units (containers, in the C4 sense — not necessarily Docker containers, though here most of them are): apps, services, and data stores, plus how they talk to each other.

```mermaid
C4Container
    title Container Diagram — Public Transport Platform

    Person(rider, "Rider")

    System_Boundary(platform, "Public Transport Platform") {
        Container(mobile, "Mobile App", "iOS / Android", "Journey search, stop browsing, live tracking")
        Container(web, "Web App", "React / TypeScript", "Journey search, stop browsing, live tracking")
        Container(gateway, "API Gateway", "Kong / equivalent", "Auth, rate limiting, request routing, response caching")

        Container(routing, "Routing Service", "Backend service", "Computes itineraries using RAPTOR over an in-memory timetable")
        Container(search, "Stop & Search Service", "Backend service", "Geospatial and text search over stops and routes")
        Container(realtime, "Realtime Service", "Backend service", "Serves live vehicle positions and delay-adjusted ETAs")
        Container(ingestion, "Ingestion Pipeline", "Batch + worker processes", "Ingests GTFS static and GTFS-RT feeds per agency")

        ContainerDb(postgres, "Static GTFS Database", "PostgreSQL + PostGIS", "Agencies, routes, trips, stop_times, stops, calendars")
        ContainerDb(redis, "Position Store", "Redis", "Live vehicle positions and delay deltas, TTL-based")
    }

    System_Ext(agencies, "Transit Agencies", "Publish GTFS static and GTFS-RT feeds")
    System_Ext(geocoding, "Geocoding Provider")

    Rel(rider, mobile, "Uses")
    Rel(rider, web, "Uses")
    Rel(mobile, gateway, "Calls", "HTTPS/JSON")
    Rel(web, gateway, "Calls", "HTTPS/JSON")

    Rel(gateway, routing, "Routes journey queries to", "HTTPS/JSON")
    Rel(gateway, search, "Routes stop/search queries to", "HTTPS/JSON")
    Rel(gateway, realtime, "Routes realtime queries to", "HTTPS/JSON")
    Rel(gateway, geocoding, "Resolves addresses via", "HTTPS")

    Rel(routing, postgres, "Reads timetable data from", "SQL")
    Rel(routing, realtime, "Requests live delay adjustments from", "HTTPS/JSON")
    Rel(search, postgres, "Reads stop/route data from", "SQL")
    Rel(realtime, redis, "Reads/writes positions and delays", "Redis protocol")

    Rel(ingestion, postgres, "Loads static schedule versions into", "SQL")
    Rel(ingestion, redis, "Writes realtime updates into", "Redis protocol")
    Rel(ingestion, agencies, "Pulls feeds from", "HTTP")

    UpdateLayoutConfig($c4ShapeInRow="3", $c4BoundaryInRow="1")
```

## Notes

- **Routing → Realtime Service** is the one synchronous inter-service call in the read path (used to adjust itinerary ETAs). It's called out explicitly here because it's the dependency most likely to need a circuit breaker / graceful degradation (see ADR-001, "Consequences").
- **Ingestion Pipeline** is drawn as a single container for simplicity, but per ADR-005 it actually runs as one process per agency for the realtime side, plus a separate scheduled job for static feeds. See the Level 3 diagram for that internal structure.
- Postgres and Redis are shown as containers here (they're separately deployable/scalable units) even though they're data stores rather than services — this is standard C4 practice.
