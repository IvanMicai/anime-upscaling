# AGENTS.md

Runbook for AI coding agents working in this repository. Humans: see the
[README](README.md) and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## At a glance

Self-hosted anime video processing. A **Go API** (`packages/api`) orchestrates
`video2x` + `ffmpeg` jobs; a **Next.js app** (`packages/app`) is the dashboard
and the only network-exposed service. They run together via Docker Compose. Read
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) before changing job orchestration,
the queues, the GPU monitor, or the app↔API boundary.

## Prerequisites

- Docker + Docker Compose (to run the stack).
- For local dev: Go 1.26+, Node.js 24 LTS, pnpm 10. NVIDIA GPU work additionally
  needs the NVIDIA Container Toolkit.

## Run it

Fastest path (CPU, prebuilt images, no build):

```bash
make quickstart
```

This generates secrets into `.env`, creates the `data/*` folders, starts the
stack from prebuilt Docker Hub images, and prints the login password. With a
GPU, build locally instead: `make run-gpu`.

## Verify

- Open <http://localhost:4750> and log in with the `AUTH_PASSWORD` printed by
  `make quickstart` (also in `.env`).
- API health (from inside the network / app proxy): `GET /api/health/gpu`.
- Drop a video in `data/input`, queue a job from the UI, watch the log stream.

## Test

```bash
# API (Go): formatting, vet, race-enabled tests
cd packages/api && gofmt -l . && go vet ./... && go test -race ./...

# App (Next.js): lint, build, component tests
cd packages/app && pnpm install && pnpm lint && pnpm build && pnpm test-storybook
```

CI runs all of the above plus `golangci-lint` and `govulncheck` — match it
before pushing. The golangci config is `packages/api/.golangci.yml`.

## Logs / stop

```bash
make logs   # docker compose logs -f
make stop   # docker compose down
```

## Operating rules an agent MUST respect

- **The Go API has no authentication.** It trusts the Compose network. Never
  publish the API port (`4751`) to the host or internet; only the app port
  (`4750`) is public. See ARCHITECTURE §3.
- **Jobs are in-memory** and are lost on API restart. Pipeline *definitions*
  persist (`pipelines.json`); job history does not.
- **Do not rename the Portuguese option values / log strings** (e.g. quality
  presets `ultra`/`alta`/`media`/`baixa`). They are public API and live in saved
  pipelines. See [CONTRIBUTING.md](CONTRIBUTING.md#a-note-on-naming).
- **New user-facing copy, comments, and docs are English.**
- **Commits use Conventional Commit titles** (`feat:`, `fix:`, `docs:`, …) —
  releases are derived from them. Include tests when changing API behavior, job
  orchestration, file handling, or pipeline validation.
- **Never commit** `.env`, media files, logs, generated binaries, `.next`, or
  `node_modules`.

## Pointers

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — system design and decisions.
- [packages/api/README.md](packages/api/README.md) — HTTP API reference.
- [CONTRIBUTING.md](CONTRIBUTING.md) — workflow and conventions.
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — self-hosted deployment.
