# Anime Upscaling

Self-hosted web UI and HTTP API for managing anime video processing jobs with
video2x and FFmpeg. The project is designed for a local media workstation or
home server where you want a browser-based queue for upscaling, interpolation,
optimization, integrity checks, and reusable processing pipelines.

## Features

- Web dashboard for jobs, logs, files, settings, and saved pipelines.
- Go API that runs video2x and FFmpeg workers inside the API container.
- Next.js app with a lightweight password gate.
- Docker Compose deployment with a generic default and an NVIDIA GPU overlay.
- Processing folders for `input`, `output`, `optimized`, `interpolated`, and
  temporary files.
- Configurable GPU streams, FFmpeg worker count, and optional hardware encoding.

## Requirements

- Docker and Docker Compose.
- For NVIDIA acceleration: a working NVIDIA driver and NVIDIA Container Toolkit.
- For local development: Go 1.23+, Node.js 22+, and pnpm 10.

## Quick Start

```bash
cp .env.example .env
mkdir -p data/input data/output data/optimized data/interpolated data/temp
```

Edit `.env` before exposing the app outside your machine. Generate the secret
with `openssl rand -hex 32` and paste the output:

```bash
AUTH_PASSWORD=replace-this-password
AUTH_SECRET=replace-with-the-output-of-openssl-rand-hex-32
```

Start the app without a GPU reservation:

```bash
docker compose up -d --build
```

Start with the NVIDIA GPU profile:

```bash
docker compose -f docker-compose.yml -f docker-compose.nvidia.yml up -d --build
```

Open the web app at [http://localhost:4750](http://localhost:4750).

Put source videos in `data/input`. Outputs are written to `data/output`,
`data/optimized`, or `data/interpolated`, depending on the job or pipeline.

You can also use the Makefile helpers:

```bash
make init
make run
make run-gpu
make logs
make stop
```

## Security Notes

The public port should be the Next.js app only. The API is intentionally exposed
only to the Compose network by default. Do not publish the API port directly to
the internet.

The built-in app authentication is a simple single-password gate for self-hosted
use. Put the service behind HTTPS, a VPN, or a trusted reverse proxy if you run
it outside a private network.

## Configuration

The main environment variables are documented in `.env.example`.

| Variable | Default | Description |
| --- | --- | --- |
| `HOST_PROCESS_DIR` | `./data` | Host directory containing media folders. |
| `PROCESS_DIR` | `/data` | Container path where media is mounted. |
| `APP_PORT` | `4750` | Public web app port. |
| `API_PORT` | `4751` | Internal API port. |
| `AUTH_PASSWORD` | `change-me` | Password for the web app. Replace it. |
| `AUTH_SECRET` | `change-me...` | Secret used to derive the session cookie. Replace it. |
| `GPU_COUNT` | `1` | Number of GPU slots exposed to the worker queue. |
| `STREAMS_PER_GPU` | `1` | Concurrent video2x streams per GPU. |
| `FFMPEG_STREAMS` | `1` | Concurrent FFmpeg workers. |
| `GPU_VENDOR` | empty | Hardware encoder vendor: `nvidia`, `amd`, `intel`, or empty. |

## Development

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
pnpm lint
pnpm build
```

From the repository root:

```bash
make dev
```

## Documentation

- [Deployment guide](docs/DEPLOYMENT.md)
- [Releasing guide](docs/RELEASING.md)
- [API reference](packages/api/README.md)
- [App notes](packages/app/README.md)
- [Contributing](CONTRIBUTING.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Security](SECURITY.md)

## License

MIT. See [LICENSE](LICENSE).
