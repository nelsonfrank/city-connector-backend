# Public Transport Platform

This repository contains the backend for a public transport platform that allows users to discover all available routes between an origin and destination, identify fastest and alternative options, browse stops, and view real-time bus positions and live arrival estimates.

## Folder Layout

```
city-connector-backend/
├── services/
│   ├── routing/     # Journey search and itinerary computation (Go)
│   ├── search/      # Geospatial and text search over stops/routes (Go)
│   ├── realtime/    # Live vehicle positions and delay-adjusted ETAs (Go)
│   └── ingestion/   # Ingests GTFS static and GTFS-RT feeds per agency (Python)
├── gateway/         # API Gateway configuration (Kong)
├── infra/
│   ├── docker/      # Docker Compose for local docker-based development
│   └── k8s/         # Kubernetes manifests for cluster deployment
├── .github/         # CI/CD workflows
├── docs/            # Architecture documentation (SAD, ADRs, C4 diagrams)
├── Makefile         # Build and run automation
├── Tiltfile         # Tilt configuration for local Kubernetes development
└── README.md
```

## Go Services Structure

Each Go service follows a `cmd/` + `internal/` structure to enforce a strict privacy boundary. The `cmd/` directory contains the application entrypoint (e.g., `main.go`) which is meant to be minimal, simply composing and starting the server. The `internal/` directory contains the actual domain logic. Go compiler rules prevent external packages from importing code from an `internal/` directory, which ensures that services remain decoupled and don't accidentally leak implementation details to other parts of the monorepo.

## Local Development

To bring up the entire stack locally with mock/stub endpoints:

```bash
# From the repository root
cd infra/docker
docker compose up --build
```

**Success looks like:**
- PostgreSQL listening on port `5432`
- Redis listening on port `6379`
- Kong Gateway proxy listening on port `8000` (Admin on `8001`)
- Three Go services (`routing`, `search`, `realtime`) started and connected.

To verify Kong is routing correctly, curl the following endpoints:
- `curl localhost:8000/journey`
- `curl localhost:8000/stops/nearby`
- `curl localhost:8000/search`
- `curl localhost:8000/realtime/vehicles`

These should return `200 OK` from the Go stub services. Rate limiting is active (e.g. 5/min on `/journey`, 20/min on others), so hitting them rapidly will yield a `429 Too Many Requests`.

## Local Kubernetes Development (Tilt)

For local Kubernetes development, this project uses [Tilt](https://tilt.dev). Tilt watches your code, rebuilds Docker images automatically, and syncs them into your local cluster (Minikube, Kind, etc.).

**Prerequisites:** `kubectl`, `tilt`, and a running local cluster (e.g. `minikube start`).

```bash
# From the repository root
make tilt-up
# or simply:
tilt up
```

Tilt will:
1. Build `routing:latest`, `search:latest`, `realtime:latest`, and `ingestion:latest` images locally.
2. Apply all manifests in `infra/k8s/`.
3. Port-forward Kong proxy → `localhost:8000`, Admin → `localhost:8001`.
4. Watch for file changes and live-update containers on save.

**Verify routes work the same as docker-compose:**
```bash
curl localhost:8000/journey
curl localhost:8000/stops/nearby
curl localhost:8000/realtime/vehicles
```

To apply or tear down the raw Kubernetes manifests without Tilt:

```bash
make k8s-apply    # kubectl apply -f infra/k8s/
make k8s-delete   # kubectl delete -f infra/k8s/
```

## Documentation & Architecture

For actual architectural reasoning, refer to `/docs`. Specifically:
- **[Software Architecture Document (SAD)](../docs/architecture-SAD/software-architecture-document.md)**
- **[C4 Diagrams](../docs/c4/)**
- **[Architecture Decision Records (ADRs)](../docs/adr/)**
- **[Roadmap](../docs/roadmap.md)**

Review these documents before assuming conventions here are arbitrary.

## Adding a new service

1. **Folder:** Create the service directory under `services/` (e.g., `services/new_service`). If Go, use `cmd/` and `internal/`.
2. **Dockerfile:** Add a `Dockerfile` inside the service directory.
3. **Gateway Route:** Add the new service and route to `gateway/kong.yml`.
4. **Compose:** Add the service block to `infra/docker/docker-compose.yml`, building from its context.
5. **Kubernetes:** Add `infra/k8s/<service>.yaml` with a `Deployment` and `Service`. Add a `docker_build()` and optional `k8s_resource()` call to the `Tiltfile`.
6. **CI:** Add a build job to `.github/workflows/go-services.yml` or create a new workflow file if using a different language.
7. **Docs:** Update this `README.md` and the C4 diagrams in `/docs` to reflect the new component.
