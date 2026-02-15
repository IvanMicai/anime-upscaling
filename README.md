## Batch Video Upscaler (Video2X)

Batch upscaler for anime videos using [Video2X](https://github.com/k4yt3x/video2x) official Docker image. Supports multiple GPUs with automatic scheduling, and processes all videos in an input directory.

Based on `ghcr.io/k4yt3x/video2x:latest` which includes Real-ESRGAN, Real-CUGAN, libplacebo (Anime4K), and RIFE — all using Vulkan (works with NVIDIA, AMD, and Intel GPUs).

### Quick Start

Build:

```bash
docker build -t batch-video2x .
```

Put your videos in `./input/`, then run:

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
| `PROCESSOR` | `realesrgan` | `realesrgan`, `libplacebo`, `realcugan`, `rife` |
| `SCALE` | `4` | Upscale factor (2, 3, 4) |
| `MODEL` | *(auto)* | Processor-specific model (see below) |
| `CODEC` | `libx265` | Output codec (`libx264`, `libx265`, `libsvtav1`, `hevc_nvenc`) |
| `OUTPUT_EXT` | `mkv` | Output file extension |
| `PIX_FMT` | *(auto)* | Pixel format (`yuv420p`, `yuv420p10le`) |
| `BIT_RATE` | `0` | Bitrate in bps (0 = auto/CRF) |
| `NOISE_LEVEL` | *(none)* | Denoising level (-1 to disable) |
| `NUM_GPUS` | `0` | Override GPU auto-detection (0 = auto) |
| `LOG_LEVEL` | `info` | `trace`, `debug`, `info`, `warn`, `error` |
| `HOST_UID` / `HOST_GID` | `0` | Set output file ownership to match host user |

### Models

**Real-ESRGAN** (default): `realesr-animevideov3` (default), `realesrgan-plus-anime`, `realesrgan-plus`

**Real-CUGAN**: `models-se` (default), `models-nose`, `models-pro`

**libplacebo**: `anime4k-v4-a` (default), `anime4k-v4-a+a`, `anime4k-v4-b`, `anime4k-v4-b+b`, `anime4k-v4-c`, `anime4k-v4-c+a`, `anime4k-v4.1-gan`

### Examples

Default (4x anime upscale with Real-ESRGAN):

```bash
./run.sh
```

Custom processor and scale:

```bash
PROCESSOR=libplacebo SCALE=2 MODEL=anime4k-v4.1-gan ./run.sh
```

With NVENC hardware encoding:

```bash
CODEC=hevc_nvenc ./run.sh
```

### Features

- Batch processes all videos in input directory
- Auto-detects and schedules across multiple GPUs
- Idempotent: skips already-processed files on re-run
- Audio and subtitles copied natively by Video2X
- Atomic writes (temp file + rename) to avoid partial outputs
- Output file ownership matches host user (not root)
