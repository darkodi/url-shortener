.PHONY: build run test docker-build docker-run docker-stop clean

# Application
APP_NAME=url-shortener
DOCKER_IMAGE=url-shortener:latest

# ============================================================
# LOCAL DEVELOPMENT
# ============================================================

build:
	go build -o bin/$(APP_NAME) ./cmd/server

run:
	go run ./cmd/server

test:
	go test -v ./...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -rf bin/ coverage.out coverage.html *.db

# ============================================================
# DOCKER
# ============================================================

docker-build:
	docker build -t $(DOCKER_IMAGE) .

docker-run:
	docker compose up -d

docker-stop:
	docker compose down

docker-logs:
	docker compose logs -f

docker-clean:
	docker compose down -v
	docker rmi $(DOCKER_IMAGE) || true

# ============================================================
# ALL-IN-ONE
# ============================================================

up: docker-build docker-run
	@echo "✅ Application running at http://localhost:8080"

down: docker-stop
	@echo "✅ Application stopped"
