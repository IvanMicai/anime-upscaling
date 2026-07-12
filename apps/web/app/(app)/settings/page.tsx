"use client";

import { useEffect, useState } from "react";
import { CheckCircle2, Info } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { OptionButtons } from "@/components/option-buttons";
import { cn } from "@/lib/utils";
import { sectionCardPlain } from "@/lib/section";
import { getSettings, updateSettings } from "@/lib/api";
import { GPU_VENDOR_OPTIONS, type GPUVendor, type Settings } from "@/lib/types";

export default function SettingsPage() {
  const [loaded, setLoaded] = useState(false);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [streamsPerGPU, setStreamsPerGPU] = useState(1);
  const [ffmpegStreams, setFfmpegStreams] = useState(1);
  const [gpuVendor, setGpuVendor] = useState<GPUVendor>("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    getSettings()
      .then((s) => {
        setSettings(s);
        setStreamsPerGPU(s.streams_per_gpu);
        setFfmpegStreams(s.ffmpeg_streams);
        setGpuVendor(s.gpu_vendor ?? "");
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoaded(true));
  }, []);

  async function save() {
    setError(null);
    setNotice(null);
    setSaving(true);
    try {
      const updated = await updateSettings({
        streams_per_gpu: streamsPerGPU,
        ffmpeg_streams: ffmpegStreams,
        gpu_vendor: gpuVendor,
      });
      setSettings(updated);
      setNotice("Settings aplicadas com sucesso.");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  if (!loaded)
    return <p className="text-sm text-muted-foreground">Carregando...</p>;

  const dirty =
    settings &&
    (streamsPerGPU !== settings.streams_per_gpu ||
      ffmpegStreams !== settings.ffmpeg_streams ||
      gpuVendor !== (settings.gpu_vendor ?? ""));

  return (
    <div className="max-w-xl space-y-4">
      <h2 className="text-xl font-bold">Settings</h2>
      <section className={cn(sectionCardPlain, "space-y-6")}>
        <h3 className="font-semibold leading-none">Concorrência</h3>
          <div className="flex items-start gap-2 rounded-md border border-blue-500/30 bg-blue-500/10 px-3 py-2 text-sm text-muted-foreground">
            <Info className="mt-0.5 size-4 shrink-0 text-blue-400" />
            <span>
              Mudanças só aplicam quando não há jobs em execução. GPUs
              detectadas: <strong>{settings?.gpu_count ?? "?"}</strong>.
            </span>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="streams_per_gpu">Streams por GPU</Label>
              <span className="font-mono text-sm tabular-nums">
                {streamsPerGPU}
              </span>
            </div>
            <Slider
              id="streams_per_gpu"
              min={1}
              max={8}
              step={1}
              value={[streamsPerGPU]}
              onValueChange={([v]) => setStreamsPerGPU(v)}
            />
            <p className="text-xs text-muted-foreground">
              Quantos processos video2x rodam simultaneamente em cada GPU (1–8).
              Útil quando a GPU não está saturada.
            </p>
          </div>

          <div className="space-y-2">
            <Label>GPU vendor (encode ffmpeg)</Label>
            <OptionButtons
              columns={2}
              value={gpuVendor}
              onChange={setGpuVendor}
              options={GPU_VENDOR_OPTIONS.map((o) => ({
                value: o.value as GPUVendor,
                label: o.label,
              }))}
            />
            <p className="text-xs text-muted-foreground">
              Habilita o toggle <em>Usar GPU</em> em jobs de optimize (encoder de
              hardware NVENC/AMF/QSV). Cada optimize-GPU consome um slot do mesmo
              pool que o upscale.
            </p>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="ffmpeg_streams">Streams de FFmpeg</Label>
              <span className="font-mono text-sm tabular-nums">
                {ffmpegStreams}
              </span>
            </div>
            <Slider
              id="ffmpeg_streams"
              min={1}
              max={8}
              step={1}
              value={[ffmpegStreams]}
              onValueChange={([v]) => setFfmpegStreams(v)}
            />
            <p className="text-xs text-muted-foreground">
              Quantos encodes FFmpeg rodam em paralelo (optimize/check/pipeline).
              Se o gargalo for CPU, aumente com cautela.
            </p>
          </div>

          {error && <p className="text-sm text-red-400">{error}</p>}
          {notice && (
            <p className="flex items-center gap-1.5 text-sm text-green-400">
              <CheckCircle2 className="size-4" />
              {notice}
            </p>
          )}

          <Button onClick={save} disabled={!dirty || saving}>
            {saving ? "Salvando..." : "Salvar"}
          </Button>
      </section>
    </div>
  );
}
