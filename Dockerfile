FROM ghcr.io/k4yt3x/video2x:latest

# Base = Arch Linux com /usr/bin/video2x, ffmpeg, Vulkan, modelos embutidos
# bash e findutils ja incluidos no Arch base

COPY --chmod=755 batch_upscale.sh /usr/local/bin/batch_upscale.sh

ENTRYPOINT ["/usr/local/bin/batch_upscale.sh"]
