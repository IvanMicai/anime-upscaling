## Batch Anime Upscaler (Real-ESRGAN + CUDA)

Batch upscaler for anime videos using [Real-ESRGAN](https://github.com/xinntao/Real-ESRGAN) with NVIDIA CUDA. Supports multiple GPUs with automatic scheduling.

### Quick Start

Build:

```bash
docker build --build-arg HOST_UID=$(id -u) --build-arg HOST_GID=$(id -g) -t anime-upscaler:latest .
```

Put your videos in `./input/`, create `./output/` and `./models/`, then run:

```bash
./run.sh
```

Or with docker compose:

```bash
HOST_UID=$(id -u) HOST_GID=$(id -g) docker compose up
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MODEL` | `realesr-animevideov3` | Model name (see below) |
| `SCALE` | `4` | Upscale factor (2, 3, 4) |
| `TILE` | `0` | Tile size (0=no tiling, 512/1024 for low VRAM GPUs) |
| `DENOISE` | `1.0` | Denoise strength (0.0-1.0) |
| `NUM_PROC` | `1` | Parallel segments per GPU |

### Models

- `realesr-animevideov3` (default) - Best for anime video, good balance of quality and speed
- `RealESRGAN_x4plus` - Highest quality, much slower
- `realesr-general-x4v3` - General purpose, faster but lower quality
- `RealESRGAN_x4plus_anime_6B` - Anime-specific, alternative to animevideov3

Models are downloaded automatically on first run. Map `./models` to persist them across container restarts.

### Examples

Default (4x anime upscale):

```bash
./run.sh
```

Custom model and scale:

```bash
MODEL=RealESRGAN_x4plus SCALE=4 ./run.sh
```

Low VRAM GPU (use tiling):

```bash
TILE=512 ./run.sh
```

Direct docker run:

```bash
docker run --gpus all --rm \
  -v /path/to/videos_in:/input \
  -v /path/to/videos_out:/output \
  -v /path/to/models:/opt/Real-ESRGAN/weights \
  -e HOST_UID=$(id -u) -e HOST_GID=$(id -g) \
  anime-upscaler:latest
```

### Features

- Batch processes all videos in input directory
- Auto-detects and schedules across multiple NVIDIA GPUs
- Skips already-processed files on re-run (idempotent)
- Remuxes audio and subtitles from original via ffmpeg
- Handles filenames with spaces and special characters
- Output file ownership matches host user (not root)
