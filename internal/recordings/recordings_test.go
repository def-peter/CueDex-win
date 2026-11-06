package recordings

import (
	"testing"
	"time"

	"github.com/def-peter/cuedex-win/internal/sound"
)

func TestBundledClipsPlayInsideACue(t *testing.T) {
	all := All()
	if len(all) < 4 {
		t.Fatalf("%d bundled clips, want the four the mac app has", len(all))
	}
	seen := map[string]bool{}
	for _, r := range all {
		if r.ID == "" {
			t.Fatalf("clip with no id: %+v", r)
		}
		if seen[r.ID] {
			t.Fatalf("duplicate clip %q", r.ID)
		}
		seen[r.ID] = true
		if r.Language != "zh" && r.Language != "en" {
			t.Errorf("%s is in %q", r.ID, r.Language)
		}
		data, err := r.Bytes()
		if err != nil {
			t.Errorf("%s: %v", r.ID, err)
			continue
		}
		// Duration rejects anything that is not the PCM 16-bit WAV a cue can
		// play, so decoding here also checks the asset itself.
		d, err := sound.Duration(data)
		if err != nil {
			t.Errorf("%s: %v", r.ID, err)
			continue
		}
		if d > 3*time.Second {
			t.Errorf("%s runs %s, past the cue", r.ID, d)
		}
	}
	if _, ok := Lookup(DefaultID); !ok {
		t.Fatalf("the default clip %q is not bundled", DefaultID)
	}
	if _, ok := Lookup("nope"); ok {
		t.Fatal("an unknown id was found")
	}
}
