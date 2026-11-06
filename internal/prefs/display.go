package prefs

import (
	"image/color"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/overlay"
)

// Palette is every colour a producer may glow in. The strings are the hex form
// a preferences file holds, so a value never changes meaning.
var Palette = []string{
	"#5EEAD4", // teal, the product colour
	"#A78BFA", // violet
	"#60A5FA", // blue
	"#F472B6", // pink
	"#FBBF24", // amber
	"#34D399", // green
	"#F87171", // coral
	"#94A3B8", // slate
}

// DefaultColors is the colour each producer glows in on a fresh install, one
// per producer so the two are told apart at a glance.
var DefaultColors = DisplayChoices{
	Codex:    DisplayPreferences{Enabled: true, Animation: overlay.Breathing, Color: "#5EEAD4"},
	OpenCode: DisplayPreferences{Enabled: true, Animation: overlay.AlternatingFlash, Color: "#A78BFA"},
}

// DisplayPreferences is one producer's presentation: whether it is drawn at
// all, how it moves, and what colour it glows.
type DisplayPreferences struct {
	Enabled   bool              `json:"enabled"`
	Animation overlay.Animation `json:"animation"`
	Color     string            `json:"color"`
}

// DisplayChoices is one presentation per producer.
type DisplayChoices struct {
	Codex    DisplayPreferences `json:"codex"`
	OpenCode DisplayPreferences `json:"opencode"`
}

// Colour is the producer's glow. Normalized has already checked the string, so
// this only guards against a value built by hand.
func (d DisplayPreferences) Colour() color.NRGBA {
	if c, ok := ParseColour(d.Color); ok {
		return c
	}
	c, _ := ParseColour(Palette[0])
	return c
}

// Secondary is the colour the two-colour flash alternates with: the same
// saturation and brightness, one hue step round, so a colour the user picked
// still has a partner instead of a fixed second colour that might clash.
func (d DisplayPreferences) Secondary() color.NRGBA {
	return shiftHue(d.Colour(), 40)
}

// CanDraw reports whether this producer is drawn.
func (d DisplayPreferences) CanDraw() bool { return d.Enabled }

// For returns one producer's presentation.
func (c DisplayChoices) For(source events.Source) DisplayPreferences {
	if source == events.OpenCode {
		return c.OpenCode
	}
	return c.Codex
}

// Set replaces one producer's presentation.
func (c DisplayChoices) Set(source events.Source, d DisplayPreferences) DisplayChoices {
	if source == events.OpenCode {
		c.OpenCode = d
	} else {
		c.Codex = d
	}
	return c
}

func normalizeDisplay(d DisplayPreferences, fallback DisplayPreferences) DisplayPreferences {
	if !d.Animation.Valid() {
		d.Animation = overlay.Breathing
	}
	if _, ok := ParseColour(d.Color); !ok || !inPalette(d.Color) {
		d.Color = fallback.Color
	}
	return d
}

func inPalette(hex string) bool {
	for _, known := range Palette {
		if known == hex {
			return true
		}
	}
	return false
}
