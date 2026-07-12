package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/runner"
)

// normalizeForVideo2x prepares a file for video2x by rendering a square-pixel,
// deinterlaced copy when the source has non-square pixels (anamorphic SAR) or is
// interlaced — the two cases video2x mishandles. video2x decodes the raw stored
// grid, upscales it, and re-encodes with square pixels, dropping the anamorphic
// flag: a 16:9 DVD (720x480 SAR 32:27) ends up 1440x960 = 3:2, which pillarboxes
// on a 16:9 player, and 4:3 sources come out stretched. Converting to real
// square pixels up front makes the upscale output display correctly everywhere
// and lets the model see the image in its true shape.
//
// It returns the directory video2x should read `filename` from:
//   - the original inputDir when no fix is needed (zero cost, no re-encode);
//   - a temp dir holding a normalized copy (same filename) otherwise.
//
// The returned cleanup removes any temp copy and must be deferred by the caller
// so it also fires across video2x's salvage/retry path. It's a no-op when the
// input is already square + progressive, so it's safe to call on every stage —
// a later stage (e.g. interpolate after upscale) simply skips it.
func normalizeForVideo2x(
	ctx context.Context,
	cfg config.Config,
	r *runner.Runner,
	filename string,
	inputDir string,
	source string,
	index int,
	onEvent func(logger.JobLog),
) (string, func(), error) {
	noop := func() {}

	srcPath := filepath.Join(inputDir, filename)
	info, err := r.ProbeAspect(ctx, srcPath)
	if err != nil {
		// Probe failed (unreadable/exotic file): fall back to the original input
		// so behavior matches the pre-change pipeline instead of failing the step.
		onEvent(logger.JobLog{Source: source, Level: "STEP", Index: index, Message: fmt.Sprintf("Normalização: probe falhou, seguindo sem normalizar (%v)", err), Time: time.Now()})
		return inputDir, noop, nil
	}

	if !info.Anamorphic() && !info.Interlaced {
		return inputDir, noop, nil
	}

	normDir := filepath.Join(cfg.TempDir, "normalized")
	subDir := filepath.Dir(filename)
	target := normDir
	if subDir != "." && subDir != "" {
		target = filepath.Join(normDir, subDir)
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return "", noop, fmt.Errorf("mkdir normalized: %w", err)
	}

	dstPath := filepath.Join(normDir, filename)
	cleanup := func() { _ = os.Remove(dstPath) }

	onEvent(logger.JobLog{
		Source: source, Level: "INFO", Index: index,
		Message: fmt.Sprintf("Normalizando %s: %s", filename, describeNormalize(info)),
		Time:    time.Now(),
	})

	if err := r.FFmpegNormalizeSquare(ctx, srcPath, dstPath, info.Interlaced, nil); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("normalize %s: %w", filename, err)
	}

	return normDir, cleanup, nil
}

// describeNormalize builds a short human log fragment for what the pre-pass does.
func describeNormalize(info runner.AspectInfo) string {
	var parts []string
	if info.Anamorphic() {
		dispW := info.Width * info.SarNum / info.SarDen
		parts = append(parts, fmt.Sprintf("pixel anamórfico %dx%d (SAR %d:%d) → ~%dx%d quadrado",
			info.Width, info.Height, info.SarNum, info.SarDen, dispW, info.Height))
	}
	if info.Interlaced {
		parts = append(parts, "deinterlace")
	}
	return strings.Join(parts, " + ")
}
