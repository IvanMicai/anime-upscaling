"""Tests for the standby server, with a fake worker instead of Demucs.

Run with: python3 -m unittest test_server   (no PyTorch needed)
"""

import json
import os
import shutil
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.request
from http.server import ThreadingHTTPServer

import server


def fake_worker(conn, model_name):
    """Copies the input to the output; 'boom' fails the job, 'die' kills the process."""
    conn.send(("ready", "fake"))
    while True:
        try:
            job = conn.recv()
        except EOFError:
            return
        if job is None:
            return
        in_path, out_path = job
        with open(in_path, "rb") as f:
            data = f.read()
        if data == b"die":
            os._exit(1)
        if data == b"boom":
            conn.send(("error", "boom"))
            continue
        shutil.copyfile(in_path, out_path)
        conn.send(("ok", None))


class ServerTest(unittest.TestCase):
    def setUp(self):
        server.WORKER = server.Worker(1.0, target=fake_worker)
        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), server.Handler)
        threading.Thread(target=self.httpd.serve_forever, daemon=True).start()
        self.url = f"http://127.0.0.1:{self.httpd.server_address[1]}"

    def tearDown(self):
        self.httpd.shutdown()
        self.httpd.server_close()
        server.WORKER._stop()

    def health(self):
        with urllib.request.urlopen(self.url + "/health") as r:
            return json.load(r)

    def separate(self, body):
        req = urllib.request.Request(self.url + "/separate", data=body, method="POST")
        with urllib.request.urlopen(req) as r:
            return r.read()

    def test_starts_in_standby_loads_on_demand_and_unloads_when_idle(self):
        self.assertEqual(self.health()["state"], "standby")
        self.assertEqual(self.separate(b"x" * 3_000_000), b"x" * 3_000_000)
        h = self.health()
        self.assertEqual((h["state"], h["device"]), ("loaded", "fake"))
        deadline = time.monotonic() + 10
        while self.health()["state"] != "standby":
            self.assertLess(time.monotonic(), deadline, "worker never unloaded")
            time.sleep(0.1)
        self.assertEqual(self.separate(b"again"), b"again")  # loads again

    def test_job_error_is_reported_and_worker_keeps_serving(self):
        with self.assertRaises(urllib.error.HTTPError) as e:
            self.separate(b"boom")
        self.assertEqual(e.exception.code, 500)
        self.assertIn("boom", e.exception.read().decode())
        self.assertEqual(self.separate(b"ok"), b"ok")

    def test_dead_worker_is_replaced_on_the_next_request(self):
        with self.assertRaises(urllib.error.HTTPError) as e:
            self.separate(b"die")
        self.assertEqual(e.exception.code, 500)
        self.assertEqual(self.health()["state"], "standby")
        self.assertEqual(self.separate(b"ok"), b"ok")

    def test_rejects_missing_body(self):
        req = urllib.request.Request(self.url + "/separate", data=b"", method="POST")
        with self.assertRaises(urllib.error.HTTPError) as e:
            urllib.request.urlopen(req)
        self.assertEqual(e.exception.code, 413)

    def test_no_temp_files_left_behind(self):
        before = set(os.listdir(tempfile.gettempdir()))
        self.separate(b"x")
        # Cleanup runs right after the response is sent, so give it a moment.
        deadline = time.monotonic() + 5
        while True:
            left = {n for n in set(os.listdir(tempfile.gettempdir())) - before
                    if n.startswith("separate-")}
            if not left or time.monotonic() > deadline:
                break
            time.sleep(0.05)
        self.assertEqual(left, set())


if __name__ == "__main__":
    unittest.main()
