FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /api ./cmd/animeup

# ---- ffmpeg: overlay an up-to-date static build (GPL: includes libx265 + nvenc) ----
# The video2x base image bundles an older ffmpeg whose libx265 is prone to
# thread-pool SIGSEGVs on some inputs. We fetch a current static build and copy
# it to /usr/local/bin, which precedes /usr/bin on PATH, so the API's `ffmpeg`
# and `ffprobe` (looked up by name) resolve to the new binaries.
# Bump FFMPEG_VARIANT to change versions; the RUN prints `ffmpeg -version` into
# the build log so the installed release is visible. If the pinned release asset
# is missing, it falls back to the master build so the image still builds.
FROM debian:bookworm-slim AS ffmpeg
ARG FFMPEG_VARIANT=ffmpeg-n8.1-latest-linux64-gpl
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl xz-utils ca-certificates \
 && base=https://github.com/BtbN/FFmpeg-Builds/releases/download/latest \
 && (curl -fsSL "$base/${FFMPEG_VARIANT}.tar.xz" -o /tmp/ffmpeg.tar.xz \
     || curl -fsSL "$base/ffmpeg-master-latest-linux64-gpl.tar.xz" -o /tmp/ffmpeg.tar.xz) \
 && mkdir -p /opt/ffmpeg \
 && tar -xJf /tmp/ffmpeg.tar.xz -C /opt/ffmpeg --strip-components=1 \
 && /opt/ffmpeg/bin/ffmpeg -version \
 && rm -rf /var/lib/apt/lists/* /tmp/ffmpeg.tar.xz

FROM ghcr.io/k4yt3x/video2x:6.4.0
COPY --from=builder /api /usr/local/bin/api
COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /usr/local/bin/ffmpeg
COPY --from=ffmpeg /opt/ffmpeg/bin/ffprobe /usr/local/bin/ffprobe
EXPOSE 4751
ENTRYPOINT ["/usr/local/bin/api", "serve"]
