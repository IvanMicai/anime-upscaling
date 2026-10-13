"use client";

import { useEffect, useRef, useState } from "react";
import { Loader2, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Slider } from "@/components/ui/slider";
import { locateMergeFrame, mergeFrameUrl } from "@/lib/api";
import type { MergeLocateResponse, MergePair, MergeVideoInfo } from "@/lib/types";
import { cn } from "@/lib/utils";

const SEEK_POINTS = [0.1, 0.3, 0.5, 0.7, 0.9];
const FIRST_LOOK_SEC = 30; // the duration is only known after the first answer

function clock(sec: number): string {
  const s = Math.max(0, Math.round(sec));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

function describe(v?: MergeVideoInfo): string {
  if (!v) return "";
  const mbps = v.bitrate > 0 ? ` · ${(v.bitrate / 1_000_000).toFixed(1)} Mbps` : "";
  return `${v.width}×${v.height}${mbps}`;
}

/**
 * Side-by-side picture comparison for a merge pair, so "which file has the
 * better picture" is answered by looking instead of by pixel count.
 *
 * The same scene is NOT at the same timestamp in two releases — openings and
 * cuts differ, and the offset changes inside an episode — so the second frame is
 * taken where the API finds the same moment by audio, and the dialog says so
 * when it could not. Frames are lossless PNG, overlaid in one box with a
 * draggable divider; double-click zooms at that point.
 */
export function MergeCompareDialog({
  open,
  onOpenChange,
  source,
  pair,
  onPick,
  frameUrl = mergeFrameUrl,
  initialTime,
}: {
  // Where a frame comes from; overridable so stories can show pictures without
  // an API (an <img> request does not go through the fetch mock).
  frameUrl?: typeof mergeFrameUrl;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  source: string;
  pair: MergePair;
  onPick: (languageTag: string) => void;
  // Opening from a sample frame should land on THAT frame, not on the generic
  // first look: the sample is what made the person want a closer look.
  initialTime?: number;
}) {
  const [time, setTime] = useState(initialTime ?? FIRST_LOOK_SEC);
  const [scrub, setScrub] = useState<number | null>(null);
  // The answer is stored WITH the request it belongs to. "Still locating" and
  // "frames still loading" are then derived instead of being flags an effect
  // has to reset — and the previous frames stay up while the next ones arrive.
  const [result, setResult] = useState<{ key: string; data?: MergeLocateResponse; error?: string } | null>(null);
  const [lastData, setLastData] = useState<MergeLocateResponse | null>(null);
  const [loadedSrc, setLoadedSrc] = useState<string[]>([]);
  const [split, setSplit] = useState(50);
  const [zoomAt, setZoomAt] = useState<{ x: number; y: number } | null>(null);
  const box = useRef<HTMLDivElement>(null);
  const dragging = useRef(false);

  // Reopening on a different sample must move: `time` survives the close.
  // Adjusted during render (not in an effect) so the stale frame never paints.
  const [openedAt, setOpenedAt] = useState({ open, initialTime });
  if (openedAt.open !== open || openedAt.initialTime !== initialTime) {
    setOpenedAt({ open, initialTime });
    if (open && initialTime != null) setTime(initialTime);
  }

  const requestKey = `${source}|${pair.a}|${pair.b}|${time}`;
  useEffect(() => {
    if (!open) return;
    let stale = false;
    locateMergeFrame(source, pair.a, pair.b, time)
      .then((data) => {
        if (stale) return;
        setResult({ key: requestKey, data });
        setLastData(data);
      })
      .catch((e) => {
        if (stale) return;
        setResult({ key: requestKey, error: e instanceof Error ? e.message : "Falha ao localizar o frame" });
      });
    return () => {
      stale = true;
    };
  }, [open, source, pair.a, pair.b, time, requestKey]);

  const current = result?.key === requestKey ? result : null;
  const locating = open && current === null;
  const error = current?.error ?? null;
  const data = current?.data ?? lastData;
  const loc = data?.location;
  const srcA = loc ? frameUrl(source, pair.a, loc.t_a) : "";
  const srcB = loc ? frameUrl(source, pair.b, loc.t_b) : "";
  const markLoaded = (src: string) => setLoadedSrc((l) => (l.includes(src) ? l : [...l.slice(-20), src]));
  const duration = data?.a?.duration ?? 0;
  const ratio = data?.a ? data.a.width / data.a.height : 4 / 3;
  const busy = locating || (loc !== undefined && !(loadedSrc.includes(srcA) && loadedSrc.includes(srcB)));

  function moveSplit(clientX: number) {
    const r = box.current?.getBoundingClientRect();
    if (r) setSplit(Math.min(100, Math.max(0, ((clientX - r.left) / r.width) * 100)));
  }

  const imgStyle = zoomAt
    ? { transform: "scale(2.5)", transformOrigin: `${zoomAt.x}% ${zoomAt.y}%` }
    : undefined;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Comparar imagem</DialogTitle>
          <DialogDescription>
            O mesmo momento nos dois arquivos, achado pelo áudio. Mova o mouse para varrer a
            divisória; duplo clique dá zoom no ponto.
          </DialogDescription>
        </DialogHeader>

        <div
          ref={box}
          className="relative w-full touch-none select-none overflow-hidden rounded-md border bg-black"
          style={{ aspectRatio: String(ratio) }}
          onPointerDown={(e) => {
            dragging.current = true;
            e.currentTarget.setPointerCapture(e.pointerId);
            moveSplit(e.clientX);
          }}
          // A mouse just moves the divider — there is nothing to grab and no
          // reason to make someone find a 1 px line to compare two pictures.
          // Touch keeps the drag: there is no hover to follow there.
          onPointerMove={(e) => {
            if (e.pointerType === "mouse" || dragging.current) moveSplit(e.clientX);
          }}
          onPointerUp={() => (dragging.current = false)}
          onDoubleClick={(e) => {
            const r = e.currentTarget.getBoundingClientRect();
            setZoomAt((z) =>
              z ? null : { x: ((e.clientX - r.left) / r.width) * 100, y: ((e.clientY - r.top) / r.height) * 100 },
            );
          }}
        >
          {loc && (
            <>
              {/* Both fill the same box so the content overlaps even when the
                  two files have different pixel sizes. */}
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={srcA}
                alt={`Frame de [${pair.lang_a.tag}]`}
                draggable={false}
                onLoad={() => markLoaded(srcA)}
                className="absolute inset-0 h-full w-full"
                style={imgStyle}
              />
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={srcB}
                alt={`Frame de [${pair.lang_b.tag}]`}
                draggable={false}
                onLoad={() => markLoaded(srcB)}
                className="absolute inset-0 h-full w-full"
                style={{ ...imgStyle, clipPath: `inset(0 0 0 ${split}%)` }}
              />
              <div className="pointer-events-none absolute inset-y-0 w-px bg-white/90 shadow-[0_0_0_1px_rgba(0,0,0,0.6)]" style={{ left: `${split}%` }} />
              <span className="pointer-events-none absolute left-2 top-2 rounded bg-black/70 px-1.5 py-0.5 font-mono text-xs text-white">
                [{pair.lang_a.tag}] {describe(data?.a)}
              </span>
              <span className="pointer-events-none absolute right-2 top-2 rounded bg-black/70 px-1.5 py-0.5 font-mono text-xs text-white">
                [{pair.lang_b.tag}] {describe(data?.b)}
              </span>
            </>
          )}
          {busy && (
            <div className="absolute inset-0 flex items-center justify-center bg-black/40">
              <Loader2 className="size-6 animate-spin text-white" />
            </div>
          )}
        </div>

        {error && <p className="text-sm text-red-400">{error}</p>}
        {loc && !error && (
          loc.matched ? (
            <p className="text-xs text-muted-foreground">
              [{pair.lang_a.tag}] {clock(loc.t_a)} ↔ [{pair.lang_b.tag}] {clock(loc.t_b)}
              {Math.abs(loc.lag) >= 0.05 && ` (${loc.lag > 0 ? "+" : ""}${loc.lag.toFixed(1)} s)`}
              {" · "}confiança {Math.round(loc.confidence)}
            </p>
          ) : (
            <p className="flex items-start gap-1 text-xs text-amber-400">
              <TriangleAlert className="mt-0.5 size-3 shrink-0" />
              Não consegui achar este momento no outro arquivo pelo áudio (trecho só de diálogo, ou que o outro
              não tem). Os dois frames estão no mesmo tempo e podem não ser a mesma cena — tente outro ponto.
            </p>
          )
        )}

        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-1.5">
            {SEEK_POINTS.map((p) => {
              const at = duration > 0 ? duration * p : null;
              return (
                <Button
                  key={p}
                  size="sm"
                  variant="outline"
                  disabled={at === null}
                  className={cn(at !== null && Math.abs(at - time) < 0.5 && "border-primary/60 bg-primary/10")}
                  onClick={() => at !== null && setTime(at)}
                >
                  {at === null ? `${p * 100}%` : clock(at)}
                </Button>
              );
            })}
            <span className="ml-auto font-mono text-xs text-muted-foreground">
              {clock(scrub ?? time)}{duration > 0 && ` / ${clock(duration)}`}
            </span>
          </div>
          <Slider
            min={0}
            max={Math.max(1, Math.floor(duration))}
            step={1}
            value={[scrub ?? time]}
            disabled={duration <= 0}
            onValueChange={([v]) => setScrub(v)}
            // Only fetch when the handle is released: each stop is two frames
            // and an audio search.
            onValueCommit={([v]) => {
              setScrub(null);
              setTime(v);
            }}
          />
        </div>

        <DialogFooter className="gap-2 sm:justify-between">
          <span className="text-xs text-muted-foreground">
            A escolha vale para o job inteiro.
          </span>
          <div className="flex gap-2">
            {[pair.lang_a.tag, pair.lang_b.tag].map((tag) => (
              <Button
                key={tag}
                onClick={() => {
                  onPick(tag);
                  onOpenChange(false);
                }}
              >
                Usar o vídeo de [{tag}]
              </Button>
            ))}
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
