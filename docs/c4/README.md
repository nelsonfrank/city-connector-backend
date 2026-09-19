# C4 Model — Public Transport Platform

This folder documents the system using the [C4 model](https://c4model.com/): a set of diagrams at increasing zoom levels, each aimed at a different audience.

| Level | Diagram | Audience | File |
|---|---|---|---|
| 1 | System Context | Anyone — non-technical stakeholders, product, agencies | `c4-level1-context.md` |
| 2 | Container | Engineers, architects — shows the deployable/runnable units | `c4-level2-container.md` |
| 3 | Component | Engineers working inside a specific service | `c4-level3-component-routing.md`, `c4-level3-component-realtime.md` |
| 4 | Code | Individual engineers, generated on demand | Not maintained as static docs — see note below |

## How to read these

Each file contains a Mermaid `C4Context`/`C4Container`/`C4Component` diagram. They render natively on GitHub, GitLab, and most modern markdown viewers. If your viewer doesn't support Mermaid, paste the code block into the [Mermaid Live Editor](https://mermaid.live).

## Why no Level 4 (Code) diagrams

Code-level diagrams (classes, methods, their relationships) go stale almost as fast as they're drawn — they change with every refactor. Per the C4 model's own guidance, these are best **generated on demand** from the actual source (e.g., via an IDE's "generate class diagram" feature) rather than hand-maintained as static documentation. If you want one for a specific hotspot (e.g., the RAPTOR engine's internal classes), ask and it can be generated from the current code at that time.

## Keeping these current

- **Level 1 (Context)** changes rarely — only when a new external system integration is added (a new agency type, a new third-party API).
- **Level 2 (Container)** changes when a service is added, split, or removed — update alongside the SAD's Building Block View (§5) and relevant ADRs.
- **Level 3 (Component)** changes as a service's internals are refactored — owned by whichever team owns that service.
