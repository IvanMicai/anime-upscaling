package dualaudio

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Tools names the binaries to run. The API passes its configured paths so this
// package resolves ffmpeg the same way the runner does.
type Tools struct {
	FFmpeg  string
	FFprobe string
}

const (
	outRate     = 48000
	outChannels = 2

	// aacEncoderDelay is the priming delay of ffmpeg's native AAC encoder. It is
	// written as initial_padding, which the Matroska muxer does NOT compensate
	// for: measured on finished files, the dubbed track came out a constant
	// 20 ms late. Dropping exactly this many samples before encoding cancels it.
	// It only holds for the native encoder — switch to libfdk_aac or opus and
	// this has to be measured again.
	aacEncoderDelay = 1024
)

func (t Tools) run(ctx context.Context, bin string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		lines := strings.Split(strings.TrimSpace(errb.String()), "\n")
		if len(lines) > 4 {
			lines = lines[len(lines)-4:]
		}
		return nil, fmt.Errorf("%s: %w: %s", bin, err, strings.Join(lines, " | "))
	}
	return out.Bytes(), nil
}

// Duration returns the container duration in seconds.
func (t Tools) Duration(ctx context.Context, path string) (float64, error) {
	out, err := t.run(ctx, t.FFprobe, nil, "-v", "error",
		"-show_entries", "format=duration", "-of", "csv=p=0", path)
	if err != nil {
		return 0, err
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("no readable duration in %s", path)
	}
	return d, nil
}

// VideoStats reads the first video stream's size and the container bitrate.
func (t Tools) VideoStats(ctx context.Context, path string) (VideoStats, error) {
	out, err := t.run(ctx, t.FFprobe, nil, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height:format=bit_rate", "-of", "default=nw=1", path)
	if err != nil {
		return VideoStats{}, err
	}
	var v VideoStats
	for _, line := range strings.Split(string(out), "\n") {
		k, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		n, _ := strconv.ParseInt(val, 10, 64)
		switch k {
		case "width":
			v.Width = int(n)
		case "height":
			v.Height = int(n)
		case "bit_rate":
			v.Bitrate = n
		}
	}
	if v.Width == 0 || v.Height == 0 {
		return v, fmt.Errorf("no video stream in %s", path)
	}
	return v, nil
}

func (t Tools) hasSubtitles(ctx context.Context, path string) bool {
	out, err := t.run(ctx, t.FFprobe, nil, "-v", "error", "-select_streams", "s",
		"-show_entries", "stream=index", "-of", "csv=p=0", path)
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// DecodePCM decodes one audio stream to interleaved signed 16-bit samples.
func (t Tools) DecodePCM(ctx context.Context, path string, rate, channels, stream int) ([]int16, error) {
	raw, err := t.run(ctx, t.FFmpeg, nil, "-v", "error", "-nostdin", "-i", path,
		"-map", fmt.Sprintf("0:a:%d", stream), "-vn",
		"-ac", strconv.Itoa(channels), "-ar", strconv.Itoa(rate), "-f", "s16le", "-")
	if err != nil {
		return nil, err
	}
	pcm := make([]int16, len(raw)/2)
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
	}
	return pcm, nil
}

// MuxOptions labels the two audio tracks of the output.
type MuxOptions struct {
	BaseLang, BaseTitle string
	DubLang, DubTitle   string
	Bitrate             string
	DubIsDefault        bool
	Tags                map[string]string // container-level tags
}

// Mux writes base video + base audio (both stream-copied) plus the rebuilt dub
// track. It writes to a temp name and renames, so a killed job never leaves a
// half-written file where a finished one is expected.
func (t Tools) Mux(ctx context.Context, basePath string, dub []int16, outPath string, o MuxOptions) error {
	if len(dub) > aacEncoderDelay*outChannels {
		dub = dub[aacEncoderDelay*outChannels:]
	}
	raw := make([]byte, 2*len(dub))
	for i, s := range dub {
		binary.LittleEndian.PutUint16(raw[2*i:], uint16(s))
	}
	tmp := outPath + ".part"
	args := []string{"-v", "error", "-y", "-i", basePath,
		"-f", "s16le", "-ar", strconv.Itoa(outRate), "-ac", strconv.Itoa(outChannels), "-i", "pipe:0",
		"-map", "0:v:0", "-map", "0:a:0", "-map", "1:a:0"}
	subs := t.hasSubtitles(ctx, basePath)
	if subs {
		args = append(args, "-map", "0:s?")
	}
	args = append(args, "-c:v", "copy", "-c:a:0", "copy",
		"-c:a:1", "aac", "-b:a:1", o.Bitrate, "-ac:a:1", strconv.Itoa(outChannels),
		"-metadata:s:a:0", "language="+o.BaseLang, "-metadata:s:a:0", "title="+o.BaseTitle,
		"-metadata:s:a:1", "language="+o.DubLang, "-metadata:s:a:1", "title="+o.DubTitle)
	if subs {
		args = append(args, "-c:s", "copy")
	}
	tagKeys := make([]string, 0, len(o.Tags))
	for k := range o.Tags {
		tagKeys = append(tagKeys, k)
	}
	sort.Strings(tagKeys)
	for _, k := range tagKeys {
		args = append(args, "-metadata", k+"="+o.Tags[k])
	}
	if o.DubIsDefault {
		args = append(args, "-disposition:a:0", "0", "-disposition:a:1", "default")
	} else {
		args = append(args, "-disposition:a:0", "default", "-disposition:a:1", "0")
	}
	// The temp name ends in .part and ffmpeg picks the muxer from the extension;
	// without -f it aborts before writing a byte.
	args = append(args, "-map_chapters", "0", "-f", "matroska", tmp)
	if _, err := t.run(ctx, t.FFmpeg, raw, args...); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, outPath)
}
