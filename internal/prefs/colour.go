package prefs

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// ParseColour reads the "#rrggbb" form a preferences file stores. Anything else
// is not a colour and the caller falls back to its default.
func ParseColour(s string) (color.NRGBA, bool) {
	s = strings.TrimSpace(s)
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}, false
	}
	value, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{
		R: byte(value >> 16 & 0xff),
		G: byte(value >> 8 & 0xff),
		B: byte(value & 0xff),
		A: 255,
	}, true
}

// FormatColour writes a colour the way a preferences file stores it.
func FormatColour(c color.NRGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}
