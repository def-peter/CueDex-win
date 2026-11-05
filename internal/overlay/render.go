package overlay

import (
	"math"
	"time"

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

// fillBand writes a band's premultiplied BGRA pixels into dst, which must hold
// w*h*4 bytes. The bitmap is premultiplied because UpdateLayeredWindow blends it
// with AC_SRC_ALPHA.
//
// The gradient only varies with the distance from the anchored edge, so the
// colour is one lookup per row (or column) rather than a float computation per
// pixel. Filling a band is then a memory write, which is what keeps the start
// of a cue short.
func fillBand(dst []byte, w, h int, edge Edge, c color.NRGBA, thickness float64) {
	steps := int(thickness) + 1
	if steps < 2 {
		steps = 2
	}
	if steps > 4096 {
		steps = 4096
	}
	alphas := make([]byte, steps)
	var premultiplied [256][4]byte
	for a := 0; a < 256; a++ {
		alpha := float64(a)
		premultiplied[a] = [4]byte{
			byte(float64(c.B) * alpha / 255), // B
			byte(float64(c.G) * alpha / 255), // G
			byte(float64(c.R) * alpha / 255), // R
			byte(a),
		}
	}
	for i := 0; i < steps; i++ {
		alphas[i] = byte(float64(c.A) * falloff(float64(i), thickness))
	}
	// A distance beyond the table is fully faded out.
	at := func(d int) byte {
		if d < 0 || d >= steps {
			return 0
		}
		return alphas[d]
	}

	if edge == EdgeTop || edge == EdgeBottom {
		for y := 0; y < h; y++ {
			d := y
			if edge == EdgeBottom {
				d = h - 1 - y
			}
			pixel := premultiplied[at(d)]
			row := dst[y*w*4 : (y+1)*w*4]
			fillRun(row, pixel)
		}
		return
	}
	for x := 0; x < w; x++ {
		d := x
		if edge == EdgeRight {
			d = w - 1 - x
		}
		pixel := premultiplied[at(d)]
		for y := 0; y < h; y++ {
			i := (y*w + x) * 4
			copy(dst[i:i+4], pixel[:])
		}
	}
}

// fillRun repeats one pixel across a contiguous row. Doubling the filled part
// reaches the end in log2(len) copies instead of one copy per pixel.
func fillRun(dst []byte, pixel [4]byte) {
	n := copy(dst, pixel[:])
	for n < len(dst) {
		n += copy(dst[n:], dst[:n])
	}
}

// keyframe is a value at a fraction of a cycle.
type keyframe struct {
	at    float64
	value float64
}

// The envelopes are the mac app's keyframes, so both builds breathe and flash
// at the same rhythm.
var (
	breathingFrames  = []keyframe{{0, 0}, {0.2, 1}, {0.48, 0.48}, {0.78, 1}, {1, 0}}
	flashEnvelope    = []keyframe{{0, 0}, {0.1, 1}, {0.9, 1}, {1, 0}}
	primaryFlashes   = []keyframe{{0, 0}, {0.08, 1}, {0.16, 0}, {0.24, 1}, {0.32, 0}, {1, 0}}
	secondaryFlashes = []keyframe{{0, 0}, {0.5, 0}, {0.58, 1}, {0.66, 0}, {0.74, 1}, {1, 0}}
	flashCyclePeriod = 950 * time.Millisecond
)

// breathe gives the whole overlay its opacity at elapsed of duration. Two
// soft pulses, silent at both ends, so the cue never appears or vanishes as
// a step.
func breathe(elapsed, duration float64) float64 {
	if elapsed <= 0 || elapsed >= duration {
		return 0
	}
	return sampleFrames(breathingFrames, elapsed/duration)
}

// sample returns how strongly the cue is lit and how the two colours share
// that light at elapsed. Breathing uses one colour and no second one; the
// flash blinks them one after the other inside a slow fade.
func sample(animation Animation, elapsed time.Duration) (envelope, primary, secondary float64) {
	if animation != AlternatingFlash {
		return breathe(elapsed.Seconds(), CueDuration.Seconds()), 1, 0
	}
	phase := flashPhase(elapsed)
	return sampleFrames(flashEnvelope, phase),
		sampleFrames(primaryFlashes, phase),
		sampleFrames(secondaryFlashes, phase)
}

// flashPhase is where in the blink cycle elapsed falls, from 0 to 1. The two
// colours take turns at the halfway point, where neither of them is lit.
func flashPhase(elapsed time.Duration) float64 {
	return float64(elapsed%flashCyclePeriod) / float64(flashCyclePeriod)
}

// sampleFrames interpolates between keyframes with a smoothstep, the easing
// the mac app gives its keyframes.
func sampleFrames(frames []keyframe, p float64) float64 {
	if len(frames) == 0 {
		return 0
	}
	if p <= frames[0].at {
		return frames[0].value
	}
	for i := 1; i < len(frames); i++ {
		if p > frames[i].at {
			continue
		}
		span := frames[i].at - frames[i-1].at
		if span <= 0 {
			return frames[i].value
		}
		x := math.Max(0, math.Min(1, (p-frames[i-1].at)/span))
		x = x * x * (3 - 2*x)
		return frames[i-1].value + (frames[i].value-frames[i-1].value)*x
	}
	return frames[len(frames)-1].value
}
