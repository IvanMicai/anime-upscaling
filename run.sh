# GPU tuning:
#  -e TILE=0         # 0=sem tiling (default), 512/1024 para GPUs com pouca VRAM
#  -e NUM_PROC=1     # segmentos paralelos por GPU (default 1)
docker run --gpus all --rm \
  -v ./input:/input \
  -v ./output:/output \
  -e HOST_UID=$(id -u) -e HOST_GID=$(id -g) \
  anime-upscaler:latest
