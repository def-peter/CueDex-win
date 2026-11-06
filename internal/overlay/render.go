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

// glowMetrics is one display's ring in its own physical pixels: how wide the
// gradient is, and the hairline the mac app strokes around the screen.
type glowMetrics struct {
	width  float64
	inset  float64
	radius float64
	stroke float64
}

// glowBand is one band's place on its display, plus the ring's metrics. It is
// what fillBand needs, so a test can paint a band that matches the overlay's
// geometry without a window.
type glowBand struct {
	width, height      int
	offsetX, offsetY   int
	displayW, displayH int
	edge               Edge
	metrics            glowMetrics
}

// glowProfile is the mac app's gradient: half opacity at the screen edge, 0.18
// at 42% of the width, clear at the inner edge. One smoothstep from full
// opacity reads as a solid bar with a soft inner edge; two stops read as light.
func glowProfile(d, width float64) float64 {
	if width <= 0 || d >= width {
		return 0
	}
	if d <= 0 {
		return 0.5
	}
	x := d / width
	if x <= 0.42 {
		return 0.5 + (0.18-0.5)*(x/0.42)
	}
	return 0.18 * (1 - (x-0.42)/0.58)
}

// hairlineAlpha is the 2.5 pt stroke the mac app draws 2 pt inside the edge,
// on a rounded rectangle. It is what makes the four bands read as one ring:
// the stroke runs unbroken around the corners, where four straight gradients
// would otherwise stop dead and leave a seam.
func hairlineAlpha(dx, dy float64, m glowMetrics) float64 {
	half := m.stroke / 2
	if half <= 0 {
		return 0
	}
	// Signed distance to the inset rounded rectangle, in physical pixels.
	qx := m.inset + m.radius - dx
	qy := m.inset + m.radius - dy
	sd := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) +
		math.Min(math.Max(qx, qy), 0) - m.radius
	// A one-pixel knee, so the stroke is not a staircase.
	knee := (half - math.Abs(sd))
	if knee <= 0 {
		return 0
	}
	if knee > 1 {
		knee = 1
	}
	return 0.62 * knee
}

// ringTable is every alpha the band can show, indexed by the pixel's distance
// to the nearest vertical edge and to the nearest horizontal one, both capped
// at the band's width. The ring is symmetric, so those two distances are all a
// pixel's opacity depends on; sharing one table is also what keeps the corners
// continuous: the last row of a horizontal band reads the same entry as the
// first row of the vertical band below it.
func ringTable(m glowMetrics, alpha uint8) ([]byte, int) {
	n := int(math.Ceil(m.width)) + 1
	if n < 2 {
		n = 2
	}
	table := make([]byte, n*n)
	for i := 0; i < n; i++ {
		glowX := glowProfile(float64(i), m.width)
		for j := 0; j < n; j++ {
			glowY := glowProfile(float64(j), m.width)
			band := 1 - (1-glowX)*(1-glowY)
			hair := hairlineAlpha(float64(i), float64(j), m)
			a := 1 - (1-band)*(1-hair)
			if a < 0 {
				a = 0
			}
			if a > 1 {
				a = 1
			}
			table[i*n+j] = byte(a * float64(alpha))
		}
	}
	return table, n
}

// edgeIndex is a pixel's distance to the nearer of two opposite edges, capped
// at the last table entry.
func edgeIndex(p, size, n int) int {
	d := p
	if other := size - 1 - p; other < d {
		d = other
	}
	if d < 0 {
		d = 0
	}
	if d >= n {
		d = n - 1
	}
	return d
}

// fillBand writes a band's premultiplied BGRA pixels into dst, which must hold
// g.width*g.height*4 bytes. The bitmap is premultiplied because
// UpdateLayeredWindow blends it with AC_SRC_ALPHA.
func fillBand(dst []byte, g glowBand, c color.NRGBA) {
	table, n := ringTable(g.metrics, c.A)
	var premultiplied [256][4]byte
	for a := 0; a < 256; a++ {
		unit := float64(a) / 255
		premultiplied[a] = [4]byte{
			byte(float64(c.B) * unit), // B
			byte(float64(c.G) * unit), // G
			byte(float64(c.R) * unit), // R
			byte(a),
		}
	}

	columns := make([]int, g.width)
	for x := range columns {
		columns[x] = edgeIndex(g.offsetX+x, g.displayW, n)
	}
	rows := make([]int, g.height)
	for y := range rows {
		rows[y] = edgeIndex(g.offsetY+y, g.displayH, n)
	}

	stride := g.width * 4
	horizontal := g.edge == EdgeTop || g.edge == EdgeBottom
	if !horizontal {
		// A vertical band sits away from the corners entirely, so its
		// columns are all it varies by; the table already holds the rest.
		for y := 0; y < g.height; y++ {
			row := dst[y*stride:]
			dy := rows[y]
			for x := 0; x < g.width; x++ {
				copy(row[x*4:x*4+4], premultiplied[table[columns[x]*n+dy]][:])
			}
		}
		return
	}

	// A horizontal band is wide: everything between the table's ends is one
	// constant pixel per row, which is most of the bitmap.
	left := n
	if left > g.width {
		left = g.width
	}
	right := g.width - n
	if right < left {
		right = left
	}
	for y := 0; y < g.height; y++ {
		row := dst[y*stride:]
		dy := rows[y]
		for x := 0; x < left; x++ {
			copy(row[x*4:x*4+4], premultiplied[table[columns[x]*n+dy]][:])
		}
		fillRun(row[left*4:right*4], premultiplied[table[columns[left-1]*n+dy]])
		for x := right; x < g.width; x++ {
			copy(row[x*4:x*4+4], premultiplied[table[columns[x]*n+dy]][:])
		}
	}
}

// fillRun repeats one pixel across a contiguous run. Doubling the filled part
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
// flash blinks them one after the other inside a single three-second fade.
func sample(animation Animation, elapsed time.Duration) (envelope, primary, secondary float64) {
	if animation != AlternatingFlash {
		return breathe(elapsed.Seconds(), CueDuration.Seconds()), 1, 0
	}
	// Two animations layered the way the mac app layers them: the colours
	// blink on a 950 ms cycle, while the envelope fades in once at the start
	// and out once at the end. Fading the envelope per blink would make every
	// other blink dimmer than the one before it.
	phase := flashPhase(elapsed)
	return sampleFrames(flashEnvelope, elapsed.Seconds()/CueDuration.Seconds()),
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
