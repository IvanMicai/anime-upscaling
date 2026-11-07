package dualaudio

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Voice separation sends a track to the optional separator service (Demucs on
// a GPU, see apps/separator) and gets back the same track without voices.
//
// Why: the two languages share music, effects and ambience but never dialogue.
// In a stretch of pure dialogue the shared part is quiet under the voices, and
// both alignment and validation lose it — on Pokemon S01 that was the reason
// for 8 of 82 episodes being held back. With the voices removed from both
// tracks the quiet shared part is all that is left, and it lines up.
//
// The tracks sent are the decoded PCM, so the result is on exactly the
// timeline the rest of the pipeline uses. The output file is still rendered
// from the ORIGINAL dub audio; the separated tracks are only listened to.

// separateVoices posts stereo PCM at outRate to the separator and writes what
// comes back (a WAV) to dir/name.wav, returning its path.
func (t Tools) separateVoices(ctx context.Context, pcm []int16, dir, name string) (string, error) {
	url := strings.TrimRight(t.Separator, "/") + "/separate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(wavBytes(pcm, outRate, outChannels)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "audio/wav")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("separator: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("separator: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	path := filepath.Join(dir, name+".wav")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return "", fmt.Errorf("separator: %w", err)
	}
	return path, f.Close()
}

// wavBytes wraps interleaved 16-bit PCM in a canonical WAV header.
func wavBytes(pcm []int16, rate, channels int) []byte {
	data := len(pcm) * 2
	var b bytes.Buffer
	b.Grow(44 + data)
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + data))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1)) // PCM
	w(uint16(channels))
	w(uint32(rate))
	w(uint32(rate * channels * 2))
	w(uint16(channels * 2))
	w(uint16(16))
	b.WriteString("data")
	w(uint32(data))
	w(pcm)
	return b.Bytes()
}
