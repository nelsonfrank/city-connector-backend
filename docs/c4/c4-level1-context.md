# C4 Level 1 — System Context

Shows the platform as a single black box, and how it relates to the people and external systems around it. No internal structure is shown at this level — that's intentional.

```mermaid
C4Context
    title System Context — Public Transport Platform

    Person(rider, "Rider", "A person planning a trip using public transit")

    System(platform, "Public Transport Platform", "Lets riders search journeys, browse stops and routes, and see real-time bus positions and ETAs across multiple agencies in one city")

    System_Ext(agencyA, "Transit Agency A", "Publishes GTFS static and GTFS-Realtime feeds")
    System_Ext(agencyB, "Transit Agency B", "Publishes GTFS static and GTFS-Realtime feeds")
    System_Ext(geocoding, "Geocoding Provider", "Resolves free-text addresses to coordinates")
    System_Ext(push, "Push Notification Service", "Delivers service alerts and arrival reminders")

    Rel(rider, platform, "Searches journeys, browses stops, views live arrivals")
    Rel(platform, agencyA, "Pulls GTFS static and GTFS-RT feeds")
    Rel(platform, agencyB, "Pulls GTFS static and GTFS-RT feeds")
    Rel(platform, geocoding, "Resolves addresses to coordinates")
    Rel(platform, push, "Sends alerts and reminders")

    UpdateLayoutConfig($c4ShapeInRow="2", $c4BoundaryInRow="1")
```

## Notes

- The platform is agency-agnostic by design: `agencyA` and `agencyB` are instances of the same relationship type (see ADR-001, ADR-003). Adding a third agency means adding another `System_Ext` node of the same shape, not a new kind of relationship.
- Geocoding and push notifications are outbound dependencies the platform calls, not systems that call into the platform.
- This diagram deliberately says nothing about routing algorithms, databases, or service boundaries — that's Level 2.
