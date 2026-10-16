SHELL := /bin/sh

APP ?= subflow
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf 'dev')
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf 'none')
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X github.com/subflow-org/subflow/internal/version.Version=$(VERSION) -X github.com/subflow-org/subflow/internal/version.Commit=$(COMMIT) -X github.com/subflow-org/subflow/internal/version.BuildTime=$(BUILD_TIME)
GO ?= go
PNPM ?= pnpm
WEB_DIR := web
BIN_DIR := bin
DIST_DIR := dist

.PHONY: help init dev frontend backend build build-all test lint check run fmt vet typecheck clean docker-build docker-up docker-down docker-logs docker-test release version

help:
	@printf '%s\n' \
		'make init          Install/check frontend and backend dependencies' \
		'make dev           Start the frontend development server' \
		'make frontend      Build the Vue frontend into web/dist' \
		'make backend       Build the Go backend binary' \
		'make build         Build frontend and embedded production binary' \
		'make build-all     Cross-build planned release targets' \
		'make test          Run Go tests' \
		'make lint          Run Go vet and frontend typecheck' \
		'make check         Run formatting, tests, lint and production build' \
		'make run           Run the local production binary' \
		'make clean         Remove generated build outputs' \
		'make docker-build   Build the Docker image' \
		'make docker-up      Start Docker Compose' \
		'make docker-down    Stop Docker Compose' \
		'make docker-logs    Show Docker Compose logs' \
		'make docker-test    Run container health checks' \
		'make release        Build release archives and checksums'

init:
	$(GO) mod download
	cd $(WEB_DIR) && $(PNPM) install --frozen-lockfile

dev:
	cd $(WEB_DIR) && $(PNPM) run dev

frontend:
	cd $(WEB_DIR) && $(PNPM) run build

backend: frontend
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP) .

build: backend
	@printf 'Built %s\n' $(BIN_DIR)/$(APP)

build-all: frontend
	@mkdir -p $(DIST_DIR)
	@set -e; \
	for t in linux-386 linux-amd64 linux-arm64 linux-armv5 linux-armv6 linux-armv7 linux-s390x darwin-amd64 darwin-arm64 windows-amd64; do \
		os=$${t%%-*}; variant=$${t#*-}; \
		case $$variant in \
			armv5) arch=arm; goarm=5 ;; \
			armv6) arch=arm; goarm=6 ;; \
			armv7) arch=arm; goarm=7 ;; \
			*) arch=$$variant; goarm= ;; \
		esac; \
		ext=""; [ "$$os" = windows ] && ext=".exe"; \
		printf 'Building %s\n' "$$t"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch GOARM=$$goarm \
			$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-$$t$$ext . ; \
	done

test:
	$(GO) test ./...

lint: vet typecheck

vet:
	$(GO) vet ./...

typecheck:
	cd $(WEB_DIR) && $(PNPM) run typecheck

check: fmt test lint build

fmt:
	@test -z "$$($(GO)fmt -l .)" || (printf 'Go files need formatting\n'; exit 1)

run: build
	./$(BIN_DIR)/$(APP)

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) $(WEB_DIR)/dist/assets $(WEB_DIR)/dist/index.html

docker-build:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME) -t $(APP):$(VERSION) .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

docker-test:
	docker compose up -d --wait
	curl --fail --silent http://127.0.0.1:$${SUBFLOW_PORT:-8080}/api/health

release: check build-all
	@mkdir -p $(DIST_DIR)
	sha256sum $(DIST_DIR)/$(APP)-* > $(DIST_DIR)/SHA256SUMS

version:
	@printf '%s\n' '$(VERSION) ($(COMMIT), $(BUILD_TIME))'
