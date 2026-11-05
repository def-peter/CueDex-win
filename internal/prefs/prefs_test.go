package prefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/def-peter/cuedex-win/internal/events"
)

func TestDefaultGivesEachProducerItsOwnSound(t *testing.T) {
	p := Default()
	codex, opencode := p.For(events.Codex), p.For(events.OpenCode)
	if codex.ID == opencode.ID {
		t.Fatalf("both producers cue with %q", codex.ID)
	}
	if codex.Volume <= 0 || codex.Volume > 100 || opencode.Volume <= 0 || opencode.Volume > 100 {
		t.Fatalf("default volumes %d and %d are outside 0-100", codex.Volume, opencode.Volume)
	}
}

func TestSetChangesOnlyOneProducer(t *testing.T) {
	p := Default()
	updated := p.Set(events.OpenCode, SoundChoice{ID: "ping", Volume: 20})
	if updated.Sound.OpenCode.ID != "ping" || updated.Sound.OpenCode.Volume != 20 {
		t.Fatalf("opencode: %+v", updated.Sound.OpenCode)
	}
	if updated.Sound.Codex != p.Sound.Codex {
		t.Fatalf("codex changed too: %+v", updated.Sound.Codex)
	}
	again := updated.Set(events.Codex, SoundChoice{ID: "knock", Volume: 0})
	if again.Sound.Codex.ID != "knock" || again.Sound.Codex.Volume != 0 {
		t.Fatalf("codex: %+v", again.Sound.Codex)
	}
	if again.Sound.OpenCode.ID != "ping" {
		t.Fatalf("setting codex changed opencode: %+v", again.Sound.OpenCode)
	}
}

func TestNormalizedRepairsAFile(t *testing.T) {
	broken := Preferences{
		Sound: SoundPreferences{
			Codex:    SoundChoice{ID: "gone", Volume: 300},
			OpenCode: SoundChoice{ID: "", Volume: -5},
		},
	}
	p := broken.Normalized()
	if p.Sound.Codex.ID != "rise" {
		t.Fatalf("a missing sound did not fall back: %+v", p.Sound.Codex)
	}
	if p.Sound.Codex.Volume != 100 || p.Sound.OpenCode.Volume != 0 {
		t.Fatalf("volumes %d and %d were not clamped", p.Sound.Codex.Volume, p.Sound.OpenCode.Volume)
	}
	if p.Sound.OpenCode.ID != "fall" {
		t.Fatalf("opencode fell back to %q", p.Sound.OpenCode.ID)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "preferences.json")
	want := Default().Set(events.Codex, SoundChoice{ID: "ping", Volume: 35})
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("loaded %+v, want %+v", got, want)
	}
	// Saving again replaces the file rather than appending.
	second := want.Set(events.OpenCode, SoundChoice{ID: "knock", Volume: 100})
	if err := Save(path, second); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil || got != second {
		t.Fatalf("second load %+v, %v", got, err)
	}
}

func TestLoadWithoutAFileIsTheDefaults(t *testing.T) {
	p, err := Load(filepath.Join(t.TempDir(), "preferences.json"))
	if err != nil {
		t.Fatalf("a missing file is not an error: %v", err)
	}
	if p != Default() {
		t.Fatalf("got %+v, want the defaults", p)
	}
}

func TestLoadRepairsACorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := os.WriteFile(path, []byte(`{"sound":`), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err == nil {
		t.Fatal("a corrupt file was accepted silently")
	}
	if p != Default() {
		t.Fatalf("got %+v, want the defaults", p)
	}
}

func TestLoadKeepsAKnownChoice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	body := `{"sound":{"codex":{"id":"knock","volume":12},"opencode":{"id":"ping","volume":88}},"futureField":true}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Sound.Codex != (SoundChoice{ID: "knock", Volume: 12}) {
		t.Fatalf("codex: %+v", p.Sound.Codex)
	}
	if p.Sound.OpenCode != (SoundChoice{ID: "ping", Volume: 88}) {
		t.Fatalf("opencode: %+v", p.Sound.OpenCode)
	}
}
