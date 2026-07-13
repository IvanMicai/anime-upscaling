# Contributing

Thanks for considering a contribution. This project aims to stay practical,
self-hostable, and easy to operate on a personal media server.

## Architecture

Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) before changing job
orchestration, the queues, the GPU monitor, or the app↔API boundary — it
explains the design decisions and trust model the code relies on.

## Development Setup

API:

```bash
cd apps/api
go test ./...
go run ./cmd/animeup serve
```

App:

```bash
cd apps/web
pnpm install
pnpm dev
```

Full validation:

```bash
cd apps/api && go test ./...
cd ../app && pnpm lint && pnpm build
```

## Pull Request Guidelines

- Use English for documentation, comments, issue titles, and user-facing copy.
- Keep changes scoped to one behavior or feature when possible.
- Use Conventional Commit titles so releases can be versioned automatically.
- Include tests when changing API behavior, job orchestration, file handling, or
  pipeline validation.
- Update documentation when changing deployment, environment variables, CLI
  commands, job options, or public API responses.
- Do not commit local `.env` files, media files, generated binaries, logs,
  `.next`, `node_modules`, or editor/agent state.

## Reporting Bugs

Please include:

- What you tried to do.
- Expected behavior.
- Actual behavior.
- Relevant logs from `docker compose logs` or the job log view.
- Host OS, Docker version, GPU vendor, and whether the NVIDIA overlay is used.

## A note on naming

Some option values and log strings in this project are in Portuguese — most
visibly the quality preset names (`ultra`, `alta`, `media`, `baixa`) and worker
log words (`Iniciando`, `Concluído`, …). These are a **stable part of the public
API and of saved-pipeline JSON**, so they are intentionally *not* renamed:
changing them would break request validation (`POST /api/jobs`) and every
`pipelines.json` already on a user's disk.

New user-facing copy should be in English (per the rule above); the existing
Portuguese artifacts are grandfathered. When in doubt, document rather than
rename.

## Commit Messages

Release versions are calculated from commits merged into `main`:

- `fix: ...` creates a patch release.
- `perf: ...` creates a patch release.
- `feat: ...` creates a minor release.
- `feat!: ...` or a `BREAKING CHANGE:` footer creates a major release.
- `docs: ...`, `test: ...`, `refactor: ...`, `chore: ...`, and `ci: ...` do
  not create a release by default.

When using squash merge, the pull request title must follow the same convention.
