GO_DIR := backend
FRONTEND_DIR := frontend

.PHONY: generate test lint build migrate-up migrate-down

generate:
	cd $(GO_DIR) && go tool oapi-codegen --config ../contracts/oapi-codegen.yaml ../contracts/openapi.yaml

test:
	cd $(GO_DIR) && go test ./...
	cd $(FRONTEND_DIR) && npm run test

lint:
	cd $(GO_DIR) && test -z "$$(gofmt -l .)" && go vet ./...
	cd $(FRONTEND_DIR) && npm run lint && npm run typecheck

build:
	cd $(GO_DIR) && go build ./cmd/api
	cd $(FRONTEND_DIR) && npm run build

migrate-up:
	cd $(GO_DIR) && go run ./cmd/migrate up

migrate-down:
	cd $(GO_DIR) && go run ./cmd/migrate down 1
