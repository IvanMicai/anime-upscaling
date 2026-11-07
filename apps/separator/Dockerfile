# Voice separator for dual-audio merges (Demucs htdemucs on PyTorch + CUDA 12.8).
# Optional service: see docker-compose.separator.yml. Runs on CPU too, slowly.
FROM pytorch/pytorch:2.8.0-cuda12.8-cudnn9-runtime

# demucs 4.0.1 is the last release with the classic API; its own requirements
# pin an older torch, so it is installed without dependencies and the few it
# needs are listed explicitly. torchaudio matches the base image's torch.
RUN pip install --no-cache-dir torchaudio==2.8.0 --index-url https://download.pytorch.org/whl/cu128 \
 && pip install --no-cache-dir --no-deps demucs==4.0.1 \
 && pip install --no-cache-dir julius==0.2.7 einops==0.8.1 openunmix==1.3.0 \
      dora-search==0.1.12 omegaconf==2.3.0 antlr4-python3-runtime==4.9.3 treetable==0.2.5 \
      lameenc==1.8.1 retrying==1.4.2 submitit==1.5.3 cloudpickle==3.1.1 pyyaml==6.0.2 \
      soundfile==0.13.1 numpy==2.2.6

# Bake the model into the image: nothing is downloaded at runtime.
ENV TORCH_HOME=/opt/models
RUN python -c "from demucs.pretrained import get_model; get_model('htdemucs')"

WORKDIR /app
COPY server.py .
EXPOSE 4752
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:4752/health')"
CMD ["python", "server.py"]
