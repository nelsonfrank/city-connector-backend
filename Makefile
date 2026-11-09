.PHONY: build test up down tilt-up k8s-apply k8s-delete fetch-feeds

build:
	cd services/routing && go build ./...
	cd services/search && go build ./...
	cd services/realtime && go build ./...

test: build
	cd services/routing && go test -race ./...
	cd services/search && go test ./...
	cd services/realtime && go test ./...

fetch-feeds:
	mkdir -p services/routing/testdata/feeds
	@echo "Fetching MBTA GTFS feed..."
	curl -fSL -o services/routing/testdata/feeds/mbta_gtfs.zip https://cdn.mbta.com/MBTA_GTFS.zip || echo "Note: Network access required to download MBTA feed"

up:
	cd infra/docker && docker compose up --build -d

down:
	cd infra/docker && docker compose down

# Kubernetes / Tilt targets
tilt-up:
	tilt up

k8s-apply:
	kubectl apply -f infra/k8s/

k8s-delete:
	kubectl delete -f infra/k8s/
