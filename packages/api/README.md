# Anime Upscaling API

Go HTTP API for managing video upscaling and optimization jobs using Docker containers (video2x, ffmpeg).

**Base URL:** `http://localhost:4751`

## Configuration

| Setting | Value |
|---------|-------|
| Port | `4751` |
| Base directory | `/mnt/SSD2/process` |
| Input directory | `{BaseDir}/input` |
| Output directory | `{BaseDir}/output` |
| Optimized directory | `{BaseDir}/optimized` |
| Supported extensions | `.mkv`, `.mp4`, `.avi` |
| CORS | All origins (`*`), methods `GET, POST, OPTIONS` |

## Endpoints

### GET /api/files

List video files in a directory.

**Query Parameters:**

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `dir` | string | `"input"` | One of `input`, `output`, `optimized` |

**Response 200:**

```json
{
  "dir": "input",
  "files": ["video1.mkv", "video2.mp4"]
}
```

**Response 400:**

```json
{ "error": "invalid dir: must be input, output, or optimized" }
```

**Example:**

```bash
curl 'http://localhost:4751/api/files?dir=input'
```

---

### GET /api/jobs

List all jobs.

**Response 200:**

```json
[
  {
    "id": "j_1708540800_1a2b",
    "type": "upscale",
    "status": "running",
    "files": ["video1.mkv"],
    "progress": {
      "total": 1,
      "completed": 0,
      "failed": 0,
      "skipped": 0,
      "current": "Processing video1.mkv"
    },
    "created_at": "2024-02-21T12:00:00Z",
    "finished_at": null
  }
]
```

**Example:**

```bash
curl http://localhost:4751/api/jobs
```

---

### POST /api/jobs

Create a new job.

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"upscale"`, `"optimize"`, or `"pipeline"` |
| `files` | string[] | no | Filenames from `input/`. If empty, uses all videos in `input/` |

```json
{
  "type": "upscale",
  "files": ["video1.mkv", "video2.mp4"]
}
```

**Response 201:**

```json
{
  "id": "j_1708540800_1a2b",
  "type": "upscale",
  "status": "running",
  "files": ["video1.mkv", "video2.mp4"]
}
```

**Response 400:**

```json
{ "error": "type must be upscale, optimize, or pipeline" }
```

```json
{ "error": "no video files found in input/" }
```

```json
{ "error": "file not found in input/: video1.mkv" }
```

**Examples:**

```bash
# Upscale specific files
curl -X POST http://localhost:4751/api/jobs \
  -H 'Content-Type: application/json' \
  -d '{"type": "upscale", "files": ["video1.mkv"]}'

# Optimize all files in input/
curl -X POST http://localhost:4751/api/jobs \
  -H 'Content-Type: application/json' \
  -d '{"type": "optimize"}'

# Full pipeline (upscale + optimize)
curl -X POST http://localhost:4751/api/jobs \
  -H 'Content-Type: application/json' \
  -d '{"type": "pipeline", "files": ["video1.mkv", "video2.mp4"]}'
```

---

### GET /api/jobs/{id}

Get job details.

**Path Parameters:**

| Param | Type | Description |
|-------|------|-------------|
| `id` | string | Job ID (e.g. `j_1708540800_1a2b`) |

**Response 200:**

```json
{
  "id": "j_1708540800_1a2b",
  "type": "upscale",
  "status": "completed",
  "files": ["video1.mkv"],
  "progress": {
    "total": 1,
    "completed": 1,
    "failed": 0,
    "skipped": 0,
    "current": ""
  },
  "created_at": "2024-02-21T12:00:00Z",
  "finished_at": "2024-02-21T12:10:00Z"
}
```

**Response 404:**

```json
{ "error": "job not found" }
```

**Example:**

```bash
curl http://localhost:4751/api/jobs/j_1708540800_1a2b
```

---

### GET /api/jobs/{id}/logs

Stream job logs via Server-Sent Events. Sends the full log history on connect, then streams new entries in real-time. The connection closes automatically when the job finishes.

**Path Parameters:**

| Param | Type | Description |
|-------|------|-------------|
| `id` | string | Job ID |

**Response Headers:**

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

**SSE Event Format:**

Each `data:` line is a JSON log entry:

```json
{
  "source": "GPU 0",
  "level": "INFO",
  "index": 1,
  "message": "Iniciando: video1.mkv",
  "time": "2024-02-21T12:00:05Z"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `source` | string | Worker identifier (`"GPU 0"`, `"GPU 1"`, `"FFMPEG"`) |
| `level` | string | Log level (see table below) |
| `index` | int | File index in the job |
| `message` | string | Log message |
| `time` | string | ISO 8601 timestamp |

**Response 404:**

```json
{ "error": "job not found" }
```

**Example:**

```bash
curl -N http://localhost:4751/api/jobs/j_1708540800_1a2b/logs
```

---

### POST /api/jobs/{id}/cancel

Cancel a running job.

**Path Parameters:**

| Param | Type | Description |
|-------|------|-------------|
| `id` | string | Job ID |

**Response 200:**

```json
{
  "id": "j_1708540800_1a2b",
  "status": "cancelled"
}
```

**Response 404:**

```json
{ "error": "job not found" }
```

**Example:**

```bash
curl -X POST http://localhost:4751/api/jobs/j_1708540800_1a2b/cancel
```

---

## Job Types

| Type | Description | Workers |
|------|-------------|---------|
| `upscale` | 2x upscale using video2x (RealESRGAN) | 2 GPU workers in parallel |
| `optimize` | H.265 compression using ffmpeg | 1 CPU worker, sequential |
| `pipeline` | Upscale then optimize in a streaming pipeline | 2 GPU + 1 CPU in parallel |

### Upscale

- Docker image: `ghcr.io/k4yt3x/video2x:6.4.0`
- Model: `realesr-animevideov3`
- Scale: 2x
- Output: `{BaseDir}/output/`
- Skips files that already exist in output

### Optimize

- Docker image: `linuxserver/ffmpeg`
- Codec: `libx265` (HEVC)
- Preset: `fast`, tune: `animation`
- CRF: 19, pixel format: `yuv420p10le` (10-bit)
- Copies audio and subtitles as-is
- Output: `{BaseDir}/optimized/`
- CPUs: half of available cores

### Pipeline

- Combines upscale and optimize in a streaming pipeline
- GPU workers feed completed files to the FFmpeg worker via channel
- FFmpeg CRF: 22 (slightly lower quality for speed)
- Does not copy subtitles in pipeline mode

## Job Statuses

| Status | Description |
|--------|-------------|
| `running` | Job is currently executing |
| `completed` | All files processed successfully |
| `failed` | An error occurred during processing |
| `cancelled` | Job was manually cancelled |

## Log Levels

| Level | Meaning | Effect on Progress |
|-------|---------|--------------------|
| `INFO` | Informational | Updates `progress.current` |
| `OK` | File completed | Increments `progress.completed` |
| `ERRO` | File failed | Increments `progress.failed` |
| `SKIP` | File skipped (already exists) | Increments `progress.skipped` |
| `WARN` | Warning | None |

## Error Response Format

All errors return JSON:

```json
{ "error": "description of the error" }
```

| Status Code | Meaning |
|-------------|---------|
| 200 | Success |
| 201 | Job created |
| 204 | CORS preflight |
| 400 | Invalid request |
| 404 | Job not found |
| 405 | Method not allowed |
| 500 | Internal server error |
