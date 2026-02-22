# Load .env and export all vars to sub-processes
-include .env
export

.PHONY: all build build-api build-app run stop dev dev-api dev-app clean

# --- Build ---

all: build

build: build-api build-app

build-api:
	docker build -t anime-upscaling-api packages/api

build-app:
	docker build -t anime-upscaling-app packages/app

# --- Run (production) ---

run: stop
	@mkdir -p logs
	@echo "Starting API on :$(API_PORT)..."
	@-docker rm -f anime-upscaling-api 2>/dev/null
	@nohup docker run --rm --name anime-upscaling-api \
		--env-file .env \
		-e API_PORT=$(API_PORT) \
		-p $(API_PORT):$(API_PORT) \
		-v /var/run/docker.sock:/var/run/docker.sock \
		anime-upscaling-api > logs/api.log 2>&1 &
	@echo "Starting App on :$(APP_PORT)..."
	@-docker rm -f anime-upscaling-app 2>/dev/null
	@nohup docker run --rm --name anime-upscaling-app \
		--env-file .env \
		-e PORT=$(APP_PORT) \
		-p $(APP_PORT):$(APP_PORT) \
		anime-upscaling-app > logs/app.log 2>&1 &
	@sleep 1
	@echo "Done. Containers: anime-upscaling-api, anime-upscaling-app."
	@echo "Logs: logs/api.log, logs/app.log"

stop:
	@-docker rm -f anime-upscaling-api 2>/dev/null
	@-docker rm -f anime-upscaling-app 2>/dev/null

# --- Dev (foreground) ---

dev:
	$(MAKE) dev-api & $(MAKE) dev-app & wait

dev-api:
	cd packages/api && go run ./cmd/animeup serve

dev-app:
	cp .env packages/app/.env.local
	cd packages/app && PORT=$(APP_PORT) pnpm dev

# --- Clean ---

clean:
	rm -rf bin packages/app/.next packages/app/node_modules
	-docker rmi anime-upscaling-api anime-upscaling-app 2>/dev/null
