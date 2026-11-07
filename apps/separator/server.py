"""Voice separator for dual-audio merges.

POST /separate with a WAV body (16-bit PCM, any rate, mono or stereo) returns
the same audio without voices, as a WAV at the input's rate and channel count.
GET /health reports whether the model is loaded.

The API sends the decoded tracks of both languages here and aligns and
validates on what comes back: music, effects and ambience are shared between
dubs, dialogue never is. The merged file itself still uses the original audio.

The service runs all the time but holds no model while idle. This process is
the standby server: plain Python, no PyTorch, a few MB of RAM, no GPU memory.
The first request starts a worker process (worker.py) that loads Demucs
(htdemucs, MIT) onto the GPU; it stays up while requests keep coming and is
stopped after SEPARATOR_IDLE_SECONDS without one, which frees its RAM and VRAM
completely. Requests run one at a time.

The model is baked into the image at build time, so nothing is downloaded at
runtime. The service has no authentication: like the API it must only be
reachable on the Compose network.
"""

import json
import multiprocessing as mp
import os
import shutil
import signal
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import worker

MODEL_NAME = os.environ.get("SEPARATOR_MODEL", "htdemucs")
PORT = int(os.environ.get("SEPARATOR_PORT", "4752"))
MAX_BYTES = int(os.environ.get("SEPARATOR_MAX_BYTES", str(1 << 30)))
# Long enough to cover the gaps inside one merge (both tracks, then the check of
# the rendered track) and between episodes of a batch; loading takes seconds.
IDLE_SECONDS = float(os.environ.get("SEPARATOR_IDLE_SECONDS", "300"))
CHUNK = 1 << 20


def log(msg):
    print("separator:", msg, flush=True)


class Worker:
    """The single separation process, started on demand and stopped when idle."""

    def __init__(self, idle_seconds, target=worker.main):
        self.idle = idle_seconds
        self.target = target
        self.lock = threading.Lock()  # one job at a time; guards proc and conn
        self.proc = None
        self.conn = None
        self.device = None
        self.last_used = 0.0
        threading.Thread(target=self._reaper, daemon=True).start()

    def loaded(self):
        return self.proc is not None

    def run(self, in_path, out_path):
        with self.lock:
            try:
                if self.proc is None:
                    self._start()
                self.conn.send((in_path, out_path))
                status, value = self.conn.recv()
            except (EOFError, OSError) as e:
                # The worker died, most likely killed for memory. Start a fresh
                # one on the next request.
                self._stop()
                raise RuntimeError(f"separation worker stopped unexpectedly ({e!r})") from e
            finally:
                self.last_used = time.monotonic()
            if status != "ok":
                raise RuntimeError(value)

    def _start(self):
        t0 = time.monotonic()
        parent, child = mp.get_context("spawn").Pipe()
        proc = mp.get_context("spawn").Process(
            target=self.target, args=(child, MODEL_NAME), daemon=True)
        proc.start()
        child.close()  # so recv() raises EOFError if the worker dies
        self.proc, self.conn = proc, parent
        _, self.device = parent.recv()  # ("ready", device)
        log(f"{MODEL_NAME} loaded on {self.device} in {time.monotonic() - t0:.1f} s")

    def _stop(self):
        proc, conn = self.proc, self.conn
        self.proc = self.conn = self.device = None
        if conn is not None:
            try:
                conn.send(None)
            except OSError:
                pass
            conn.close()
        if proc is not None:
            proc.join(10)
            if proc.is_alive():
                proc.kill()
                proc.join()

    def _reaper(self):
        while True:
            time.sleep(max(1.0, min(30.0, self.idle / 4)))
            # Never wait behind a running job; check again on the next tick.
            if not self.lock.acquire(blocking=False):
                continue
            try:
                if self.proc is not None and time.monotonic() - self.last_used >= self.idle:
                    self._stop()
                    log(f"idle for {self.idle:.0f} s: model unloaded, back to standby")
            finally:
                self.lock.release()


WORKER = None  # created in main(): spawned workers re-import this module


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
        body = {
            "ok": True,
            "model": MODEL_NAME,
            "state": "loaded" if WORKER.loaded() else "standby",
            "device": WORKER.device,
            "idle_seconds": IDLE_SECONDS,
        }
        self._reply(200, json.dumps(body).encode())

    def do_POST(self):
        if self.path != "/separate":
            return self._reply(404, b'{"error":"not found"}')
        n = int(self.headers.get("Content-Length") or 0)
        if n <= 0 or n > MAX_BYTES:
            return self._reply(413, b'{"error":"body missing or too large"}')
        # Spool through files: a 22-minute track is ~250 MB as WAV, and the
        # standby server should not hold it in memory.
        tmp = tempfile.mkdtemp(prefix="separate-")
        try:
            in_path = os.path.join(tmp, "in.wav")
            out_path = os.path.join(tmp, "out.wav")
            with open(in_path, "wb") as f:
                left = n
                while left > 0:
                    chunk = self.rfile.read(min(CHUNK, left))
                    if not chunk:
                        return self._reply(400, b'{"error":"body ended early"}')
                    f.write(chunk)
                    left -= len(chunk)
            try:
                WORKER.run(in_path, out_path)
            except Exception as e:  # report, do not crash the service
                return self._reply(500, json.dumps({"error": str(e)}).encode())
            self.send_response(200)
            self.send_header("Content-Type", "audio/wav")
            self.send_header("Content-Length", str(os.path.getsize(out_path)))
            self.end_headers()
            with open(out_path, "rb") as f:
                shutil.copyfileobj(f, self.wfile, CHUNK)
        finally:
            shutil.rmtree(tmp, ignore_errors=True)

    def log_message(self, fmt, *args):
        if self.path != "/health":  # the healthcheck polls every 30 s
            log(fmt % args)


def main():
    # As PID 1 in a container Python gets no default SIGTERM handler; exit
    # cleanly so `docker stop` does not wait for the kill timeout.
    global WORKER
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
    WORKER = Worker(IDLE_SECONDS)
    server = ThreadingHTTPServer(("0.0.0.0", PORT), Handler)
    log(f"standby on port {PORT}: {MODEL_NAME} loads on the first request, "
        f"unloads after {IDLE_SECONDS:.0f} s idle")
    try:
        server.serve_forever()
    finally:
        WORKER._stop()


if __name__ == "__main__":
    main()
