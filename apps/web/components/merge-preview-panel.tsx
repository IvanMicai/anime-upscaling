"use client";

import { useEffect, useState } from "react";
import { ArrowRight, Images, TriangleAlert } from "lucide-react";
import { MergeCompareDialog } from "@/components/merge-compare-dialog";
import { previewMerge } from "@/lib/api";
import type { MergePair, MergePreview } from "@/lib/types";

const baseName = (p: string) => p.slice(p.lastIndexOf("/") + 1);

/**
 * What a merge job WOULD pair, before it runs. Pairing is by file name, so it is
 * only trustworthy if it can be seen: which two files become which output, what
 * is left without a partner and why.
 */
export function MergePreviewPanel({
  source,
  path,
  files,
  dirs,
  onLanguages,
  onFirstPair,
  video,
  onPickVideo,
}: {
  source: string;
  path: string;
  files: string[];
  dirs: string[];
  onLanguages: (tags: string[]) => void;
  // A representative pair, for the job-level picture choice. The whole season
  // comes from the same two releases, so one pair answers for all of them.
  onFirstPair?: (pair: MergePair | null) => void;
  // Which file supplies the picture ("auto" or a language tag), and how to
  // change it — set from the picture comparison.
  video: string;
  onPickVideo: (languageTag: string) => void;
}) {
  const [comparing, setComparing] = useState<MergePair | null>(null);
  const [preview, setPreview] = useState<MergePreview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const nothingPicked = files.length === 0 && dirs.length === 0;

  useEffect(() => {
    let stale = false;
    // Debounced: shift-selecting a range fires one change per file.
    const timer = setTimeout(() => {
      previewMerge({
        source,
        path: nothingPicked ? path : undefined,
        files: dirs.length === 0 && files.length > 0 ? files : undefined,
        paths: dirs.length > 0 ? dirs : undefined,
      })
        .then((p) => {
          if (stale) return;
          setPreview(p);
          setError(null);
          const tags = new Set<string>();
          for (const pair of p.pairs) {
            tags.add(pair.lang_a.tag);
            tags.add(pair.lang_b.tag);
          }
          onLanguages([...tags].sort());
          onFirstPair?.(p.pairs[0] ?? null);
        })
        .catch((e) => {
          if (stale) return;
          setPreview(null);
          onFirstPair?.(null);
          setError(e instanceof Error ? e.message : "Falha ao calcular os pares");
        });
    }, 250);
    return () => {
      stale = true;
      clearTimeout(timer);
    };
    // onLanguages is a state setter from the parent; files/dirs are the inputs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [source, path, files, dirs, nothingPicked]);

  if (error) return <p className="text-sm text-red-400">{error}</p>;
  if (!preview) return <p className="text-sm text-muted-foreground">Calculando pares…</p>;

  const pending = preview.pairs.filter((p) => !p.exists).length;
  return (
    <div className="rounded-md border border-border bg-secondary/20 p-3 text-sm">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <span className="font-medium">
          {preview.pairs.length} par(es)
          {preview.pairs.length > pending && (
            <span className="ml-1 text-muted-foreground">
              · {preview.pairs.length - pending} já existe(m) em Merged
            </span>
          )}
        </span>
        <span className="text-xs text-muted-foreground">
          vídeo: <span className="text-foreground">{video === "auto" ? "automático" : `[${video}]`}</span>
          {" · "}
          {nothingPicked
            ? `tudo em ${path || "/"}`
            : dirs.length > 0
              ? `${dirs.length} pasta(s)`
              : `${files.length} arquivo(s)`}
        </span>
      </div>

      {preview.pairs.length > 0 && (
        <ul className="scrollbar-dark mt-2 max-h-36 space-y-1 overflow-auto font-mono text-xs">
          {preview.pairs.map((p) => (
            <li key={p.output} className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => setComparing(p)}
                title="Comparar a imagem dos dois arquivos"
                className="inline-flex shrink-0 items-center gap-1 rounded border border-border px-1.5 py-0.5 font-sans text-[11px] text-muted-foreground hover:bg-secondary/60 hover:text-foreground"
              >
                <Images className="size-3" />
                Comparar
              </button>
              <span className={p.exists ? "text-muted-foreground line-through" : undefined}>
                <span className="text-muted-foreground">[{p.lang_a.tag}]</span> {baseName(p.a)}
                <span className="text-muted-foreground"> + [{p.lang_b.tag}]</span> {baseName(p.b)}
                <ArrowRight className="mx-1 inline size-3 text-muted-foreground" />
                <span className="text-pink-400">{p.output}</span>
              </span>
            </li>
          ))}
        </ul>
      )}

      {preview.unpaired.length > 0 && (
        <details className="mt-2 text-xs text-amber-400">
          <summary className="cursor-pointer">
            <TriangleAlert className="mr-1 inline size-3" />
            {preview.unpaired.length} arquivo(s) sem par — não entram no job
          </summary>
          <ul className="scrollbar-dark mt-1 max-h-28 space-y-0.5 overflow-auto font-mono">
            {preview.unpaired.map((u) => (
              <li key={u.file}>
                {u.file} <span className="text-muted-foreground">— {u.reason}</span>
              </li>
            ))}
          </ul>
        </details>
      )}

      {comparing && (
        <MergeCompareDialog
          open
          onOpenChange={(o) => !o && setComparing(null)}
          source={source}
          pair={comparing}
          onPick={onPickVideo}
        />
      )}

      {preview.pairs.length === 0 && (
        <p className="mt-1 text-xs text-muted-foreground">
          Nenhum par. O merge precisa de dois arquivos de mesmo nome e idiomas diferentes, ex.:
          <code className="mx-1">nome.pt-br.mp4</code>+<code className="mx-1">nome.en.mp4</code>.
        </p>
      )}
    </div>
  );
}
