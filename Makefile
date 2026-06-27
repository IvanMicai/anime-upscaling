# Load .env and export all vars to sub-processes
-include .env
export

API_PORT ?= 4751
APP_PORT ?= 4750
HOST_PROCESS_DIR ?= ./data
PROCESS_DIR ?= $(abspath $(HOST_PROCESS_DIR))

.PHONY: all init gen-secrets quickstart build build-api build-app run run-gpu stop logs dev dev-api dev-app clean deploy

# --- Build ---

all: build

init: gen-secrets
	@mkdir -p "$(HOST_PROCESS_DIR)/input" \
		"$(HOST_PROCESS_DIR)/output" \
		"$(HOST_PROCESS_DIR)/optimized" \
		"$(HOST_PROCESS_DIR)/interpolated" \
		"$(HOST_PROCESS_DIR)/temp"
	@echo "Initialized .env and media folders under $(HOST_PROCESS_DIR)."

# Generate a strong AUTH_SECRET and a random AUTH_PASSWORD into .env. Idempotent:
# only rewrites lines that still hold the change-me placeholder, so re-running
# never clobbers a password you already set. Works on both BSD/macOS and GNU sed.
gen-secrets:
	@test -f .env || cp .env.example .env
	@if grep -q '^AUTH_SECRET=change-me' .env; then \
		s=$$(openssl rand -hex 32); \
		sed -i.bak "s|^AUTH_SECRET=.*|AUTH_SECRET=$$s|" .env && rm -f .env.bak; \
		echo "Generated AUTH_SECRET."; \
	fi
	@if grep -q '^AUTH_PASSWORD=change-me' .env; then \
		p=$$(openssl rand -hex 12); \
		sed -i.bak "s|^AUTH_PASSWORD=.*|AUTH_PASSWORD=$$p|" .env && rm -f .env.bak; \
		echo "Generated AUTH_PASSWORD: $$p  (also saved in .env)"; \
	fi

# --- Quickstart (fastest trial: prebuilt images, no local build) ---

quickstart: init
	docker compose -f docker-compose.hub.yml up -d
	@echo ""
	@echo "App: http://localhost:$(APP_PORT)"
	@echo "Log in with the AUTH_PASSWORD from .env:"
	@grep '^AUTH_PASSWORD=' .env

build: build-api build-app

# --platform: video2x base is amd64-only; lets the build run on Apple Silicon.
build-api:
	docker build --platform=linux/amd64 -t anime-upscaling-api packages/api

build-app:
	docker build -t anime-upscaling-app packages/app

# --- Run (production) ---

run: init
	docker compose up -d --build
	@echo "App: http://localhost:$(APP_PORT)"

run-gpu: init
	docker compose -f docker-compose.yml -f docker-compose.nvidia.yml up -d --build
	@echo "App: http://localhost:$(APP_PORT)"

stop:
	docker compose down

logs:
	docker compose logs -f

# --- Dev (foreground) ---

dev:
	$(MAKE) dev-api & $(MAKE) dev-app & wait

dev-api:
	@mkdir -p "$(PROCESS_DIR)/input" "$(PROCESS_DIR)/output" "$(PROCESS_DIR)/optimized" \
		"$(PROCESS_DIR)/interpolated" "$(PROCESS_DIR)/temp"
	cd packages/api && go run ./cmd/animeup serve

dev-app:
	cp .env packages/app/.env.local
	cd packages/app && PORT=$(APP_PORT) pnpm dev

# --- Deploy ---

deploy:
	@echo "See docs/DEPLOYMENT.md for a generic self-hosted deployment flow."
	@exit 1

# --- Clean ---

clean:
	rm -rf bin packages/api/animeup packages/app/.next packages/app/node_modules
	-docker rmi anime-upscaling-api anime-upscaling-app 2>/dev/null
