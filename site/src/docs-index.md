# Documentation

Anime Upscaling is a self-hosted dashboard for a local media workstation or home
server. You drop source files into a folder, queue jobs from the browser, and the
API container runs video2x and FFmpeg with GPU acceleration.

These pages are generated from the markdown in the repository, so they always
match the code that shipped.

## Start here

- **[Architecture](architecture.html)** — how the Go API, the Next.js app and the
  worker queues fit together, and why the trust boundary sits where it does.
- **[Deployment](deployment.html)** — Docker Compose, the NVIDIA overlay, and the
  Portainer stack for home servers.
- **[API reference](api.html)** — every HTTP endpoint the dashboard talks to.
- **[Releasing](releasing.html)** — how versions are cut and images are published.
- **[Contributing](contributing.html)** — local setup, tests and conventions.

## Running it

The fastest path is one command from a clone:

```bash
git clone https://github.com/IvanMicai/anime-upscaling.git
cd anime-upscaling
make quickstart
```

That generates a strong `AUTH_SECRET` and a random `AUTH_PASSWORD`, creates the
media folders, and starts the stack from prebuilt Docker Hub images. It prints
the password when it finishes; open <http://localhost:4750> and log in with it.

To build locally and run on an NVIDIA GPU instead:

```bash
cp .env.example .env
make run-gpu
```

## Configuration

The full list lives in `.env.example`. The ones you will actually touch:

| Variable | Default | Description |
| --- | --- | --- |
| `HOST_PROCESS_DIR` | `./data` | Host directory containing the media folders. |
| `APP_PORT` | `4750` | Public web app port. |
| `AUTH_PASSWORD` | `change-me` | Password for the web app. Replace it. |
| `AUTH_SECRET` | `change-me…` | Secret used to derive the session cookie. Replace it. |
| `GPU_COUNT` | `1` | Number of GPU slots exposed to the worker queue. |
| `STREAMS_PER_GPU` | `1` | Concurrent video2x streams per GPU. |
| `FFMPEG_STREAMS` | `1` | Concurrent FFmpeg workers. |
| `GPU_VENDOR` | _empty_ | Hardware encoder vendor: `nvidia`, `amd`, `intel`, or empty. |

## Before you expose it

The public port should be the Next.js app only. The API is deliberately reachable
only on the Compose network — do not publish its port to the internet.

The built-in authentication is a single shared password for self-hosted use. If
`AUTH_PASSWORD` is empty the gate is disabled entirely and anyone who can reach
the port gets in. Put the service behind HTTPS, a VPN or a trusted reverse proxy
if it runs outside a private network.

## Container images

| Image | Contents |
| --- | --- |
| [`ivanmicai/anime-upscaling-web`](https://hub.docker.com/r/ivanmicai/anime-upscaling-web) | Next.js dashboard |
| [`ivanmicai/anime-upscaling-api`](https://hub.docker.com/r/ivanmicai/anime-upscaling-api) | Go API, video2x and FFmpeg workers |

Both are tagged `latest`, `X`, `X.Y` and `X.Y.Z` on every release.
