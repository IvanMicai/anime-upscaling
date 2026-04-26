# Deployment Guide

This guide covers a generic self-hosted deployment. It avoids machine-specific
paths, private IPs, and one-off scripts so the project can be cloned and run on
a new server.

## 1. Install Prerequisites

Required:

- Docker
- Docker Compose

Optional for NVIDIA GPU processing:

- NVIDIA driver
- NVIDIA Container Toolkit

Validate NVIDIA container support before starting the app:

```bash
docker run --rm --gpus all nvidia/cuda:12.6.0-base-ubuntu24.04 nvidia-smi
```

## 2. Prepare Configuration

From the repository root:

```bash
cp .env.example .env
mkdir -p data/input data/output data/optimized data/interpolated data/temp
```

Generate a session secret:

```bash
openssl rand -hex 32
```

Edit `.env` and replace at least:

```bash
AUTH_PASSWORD=replace-this-password
AUTH_SECRET=replace-with-generated-secret
```

For a different host media directory, update:

```bash
HOST_PROCESS_DIR=/absolute/path/on/host
PROCESS_DIR=/data
```

`HOST_PROCESS_DIR` is the path on the server. `PROCESS_DIR` is the path used
inside the containers. In most deployments, leave `PROCESS_DIR=/data`.

## 3. Start the Stack

Default Compose stack:

```bash
docker compose up -d --build
```

NVIDIA GPU stack:

```bash
docker compose -f docker-compose.yml -f docker-compose.nvidia.yml up -d --build
```

Published Docker Hub images can be used instead of local builds after a release
has been created. Add the Docker Hub namespace and image tag to `.env`:

```bash
DOCKERHUB_NAMESPACE=my-user
IMAGE_TAG=1.2.3
```

Then start the stack from published images:

```bash
docker compose -f docker-compose.hub.yml pull
docker compose -f docker-compose.hub.yml up -d
```

For NVIDIA GPU processing with published images:

```bash
docker compose -f docker-compose.hub.yml -f docker-compose.nvidia.yml pull
docker compose -f docker-compose.hub.yml -f docker-compose.nvidia.yml up -d
```

View logs:

```bash
docker compose logs -f
```

Stop the stack:

```bash
docker compose down
```

### Portainer Stack

`docker-compose.portainer.yml` is a single-file stack tailored for
[Portainer](https://www.portainer.io/) deployments. It pulls the published
Docker Hub images, bundles the NVIDIA GPU reservation inline (no overlay
file), and exposes only the app port on the host.

In Portainer:

1. Go to **Stacks → Add stack**.
2. Choose **Web editor** and paste the contents of
   `docker-compose.portainer.yml` (or use **Repository** and point it at this
   repo with `docker-compose.portainer.yml` as the compose path).
3. Under **Environment variables**, set at least:

   ```bash
   AUTH_PASSWORD=replace-this-password
   AUTH_SECRET=replace-with-output-of-openssl-rand-hex-32
   HOST_PROCESS_DIR=/absolute/path/on/host
   PROCESS_DIR=/absolute/path/on/host
   ```

   `HOST_PROCESS_DIR` and `PROCESS_DIR` should match so the host and container
   see the media directory at the same path. Optionally override `APP_PORT`,
   `API_PORT`, `IMAGE_TAG`, `GPU_COUNT`, `STREAMS_PER_GPU`, `FFMPEG_STREAMS`,
   or `GPU_VENDOR`.

4. Deploy the stack. The web app is reachable on `APP_PORT` (default `4750`);
   the API stays internal to the stack network.

The host needs the NVIDIA driver and NVIDIA Container Toolkit installed for
the GPU reservation to start. Drop the `deploy.resources` block in the editor
if you want to run CPU-only.

## 4. Add Media

Place input videos in:

```text
data/input
```

The app writes processed files to:

```text
data/output
data/optimized
data/interpolated
```

Runtime settings and saved pipelines are stored under the same mounted media
directory as JSON files.

## 5. Reverse Proxy

Expose only the app port. The API should stay internal to Docker Compose.

Example Caddy route:

```caddyfile
anime-upscaling.example.com {
  reverse_proxy 127.0.0.1:4750
}
```

Use HTTPS whenever the app is reachable outside localhost or a VPN.

## 6. Upgrades

```bash
git pull
docker compose pull
docker compose up -d --build
```

When using published Docker Hub images, update `IMAGE_TAG`, then run:

```bash
docker compose -f docker-compose.hub.yml pull
docker compose -f docker-compose.hub.yml up -d
```

Back up the media directory before upgrades if it contains pipelines, runtime
settings, or files you cannot recreate.

## 7. Troubleshooting

If the app cannot reach the API, check that `API_URL=http://api:4751` is set in
`.env` for Docker Compose.

If GPU jobs fail immediately, confirm that the NVIDIA overlay is used and that
`docker run --gpus all ... nvidia-smi` works on the host.

If generated files are owned by an unexpected user, check the permissions on
`HOST_PROCESS_DIR` and run Docker with a user/group that can write there.

If you change ports, restart the stack after editing `.env`.
