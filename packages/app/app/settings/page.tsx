"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { getSettings, updateSettings } from "@/lib/api";
import { GPU_VENDOR_OPTIONS, type GPUVendor, type Settings } from "@/lib/types";

const NONE_VENDOR = "none";
const toVendorUI = (v: GPUVendor): string => (v === "" ? NONE_VENDOR : v);
const fromVendorUI = (v: string): GPUVendor => (v === NONE_VENDOR ? "" : (v as GPUVendor));

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

  if (!loaded) return <p className="text-sm text-muted-foreground">Carregando...</p>;

  const dirty =
    settings &&
    (streamsPerGPU !== settings.streams_per_gpu ||
      ffmpegStreams !== settings.ffmpeg_streams ||
      gpuVendor !== (settings.gpu_vendor ?? ""));

  return (
    <div className="space-y-4 max-w-xl">
      <Card>
        <CardHeader>
          <CardTitle>Concorrência</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            Ajuste quantos streams rodam em paralelo. Mudanças só aplicam quando
            não houver jobs em execução. GPUs detectadas:{" "}
            <strong>{settings?.gpu_count ?? "?"}</strong>.
          </p>

          <div className="space-y-2">
            <Label htmlFor="streams_per_gpu">Streams por GPU</Label>
            <input
              id="streams_per_gpu"
              type="number"
              min={1}
              max={8}
              value={streamsPerGPU}
              onChange={(e) =>
                setStreamsPerGPU(Math.max(1, parseInt(e.target.value || "1", 10)))
              }
              className="w-full rounded-md border bg-transparent px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
            />
            <p className="text-xs text-muted-foreground">
              Quantos processos video2x rodam simultaneamente em cada GPU. Útil
              quando a GPU não está saturada (aumenta uso útil preenchendo gaps
              de I/O e CPU encode).
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="gpu_vendor">GPU vendor (encode ffmpeg)</Label>
            <Select
              value={toVendorUI(gpuVendor)}
              onValueChange={(v) => setGpuVendor(fromVendorUI(v))}
            >
              <SelectTrigger id="gpu_vendor" className="h-9">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {GPU_VENDOR_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value || NONE_VENDOR} value={opt.value || NONE_VENDOR}>
                    {opt.label} — {opt.desc}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Habilita o toggle <em>Usar GPU</em> em jobs de optimize, que roda
              o ffmpeg no encoder de hardware correspondente (NVENC/AMF/QSV).
              Cada optimize-GPU consome um slot do mesmo pool que o upscale,
              então com 2 GPUs × 2 streams são 4 slots compartilhados.
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="ffmpeg_streams">Streams de FFmpeg</Label>
            <input
              id="ffmpeg_streams"
              type="number"
              min={1}
              max={8}
              value={ffmpegStreams}
              onChange={(e) =>
                setFfmpegStreams(Math.max(1, parseInt(e.target.value || "1", 10)))
              }
              className="w-full rounded-md border bg-transparent px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
            />
            <p className="text-xs text-muted-foreground">
              Quantos encodes FFmpeg rodam em paralelo (optimize/check/pipeline).
              Se o gargalo for CPU, aumente com cautela.
            </p>
          </div>

          {error && <p className="text-sm text-red-400">{error}</p>}
          {notice && <p className="text-sm text-green-400">{notice}</p>}

          <Button onClick={save} disabled={!dirty || saving}>
            {saving ? "Salvando..." : "Salvar"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
