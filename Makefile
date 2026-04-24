# Load .env and export all vars to sub-processes
-include .env
export

API_PORT ?= 4751
APP_PORT ?= 4750
HOST_PROCESS_DIR ?= ./data

.PHONY: all init build build-api build-app run run-gpu stop logs dev dev-api dev-app clean deploy

# --- Build ---

all: build

init:
	@test -f .env || cp .env.example .env
	@mkdir -p "$(HOST_PROCESS_DIR)/input" \
		"$(HOST_PROCESS_DIR)/output" \
		"$(HOST_PROCESS_DIR)/optimized" \
		"$(HOST_PROCESS_DIR)/interpolated" \
		"$(HOST_PROCESS_DIR)/temp"
	@echo "Initialized .env and media folders under $(HOST_PROCESS_DIR)."

build: build-api build-app

build-api:
	docker build -t anime-upscaling-api packages/api

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
