FROM nvidia/cuda:12.8.1-cudnn-devel-ubuntu24.04

ENV DEBIAN_FRONTEND=noninteractive \
    NVIDIA_VISIBLE_DEVICES=all \
    NVIDIA_DRIVER_CAPABILITIES=compute,utility \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

# Accept UID/GID as build args
ARG HOST_UID=1000
ARG HOST_GID=1000

# Install OS dependencies
RUN apt-get update && apt-get install -y \
    python3 python3-pip python3-venv git ffmpeg libgl1 wget && \
    rm -rf /var/lib/apt/lists/*

# Python venv
ENV VENV=/opt/venv
RUN python3 -m venv $VENV
ENV PATH="$VENV/bin:$PATH" \
    PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1 PIP_ROOT_USER_ACTION=ignore

# PyTorch + libs (CUDA 12.4 wheels)
RUN pip install --upgrade pip && \
    pip install --index-url https://download.pytorch.org/whl/cu129 \
        torch==2.8.0+cu129 torchvision==0.23.0+cu129 torchaudio==2.8.0+cu129 && \
    pip install git+https://github.com/XPixelGroup/BasicSR.git && \
    pip install numpy facexlib gfpgan opencv-python ffmpeg-python

# Clone and install Real-ESRGAN from source
RUN git clone https://github.com/xinntao/Real-ESRGAN.git /opt/Real-ESRGAN

COPY nb_frames.patch /opt/Real-ESRGAN

RUN cd /opt/Real-ESRGAN && git apply nb_frames.patch && \
    pip3 install --no-cache-dir -r requirements.txt --no-deps && \
    python3 setup.py develop

# Declare weights directory as external volume
VOLUME ["/opt/Real-ESRGAN/weights"]
RUN mkdir -p /opt/Real-ESRGAN/weights

# Copy entrypoint script
COPY batch_upscale.sh /usr/local/bin/batch_upscale.sh
RUN chmod +x /usr/local/bin/batch_upscale.sh

# Create non-root user matching host UID/GID
RUN groupadd -g $HOST_GID hostgroup && \
    useradd -m -u $HOST_UID -g $HOST_GID hostuser && \
    chown -R hostuser:hostgroup /opt/Real-ESRGAN

USER hostuser
WORKDIR /data

ENTRYPOINT ["batch_upscale.sh"]
