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

// Version is what this build writes into the file.
const Version = 2

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

// SpeechPreferences is the spoken announcement. The text is not stored: it is
// a constant per producer, so no file can make the app read a reply aloud.
type SpeechPreferences struct {
	// Voice is the SAPI token id; empty means the system default.
	Voice string `json:"voice"`
	// Codex and OpenCode say whether that producer is announced.
	Codex    bool `json:"codex"`
	OpenCode bool `json:"opencode"`
	// ReplaceSound silences the cue sound when a phrase is spoken. Speech is
	// additive by default, so a voice that fails never leaves a cue silent.
	ReplaceSound bool `json:"replaceSound"`
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
	return p
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
	return stored.Normalized(), nil
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
