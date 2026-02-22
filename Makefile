.PHONY: all build build-api build-app dev dev-api dev-app clean

# --- Build ---

all: build

build: build-api build-app

build-api:
	cd packages/api && go build -o ../../bin/animeup ./cmd/animeup

build-app:
	cd packages/app && pnpm install && pnpm build

# --- Dev (foreground) ---

dev:
	$(MAKE) dev-api & $(MAKE) dev-app & wait

dev-api:
	cd packages/api && go run ./cmd/animeup serve

dev-app:
	cd packages/app && pnpm dev

# --- Clean ---

clean:
	rm -rf bin/animeup packages/app/.next packages/app/node_modules
