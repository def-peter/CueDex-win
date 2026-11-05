package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// The two colors a cue flashes between. The product reads them from the
// preference of the source that fired; the spike only needs colors that are
// unmistakable on both a dark and a light desktop.
var palette = [2]ui.Color{ui.Hex("#3B82F6"), ui.Hex("#F472B6")}

// layers is the edge glow's stack, as the SwiftUI recreation of Apple
// Intelligence's glow draws it and as examples/chat-glow ports it: the
// outline stroked inside the edge this wide, and blurred this much over it.
// The widest, most blurred layer is the halo; the first is the crisp line.
var layers = []struct{ width, blur float32 }{{6, 0}, {9, 4}, {11, 12}, {15, 15}}

// glow is one display's cue: where it covers, when it started, and which
// animation it runs.
type glow struct {
	bounds rect
	// cueAt is when the cue begins. Until then the view asks for one frame
	// at that moment and draws nothing, so an idle cue costs no frames.
	cueAt time.Time
	dur   time.Duration
	flash bool
	// first is the time of the first frame that drew, for the spike's
	// latency check. frames counts the frames after it.
	first  time.Time
	frames int
	// gpu says whether the last frame painted on the GPU, where a blurred
	// full-screen edge costs a fraction of a millisecond instead of
	// hundreds: the number that decides whether this design is affordable.
	gpu bool
}

func newGlow(bounds rect, cueAt time.Time, dur time.Duration, flash bool) *glow {
	return &glow{bounds: bounds, cueAt: cueAt, dur: dur, flash: flash}
}

// progress is how far the cue has come, from 0 to 1, and whether it is
// still cueing at now.
func (g *glow) progress(now time.Time) (float64, bool) {
	if g.dur <= 0 {
		return 0, false
	}
	p := now.Sub(g.cueAt).Seconds() / g.dur.Seconds()
	return p, p >= 0 && p < 1
}

// alpha breathes the cue in and out: nothing at either end, full in the
// middle, so the edge never appears or vanishes in one frame.
func alpha(p float64) float64 {
	return math.Sin(math.Pi * p)
}

// stops are the colors the edge is stroked with at p. Both animations run
// the same two colors around the border and turn the gradient; the flash
// swaps the colors four times a second instead of blending them gradually.
func (g *glow) stops(p float64) []ui.GradientStop {
	a, b := palette[0], palette[1]
	if g.flash && int(p*g.dur.Seconds()*4)%2 == 1 {
		a, b = b, a
	}
	return []ui.GradientStop{{Color: a, At: 0}, {Color: b, At: 0.25}, {Color: a, At: 0.5}, {Color: b, At: 0.75}, {Color: a, At: 1}}
}

// angle turns the gradient once and a half over the cue, so the glow is
// visibly alive even after its brightness settles.
func (g *glow) angle(p float64) float32 {
	return float32(90 + 540*p)
}

// view draws the cue: the window's outline in four layers, over a window
// whose background is never painted, so only the edges show.
func (g *glow) view(c *ui.Context) {
	if now := c.Now(); now.Before(g.cueAt) {
		// A single frame at the cue's start, and none until then.
		c.After(g.cueAt.Sub(now))
		return
	}
	p, live := g.progress(c.Now())
	if !live {
		return
	}
	if g.first.IsZero() {
		g.first = c.Now()
	}
	g.frames++
	// Asks for the next frame for as long as the cue runs, and no frame
	// after it: the window costs nothing while it has nothing to show.
	c.AnimationFrame()

	// The window's own background is not what a cue shows: the desktop
	// behind it is, and only the edges light up.
	c.Root().Background(ui.Transparent)

	gradient := ui.ConicGradient{Stops: g.stops(p), Angle: g.angle(p), Oklab: true}
	if g.frames == 1 {
		// One probe box, painted once, to learn what the frames draw on.
		ui.Box(c).Absolute().Top(0).Left(0).Width(1).Height(1).
			Draw(func(p *ui.Painter, _ ui.Rect) { g.gpu = p.Accelerated() })
	}
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().
		Opacity(float32(alpha(p))).Children(func() {
		for _, l := range layers {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().
				Border(l.width, ui.Color{}).BorderConicGradient(gradient).Blur(0.95 * l.blur)
		}
	})
}
