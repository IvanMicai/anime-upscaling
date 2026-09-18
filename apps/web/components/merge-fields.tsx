"use client";

import { OptionButtons } from "@/components/option-buttons";
import { Checkbox } from "@/components/ui/checkbox";
import type { MergeConfig } from "@/lib/types";

export const MERGE_DEFAULTS: MergeConfig = {
  video: "auto",
  tick: 5,
  gapFill: "base",
  force: false,
  defaultAudio: "",
};

const TICKS = [
  { value: 10, label: "10 s", desc: "Rápido" },
  { value: 5, label: "5 s", desc: "Padrão" },
  { value: 2, label: "2 s", desc: "Fino" },
  { value: 1, label: "1 s", desc: "Máximo" },
] as const;

const GAP_FILL = [
  { value: "base", label: "Áudio do vídeo", desc: "Onde a outra faixa não tem conteúdo, toca o áudio original" },
  { value: "silence", label: "Silêncio", desc: "Deixa o buraco mudo" },
] as const;

function Label({ children }: { children: React.ReactNode }) {
  return (
    <label className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
      {children}
    </label>
  );
}

/**
 * Settings of a merge (dual audio) job. `languages` are the tags found in the
 * current selection, so "which file supplies the picture" can be answered by
 * name instead of guessed at.
 */
export function MergeFields({
  config,
  onChange,
  languages,
}: {
  config: MergeConfig;
  onChange: (patch: Partial<MergeConfig>) => void;
  languages: string[];
}) {
  const videoOptions = [
    { value: "auto", label: "Automático", desc: "Maior resolução; no empate, maior bitrate" },
    ...languages.map((l) => ({ value: l, label: `[${l}]`, desc: "Usar o vídeo deste idioma" })),
  ];
  const audioOptions = [
    { value: "", label: "Do outro arquivo", desc: "O idioma que NÃO forneceu o vídeo" },
    ...languages.map((l) => ({ value: l, label: `[${l}]` })),
  ];

  return (
    <div className="space-y-5">
      <p className="rounded-md border border-border bg-secondary/30 px-3 py-2 text-sm text-muted-foreground">
        Une arquivos de <span className="text-foreground">mesmo nome e idiomas diferentes</span>
        {" "}— <code>nome.pt-br.mp4</code> + <code>nome.en.mp4</code> → <code>merged/nome.mkv</code>
        {" "}— com o vídeo de um e os dois áudios sincronizados. O resultado fica na pasta
        {" "}<span className="text-pink-400">Merged</span> e serve de origem para upscale, interpolação e otimização.
      </p>

      <div className="space-y-1.5">
        <Label>Vídeo</Label>
        <OptionButtons
          value={config.video}
          options={videoOptions}
          onChange={(video) => onChange({ video })}
          columns={Math.min(videoOptions.length, 3)}
        />
        {languages.length === 0 && (
          <p className="text-xs text-muted-foreground">
            Selecione os arquivos para escolher o idioma pelo nome. Mais pixels não é sempre melhor imagem:
            um upscale pode perder para a fonte limpa de menor resolução.
          </p>
        )}
      </div>

      <div className="space-y-1.5">
        <Label>Verificar a sincronia a cada</Label>
        <OptionButtons value={config.tick} options={TICKS} onChange={(tick) => onChange({ tick })} columns={4} />
        <p className="text-xs text-muted-foreground">
          A faixa remontada é conferida contra o áudio do vídeo neste intervalo. Mais fino acha trechos
          fora de sincronia mais curtos e demora mais. O resultado fica gravado no arquivo e aparece no
          tooltip do explorador.
        </p>
      </div>

      <div className="space-y-1.5">
        <Label>Buracos</Label>
        <OptionButtons value={config.gapFill} options={GAP_FILL} onChange={(gapFill) => onChange({ gapFill })} />
      </div>

      <div className="space-y-1.5">
        <Label>Faixa de áudio padrão</Label>
        <OptionButtons
          value={config.defaultAudio}
          options={audioOptions}
          onChange={(defaultAudio) => onChange({ defaultAudio })}
          columns={Math.min(audioOptions.length, 3)}
        />
      </div>

      <label className="flex cursor-pointer items-start gap-2 text-sm">
        <Checkbox
          checked={config.force}
          onCheckedChange={(v) => onChange({ force: v === true })}
          className="mt-0.5"
        />
        <span>
          Gravar mesmo sem sincronia garantida
          <span className="block text-xs text-muted-foreground">
            Por padrão, um episódio que não passa na verificação NÃO é gravado e o motivo vai para o log.
            Marcado, ele é gravado com o aviso nos metadados.
          </span>
        </span>
      </label>
    </div>
  );
}
