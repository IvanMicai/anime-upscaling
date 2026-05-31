# Architecture

This document explains how Anime Upscaling is put together and *why* — the
design decisions, trust boundaries, and concurrency model a new contributor
needs before changing job orchestration, the queue, or the app↔API boundary.

For the HTTP contract, see the [API reference](../packages/api/README.md). For
running and deploying, see the [README](../README.md) and
[Deployment guide](DEPLOYMENT.md).

## Table of contents

- [1. System overview](#1-system-overview)
- [2. Why Go + Next.js](#2-why-go--nextjs)
- [3. The app ↔ API trust boundary](#3-the-app--api-trust-boundary)
- [4. Job lifecycle](#4-job-lifecycle)
- [5. Concurrency model: the two queues](#5-concurrency-model-the-two-queues)
- [6. GPU health monitor](#6-gpu-health-monitor)
- [7. Pipelines and the data directory](#7-pipelines-and-the-data-directory)
- [8. Log streaming and progress](#8-log-streaming-and-progress)
- [9. The ffmpeg overlay decision](#9-the-ffmpeg-overlay-decision)
- [10. Design constraints and known limitations](#10-design-constraints-and-known-limitations)
- [11. Repository layout and build](#11-repository-layout-and-build)

## 1. System overview

Two processes deployed together by Docker Compose:

- **`packages/app`** — a Next.js web app. The only port published to the host.
- **`packages/api`** — a Go HTTP server that orchestrates `video2x` and `ffmpeg`
  subprocesses. Reachable only on the internal Compose network.

```
                    host:4750 (published)
                         │
  browser ──────────────▶│
                ┌────────▼─────────┐        ┌──────────────────────┐
                │  app (Next.js)   │  HTTP  │   api (Go)           │
                │  - login gate    │───────▶│  - job manager       │
                │  - /api/* proxy  │ :4751  │  - GPU + ffmpeg queue│
                │  - file download │ (internal)  - GPU monitor     │
                └────────┬─────────┘        └──────────┬───────────┘
                         │                             │ spawns
                         │  read-only mount            │
                         ▼                             ▼
                   ┌──────────────  /data  ──────────────┐
                   │ input/ output/ interpolated/        │
                   │ optimized/ temp/ + *.json state     │
                   └─────────────────────────────────────┘
```

Both containers mount the same media directory (`HOST_PROCESS_DIR` → `/data`):
the API read-write, the app read-only (it only streams downloads from it). Only
the app port is published; the API port is `expose`d to the Compose network but
never mapped to the host. See [`docker-compose.yml`](../docker-compose.yml).

## 2. Why Go + Next.js

The split mirrors the two very different jobs the system does.

The **API is a process supervisor**. It spawns long-running `video2x`/`ffmpeg`
children, gates how many run at once, parses their progress output, kills them on
cancel, and survives a wedged GPU driver. Go fits this: cheap goroutines, first-
class `context` cancellation, `os/exec` with `CombinedOutput`/pipes, and a single
static binary that drops cleanly onto the heavy CUDA base image. Entry point:
[`cmd/animeup/main.go`](../packages/api/cmd/animeup/main.go) (the `serve`
subcommand; there are also CLI subcommands for direct upscale/optimize/pipeline
runs).

The **app is a UI plus a thin trusted proxy**. Next.js renders the dashboard and
exposes a server-side `/api/*` route that is the *only* caller of the Go API.
Keeping the proxy server-side means the browser never holds an API URL or talks
to the API directly — the password gate and the network boundary both live in
front of it.

## 3. The app ↔ API trust boundary

This is the most important design decision to understand before touching auth or
networking.

**The Go API has no authentication.** It trusts its network: every handler is
wrapped only in [`corsMiddleware`](../packages/api/internal/server/server.go),
which sets permissive CORS and short-circuits `OPTIONS` — there is no token
check. This is deliberate, and it is why **the API port must never be published
to the host or the internet**. The security model is "the API is only reachable
from the app container."

**Auth lives entirely in the app proxy.** Every request flows through
[`app/api/[...path]/route.ts`](../packages/app/app/api/[...path]/route.ts), which
calls `isValidSession` *before* forwarding upstream and returns `401` otherwise.
The session model ([`lib/auth.ts`](../packages/app/lib/auth.ts)):

- The cookie value is `SHA-256(AUTH_PASSWORD + AUTH_SECRET)`, hex-encoded.
- A request is valid when its cookie equals that hash.
- **If `AUTH_PASSWORD` is unset, `isValidSession` returns `true` for everyone** —
  auth is disabled. This is the zero-config trial mode; set `AUTH_PASSWORD`
  (and a random `AUTH_SECRET`) before exposing the app anywhere.

The proxy has two special paths besides plain JSON forwarding:

- **Binary downloads** (`GET /api/files/download`) are served directly from the
  read-only `/data` mount by the app (`handleDownload`), *not* proxied from the
  API. Streaming multi-GB bodies back through `fetch`/undici buffers them in
  memory and OOMs the 1 GB app container — so the app reads the file off disk and
  streams it itself.
- **`text/event-stream`** responses are passed through unbuffered. (The job-log
  endpoint itself uses polling, not SSE — see §8 — but the passthrough exists.)

## 4. Job lifecycle

A *job* is a unit of work over a set of files. Jobs are created at
`POST /api/jobs` (validated in
[`handleCreateJob`](../packages/api/internal/server/server.go)) and managed by
the `JobManager` in [`internal/server/jobs.go`](../packages/api/internal/server/jobs.go).

States and transitions:

```
queued ──(first worker starts)──▶ running ──(all files done)──▶ completed
   │                                  │
   │                                  ├──(any Progress.Failed > 0)──▶ failed
   └──────────────────────────────────┴──(context cancelled)──────▶ cancelled
```

- `setRunningOnce()` flips `queued → running` exactly once when the first worker
  begins.
- `finish()` computes the terminal state: `cancelled` if the job context was
  cancelled, else `failed` if any file failed, else `completed`.
- Job types: `upscale`, `interpolate`, `optimize`, `check`, plus custom
  pipelines (`StartPipelineJob`).

**Accounting invariant:** `Completed + Failed + Skipped == Total`. Before
dispatch, files whose output already exists are marked `SKIP`; pipeline step
failures emit one real `ERRO` plus placeholder errors for the steps that won't
run, so the totals always balance (see §7 and `failRemaining` in
[`custom_pipeline.go`](../packages/api/internal/process/custom_pipeline.go)).

**Jobs are in-memory only.** `JobManager.jobs` is a plain `map[string]*Job`; jobs
do not survive an API restart. Pipeline *definitions* persist to
`pipelines.json` under `/data`, but job history does not. This is a known
limitation (§10).

Cancellation cancels the job's `context`, which both unblocks any queue `Acquire`
and stops the running subprocess; `DeleteJob` waits briefly for a graceful stop
before removing the job.

## 5. Concurrency model: the two queues

All throughput control is two priority-aware worker pools in
[`internal/queue/queue.go`](../packages/api/internal/queue/queue.go), built in
`NewJobManager`:

- **`GPUQueue`** — `GPU_COUNT × STREAMS_PER_GPU` slots, each a `(gpuID,
  streamIdx)` pair. GPU work (upscale, interpolate, and GPU-encoded optimize)
  acquires here. Slot order interleaves GPU ids so the first acquires spread
  across distinct GPUs. `streamIdx` disambiguates concurrent streams on one GPU
  for log/progress labels.
- **`Queue`** (FFmpeg) — `FFMPEG_STREAMS` slots for CPU encode/decode work
  (`optimize` without hardware encode, and `check`).

Both pools serve waiters **highest-priority-first**, not FIFO. This matters
because a custom pipeline launches a goroutine per file up front and lets the
queue order them. The composite priority
([`pipelinePriority`](../packages/api/internal/process/custom_pipeline.go)) is:

```
priority = stepIdx * 1_000_000 - index
```

so episodes further along a pipeline outrank episodes still on earlier steps
(*finish what's started before opening new fronts*), and within a step the
lower-indexed (earlier natural-sorted) file wins the tiebreak. Priority is
**global** across all pipeline jobs sharing the queue — a new job's step-0 work
intentionally loses to an older job's later steps.

**Optimize routing.** An `optimize` step uses the GPU pool only when
`UseGPU && GPUVendor != "" && codec ∉ {copy, libvpx-vp9}`; otherwise it uses the
FFmpeg pool. (`handleCreateJob` rejects `use_gpu` with those codecs or with no
vendor configured.)

**Runtime reconfiguration.** `ApplySettings` rebuilds both queues with new
concurrency values and is only safe when idle — callers must check
`HasActiveJobs()` first, because rebuilding would otherwise discard queues that
still hold acquired slots. The GPU gate (§6) is re-installed on the rebuilt
queue.

## 6. GPU health monitor

NVIDIA drivers can wedge (e.g. NVRM Xid 119 / GSP RPC timeouts). When that
happens, feeding the GPU new work just piles up uninterruptible
`nvidia-container-cli` processes and makes recovery harder. The monitor
([`internal/gpu/monitor.go`](../packages/api/internal/gpu/monitor.go)) stops the
bleeding by gating dispatch.

How it works:

- It runs a cheap probe — `nvidia-smi -L` — every `30s` with an `8s` hard
  timeout. Listing devices is enough to exercise the GSP RPC path without doing
  any real allocation.
- After **2 consecutive failures** it flips to *unhealthy*; it recovers to
  *healthy* on the next successful probe.
- It is installed as the `GPUQueue`'s pre-acquisition gate via
  `jm.SetGPUGate(monitor.WaitHealthy)` ([`server.go`](../packages/api/internal/server/server.go)).
  While unhealthy, `GPUQueue.Acquire` blocks in `WaitHealthy` until the driver
  recovers or the caller's context is cancelled. The FFmpeg pool is unaffected.
- It only probes when `GPU_VENDOR=nvidia`. For CPU-only and AMD/Intel it is a
  no-op that reports healthy forever (it starts healthy).
- Current state is exposed at `GET /api/health/gpu`.

**Scope boundary:** the monitor only *stops dispatch*. It does not recover the
GPU — host-side recovery (PCI remove+rescan, restarting the container) is out of
process and is the operator's responsibility.

## 7. Pipelines and the data directory

The `/data` volume holds both media and JSON state. Directories are defined in
[`internal/config/config.go`](../packages/api/internal/config/config.go):

| Directory        | Holds                                              |
| ---------------- | -------------------------------------------------- |
| `input/`         | Source files you drop in                           |
| `output/`        | Upscale results                                    |
| `interpolated/`  | Frame-interpolation (RIFE) results                 |
| `optimized/`     | Final re-encodes                                   |
| `temp/`          | Scratch space, wiped on API startup                |

A **pipeline** is an ordered list of steps (`upscale`, `interpolate`,
`optimize`). `RunCustomPipelineForFile`
([`custom_pipeline.go`](../packages/api/internal/process/custom_pipeline.go))
runs all steps for one file, advancing the input directory as it goes: each step
reads from the previous step's canonical output dir
(`input → output → interpolated → optimized`). Encodes write into `temp/` and are
renamed into place on success so partial outputs never appear as finished files.
Filenames with spaces are hard-linked to a sanitized name for `video2x` and
restored afterward (see [`internal/runner/runner.go`](../packages/api/internal/runner/runner.go)).

Pipeline definitions are stored in `pipelines.json` (a `pipeline.Store`) and
managed via `/api/pipelines`; a run is triggered with
`POST /api/pipelines/{id}/run`.

## 8. Log streaming and progress

> **Note:** despite the API reference historically calling this "SSE", job logs
> are delivered by **polling**, not Server-Sent Events.

Each job keeps an in-memory append-only log slice. The client hook
[`lib/use-log-stream.ts`](../packages/app/lib/use-log-stream.ts) polls
`GET /api/jobs/{id}/logs?since=<cursor>` every ~1.5s; the server
(`handleJobLogs`) returns `{ entries, total, running }` where `total` is the new
cursor the client sends next. Polling stops when `running` is false. (The proxy
*can* pass through `text/event-stream` generically, but this endpoint does not
use it.)

Per-file progress is parsed from worker output — `ffmpeg -progress pipe:2` and
`video2x` stdout — by the runner's progress writers
([`internal/runner/progress.go`](../packages/api/internal/runner/progress.go)),
and surfaced as `Progress.Containers[source]`, keyed by worker label
(`"GPU 0"`, `"FFMPEG"`, …).

## 9. The ffmpeg overlay decision

The API image is built `FROM ghcr.io/k4yt3x/video2x:6.4.0`, which bundles an
older `ffmpeg` whose `libx265` is prone to thread-pool `SIGSEGV`s on some inputs.
Rather than fork video2x, the [`Dockerfile`](../packages/api/Dockerfile) fetches
a current static GPL build (libx265 + nvenc) from BtbN and copies `ffmpeg`/
`ffprobe` into `/usr/local/bin`, which precedes `/usr/bin` on `PATH`. The API
looks up `ffmpeg`/`ffprobe` by name, so it resolves to the newer binaries with no
code change. Bump `FFMPEG_VARIANT` to change versions; the build prints
`ffmpeg -version` into the log.

Relatedly, the **salvage** path
([`internal/process/salvage.go`](../packages/api/internal/process/salvage.go))
treats a signal-killed `video2x` run as success when the output is fully written
and the success marker is in the log — working around a glslang teardown crash
that kills the process *after* it has finished the actual work.

## 10. Design constraints and known limitations

- **Jobs are in-memory** — history is lost on API restart (§4). Pipeline
  definitions persist; jobs don't.
- **No API-level auth** — the API trusts the network boundary; never publish its
  port (§3).
- **Single shared password** — the app gate is one password for everyone, not a
  multi-user system. Put an identity-aware proxy in front if you need more.
- **Personal image namespace** — published images live under the maintainer's
  `ivanmicai` Docker Hub namespace (the zero-config trial default); override
  `DOCKERHUB_NAMESPACE` to use your own.
- **Mixed-language artifacts** — some option values and log strings are in
  Portuguese (e.g. quality presets `ultra`/`alta`/`media`/`baixa`, log words like
  `Iniciando`). These are a stable part of the public API and saved-pipeline
  JSON and are intentionally *not* renamed; see
  [CONTRIBUTING.md](../CONTRIBUTING.md#a-note-on-naming).

## 11. Repository layout and build

```
.
├── packages/
│   ├── api/                 Go HTTP API + processing
│   │   ├── cmd/animeup/      CLI entry point (serve + direct subcommands)
│   │   ├── internal/
│   │   │   ├── server/       HTTP handlers, JobManager, job lifecycle
│   │   │   ├── queue/        GPUQueue + FFmpeg Queue (priority pools)
│   │   │   ├── runner/       subprocess spawning, progress parsing
│   │   │   ├── process/      upscale / interpolate / optimize / pipeline / salvage
│   │   │   ├── gpu/          health monitor
│   │   │   ├── pipeline/     pipeline store + model/codec validation tables
│   │   │   ├── files/        listing, natural sort, safe paths
│   │   │   ├── cache/        file-status cache (resolution/track metadata)
│   │   │   └── config/       env + persisted settings
│   │   └── Dockerfile        video2x base + ffmpeg overlay
│   └── app/                 Next.js dashboard (App Router)
│       ├── app/api/[...path] server-side proxy + auth gate
│       ├── components/       UI + Storybook stories
│       └── lib/              auth, polling hooks, API client, types
├── docker-compose*.yml      default / nvidia / hub / portainer stacks
└── Makefile                 init, quickstart, run, dev helpers
```

**Non-standard workspace.** `pnpm-workspace.yaml` and `pnpm-lock.yaml` live in
`packages/app`, not the repo root. Consequence: pnpm commands run from
`packages/app`, and CI / Dependabot target that directory rather than `/`. Go
tooling targets `packages/api` (where `go.mod` lives). Keep this in mind when
adding tooling that assumes a root-level manifest.
