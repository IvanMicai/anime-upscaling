# Contributing

Thanks for considering a contribution. This project aims to stay practical,
self-hostable, and easy to operate on a personal media server.

## Development Setup

API:

```bash
cd packages/api
go test ./...
go run ./cmd/animeup serve
```

App:

```bash
cd packages/app
pnpm install
pnpm dev
```

Full validation:

```bash
cd packages/api && go test ./...
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

## Commit Messages

Release versions are calculated from commits merged into `main`:

- `fix: ...` creates a patch release.
- `perf: ...` creates a patch release.
- `feat: ...` creates a minor release.
- `feat!: ...` or a `BREAKING CHANGE:` footer creates a major release.
- `docs: ...`, `test: ...`, `refactor: ...`, `chore: ...`, and `ci: ...` do
  not create a release by default.

When using squash merge, the pull request title must follow the same convention.
