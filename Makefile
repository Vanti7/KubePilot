.PHONY: dev dev-infra dev-backend dev-frontend build test clean migrate help

## dev: Start full stack with docker-compose
dev:
	docker compose up --build

## dev-infra: Start only PostgreSQL and Redis
dev-infra:
	docker compose up postgres redis

## dev-backend: Run backend locally (requires dev-infra)
dev-backend:
	cd backend && go run ./cmd/server

## dev-frontend: Run frontend dev server
dev-frontend:
	cd frontend && npm run dev

## build-backend: Build backend binary
build-backend:
	cd backend && go build -o bin/kubepilot ./cmd/server

## build-frontend: Build frontend for production
build-frontend:
	cd frontend && npm run build

## test-backend: Run backend tests
test-backend:
	cd backend && go test ./... -v -race

## test-frontend: Run frontend tests
test-frontend:
	cd frontend && npm run test

## lint-backend: Run Go linter
lint-backend:
	cd backend && golangci-lint run ./...

## lint-frontend: Run ESLint
lint-frontend:
	cd frontend && npm run lint

## migrate: Run database migrations
migrate:
	docker compose exec postgres psql -U kubepilot -d kubepilot -f /docker-entrypoint-initdb.d/001_initial.sql

## clean: Remove build artifacts and volumes
clean:
	docker compose down -v
	rm -rf backend/bin frontend/dist

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
