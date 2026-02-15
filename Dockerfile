FROM ghcr.io/k4yt3x/video2x:latest

# Base = Arch Linux com /usr/bin/video2x, ffmpeg, Vulkan, modelos embutidos
# WORKDIR original = /host, ENTRYPOINT original = /usr/bin/video2x

RUN pacman -Sy --noconfirm bash findutils && \
    rm -rf /var/cache/pacman/pkg/*

COPY batch_upscale.sh /usr/local/bin/batch_upscale.sh
RUN chmod +x /usr/local/bin/batch_upscale.sh

ENTRYPOINT ["/usr/local/bin/batch_upscale.sh"]
