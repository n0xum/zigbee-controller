.PHONY: build run docker-up docker-logs tidy test lint

build:
	go build -o bin/bridge ./cmd/bridge

run:
	go run ./cmd/bridge

docker-up:
	docker compose up -d

docker-logs:
	docker compose logs -f zigbee2mqtt

tidy:
	go mod tidy

test:
	go test ./...

lint:
	go vet ./...
