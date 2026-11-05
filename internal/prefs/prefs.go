// Package prefs stores the user's choices: right now the cue sound and volume
// for each producer. A missing or unreadable file is not an error the user has
// to fix; the defaults cue instead.
package prefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/sound"
)

// Preferences is the whole file. Fields are added, not renamed, so a file
// written by an older build keeps working.
type Preferences struct {
	Sound SoundPreferences `json:"sound"`
}

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

// Default is what a fresh install cues: a rising chime for Codex, a falling one
// for OpenCode, both at a volume that is audible without being startling.
func Default() Preferences {
	return Preferences{
		Sound: SoundPreferences{
			Codex:    SoundChoice{ID: "rise", Volume: 70},
			OpenCode: SoundChoice{ID: "fall", Volume: 70},
		},
	}
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

// Normalized fills in what a file left out and repairs what a file got wrong.
// A clue that no longer exists falls back to the default sound rather than
// leaving the producer silent; a volume outside 0-100 is clamped.
func (p Preferences) Normalized() Preferences {
	defaults := Default()
	p.Sound.Codex = normalizeChoice(p.Sound.Codex, defaults.Sound.Codex)
	p.Sound.OpenCode = normalizeChoice(p.Sound.OpenCode, defaults.Sound.OpenCode)
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
