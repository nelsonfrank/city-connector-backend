.PHONY: build test up down tilt-up k8s-apply k8s-delete

build:
	cd services/routing && go build ./...
	cd services/search && go build ./...
	cd services/realtime && go build ./...

test: build
	cd services/routing && go test ./...
	cd services/search && go test ./...
	cd services/realtime && go test ./...

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
