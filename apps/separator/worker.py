"""Separation worker: the only process that loads PyTorch and the model.

server.py starts it on the first request and stops it after an idle timeout.
Ending the process is what gives the memory back: a live CUDA context keeps
hundreds of MB of VRAM even after the model is freed, so unloading in place is
not enough.

Protocol over a multiprocessing pipe: the worker sends ("ready", device) once
the model is loaded, then answers each (in_path, out_path) job with ("ok", None)
or ("error", message). Torch is imported inside main() so that importing this
module from the standby server costs nothing.
"""


def main(conn, model_name):
    import julius
    import numpy as np
    import soundfile as sf
    import torch
    from demucs.apply import apply_model
    from demucs.pretrained import get_model

    device = "cuda" if torch.cuda.is_available() else "cpu"
    model = get_model(model_name)
    model.to(device).eval()
    vocals = model.sources.index("vocals")
    conn.send(("ready", device))

    def separate(in_path, out_path):
        audio, rate = sf.read(in_path, dtype="float32", always_2d=True)
        channels = audio.shape[1]
        n = audio.shape[0]
        x = torch.from_numpy(audio.T.copy())  # (channels, samples)
        del audio
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
            # shifts=0: no random time shift, so the same track always separates
            # the same way. With the CLI's default of 1 the verdict of one
            # episode changed between runs.
            torch.manual_seed(0)
            sources = apply_model(model, x[None], device=device, shifts=0, split=True,
                                  overlap=0.25, progress=False)[0]
        del x
        # no_vocals = mixture minus vocals, summed in place to keep one copy.
        out = sources.sum(0)
        out -= sources[vocals]
        del sources
        out = (out * std + mean * (len(model.sources) - 1)).cpu()
        if device == "cuda":
            torch.cuda.empty_cache()
        if rate != model.samplerate:
            out = julius.resample_frac(out, model.samplerate, rate)
        out = out[:, :n]
        if out.shape[1] < n:  # resampling rounding: keep the exact length
            out = torch.nn.functional.pad(out, (0, n - out.shape[1]))
        if channels == 1:
            out = out.mean(0, keepdim=True)
        sf.write(out_path, np.clip(out.numpy().T, -1, 1), rate, subtype="PCM_16", format="WAV")

    while True:
        try:
            job = conn.recv()
        except EOFError:  # server went away
            return
        if job is None:
            return
        try:
            separate(*job)
            conn.send(("ok", None))
        except Exception as e:  # report, keep serving
            conn.send(("error", str(e)))
