# Arch Linux base for the video2x stages below (declared here: FROM needs it global).
ARG ARCH_TAG=base-20260315.0.500537

FROM golang:1.27-alpine AS builder
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
# Vulkan runtime matches the 6.4.0 image we used before.
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
 && sed -i 's|STR("realesr-generalv3"),|&\n        STR("nomos8ksc"), STR("lsdirplusc"), STR("nmkd-siax"), STR("hfa2k"),|' \
        tools/video2x/src/validators.cpp \
 && grep -q 'STR("nmkd-siax")' tools/video2x/src/validators.cpp \
 && cmake -B build -S . \
        -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_INSTALL_PREFIX=/usr \
        -DCMAKE_CXX_COMPILER=clang++ \
 && cmake --build build --config Release --parallel \
 && DESTDIR=/out cmake --install build \
 && test -f /out/usr/share/video2x/models/realesrgan/realesr-generalv3-x4.bin

# ---- extra Real-ESRGAN (RRDBNet) models for live action and anime films ----
# ncnn conversions from upscayl/custom-models (pinned commit, checksummed),
# renamed to video2x's <model>-x<scale> layout. The sed above adds the same
# names to video2x's model allow-list. Licenses: Nomos8kSC, LSDIRplusC and
# HFA2k by Phhofm are CC-BY-4.0; NMKD Siax is WTFPL (credited in the README).
ARG CUSTOM_MODELS_REF=4b6d2cfa59c7442af115dfc6e50fd8d7d40b96ef
RUN set -e; dest=/out/usr/share/video2x/models/realesrgan; \
    base="https://raw.githubusercontent.com/upscayl/custom-models/${CUSTOM_MODELS_REF}/models"; \
    while read -r name src sum_param sum_bin; do \
      curl -fsSL "$base/$src.param" -o "$dest/$name-x4.param"; \
      curl -fsSL "$base/$src.bin" -o "$dest/$name-x4.bin"; \
      echo "$sum_param  $dest/$name-x4.param" | sha256sum -c -; \
      echo "$sum_bin  $dest/$name-x4.bin" | sha256sum -c -; \
    done <<'EOF'
nomos8ksc 4xNomos8kSC 4576ed5c2fc5fa250d3c3d585ef02248f26abdfc1867088078f501fe71e5d61e da16e3880d87b177b7c6b659bbd880f8a101b868eb9ebc08d69eaa6d3edc4517
lsdirplusc 4xLSDIRplusC 4576ed5c2fc5fa250d3c3d585ef02248f26abdfc1867088078f501fe71e5d61e 6df18c714bb7d4ac63ca8a1fb1f1b2712fe3ef5a4b36f09bfa9b827da167ae81
nmkd-siax 4x_NMKD-Siax_200k 519bffb4912e295155d3cb5d6a29639d217ccca55f80962d8f677ef777a65b7a b2abdffa30fb15be752ac681e0c0edf8b543745c33a989aadc9d7d3394dd3110
hfa2k 4xHFA2k 4576ed5c2fc5fa250d3c3d585ef02248f26abdfc1867088078f501fe71e5d61e 8a135402b4f39286121b76abb47601a6b7b7e8d4f3e999a5aaa45ed277824fb4
EOF

# ---- seedvr2-build: the SeedVR2 runtime, for the opt-in seedvr2 image ----
# SeedVR2 (ByteDance, Apache-2.0) runs through the headless CLI of
# numz/ComfyUI-SeedVR2_VideoUpscaler, pinned below; the repo is dormant, so it
# is treated as frozen. Python comes from uv's standalone builds (the Arch
# snapshot's Python is newer than PyTorch 2.9 supports well), and the packages
# from seedvr2/requirements.txt, a lock compiled against the pinned commit's
# date (see seedvr2/requirements.in). The PyTorch wheels bundle the CUDA 12.8
# runtime, which has Blackwell (sm_120) kernels; the driver comes from the
# host through the NVIDIA Container Toolkit. Weights are not baked in: they
# download on first use into $PROCESS_DIR/models/seedvr2.
FROM arch AS seedvr2-build
ARG SEEDVR2_REF=4490bd1f482e026674543386bb2a4d176da245b9
COPY --from=ghcr.io/astral-sh/uv:0.12.19 /uv /usr/local/bin/uv
ENV UV_PYTHON_INSTALL_DIR=/opt/seedvr2/python UV_NO_CACHE=1 UV_LINK_MODE=copy
RUN pacman -Syyuu --noconfirm --needed git \
 && rm -rf /var/cache/pacman/pkg/*
RUN git clone https://github.com/numz/ComfyUI-SeedVR2_VideoUpscaler.git /opt/seedvr2/app \
 && git -C /opt/seedvr2/app checkout "$SEEDVR2_REF" \
 && rm -rf /opt/seedvr2/app/.git /opt/seedvr2/app/docs /opt/seedvr2/app/example_workflows
COPY seedvr2/requirements.txt /tmp/seedvr2-requirements.txt
RUN uv venv --python 3.12 /opt/seedvr2/venv \
 && VIRTUAL_ENV=/opt/seedvr2/venv uv pip install --no-deps -r /tmp/seedvr2-requirements.txt \
        --index-url https://download.pytorch.org/whl/cu128 \
        --extra-index-url https://pypi.org/simple --index-strategy unsafe-best-match \
 && /opt/seedvr2/venv/bin/python -c "import torch; assert 'sm_120' in torch._C._cuda_getArchFlags(), torch._C._cuda_getArchFlags(); print('torch', torch.__version__, torch.version.cuda)" \
 && cd /opt/seedvr2/app && /opt/seedvr2/venv/bin/python inference_cli.py --help > /dev/null

# ---- runtime: what both API images share ----
FROM arch AS runtime
# Arch installs the Mesa manifests without the .x86_64 suffix Debian/Ubuntu use.
ENV VK_ICD_FILENAMES=/usr/share/vulkan/icd.d/nvidia_icd.json\
:/usr/share/vulkan/icd.d/radeon_icd.json\
:/usr/share/vulkan/icd.d/intel_icd.json\
:/usr/share/vulkan/icd.d/lvp_icd.json
# nvidia-utils is deliberately NOT installed: the NVIDIA Container Toolkit
# mounts the host driver's userspace libs, and ldconfig points
# libGLX_nvidia.so.0 at the highest version present. A packaged nvidia-utils
# newer than the host driver would win and Vulkan would fail to create an
# instance. We only ship the ICD manifest that nvidia-utils used to provide.
RUN pacman -Syyuu --noconfirm --needed \
        libglvnd vulkan-icd-loader vulkan-radeon vulkan-intel vulkan-swrast \
        ffmpeg ncnn openmp spdlog boost-libs \
 && rm -rf /var/cache/pacman/pkg/* \
 && printf '%s\n' '{' \
      '    "file_format_version" : "1.0.1",' \
      '    "ICD": {' \
      '        "library_path": "libGLX_nvidia.so.0",' \
      '        "api_version" : "1.3.0"' \
      '    }' \
      '}' > /usr/share/vulkan/icd.d/nvidia_icd.json
COPY --from=video2x /out/usr/ /usr/
RUN video2x --version
WORKDIR /host
EXPOSE 4751
ENTRYPOINT ["/usr/local/bin/api", "serve"]

# ---- seedvr2: the API image plus SeedVR2 (docker build --target seedvr2) ----
# Several GB larger, and only useful with an NVIDIA GPU. The runtime goes in
# before the API binary so a Go-only change does not rebuild it.
FROM runtime AS seedvr2
COPY --from=seedvr2-build /opt/seedvr2 /opt/seedvr2
COPY --from=builder /api /usr/local/bin/api
COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /usr/local/bin/ffmpeg
COPY --from=ffmpeg /opt/ffmpeg/bin/ffprobe /usr/local/bin/ffprobe

# ---- api: the default image (last stage, so a plain `docker build` gets it) ----
FROM runtime AS api
COPY --from=builder /api /usr/local/bin/api
COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /usr/local/bin/ffmpeg
COPY --from=ffmpeg /opt/ffmpeg/bin/ffprobe /usr/local/bin/ffprobe
