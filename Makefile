.PHONY: test test-integration-docker build build-web docker-up docker-down

test:
	GOCACHE=/tmp/go-build-cache go test ./...

build:
	GOCACHE=/tmp/go-build-cache go build -o stock-options .

build-web:
	cd web && npm run build

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

test-integration-docker:
	./scripts/test-integration-docker.sh
