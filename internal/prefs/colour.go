package prefs

import (
	"fmt"
	"image/color"
	"math"
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

// shiftHue moves a colour round the wheel, keeping its saturation and
// brightness, so the second colour of a flash looks like a relative of the
// first rather than a random one.
func shiftHue(c color.NRGBA, degrees float64) color.NRGBA {
	h, s, v := rgbToHSV(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
	h = math.Mod(h+degrees, 360)
	if h < 0 {
		h += 360
	}
	r, g, b := hsvToRGB(h, s, v)
	return color.NRGBA{R: toByte(r), G: toByte(g), B: toByte(b), A: 255}
}

func toByte(v float64) byte {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return byte(math.Round(v * 255))
}

func rgbToHSV(r, g, b float64) (h, s, v float64) {
	high := math.Max(r, math.Max(g, b))
	low := math.Min(r, math.Min(g, b))
	v = high
	delta := high - low
	if high > 0 {
		s = delta / high
	}
	if delta == 0 {
		return 0, s, v
	}
	switch high {
	case r:
		h = math.Mod((g-b)/delta, 6)
	case g:
		h = (b-r)/delta + 2
	default:
		h = (r-g)/delta + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}

func hsvToRGB(h, s, v float64) (r, g, b float64) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}
