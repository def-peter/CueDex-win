package prefs

import (
	"image/color"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/overlay"
)

// The defaults keep one product colour across both effects: 单色呼吸 pulses in
// the blue, and 双色交替闪烁 keeps that blue as its primary and adds the red
// the mac app flashes as a warning accent.
const (
	// DefaultColour is the one colour 单色呼吸 pulses in: the product blue.
	DefaultColour = "#3D94FF"
	// DefaultFlashPrimary is the main colour of 双色交替闪烁, the same blue, and
	// DefaultFlashSecondary the red it alternates with, as a warning light.
	DefaultFlashPrimary   = DefaultColour
	DefaultFlashSecondary = "#FF141F"
)

// DefaultColors is the appearance each producer has on a fresh install. Both
// start in 单色呼吸 with the product blue, as the mac app does.
var DefaultColors = DisplayChoices{
	Codex: DisplayPreferences{
		Enabled: true, Animation: overlay.Breathing,
		Color: DefaultColour, Primary: DefaultFlashPrimary, Secondary: DefaultFlashSecondary,
	},
	OpenCode: DisplayPreferences{
		Enabled: true, Animation: overlay.Breathing,
		Color: DefaultColour, Primary: DefaultFlashPrimary, Secondary: DefaultFlashSecondary,
	},
}

// DisplayPreferences is one producer's appearance: whether it is drawn at all,
// how it moves, and the colours it moves in.
type DisplayPreferences struct {
	Enabled   bool              `json:"enabled"`
	Animation overlay.Animation `json:"animation"`
	// Color is the colour of 单色呼吸.
	Color string `json:"color"`
	// Primary and Secondary are the pair 双色交替闪烁 alternates. They are kept
	// apart from Color because the two effects are chosen separately: switching
	// the effect must not repaint the other one, and each keeps a sensible
	// default of its own.
	Primary   string `json:"primary,omitempty"`
	Secondary string `json:"secondary,omitempty"`
}

// DisplayChoices is one presentation per producer.
type DisplayChoices struct {
	Codex    DisplayPreferences `json:"codex"`
	OpenCode DisplayPreferences `json:"opencode"`
}

// Colour is the colour of 单色呼吸. Normalized has already checked the string,
// so this only guards against a value built by hand.
func (d DisplayPreferences) Colour() color.NRGBA {
	if c, ok := ParseColour(d.Color); ok {
		return c
	}
	c, _ := ParseColour(DefaultColour)
	return c
}

// FlashPrimary and FlashSecondary are the pair 双色交替闪烁 alternates.
func (d DisplayPreferences) FlashPrimary() color.NRGBA {
	if c, ok := ParseColour(d.Primary); ok {
		return c
	}
	c, _ := ParseColour(DefaultFlashPrimary)
	return c
}

func (d DisplayPreferences) FlashSecondary() color.NRGBA {
	if c, ok := ParseColour(d.Secondary); ok {
		return c
	}
	c, _ := ParseColour(DefaultFlashSecondary)
	return c
}

// Cue is the pair the overlay draws with. 单色呼吸 uses one colour and ignores
// the second; 双色交替闪烁 alternates the stored pair.
func (d DisplayPreferences) Cue() (primary, secondary color.NRGBA) {
	if d.Animation == overlay.AlternatingFlash {
		return d.FlashPrimary(), d.FlashSecondary()
	}
	c := d.Colour()
	return c, c
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
	if _, ok := ParseColour(d.Color); !ok {
		d.Color = fallback.Color
	}
	if _, ok := ParseColour(d.Primary); !ok {
		d.Primary = fallback.Primary
	}
	if _, ok := ParseColour(d.Secondary); !ok {
		d.Secondary = fallback.Secondary
	}
	return d
}
