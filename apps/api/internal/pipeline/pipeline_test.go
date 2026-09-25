package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestListSortedByName verifies List() returns pipelines in stable natural
// order by name, regardless of insertion order. This is the fix for the UI
// list reshuffling on every request (Go map iteration order is randomized).
func TestListSortedByName(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "pipelines.json"))

	// Insert deliberately out of alphabetical order, including a case and a
	// numeric case that plain byte sort would get wrong ("Pipeline 10" vs "2").
	for _, name := range []string{"banana", "Apple", "Pipeline 10", "Pipeline 2", "cherry"} {
		if _, err := store.Create(name, []PipelineStep{{Operation: "upscale"}}); err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
	}

	want := []string{"Apple", "banana", "cherry", "Pipeline 2", "Pipeline 10"}

	// Call List() several times: map iteration is randomized per call, so a
	// missing sort would surface as a different order on some iteration.
	for attempt := 0; attempt < 20; attempt++ {
		list := store.List()
		if len(list) != len(want) {
			t.Fatalf("attempt %d: got %d pipelines, want %d", attempt, len(list), len(want))
		}
		for i, p := range list {
			if p.Name != want[i] {
				t.Fatalf("attempt %d: position %d = %q, want %q (full order: %v)", attempt, i, p.Name, want[i], names(list))
			}
		}
	}
}

func names(list []Pipeline) []string {
	out := make([]string, len(list))
	for i, p := range list {
		out[i] = p.Name
	}
	return out
}

// TestValidModelScaleRealESRGANGeneral covers realesr-generalv3, which video2x
// only ships at 4x (plain and -wdn denoise variants).
func TestValidModelScaleRealESRGANGeneral(t *testing.T) {
	if !ValidUpscaleModels["realesr-generalv3"] {
		t.Fatal("realesr-generalv3 should be a valid upscale model")
	}
	for scale, want := range map[int]bool{2: false, 3: false, 4: true} {
		if got := ValidModelScale("realesr-generalv3", scale); got != want {
			t.Errorf("ValidModelScale(realesr-generalv3, %d) = %v, want %v", scale, got, want)
		}
	}
}

// communityModels are the RRDBNet models the Dockerfile downloads and adds to
// video2x's allow-list; all of them are 4x only.
var communityModels = []string{"nomos8ksc", "lsdirplusc", "nmkd-siax", "hfa2k"}

func TestValidModelScaleCommunityModels(t *testing.T) {
	for _, model := range communityModels {
		for scale, want := range map[int]bool{2: false, 3: false, 4: true} {
			if got := ValidModelScale(model, scale); got != want {
				t.Errorf("ValidModelScale(%s, %d) = %v, want %v", model, scale, got, want)
			}
		}
	}
}

// TestDockerfileShipsCommunityModels keeps the image in step with the API: a
// model accepted here but missing from video2x's allow-list or the model
// download fails every job that picks it.
func TestDockerfileShipsCommunityModels(t *testing.T) {
	data, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(data)
	for _, model := range communityModels {
		if !strings.Contains(dockerfile, `STR("`+model+`")`) {
			t.Errorf("Dockerfile does not add %s to video2x's model allow-list", model)
		}
		if !strings.Contains(dockerfile, "\n"+model+" ") {
			t.Errorf("Dockerfile does not download the %s model files", model)
		}
	}
}

func TestValidateUpscale(t *testing.T) {
	cases := []struct {
		name                 string
		processor, model     string
		scale, noise, height int
		wantErr              string
	}{
		{"defaults", "", "", 2, 0, 0, ""},
		{"realesrgan 4x", "realesrgan", "realesrgan-plus", 4, 0, 0, ""},
		{"bad scale for model", "realesrgan", "realesrgan-plus", 2, 0, 0, "does not support scale"},
		{"bad scale", "realesrgan", "", 5, 0, 0, "scale must be"},
		{"bad noise", "realcugan", "models-se", 2, 4, 0, "noise_level"},
		{"unknown processor", "topaz", "", 2, 0, 0, "invalid processor"},
		{"seedvr2 defaults", "seedvr2", "", 2, 0, 0, ""},
		{"seedvr2 ignores scale", "seedvr2", "seedvr2-7b-sharp", 0, 0, 2160, ""},
		{"seedvr2 bad height", "seedvr2", "seedvr2-3b", 2, 0, 1000, "target_height"},
		{"seedvr2 with a video2x model", "seedvr2", "realesrgan-plus", 4, 0, 0, "not a SeedVR2 model"},
		{"seedvr2 model on video2x", "realesrgan", "seedvr2-7b", 4, 0, 0, "needs the seedvr2 processor"},
	}
	for _, c := range cases {
		err := ValidateUpscale(c.processor, c.model, c.scale, c.noise, c.height)
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: error = %v, want one containing %q", c.name, err, c.wantErr)
		}
	}
}
