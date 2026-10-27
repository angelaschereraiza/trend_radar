SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

BACKEND_DIR := backend
FRONTEND_DIR := frontend

.PHONY: dev db-up db-down generate test lint build migrate-up migrate-down fixture-import import-yahoo

dev:
	@if [ ! -f .env ]; then echo "Missing .env; run: cp .env.example .env"; exit 1; fi
	@set -a; . ./.env; set +a; \
	podman compose up -d postgres; \
	for attempt in $$(seq 1 30); do \
		if podman compose exec -T postgres pg_isready -U "$${DB_USER:-trend_radar}" -d "$${DB_NAME:-trend_radar}" >/dev/null 2>&1; then break; fi; \
		if [ "$$attempt" -eq 30 ]; then echo "PostgreSQL did not become ready within 60 seconds"; exit 1; fi; \
		sleep 2; \
	done; \
	cd $(BACKEND_DIR); \
	go run ./cmd/migrate up; \
	pids=(); \
	cleanup() { \
		trap - EXIT INT TERM; \
		for pid in "$${pids[@]}"; do kill "$$pid" 2>/dev/null || true; done; \
		for pid in "$${pids[@]}"; do wait "$$pid" 2>/dev/null || true; done; \
	}; \
	trap cleanup EXIT; \
	trap 'exit 130' INT TERM; \
	go run ./cmd/api & pids+=("$$!"); \
	go run ./cmd/worker --schedule & pids+=("$$!"); \
	cd ../$(FRONTEND_DIR); \
	npm run dev & pids+=("$$!"); \
	echo "TrendRadar is running. Frontend: http://127.0.0.1:5173 (Ctrl+C to stop services)"; \
	wait -n "$${pids[@]}"

import-yahoo:
	@if [ ! -f .env ]; then echo "Missing .env; run: cp .env.example .env"; exit 1; fi
	@set -a; . ./.env; set +a; cd $(BACKEND_DIR); go run ./cmd/collector --yahoo

fixture-import:
	@if [ ! -f .env ]; then echo "Missing .env; run: cp .env.example .env"; exit 1; fi
	@set -a; . ./.env; set +a; cd $(BACKEND_DIR); go run ./cmd/collector --fixture

db-up:
	podman compose up -d postgres

db-down:
	podman compose stop postgres

generate:
	cd $(BACKEND_DIR) && go tool oapi-codegen --config ../contracts/oapi-codegen.yaml ../contracts/openapi.yaml

test:
	cd $(BACKEND_DIR) && go test ./...
	cd $(FRONTEND_DIR) && npm run test

lint:
	cd $(BACKEND_DIR) && test -z "$$(gofmt -l .)" && go vet ./...
	cd $(FRONTEND_DIR) && npm run lint && npm run typecheck

build:
	cd $(BACKEND_DIR) && go build ./cmd/api
	cd $(FRONTEND_DIR) && npm run build

migrate-up:
	cd $(BACKEND_DIR) && go run ./cmd/migrate up

migrate-down:
	cd $(BACKEND_DIR) && go run ./cmd/migrate down 1
