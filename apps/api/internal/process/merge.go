package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/dualaudio"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/runner"
)

// MergeOptions are the per-job settings of a merge.
type MergeOptions struct {
	Video        string // dualaudio.VideoAuto or a language tag
	TickSec      float64
	GapFill      dualaudio.GapFill
	Force        bool
	DefaultAudio string
}

var mergePhaseLabel = map[string]string{
	dualaudio.PhaseDecode:   "Decodificando áudio",
	dualaudio.PhaseAlign:    "Alinhando",
	dualaudio.PhaseValidate: "Validando sincronia",
	dualaudio.PhaseWrite:    "Gravando",
}

// mergePhasePercent gives the progress bar something honest to show: the phases
// are not equal in cost, and there is no frame counter to read.
var mergePhasePercent = map[string]float64{
	dualaudio.PhaseDecode:   5,
	dualaudio.PhaseAlign:    30,
	dualaudio.PhaseValidate: 60,
	dualaudio.PhaseWrite:    90,
}

// MergeFile builds one dual-audio file from a name pair and writes it under the
// merged folder. It reports whether the file was written.
func MergeFile(ctx context.Context, cfg config.Config, pair dualaudio.NamePair, index int, sourceDir, logSource string, opt MergeOptions, onEvent func(logger.JobLog), onProgress func(runner.Progress)) bool {
	if logSource == "" {
		logSource = "FFMPEG"
	}
	say := func(level, msg string) {
		onEvent(logger.JobLog{Source: logSource, Level: level, Index: index, Message: msg, Time: time.Now()})
	}
	outPath := filepath.Join(cfg.MergedDir, filepath.FromSlash(pair.Output))
	if files.FileExists(outPath) {
		say("SKIP", "Pulando "+pair.Output+" (já existe)")
		return false
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		say("ERRO", fmt.Sprintf("Erro: %s (%v)", pair.Output, err))
		return false
	}
	say("INFO", fmt.Sprintf("Unindo: %s + %s", filepath.Base(pair.A), filepath.Base(pair.B)))

	res, err := dualaudio.Merge(ctx, dualaudio.Tools{FFmpeg: cfg.FFmpegBin, FFprobe: cfg.FFprobeBin}, dualaudio.MergeRequest{
		PathA: filepath.Join(sourceDir, filepath.FromSlash(pair.A)), LangA: pair.LangA,
		PathB: filepath.Join(sourceDir, filepath.FromSlash(pair.B)), LangB: pair.LangB,
		OutPath: outPath, Video: opt.Video, GapFill: opt.GapFill, TickSec: opt.TickSec,
		Force: opt.Force, DefaultAudio: opt.DefaultAudio,
		OnPhase: func(phase string) {
			onProgress(runner.Progress{Source: logSource, Filename: pair.Output,
				Phase: mergePhaseLabel[phase], Percent: mergePhasePercent[phase]})
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			say("ERRO", "Cancelado: "+pair.Output)
		} else {
			say("ERRO", fmt.Sprintf("Erro: %s (%v)", pair.Output, err))
		}
		return false
	}
	sync := res.SyncInfo()
	if !res.Written {
		// Held back by the gate: nothing was written, and the reason is the
		// useful part — it says where the sync could not be guaranteed.
		say("ERRO", fmt.Sprintf("Sincronia não garantida, arquivo NÃO gravado: %s — %s", pair.Output, joinNotes(res.Notes)))
		return false
	}
	_ = os.Chown(outPath, cfg.UserID, cfg.GroupID)
	level, verdict := "OK", "Sincronizado"
	if res.Status != dualaudio.StatusOK {
		verdict = "Gravado SEM garantia de sincronia (forçado)"
	}
	say(level, fmt.Sprintf("%s: %s — vídeo de [%s], %s", verdict, pair.Output, res.VideoFrom, sync.Summary()))
	return true
}

func joinNotes(notes []string) string {
	if len(notes) == 0 {
		return "sem detalhe"
	}
	out := notes[0]
	for _, n := range notes[1:] {
		out += "; " + n
	}
	return out
}
