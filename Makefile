# Load .env and export all vars to sub-processes
-include .env
export

.PHONY: all build build-api build-app run stop dev dev-api dev-app clean

# --- Build ---

all: build

build: build-api build-app

build-api:
	cd packages/api && GOOS=linux GOARCH=amd64 go build -o ../../bin/api ./cmd/animeup

build-app:
	docker build -t anime-upscaling-app packages/app

# --- Run (production) ---

run: stop
	@mkdir -p pids logs
	@echo "Starting API on :$(API_PORT)..."
	@nohup ./bin/api serve > logs/api.log 2>&1 & echo $$! > pids/api.pid
	@echo "Starting App on :$(APP_PORT)..."
	@-docker rm -f anime-upscaling-app 2>/dev/null
	@nohup docker run --rm --name anime-upscaling-app \
		--env-file .env \
		-e PORT=$(APP_PORT) \
		-p $(APP_PORT):$(APP_PORT) \
		anime-upscaling-app > logs/app.log 2>&1 &
	@sleep 1
	@echo "Done. API pid=$$(cat pids/api.pid). App container=anime-upscaling-app."
	@echo "Logs: logs/api.log, logs/app.log"

stop:
	@-if [ -f pids/api.pid ]; then kill $$(cat pids/api.pid) 2>/dev/null; rm -f pids/api.pid; fi
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
	rm -rf bin/api packages/app/.next packages/app/node_modules
	-docker rmi anime-upscaling-app 2>/dev/null
