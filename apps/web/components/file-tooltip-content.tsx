import { formatBytes, formatFrameRate, syncShareInPlace, type FolderEntry } from "@/lib/file-utils";
import type { SyncInfo } from "@/lib/types";
import { cn } from "@/lib/utils";

const SYNC_LABEL: Record<SyncInfo["status"], string> = {
  ok: "Sincronia garantida",
  review: "Sincronia NÃO garantida",
  fail: "Sincronia falhou",
};

// The dual-audio sync verdict, read from the merged file's own tags: how well
// the second audio track lines up with the picture, and how that was checked.
function SyncVerdict({ sync }: { sync: SyncInfo }) {
  return (
    <div className="border-t border-border/60 pt-1">
      <div className={cn("font-medium", sync.status === "ok" ? "text-green-400" : "text-amber-400")}>
        {SYNC_LABEL[sync.status] ?? sync.status}
      </div>
      <div className="ml-2 text-muted-foreground">
        <div>Erro mediano: {sync.residual_ms} ms (p95 {sync.p95_ms} ms)</div>
        <div>
          No lugar: {sync.in_place}/{sync.checked} verificações ({syncShareInPlace(sync)}%), a cada {sync.tick_s} s
        </div>
        {sync.off_s > 0 && <div className="text-amber-400">{sync.off_s} s em offset errado</div>}
        <div>
          Cobertura: {Math.round(sync.coverage * 100)}% · {sync.segments} trecho(s)
          {sync.gaps > 0 ? ` · ${sync.gaps} buraco(s)` : ""}
        </div>
        {sync.video && <div>Vídeo de: [{sync.video}]</div>}
        {sync.notes?.map((n, i) => (
          <div key={i} className="text-amber-400">{n}</div>
        ))}
      </div>
    </div>
  );
}

export function FileTooltipContent({ entry }: { entry: FolderEntry }) {
  return (
    <div className="space-y-1 text-xs">
      <div>Size: {formatBytes(entry.size)}</div>
      {entry.width && entry.height && (
        <div>Resolution: {entry.width}x{entry.height}</div>
      )}
      {entry.frameRate ? (
        <div>Framerate: {formatFrameRate(entry.frameRate)}</div>
      ) : null}
      {entry.audio && entry.audio.length > 0 && (
        <div>
          <div className="font-medium">Audio ({entry.audio.length}):</div>
          {entry.audio.map((a, i) => (
            <div key={i} className="ml-2 text-muted-foreground">
              {[a.title, a.language, a.codec, a.channels ? `${a.channels}ch` : null]
                .filter(Boolean).join(" · ") || `Track ${a.index}`}
            </div>
          ))}
        </div>
      )}
      {entry.subtitles && entry.subtitles.length > 0 && (
        <div>
          <div className="font-medium">Subtitles ({entry.subtitles.length}):</div>
          {entry.subtitles.map((s, i) => (
            <div key={i} className="ml-2 text-muted-foreground">
              {[s.title, s.language, s.codec].filter(Boolean).join(" · ") || `Track ${s.index}`}
            </div>
          ))}
        </div>
      )}
      {entry.sync && <SyncVerdict sync={entry.sync} />}
    </div>
  );
}
