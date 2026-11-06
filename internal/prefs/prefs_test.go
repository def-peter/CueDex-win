package prefs

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/overlay"
	"github.com/def-peter/cuedex-win/internal/recordings"
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

func TestSpeechIsOffByDefault(t *testing.T) {
	p := Default()
	if p.Speech.On(events.Codex) || p.Speech.On(events.OpenCode) {
		t.Fatalf("speech is on by default: %+v", p.Speech)
	}
	if p.Speech.ReplaceSound {
		t.Fatal("speech replaces the sound by default")
	}
}

func TestSetSpeechKeepsTheOtherProducerMapping(t *testing.T) {
	p := Default().SetSpeech(SpeechPreferences{Voice: "  voice-id  ", OpenCode: true})
	if p.Speech.Voice != "voice-id" {
		t.Fatalf("voice %q was not trimmed", p.Speech.Voice)
	}
	if !p.Speech.On(events.OpenCode) || p.Speech.On(events.Codex) {
		t.Fatalf("toggles %+v", p.Speech)
	}
	if p.Speech.ReplaceSound {
		t.Fatal("replace was turned on unexpectedly")
	}
}

func TestSpeechChoicesSurviveASave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	want := Default().SetSpeech(SpeechPreferences{Voice: "voice-a", Codex: true, ReplaceSound: true})
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
}

func TestAnnouncementDefaultsToTheSystemVoiceAndTheDefaultClip(t *testing.T) {
	p := Default()
	for _, source := range []events.Source{events.Codex, events.OpenCode} {
		if got := p.Speech.Mode(source); got != SpeechSystem {
			t.Errorf("%s announces with %q, want the system voice", source, got)
		}
		if got := p.Speech.Recording(source); got != recordings.DefaultID {
			t.Errorf("%s plays %q, want the default clip", source, got)
		}
	}
}

func TestSpeechModeAndClipSurviveASave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	want := Default().SetSpeech(SpeechPreferences{
		Codex:             true,
		CodexMode:         SpeechRecording,
		CodexRecording:    "check-your-messages-female",
		OpenCode:          true,
		OpenCodeMode:      SpeechSystem,
		OpenCodeRecording: "message-reminder-female",
	})
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("loaded %+v, want %+v", got.Speech, want.Speech)
	}
	if got.Speech.Mode(events.Codex) != SpeechRecording {
		t.Fatal("Codex lost its recorded announcement")
	}
	if got.Speech.Recording(events.Codex) != "check-your-messages-female" {
		t.Fatalf("Codex plays %q", got.Speech.Recording(events.Codex))
	}
}

func TestAnUnknownSpeechModeOrClipFallsBack(t *testing.T) {
	p := Default().SetSpeech(SpeechPreferences{
		CodexMode:      "telepathy",
		CodexRecording: "not-a-clip",
		OpenCodeMode:   SpeechRecording,
	})
	if p.Speech.CodexMode != "" {
		t.Fatalf("an unknown mode was kept: %q", p.Speech.CodexMode)
	}
	if got := p.Speech.Mode(events.Codex); got != SpeechSystem {
		t.Fatalf("an unknown mode announced with %q", got)
	}
	if got := p.Speech.Recording(events.Codex); got != recordings.DefaultID {
		t.Fatalf("an unknown clip played %q", got)
	}
	if got := p.Speech.Mode(events.OpenCode); got != SpeechRecording {
		t.Fatalf("recording mode was lost: %q", got)
	}
	if got := p.Speech.Recording(events.OpenCode); got != recordings.DefaultID {
		t.Fatalf("a file with no clip played %q", got)
	}
}

func TestQuietHoursCrossMidnight(t *testing.T) {
	quiet := QuietHours{Enabled: true, Start: 22 * 60, End: 7 * 60}
	cases := []struct {
		hour, minute int
		want         bool
	}{
		{21, 59, false},
		{22, 0, true},
		{23, 30, true},
		{0, 0, true},
		{6, 59, true},
		{7, 0, false},
		{12, 0, false},
	}
	for _, c := range cases {
		at := time.Date(2026, 11, 5, c.hour, c.minute, 0, 0, time.Local)
		if got := quiet.Quiet(at); got != c.want {
			t.Errorf("%02d:%02d: quiet = %v, want %v", c.hour, c.minute, got, c.want)
		}
	}
}

func TestQuietHoursEqualEndsBlockTheWholeDay(t *testing.T) {
	quiet := QuietHours{Enabled: true, Start: 9 * 60, End: 9 * 60}
	for _, hour := range []int{0, 9, 15, 23} {
		at := time.Date(2026, 11, 5, hour, 0, 0, 0, time.Local)
		if !quiet.Quiet(at) {
			t.Errorf("%02d:00 is not quiet although the window covers the day", hour)
		}
	}
	if (QuietHours{Enabled: false, Start: 9 * 60, End: 9 * 60}).Quiet(time.Now()) {
		t.Fatal("a disabled window is quiet")
	}
}

func TestPauseExpiresOnItsOwn(t *testing.T) {
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	paused := General{Enabled: true}.PauseFor(at, 15*time.Minute)
	if !paused.PausedAt(at) {
		t.Fatal("a fresh pause is not in effect")
	}
	if !paused.PausedAt(at.Add(14 * time.Minute)) {
		t.Fatal("the pause ended early")
	}
	if paused.PausedAt(at.Add(15 * time.Minute)) {
		t.Fatal("the pause outlived its deadline")
	}
	if got := paused.Remaining(at); got != 15*time.Minute {
		t.Fatalf("remaining %v", got)
	}
	if got := paused.Remaining(at.Add(20 * time.Minute)); got != 0 {
		t.Fatalf("remaining after the deadline is %v", got)
	}
}

func TestPauseUntilResumeHasNoDeadline(t *testing.T) {
	at := time.Now()
	paused := General{Enabled: true}.PauseFor(at, 0)
	if !paused.PausedAt(at.Add(72 * time.Hour)) {
		t.Fatal("a pause until resume expired")
	}
	if resumed := paused.Resume(); resumed.PausedAt(at.Add(time.Minute)) {
		t.Fatal("resume left the pause on")
	}
}

func TestCuesRespectsEverySwitch(t *testing.T) {
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	open := Default()
	if !open.Cues(events.Codex, at) || !open.Cues(events.OpenCode, at) {
		t.Fatal("the defaults do not cue")
	}

	off := open.SetGeneral(General{Enabled: false})
	if off.Cues(events.Codex, at) {
		t.Fatal("a disabled app cued")
	}

	hidden := open.SetDisplay(events.Codex, DisplayPreferences{Enabled: false})
	if hidden.Cues(events.Codex, at) {
		t.Fatal("a hidden producer cued")
	}
	if !hidden.Cues(events.OpenCode, at) {
		t.Fatal("hiding codex also hid opencode")
	}

	paused := open.SetGeneral(open.General.PauseFor(at, time.Hour))
	if paused.Cues(events.Codex, at) {
		t.Fatal("a paused app cued")
	}

	quiet := open.SetGeneral(General{Enabled: true, QuietHours: QuietHours{Enabled: true, Start: 11 * 60, End: 13 * 60}})
	if quiet.Cues(events.Codex, at) {
		t.Fatal("a cue played inside the quiet hours")
	}
}

func TestNormalizedRepairsTheDisplayBlock(t *testing.T) {
	p := Preferences{
		Display: DisplayChoices{
			Codex:    DisplayPreferences{Enabled: true, Animation: overlay.Animation(99), Color: "red"},
			OpenCode: DisplayPreferences{Enabled: true, Animation: overlay.AlternatingFlash, Color: "#123456"},
		},
	}.Normalized()
	if p.Display.Codex.Animation != overlay.Breathing {
		t.Fatalf("an unknown animation stayed: %v", p.Display.Codex.Animation)
	}
	if p.Display.Codex.Color != DefaultColors.Codex.Color {
		t.Fatalf("an invalid colour stayed: %q", p.Display.Codex.Color)
	}
	if p.Display.OpenCode.Color != "#123456" {
		t.Fatalf("a colour the picker can set was replaced: %q", p.Display.OpenCode.Color)
	}
	if p.Display.OpenCode.Animation != overlay.AlternatingFlash {
		t.Fatalf("a valid animation was changed: %v", p.Display.OpenCode.Animation)
	}
}

func TestLoadUpgradesAFileFromTheFirstBuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	body := `{"sound":{"codex":{"id":"knock","volume":12}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != Version {
		t.Fatalf("version %d after loading an old file", p.Version)
	}
	if !p.General.Enabled {
		t.Fatal("an old file switched the master switch off")
	}
	if p.General.Language != LanguageSystem {
		t.Fatalf("an old file chose the language %q", p.General.Language)
	}
	if !p.Display.Codex.Enabled || p.Display.Codex.Color != DefaultColors.Codex.Color {
		t.Fatalf("an old file left the display block empty: %+v", p.Display.Codex)
	}
	if p.Sound.Codex.ID != "knock" {
		t.Fatalf("the upgrade lost the stored sound: %+v", p.Sound.Codex)
	}
}

func TestDisplayChoicesSurviveASave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	choice := DisplayPreferences{Enabled: true, Animation: overlay.AlternatingFlash, Color: "#F472B6"}
	want := Default().
		SetDisplay(events.OpenCode, choice).
		SetGeneral(General{Language: LanguageEnglish, Enabled: true, OpenAtLogin: true})
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
	if got.Display.OpenCode.Colour() != (color.NRGBA{R: 0xF4, G: 0x72, B: 0xB6, A: 255}) {
		t.Fatalf("the colour came back as %v", got.Display.OpenCode.Colour())
	}
}

func TestEachEffectDrawsItsOwnColours(t *testing.T) {
	pair := DisplayPreferences{
		Color:     "#5EEAD4",
		Primary:   "#F472B6",
		Secondary: "#A78BFA",
	}
	if got, want := pair.FlashSecondary(), (color.NRGBA{R: 0xA7, G: 0x8B, B: 0xFA, A: 255}); got != want {
		t.Fatalf("flash partner = %v, want %v", got, want)
	}
	// Monochrome breathing draws one colour and ignores the pair.
	breathing := pair
	breathing.Animation = overlay.Breathing
	primary, secondary := breathing.Cue()
	if primary != breathing.Colour() || secondary != primary {
		t.Fatalf("breathing draws %v and %v", primary, secondary)
	}
	// The two-color flash draws the pair, not the single colour.
	flash := pair
	flash.Animation = overlay.AlternatingFlash
	primary, secondary = flash.Cue()
	if primary != flash.FlashPrimary() || secondary != flash.FlashSecondary() {
		t.Fatalf("the flash draws %v and %v", primary, secondary)
	}
	// A string that is not a colour falls back to the default pair rather than
	// reaching the overlay.
	repaired := Default().
		SetDisplay(events.Codex, DisplayPreferences{Color: "#5EEAD4", Primary: "not-a-colour", Secondary: "nope"}).
		Display.Codex
	if repaired.Primary != DefaultFlashPrimary || repaired.Secondary != DefaultFlashSecondary {
		t.Fatalf("a broken pair survived as %q / %q", repaired.Primary, repaired.Secondary)
	}
}

func TestSpeechTextIsTrimmedAndCapped(t *testing.T) {
	long := strings.Repeat("提", MaxText+40)
	p := Default().SetSpeech(SpeechPreferences{CodexText: "  " + long + "  "})
	if got := p.Speech.Text(events.Codex); len([]rune(got)) != MaxText {
		t.Fatalf("stored phrase is %d runes, want %d", len([]rune(got)), MaxText)
	}
	if got := p.Speech.Text(events.OpenCode); got != "" {
		t.Fatalf("opencode picked up codex's phrase: %q", got)
	}
	empty := Default()
	if got := empty.Speech.Text(events.Codex); got != "" {
		t.Fatalf("a fresh install speaks %q instead of the built-in phrase", got)
	}
}

func TestSetTextChangesOnlyOneProducer(t *testing.T) {
	p := Default().SetSpeech(SpeechPreferences{CodexText: "a", OpenCodeText: "b"})
	updated := p.SetSpeech(p.Speech.SetText(events.OpenCode, "c"))
	if updated.Speech.OpenCodeText != "c" || updated.Speech.CodexText != "a" {
		t.Fatalf("phrases are %q and %q", updated.Speech.CodexText, updated.Speech.OpenCodeText)
	}
}

func TestSpeechTextSurvivesASave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	want := Default().SetSpeech(SpeechPreferences{Codex: true, CodexText: "Codex 已完成，请回来查看。"})
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
}

func TestFreshInstallBreathesBlueAndFlashesBlueThenRed(t *testing.T) {
	p := Default()
	for _, source := range []events.Source{events.Codex, events.OpenCode} {
		d := p.Display.For(source)
		if d.Animation != overlay.Breathing {
			t.Errorf("%s starts in %v, want Monochrome Breathing", source, d.Animation)
		}
		if d.Color != DefaultColour {
			t.Errorf("%s breathes in %q, want %q", source, d.Color, DefaultColour)
		}
		if got := d.Colour(); got != (color.NRGBA{R: 0x3D, G: 0x94, B: 0xFF, A: 255}) {
			t.Errorf("%s resolves to %v", source, got)
		}
		// The flash keeps the same blue as its main colour and adds the red.
		if got := d.FlashPrimary(); got != d.Colour() {
			t.Errorf("%s flashes %v first, want the product blue", source, got)
		}
		if got := d.FlashSecondary(); got != (color.NRGBA{R: 0xFF, G: 0x14, B: 0x1F, A: 255}) {
			t.Errorf("%s alternates with %v, want the red", source, got)
		}
	}
}

func TestUpgradeSwapsTheVersionFourFlashPair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	body := `{"version":4,"display":{"codex":{"enabled":true,"animation":"alternating-flash","color":"#3D94FF","primary":"#FF141F","secondary":"#267AFF"}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d := p.Display.Codex
	if d.Primary != DefaultFlashPrimary || d.Secondary != DefaultFlashSecondary {
		t.Fatalf("the version four pair survived as %q / %q", d.Primary, d.Secondary)
	}
}

// TestUpgradeKeepsAChosenFlashPair stops the version 5 swap from repainting a
// pair the user picked.
func TestUpgradeKeepsAChosenFlashPair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	body := `{"version":4,"display":{"codex":{"enabled":true,"animation":"alternating-flash","color":"#3D94FF","primary":"#F472B6","secondary":"#A78BFA"}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d := p.Display.Codex
	if d.Primary != "#F472B6" || d.Secondary != "#A78BFA" {
		t.Fatalf("a chosen pair became %q / %q", d.Primary, d.Secondary)
	}
}

func TestOlderFileGetsTheDefaultFlashPair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	body := `{"version":3,"display":{"codex":{"enabled":true,"animation":"alternating-flash","color":"#3D94FF","secondary":"#A78BFA"}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d := p.Display.Codex
	if d.Color != "#3D94FF" {
		t.Fatalf("the single colour was lost: %q", d.Color)
	}
	if d.Primary != DefaultFlashPrimary || d.Secondary != DefaultFlashSecondary {
		t.Fatalf("the old violet survived as %q / %q", d.Primary, d.Secondary)
	}
}

func TestUpgradeMovesTheOldDefaultColoursToBlue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	body := `{"version":2,"display":{
		"codex":{"enabled":true,"animation":"breathing","color":"#5EEAD4"},
		"opencode":{"enabled":true,"animation":"alternating-flash","color":"#A78BFA"}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Display.Codex.Color != DefaultColour || p.Display.OpenCode.Color != DefaultColour {
		t.Fatalf("old defaults survived: %+v", p.Display)
	}
}

func TestUpgradeKeepsAColourTheUserPicked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	body := `{"version":2,"display":{"codex":{"enabled":true,"animation":"breathing","color":"#F87171"}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Display.Codex.Color != "#F87171" {
		t.Fatalf("a chosen colour was replaced by %q", p.Display.Codex.Color)
	}
}
