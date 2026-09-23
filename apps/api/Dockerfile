# Arch Linux base for the video2x stages below (declared here: FROM needs it global).
ARG ARCH_TAG=base-20260315.0.500537

FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /api ./cmd/animeup

# ---- ffmpeg: overlay an up-to-date static build (GPL: includes libx265 + nvenc) ----
# The distro ffmpeg that video2x links against (and that the 6.4.0 image
# shipped) had a libx265 prone to thread-pool SIGSEGVs on some inputs, and we
# want the encoder pinned independently of the Arch snapshot. We fetch a current
# static build and copy it to /usr/local/bin, which precedes /usr/bin on PATH,
# so the API's `ffmpeg`
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

# ---- video2x: built from source at a pinned upstream commit ----
# The last video2x release (6.4.0) predates the Real-ESRGAN general models
# (realesr-generalv3 and its -wdn denoise variant), so we build a pinned master
# commit instead of using the prebuilt image. This mirrors upstream's own
# packaging/docker/Dockerfile (Arch Linux, system ncnn/spdlog/boost), so the
# runtime (Vulkan ICDs, nvidia-utils) matches the 6.4.0 image we used before.
# Packages come from a dated Arch Linux Archive snapshot so rebuilds are
# reproducible. Bump VIDEO2X_REF, ARCH_TAG (top of file) and ARCH_SNAPSHOT together.

FROM archlinux:${ARCH_TAG} AS arch
ARG ARCH_SNAPSHOT=2026/03/15
RUN echo "Server = https://archive.archlinux.org/repos/${ARCH_SNAPSHOT}/\$repo/os/\$arch" \
      > /etc/pacman.d/mirrorlist

FROM arch AS video2x
ARG VIDEO2X_REF=7db9c18d6278bbad9c3eda0e4e4ae210f9a688eb
RUN pacman -Syyuu --noconfirm --needed \
        base-devel git cmake clang pkgconf \
        ffmpeg ncnn vulkan-headers openmp spdlog boost \
 && rm -rf /var/cache/pacman/pkg/*
RUN git clone https://github.com/k4yt3x/video2x.git /src \
 && cd /src \
 && git checkout "$VIDEO2X_REF" \
 && git submodule update --init --recursive \
        third_party/librealesrgan_ncnn_vulkan \
        third_party/librealcugan_ncnn_vulkan \
        third_party/librife_ncnn_vulkan \
 && cmake -B build -S . \
        -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_INSTALL_PREFIX=/usr \
        -DCMAKE_CXX_COMPILER=clang++ \
 && cmake --build build --config Release --parallel \
 && DESTDIR=/out cmake --install build \
 && test -f /out/usr/share/video2x/models/realesrgan/realesr-generalv3-x4.bin

FROM arch
ENV VK_ICD_FILENAMES=/usr/share/vulkan/icd.d/nvidia_icd.json\
:/usr/share/vulkan/icd.d/radeon_icd.x86_64.json\
:/usr/share/vulkan/icd.d/intel_icd.x86_64.json\
:/usr/share/vulkan/icd.d/lvp_icd.x86_64.json
RUN pacman -Syyuu --noconfirm --needed \
        nvidia-utils vulkan-radeon vulkan-intel vulkan-swrast \
        ffmpeg ncnn openmp spdlog boost-libs \
 && rm -rf /var/cache/pacman/pkg/*
COPY --from=video2x /out/usr/ /usr/
RUN video2x --version
WORKDIR /host
COPY --from=builder /api /usr/local/bin/api
COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /usr/local/bin/ffmpeg
COPY --from=ffmpeg /opt/ffmpeg/bin/ffprobe /usr/local/bin/ffprobe
EXPOSE 4751
ENTRYPOINT ["/usr/local/bin/api", "serve"]
