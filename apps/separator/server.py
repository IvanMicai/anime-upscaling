"""Voice separator for dual-audio merges.

POST /separate with a WAV body (16-bit PCM, any rate, mono or stereo) returns
the same audio without voices, as a WAV at the input's rate and channel count.
GET /health reports the device and model.

The API sends the decoded tracks of both languages here and aligns and
validates on what comes back: music, effects and ambience are shared between
dubs, dialogue never is. The merged file itself still uses the original audio.

Demucs (htdemucs, MIT) runs one request at a time; the model is baked into the
image at build time, so nothing is downloaded at runtime. The service has no
authentication: like the API it must only be reachable on the Compose network.
"""

import io
import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import julius
import numpy as np
import soundfile as sf
import torch
from demucs.apply import apply_model
from demucs.pretrained import get_model

MODEL_NAME = os.environ.get("SEPARATOR_MODEL", "htdemucs")
PORT = int(os.environ.get("SEPARATOR_PORT", "4752"))
MAX_BYTES = int(os.environ.get("SEPARATOR_MAX_BYTES", str(1 << 30)))
DEVICE = "cuda" if torch.cuda.is_available() else "cpu"

model = get_model(MODEL_NAME)
model.to(DEVICE).eval()
lock = threading.Lock()


def separate(wav_bytes: bytes) -> bytes:
    audio, rate = sf.read(io.BytesIO(wav_bytes), dtype="float32", always_2d=True)
    channels = audio.shape[1]
    x = torch.from_numpy(audio.T.copy())  # (channels, samples)
    if channels == 1:
        x = x.repeat(2, 1)
    elif channels > 2:
        x = x[:2]
    if rate != model.samplerate:
        x = julius.resample_frac(x, rate, model.samplerate)
    # Same normalisation as the demucs CLI.
    ref = x.mean(0)
    mean, std = ref.mean(), ref.std() + 1e-8
    x = (x - mean) / std
    with torch.no_grad():
        # shifts=0: no random time shift, so the same track always separates the
        # same way. With the CLI's default of 1 the verdict of one episode
        # changed between runs.
        torch.manual_seed(0)
        sources = apply_model(model, x[None], device=DEVICE, shifts=0, split=True,
                              overlap=0.25, progress=False)[0]
    del x
    vocals = model.sources.index("vocals")
    # no_vocals = mixture minus vocals, summed in place to keep one copy.
    out = sources.sum(0)
    out -= sources[vocals]
    del sources
    out = (out * std + mean * (len(model.sources) - 1)).cpu()
    if DEVICE == "cuda":
        torch.cuda.empty_cache()
    if rate != model.samplerate:
        out = julius.resample_frac(out, model.samplerate, rate)
    n = audio.shape[0]
    out = out[:, :n]
    if out.shape[1] < n:  # resampling rounding: keep the exact length
        out = torch.nn.functional.pad(out, (0, n - out.shape[1]))
    if channels == 1:
        out = out.mean(0, keepdim=True)
    buf = io.BytesIO()
    sf.write(buf, np.clip(out.numpy().T, -1, 1), rate, subtype="PCM_16", format="WAV")
    return buf.getvalue()


class Handler(BaseHTTPRequestHandler):
    def _reply(self, code, body, ctype="application/json"):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path != "/health":
            return self._reply(404, b'{"error":"not found"}')
        body = {"ok": True, "device": DEVICE, "model": MODEL_NAME}
        self._reply(200, json.dumps(body).encode())

    def do_POST(self):
        if self.path != "/separate":
            return self._reply(404, b'{"error":"not found"}')
        n = int(self.headers.get("Content-Length") or 0)
        if n <= 0 or n > MAX_BYTES:
            return self._reply(413, b'{"error":"body missing or too large"}')
        # One request at a time from the body on: a 22-minute track is ~250 MB
        # as WAV and several GB as tensors, and two at once ran the container
        # out of memory. Others wait here; /health keeps answering.
        with lock:
            try:
                out = separate(self.rfile.read(n))
            except Exception as e:  # report, do not crash the service
                return self._reply(500, json.dumps({"error": str(e)}).encode())
        self._reply(200, out, "audio/wav")

    def log_message(self, fmt, *args):
        print("separator:", fmt % args, flush=True)


if __name__ == "__main__":
    print(f"separator: {MODEL_NAME} on {DEVICE}, port {PORT}", flush=True)
    ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
