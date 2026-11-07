package dualaudio

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestWavBytesHeader(t *testing.T) {
	b := wavBytes([]int16{1, -2, 3, -4}, 48000, 2)
	if len(b) != 44+8 || string(b[:4]) != "RIFF" || string(b[8:16]) != "WAVEfmt " || string(b[36:40]) != "data" {
		t.Fatalf("bad header: % x", b[:44])
	}
	if rate := binary.LittleEndian.Uint32(b[24:]); rate != 48000 {
		t.Errorf("rate %d", rate)
	}
	if ch := binary.LittleEndian.Uint16(b[22:]); ch != 2 {
		t.Errorf("channels %d", ch)
	}
	if n := binary.LittleEndian.Uint32(b[40:]); n != 8 {
		t.Errorf("data size %d", n)
	}
}

func TestSeparateVoicesRoundTrip(t *testing.T) {
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/separate" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		got, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte("separated"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	path, err := Tools{Separator: srv.URL + "/"}.separateVoices(context.Background(), []int16{7, 8}, dir, "dub")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, wavBytes([]int16{7, 8}, outRate, outChannels)) {
		t.Error("separator did not receive the PCM as WAV")
	}
	if out, _ := os.ReadFile(path); string(out) != "separated" {
		t.Errorf("wrote %q", out)
	}
}

func TestSeparateVoicesReportsServiceErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "CUDA out of memory", http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := Tools{Separator: srv.URL}.separateVoices(context.Background(), []int16{1}, t.TempDir(), "base")
	if err == nil || !strings.Contains(err.Error(), "CUDA out of memory") {
		t.Errorf("err = %v, want the service's message", err)
	}
}

// Separation is an aid: without a separator the merge goes on with the
// originals, and says so.
func TestSeparateBothWithoutService(t *testing.T) {
	j := Job{BasePath: "a", DubPath: "b"}
	if got := (Tools{}).separateBoth(context.Background(), &j, t.TempDir()); !strings.HasPrefix(got, "unavailable") {
		t.Errorf("got %q", got)
	}
	if j.BaseAnalysisPath != "" || j.DubAnalysisPath != "" {
		t.Error("analysis paths set without a separator")
	}
}
