// Package prefs stores the user's choices: the master switch, the pause and the
// quiet hours, the interface language, whether the app starts with Windows, and
// each producer's colour, animation, sound and announcement. A missing or
// unreadable file is not an error the user has to fix; the defaults cue
// instead.
package prefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/recordings"
	"github.com/def-peter/cuedex-win/internal/sound"
)

// Preferences is the whole file. Fields are added, not renamed, so a file
// written by an older build keeps working.
type Preferences struct {
	// Version is 2 once the general and display blocks exist. A file without
	// it was written by the first build, whose missing blocks take defaults.
	Version int               `json:"version"`
	General General           `json:"general"`
	Display DisplayChoices    `json:"display"`
	Sound   SoundPreferences  `json:"sound"`
	Speech  SpeechPreferences `json:"speech"`
}

// Version is what this build writes into the file. Version 2 is the first that
// has the general and display blocks; 3 is the first whose default glow colour
// is the product blue. A file without a version was written by the first
// build, whose missing blocks take defaults.
const Version = 5

// SoundPreferences is the cue sound per producer. Each producer has its own, so
// the two are told apart by ear as well as by colour.
type SoundPreferences struct {
	Codex    SoundChoice `json:"codex"`
	OpenCode SoundChoice `json:"opencode"`
}

// SoundChoice is one producer's sound: the bundled WAV's ID and how loud it
// plays, 0 being silent.
type SoundChoice struct {
	ID     string `json:"id"`
	Volume int    `json:"volume"`
}

// SpeechPreferences is the spoken announcement. Only this text is ever spoken:
// it is the user's own fixed phrase per producer, never a reply, a prompt or
// anything read from an event.
type SpeechPreferences struct {
	// Voice is the SAPI token id; empty means the system default.
	Voice string `json:"voice"`
	// Codex and OpenCode say whether that producer is announced.
	Codex    bool `json:"codex"`
	OpenCode bool `json:"opencode"`
	// CodexMode and OpenCodeMode say how each producer is announced; an
	// empty one is the system voice, which is what a file written before
	// the bundled clips existed means.
	CodexMode    SpeechMode `json:"codexMode,omitempty"`
	OpenCodeMode SpeechMode `json:"opencodeMode,omitempty"`
	// CodexRecording and OpenCodeRecording name the bundled clip played in
	// recording mode. An empty one is the default clip.
	CodexRecording    string `json:"codexRecording,omitempty"`
	OpenCodeRecording string `json:"opencodeRecording,omitempty"`
	// ReplaceSound silences the cue sound when a phrase is spoken. Speech is
	// additive by default, so a voice that fails never leaves a cue silent.
	ReplaceSound bool `json:"replaceSound"`
	// CodexText and OpenCodeText are the phrases spoken for each producer. An
	// empty one speaks the built-in phrase.
	CodexText    string `json:"codexText,omitempty"`
	OpenCodeText string `json:"opencodeText,omitempty"`
}

// SpeechMode is how a producer is announced: read aloud by an installed SAPI
// voice, or played as one of the bundled clips. A machine with no SAPI voice
// can still be announced with a clip.
type SpeechMode string

const (
	// SpeechSystem reads the stored phrase with a SAPI voice.
	SpeechSystem SpeechMode = "system"
	// SpeechRecording plays a clip that ships with the app.
	SpeechRecording SpeechMode = "recording"
)

// Mode is how a producer is announced. An empty or unknown value is the
// system voice, so a file written by an older build reads as it always did.
func (s SpeechPreferences) Mode(source events.Source) SpeechMode {
	mode := s.CodexMode
	if source == events.OpenCode {
		mode = s.OpenCodeMode
	}
	if mode == SpeechRecording {
		return SpeechRecording
	}
	return SpeechSystem
}

// SetMode replaces one producer's announcement mode and returns the result.
func (s SpeechPreferences) SetMode(source events.Source, mode SpeechMode) SpeechPreferences {
	if source == events.OpenCode {
		s.OpenCodeMode = mode
	} else {
		s.CodexMode = mode
	}
	return s
}

// Recording is the bundled clip a producer plays in recording mode. An empty
// or unknown value is the default clip, so the choice is always playable.
func (s SpeechPreferences) Recording(source events.Source) string {
	id := s.CodexRecording
	if source == events.OpenCode {
		id = s.OpenCodeRecording
	}
	if _, ok := recordings.Lookup(id); ok {
		return id
	}
	return recordings.DefaultID
}

// SetRecording replaces one producer's clip and returns the result.
func (s SpeechPreferences) SetRecording(source events.Source, id string) SpeechPreferences {
	if source == events.OpenCode {
		s.OpenCodeRecording = id
	} else {
		s.CodexRecording = id
	}
	return s
}

// MaxText is how long a spoken phrase may be, in runes. A cue says one short
// sentence; anything longer is a paste, not an announcement.
const MaxText = 120

// Text is the phrase spoken for a producer, or "" to speak the built-in one.
func (s SpeechPreferences) Text(source events.Source) string {
	if source == events.OpenCode {
		return s.OpenCodeText
	}
	return s.CodexText
}

// SetText replaces one producer's phrase and returns the result.
func (s SpeechPreferences) SetText(source events.Source, text string) SpeechPreferences {
	if source == events.OpenCode {
		s.OpenCodeText = text
	} else {
		s.CodexText = text
	}
	return s
}

// On reports whether a producer is announced.
func (s SpeechPreferences) On(source events.Source) bool {
	if source == events.OpenCode {
		return s.OpenCode
	}
	return s.Codex
}

// Default is what a fresh install cues: a rising chime for Codex, a falling one
// for OpenCode, both at a volume that is audible without being startling.
func Default() Preferences {
	return Preferences{
		Version: Version,
		General: General{
			Language:    LanguageSystem,
			Enabled:     true,
			QuietHours:  QuietHours{Enabled: false, Start: 22 * 60, End: 7 * 60},
			OpenAtLogin: false,
		},
		Display: DefaultColors,
		Sound: SoundPreferences{
			Codex:    SoundChoice{ID: "rise", Volume: 70},
			OpenCode: SoundChoice{ID: "fall", Volume: 70},
		},
		Speech: SpeechPreferences{Voice: "", Codex: false, OpenCode: false},
	}
}

// Display returns one producer's presentation.
func (p Preferences) DisplayChoice(source events.Source) DisplayPreferences {
	return p.Display.For(source)
}

// SetDisplay replaces one producer's presentation and returns the result.
func (p Preferences) SetDisplay(source events.Source, d DisplayPreferences) Preferences {
	p.Display = p.Display.Set(source, d)
	return p.Normalized()
}

// SetGeneral replaces the application-wide settings and returns the result.
func (p Preferences) SetGeneral(g General) Preferences {
	p.General = g
	return p.Normalized()
}

// Cues reports whether at is a moment a cue from source may play: the master
// switch is on, the producer is drawn, the app is not paused, and it is not
// inside the quiet hours.
func (p Preferences) Cues(source events.Source, at time.Time) bool {
	if !p.General.Enabled {
		return false
	}
	if !p.Display.For(source).CanDraw() {
		return false
	}
	if p.General.PausedAt(at) {
		return false
	}
	return !p.General.QuietHours.Quiet(at)
}

// For returns the choice that applies to a producer.
func (p Preferences) For(source events.Source) SoundChoice {
	if source == events.OpenCode {
		return p.Sound.OpenCode
	}
	return p.Sound.Codex
}

// Set replaces one producer's choice and returns the result, so the caller
// keeps a value it can save.
func (p Preferences) Set(source events.Source, choice SoundChoice) Preferences {
	if source == events.OpenCode {
		p.Sound.OpenCode = choice
	} else {
		p.Sound.Codex = choice
	}
	return p.Normalized()
}

// SetSpeech replaces the speech preferences and returns the result.
func (p Preferences) SetSpeech(s SpeechPreferences) Preferences {
	p.Speech = s
	return p.Normalized()
}

// Normalized fills in what a file left out and repairs what a file got wrong.
// A clue that no longer exists falls back to the default sound rather than
// leaving the producer silent; a volume outside 0-100 is clamped.
func (p Preferences) Normalized() Preferences {
	defaults := Default()
	p.Version = Version
	p.General = normalizeGeneral(p.General)
	p.Display.Codex = normalizeDisplay(p.Display.Codex, defaults.Display.Codex)
	p.Display.OpenCode = normalizeDisplay(p.Display.OpenCode, defaults.Display.OpenCode)
	p.Sound.Codex = normalizeChoice(p.Sound.Codex, defaults.Sound.Codex)
	p.Sound.OpenCode = normalizeChoice(p.Sound.OpenCode, defaults.Sound.OpenCode)
	p.Speech.Voice = strings.TrimSpace(p.Speech.Voice)
	p.Speech.CodexText = normalizeText(p.Speech.CodexText)
	p.Speech.OpenCodeText = normalizeText(p.Speech.OpenCodeText)
	p.Speech.CodexMode = normalizeMode(p.Speech.CodexMode)
	p.Speech.OpenCodeMode = normalizeMode(p.Speech.OpenCodeMode)
	p.Speech.CodexRecording = normalizeRecording(p.Speech.CodexRecording)
	p.Speech.OpenCodeRecording = normalizeRecording(p.Speech.OpenCodeRecording)
	return p
}

// normalizeMode keeps a mode this build knows and drops one it does not, so a
// file from a newer build still opens and reads as the system voice.
func normalizeMode(mode SpeechMode) SpeechMode {
	switch mode {
	case SpeechSystem, SpeechRecording:
		return mode
	}
	return ""
}

// normalizeRecording keeps a clip this build bundles. A name from another
// build is dropped, and Recording then falls back to the default clip.
func normalizeRecording(id string) string {
	if _, ok := recordings.Lookup(id); ok {
		return id
	}
	return ""
}

// normalizeText trims a phrase and cuts it to MaxText runes, so a pasted
// paragraph cannot become a minute of speech from a single cue.
func normalizeText(text string) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= MaxText {
		return text
	}
	return strings.TrimSpace(string(runes[:MaxText]))
}

func normalizeChoice(choice, fallback SoundChoice) SoundChoice {
	if _, ok := sound.Lookup(choice.ID); !ok {
		choice.ID = fallback.ID
	}
	if choice.Volume < 0 {
		choice.Volume = 0
	}
	if choice.Volume > 100 {
		choice.Volume = 100
	}
	return choice
}

// Load reads the file. A file that is not there is the defaults; a file that is
// corrupt is the defaults plus the reason, so the caller can log one line and
// carry on cueing.
func Load(path string) (Preferences, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Default(), err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return Default(), nil
	}
	var stored Preferences
	if err := json.Unmarshal(data, &stored); err != nil {
		return Default(), fmt.Errorf("prefs: %s: %w", path, err)
	}
	// A file from the first build has no version, and a zero-value General
	// there would switch the master switch off and the language to Chinese.
	// An absent block is not a choice, so the defaults fill it in.
	if stored.Version == 0 {
		defaults := Default()
		stored.General = defaults.General
		stored.Display = defaults.Display
	}
	// Version 3 made the product blue the default glow. A file from before it
	// carries the teal and violet that version 2 wrote for a fresh install,
	// which were never a choice, so the upgrade moves them to the new default.
	if stored.Version > 0 && stored.Version < 3 {
		stored.Display.Codex = replaceOldDefault(stored.Display.Codex, "#5EEAD4")
		stored.Display.OpenCode = replaceOldDefault(stored.Display.OpenCode, "#A78BFA")
	}
	// Version 4 gave 双色交替闪烁 a pair of its own, stored apart from the
	// single colour 单色呼吸 uses. A file from before it carries one colour for
	// both effects, and possibly the violet version 3 derived as a partner;
	// the pair starts from the defaults instead.
	if stored.Version > 0 && stored.Version < 4 {
		stored.Display.Codex.Primary, stored.Display.Codex.Secondary = "", ""
		stored.Display.OpenCode.Primary, stored.Display.OpenCode.Secondary = "", ""
	}
	// Version 5 reads the flash the other way round: the product blue is the
	// main colour and the red is the accent it alternates with. Only that exact
	// pair moves; a pair the user chose stays.
	if stored.Version > 0 && stored.Version < 5 {
		stored.Display.Codex = swapOldFlashPair(stored.Display.Codex)
		stored.Display.OpenCode = swapOldFlashPair(stored.Display.OpenCode)
	}
	return stored.Normalized(), nil
}

// swapOldFlashPair moves the pair version 4 wrote for a fresh install to the
// one this build ships: the blue first, the red second.
func swapOldFlashPair(d DisplayPreferences) DisplayPreferences {
	if d.Primary == "#FF141F" && d.Secondary == "#267AFF" {
		d.Primary, d.Secondary = DefaultFlashPrimary, DefaultFlashSecondary
	}
	return d
}

// replaceOldDefault moves a colour that was the default of an older version to
// the one the app now ships. Only that exact colour moves: any other value is
// the user's own choice, even one that is close to it.
func replaceOldDefault(d DisplayPreferences, old string) DisplayPreferences {
	if d.Color == old {
		d.Color = DefaultColour
	}
	return d
}

// Save writes the file, replacing whatever was there. The write goes through a
// temporary file so a crash mid-save cannot leave half a preferences file.
func Save(path string, p Preferences) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p.Normalized(), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".preferences-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		os.Remove(name)
		return err
	}
	if err := temp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
