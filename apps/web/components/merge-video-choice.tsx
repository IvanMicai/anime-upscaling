"use client";

import { useEffect, useState } from "react";
import { Loader2, TriangleAlert, ZoomIn } from "lucide-react";
import { MergeCompareDialog } from "@/components/merge-compare-dialog";
import { Button } from "@/components/ui/button";
import { locateMergeFrame, mergeFrameUrl } from "@/lib/api";
import type { MergePair, MergeVideoInfo } from "@/lib/types";
import { cn } from "@/lib/utils";

// Three points spread over the episode, past the opening and short of the
// credits: an upscale's tells (flat colour, hard edges, smeared crowds) show on
// ordinary scenes, not on a title card.
const SAMPLE_POINTS = [0.25, 0.5, 0.75];
const FIRST_LOOK_SEC = 30; // the duration is only known after the first answer

function describe(v?: MergeVideoInfo): string {
  if (!v) return "";
  const mbps = v.bitrate > 0 ? ` · ${(v.bitrate / 1_000_000).toFixed(1)} Mbps` : "";
  return `${v.width}×${v.height}${mbps}`;
}

type Sample = { ta: number; tb: number; matched: boolean };

/**
 * Which release supplies the picture, decided by looking, for the whole job.
 *
 * It sits AFTER the files are chosen because that is when the languages are
 * known: asked before, the only answer available is "Automático" — and that one
 * keeps whichever picture has more pixels, which is a measure, not a judgement.
 * An AI upscale has four times the pixels of its clean source and can be worse:
 * flat colour, hard outlines, a deformed crowd, sometimes a burnt-in watermark.
 *
 * The frames are NOT at the same timestamp. Two releases cut differently and the
 * offset moves inside an episode, so each right-hand frame is taken where the
 * API finds the same moment by audio; a point it could not match says so rather
 * than showing two unrelated scenes.
 */
export function MergeVideoChoice({
  source,
  pair,
  video,
  onPick,
  frameUrl = mergeFrameUrl,
}: {
  source: string;
  pair: MergePair | null;
  video: string;
  onPick: (choice: string) => void;
  frameUrl?: typeof mergeFrameUrl;
}) {
  // The answer is stored WITH the pair it belongs to, so "still loading" is
  // derived instead of being flags an effect has to reset on every new pair.
  const pairKey = pair ? `${source}|${pair.a}|${pair.b}` : "";
  const [result, setResult] = useState<{
    key: string;
    samples: Sample[];
    info: { a?: MergeVideoInfo; b?: MergeVideoInfo };
    error: string | null;
    done: boolean;
  } | null>(null);
  const current = result?.key === pairKey ? result : null;
  const samples = current?.samples ?? [];
  const info = current?.info ?? {};
  const error = current?.error ?? null;
  const loading = !!pair && !current?.done;
  // Which sample is open side by side. The thumbnails are small on purpose —
  // an upscale's tells do not survive a 4 cm wide picture — so every one of
  // them opens the full comparison at its own moment.
  const [zoomTime, setZoomTime] = useState<number | null>(null);

  useEffect(() => {
    if (!pair) return;
    let stale = false;
    const key = `${source}|${pair.a}|${pair.b}`;

    // One probe to learn the duration, then the real sample points. Each locate
    // is about a second of work, so three is cheap and one is not enough to
    // judge a picture.
    locateMergeFrame(source, pair.a, pair.b, FIRST_LOOK_SEC)
      .then(async (first) => {
        if (stale) return;
        const firstInfo = { a: first.a, b: first.b };
        setResult({ key, samples: [], info: firstInfo, error: null, done: false });
        const dur = first.a?.duration ?? 0;
        if (dur <= 0) throw new Error("duração desconhecida");
        const points = SAMPLE_POINTS.map((p) => dur * p);
        const found = await Promise.all(
          points.map((t) =>
            locateMergeFrame(source, pair.a, pair.b, t)
              .then((r) => ({
                ta: r.location.t_a,
                tb: r.location.t_b,
                matched: r.location.matched,
              }))
              .catch(() => ({ ta: t, tb: t, matched: false })),
          ),
        );
        if (!stale) setResult({ key, samples: found, info: firstInfo, error: null, done: true });
      })
      .catch((e) => {
        if (stale) return;
        const message = e instanceof Error ? e.message : "não foi possível ler os quadros";
        setResult((prev) => ({
          key,
          samples: [],
          info: prev?.key === key ? prev.info : {},
          error: message,
          done: true,
        }));
      });

    return () => {
      stale = true;
    };
  }, [source, pair]);

  if (!pair) {
    return (
      <p className="rounded-md border border-border bg-secondary/30 px-3 py-2 text-sm text-muted-foreground">
        Nenhum par para comparar. Volte e selecione as duas versões.
      </p>
    );
  }

  const columns = [
    { tag: pair.lang_a.tag, title: pair.lang_a.title, file: pair.a, info: info.a, side: "a" as const },
    { tag: pair.lang_b.tag, title: pair.lang_b.title, file: pair.b, info: info.b, side: "b" as const },
  ];

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          Amostras de <span className="font-medium text-foreground">{pair.output}</span>, no mesmo
          instante das duas cópias (localizado por áudio).
        </p>
        <Button
          type="button"
          size="sm"
          variant={video === "auto" ? "default" : "outline"}
          onClick={() => onPick("auto")}
        >
          Automático
        </Button>
      </div>

      {error && (
        <p className="flex items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-500">
          <TriangleAlert className="size-4 shrink-0" />
          {error}
        </p>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        {columns.map((c) => {
          const picked = video === c.tag;
          return (
            <div
              key={c.tag}
              className={cn(
                "flex flex-col gap-2 rounded-lg border p-3 transition-colors",
                picked ? "border-primary/60 bg-primary/5" : "border-border",
              )}
            >
              <div className="flex items-baseline justify-between gap-2">
                <span className="text-sm font-medium">
                  {c.title} <span className="text-muted-foreground">[{c.tag}]</span>
                </span>
                <span className="text-xs tabular-nums text-muted-foreground">{describe(c.info)}</span>
              </div>

              <div className="grid grid-cols-3 gap-1.5">
                {loading && samples.length === 0
                  ? SAMPLE_POINTS.map((_, i) => (
                      <div
                        key={i}
                        className="flex aspect-[4/3] items-center justify-center rounded bg-secondary/50"
                      >
                        <Loader2 className="size-4 animate-spin text-muted-foreground" />
                      </div>
                    ))
                  : samples.map((s, i) => (
                      <button
                        key={i}
                        type="button"
                        onClick={() => setZoomTime(s.ta)}
                        title="Abrir comparação neste momento"
                        className="group relative cursor-zoom-in overflow-hidden rounded focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                      >
                        {/* eslint-disable-next-line @next/next/no-img-element */}
                        <img
                          src={frameUrl(source, c.file, c.side === "a" ? s.ta : s.tb)}
                          alt={`${c.tag} em ${Math.round(c.side === "a" ? s.ta : s.tb)} s`}
                          className="aspect-[4/3] w-full object-cover transition-transform group-hover:scale-105"
                          loading="lazy"
                        />
                        <span className="absolute inset-0 hidden items-center justify-center bg-black/40 group-hover:flex">
                          <ZoomIn className="size-5 text-white" />
                        </span>
                        {!s.matched && (
                          <span className="absolute inset-x-0 bottom-0 bg-black/70 px-1 py-0.5 text-[10px] text-amber-400">
                            sem casar
                          </span>
                        )}
                      </button>
                    ))}
              </div>

              <Button
                type="button"
                size="sm"
                variant={picked ? "default" : "outline"}
                onClick={() => onPick(c.tag)}
              >
                {picked ? "Vídeo deste" : `Usar o vídeo de [${c.tag}]`}
              </Button>
            </div>
          );
        })}
      </div>

      {zoomTime != null && (
        <MergeCompareDialog
          open
          onOpenChange={(o) => !o && setZoomTime(null)}
          source={source}
          pair={pair}
          initialTime={zoomTime}
          onPick={(tag) => {
            onPick(tag);
            setZoomTime(null);
          }}
        />
      )}

      <p className="text-xs text-muted-foreground">
        Clique em qualquer quadro para abrir a comparação: o mouse varre a divisória entre as duas
        imagens e o duplo clique dá zoom. Olhe uma linha fina — contorno de personagem, texto na tela — a 100%. Mais pixels não é mais
        detalhe: uma cópia já upscalada tem quatro vezes a contagem da fonte limpa e costuma ficar
        pior.
      </p>
    </div>
  );
}
