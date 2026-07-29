---
name: run-anime-upscaling
description: >-
  Launch, verify, and stop the Anime Upscaling app (Go API + Next.js dashboard,
  Docker Compose). Use when asked to run, start, launch, boot, or smoke-test
  this project locally, or to confirm a change works in the real app.
---

# Run Anime Upscaling

This project is a self-hosted Go API + Next.js dashboard for anime video
processing, run via Docker Compose. The canonical, always-up-to-date runbook is
[AGENTS.md](../../../AGENTS.md); this skill is the short path to launch it.

## Launch (CPU, prebuilt images — fastest)

```bash
make quickstart
```

What it does: generates `AUTH_SECRET` + a random `AUTH_PASSWORD` into `.env`,
creates the `data/*` folders, starts the stack from prebuilt Docker Hub images
(no build), and prints the login password. Idempotent — safe to re-run.

For a local build / GPU run instead:

```bash
make run       # build locally, CPU
make run-gpu   # build locally, NVIDIA overlay
```

## Verify it's up

1. `docker compose ps` — `anime-upscaling-app` and `anime-upscaling-api` are up.
2. Open <http://localhost:4750> and log in with the `AUTH_PASSWORD` from `.env`
   (`grep '^AUTH_PASSWORD=' .env`).
3. Health probe: the API exposes `GET /api/health/gpu` (reports healthy on
   CPU-only too).
4. To exercise a job: drop a video into `data/input`, queue an `optimize` job
   from the UI, and watch the per-job log stream.

## Logs / stop

```bash
make logs   # follow container logs
make stop   # docker compose down
```

## Must-know constraints

- The **Go API has no auth** and must stay internal — only the app port `4750`
  is published; never expose API port `4751`.
- **Jobs are in-memory** (lost on API restart); only pipeline *definitions*
  persist.
- GPU processing needs the NVIDIA Container Toolkit on the host; without it, use
  the CPU path (`make quickstart`).

See [AGENTS.md](../../../AGENTS.md) for the full runbook, testing commands, and
contribution rules.
