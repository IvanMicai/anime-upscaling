# Anime Upscaling App

Next.js web UI for Anime Upscaling. The app talks to the Go API through
server-side API routes, so browsers only need access to the app port.

## Environment

| Variable | Description |
| --- | --- |
| `PORT` | App port. Defaults to `3000` in the Docker image and `4750` in the root Compose file. |
| `API_URL` | Internal API URL. Use `http://api:4751` with Docker Compose. |
| `AUTH_PASSWORD` | Password required by the login page. If unset, authentication is disabled. |
| `AUTH_SECRET` | Secret used to derive the session cookie value. |

## Development

```bash
pnpm install
pnpm dev
```

Open [http://localhost:4750](http://localhost:4750) when running through the
root Makefile, or [http://localhost:3000](http://localhost:3000) when running
the package directly without overriding `PORT`.

## Validation

```bash
pnpm lint
pnpm build
```

## Docker

The package Dockerfile builds a standalone Next.js server. In normal operation,
use the root `docker-compose.yml` so the app and API share a private Compose
network.
