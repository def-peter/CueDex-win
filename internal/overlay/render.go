package overlay

import (
	"math"

	"image/color"
)

// Edge says which side of a band touches the edge of the display: the side
// the glow starts from and fades away from.
type Edge int

const (
	EdgeTop Edge = iota
	EdgeBottom
	EdgeLeft
	EdgeRight
)

// edgeRect returns the band's rectangle for a display bounds and thickness,
// in physical pixels. Corners belong to the top and bottom bands, so the
// vertical bands start below and end above them and never overlap.
func edgeRect(bounds rect, edge Edge, thickness int32) rect {
	t := thickness
	switch edge {
	case EdgeTop:
		return rect{bounds.Left, bounds.Top, bounds.Right, bounds.Top + t}
	case EdgeBottom:
		return rect{bounds.Left, bounds.Bottom - t, bounds.Right, bounds.Bottom}
	case EdgeLeft:
		return rect{bounds.Left, bounds.Top + t, bounds.Left + t, bounds.Bottom - t}
	default:
		return rect{bounds.Right - t, bounds.Top + t, bounds.Right, bounds.Bottom - t}
	}
}

// falloff says how opaque a pixel is at distance d from the glowing edge,
// between 0 and thickness. It is a smooth curve rather than a line: a glow
// that fades linearly reads as a flat bar with a hard inner edge.
func falloff(d, thickness float64) float64 {
	if d <= 0 {
		return 1
	}
	if d >= thickness {
		return 0
	}
	x := 1 - d/thickness
	return x * x * (3 - 2*x) // smoothstep
}

// bandPixels fills a w×h image with premultiplied BGRA for one band. The
// bitmap is premultiplied because UpdateLayeredWindow blends it with
// AC_SRC_ALPHA.
func bandPixels(w, h int, edge Edge, c color.NRGBA, thickness float64) []byte {
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var d float64
			switch edge {
			case EdgeTop:
				d = float64(y)
			case EdgeBottom:
				d = float64(h - 1 - y)
			case EdgeLeft:
				d = float64(x)
			default:
				d = float64(w - 1 - x)
			}
			a := float64(c.A) * falloff(d, thickness)
			if a <= 0 {
				continue
			}
			i := (y*w + x) * 4
			out[i+0] = byte(float64(c.B) * a / 255) // B
			out[i+1] = byte(float64(c.G) * a / 255) // G
			out[i+2] = byte(float64(c.R) * a / 255) // R
			out[i+3] = byte(a)                      // A
		}
	}
	return out
}

// breathe gives the whole overlay its opacity at elapsed of duration. Two
// soft pulses, silent at both ends, so the cue never appears or vanishes as
// a step.
func breathe(elapsed, duration float64) float64 {
	if elapsed <= 0 || elapsed >= duration {
		return 0
	}
	p := elapsed / duration
	envelope := math.Sin(math.Pi * p)
	envelope = math.Pow(envelope, 0.7)
	wave := 0.55 + 0.45*math.Cos(2*math.Pi*2*p)
	return envelope * wave
}
