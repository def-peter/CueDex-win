package overlay

import (
	"image/color"
	"math"
	"testing"
	"time"
)

var white = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

// ringBand builds one edge's shape the way the overlay does, so the tests paint
// the same geometry the windows show.
func ringBand(bounds rect, edge Edge, thickness, inset, radius, stroke float64) glowBand {
	t := int32(math.Round(thickness))
	r := edgeRect(bounds, edge, t)
	return glowBand{
		width:    int(r.width()),
		height:   int(r.height()),
		offsetX:  int(r.Left - bounds.Left),
		offsetY:  int(r.Top - bounds.Top),
		displayW: int(bounds.width()),
		displayH: int(bounds.height()),
		edge:     edge,
		metrics:  glowMetrics{width: thickness, inset: inset, radius: radius, stroke: stroke},
	}
}

func paint(g glowBand, c color.NRGBA) []byte {
	out := make([]byte, g.width*g.height*4)
	fillBand(out, g, c)
	return out
}

// alpha is the opacity of one pixel of a painted band.
func alpha(pix []byte, width, x, y int) byte { return pix[(y*width+x)*4+3] }

// TestGlowProfileMatchesTheMacGradient pins the two stops the mac app uses:
// half opacity at the screen edge, 0.18 at 42% of the width, nothing inside.
// A profile that starts at full opacity is what reads as a solid bar.
func TestGlowProfileMatchesTheMacGradient(t *testing.T) {
	const width = 100.0
	for _, tc := range []struct {
		d, want float64
	}{
		{0, 0.5},
		{42, 0.18},
		{width, 0},
		{width + 1, 0},
	} {
		if got := glowProfile(tc.d, width); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("glowProfile(%v) = %v, want %v", tc.d, got, tc.want)
		}
	}
	prev := glowProfile(0, width)
	for d := 1.0; d <= width; d++ {
		got := glowProfile(d, width)
		if got < 0 || got > prev {
			t.Fatalf("glowProfile(%v) = %v, not decreasing from %v", d, got, prev)
		}
		prev = got
	}
}

// TestBandIsHalfOpaqueAtTheEdge is the other half of the mac look: the glow is
// translucent even at the screen border, so it lights up the desktop instead of
// covering it.
func TestBandIsHalfOpaqueAtTheEdge(t *testing.T) {
	bounds := rect{0, 0, 800, 600}
	top := paint(ringBand(bounds, EdgeTop, 56, 2, 22, 2.5), white)
	if got := alpha(top, 800, 400, 0); math.Abs(float64(got)-127) > 1 {
		t.Fatalf("edge alpha = %d, want 127 (half of 255)", got)
	}
	if got := alpha(top, 800, 400, 55); got > 8 {
		t.Fatalf("inner edge alpha = %d, want nearly nothing", got)
	}
}

// TestHairlineRunsAlongTheEdge checks the 2.5 pt stroke the mac app draws: it
// is what makes the four bands read as one ring.
func TestHairlineRunsAlongTheEdge(t *testing.T) {
	bounds := rect{0, 0, 800, 600}
	top := paint(ringBand(bounds, EdgeTop, 56, 2, 22, 2.5), white)
	stroked := alpha(top, 800, 400, 2)
	plain := alpha(top, 800, 400, 0)
	if stroked < 180 {
		t.Fatalf("hairline alpha = %d, want the stroke to be visible", stroked)
	}
	if stroked <= plain {
		t.Fatalf("hairline %d is not brighter than the edge %d", stroked, plain)
	}
}

// TestCornersMeetWithoutASeam is the regression this shape exists for. The top
// band owns the corner square and the vertical band starts below it; if each
// one ramps only along its own axis the glow stops dead at the joint, which
// reads as four bars rather than one ring.
func TestCornersMeetWithoutASeam(t *testing.T) {
	bounds := rect{0, 0, 800, 600}
	const thickness = 56.0
	top := paint(ringBand(bounds, EdgeTop, thickness, 2, 22, 2.5), white)
	left := paint(ringBand(bounds, EdgeLeft, thickness, 2, 22, 2.5), white)
	tall := int(thickness)

	for x := 0; x < tall; x++ {
		a := alpha(top, 800, x, tall-1)
		b := alpha(left, tall, x, 0)
		if diff := int(a) - int(b); diff > 3 || diff < -3 {
			t.Fatalf("column %d: top band %d, left band %d", x, a, b)
		}
	}
	// And the corner itself has to be lit, not clipped away by the hairline's
	// rounded path.
	if got := alpha(top, 800, 0, 0); got < 150 {
		t.Fatalf("corner alpha = %d, want a lit corner", got)
	}
}

func TestBandPixelsArePremultiplied(t *testing.T) {
	c := color.NRGBA{R: 200, G: 100, B: 50, A: 255}
	pix := paint(ringBand(rect{0, 0, 800, 600}, EdgeTop, 56, 2, 22, 2.5), c)
	a := alpha(pix, 800, 0, 20)
	b := pix[(20*800+0)*4+0]
	if a == 0 || a == 255 {
		t.Fatalf("expected a partial alpha on row 20, got %d", a)
	}
	want := byte(float64(c.B) * float64(a) / 255)
	if b != want {
		t.Fatalf("blue = %d, want %d for alpha %d", b, want, a)
	}
}

func TestBreatheIsSilentAtBothEnds(t *testing.T) {
	const duration = 3.0
	if got := breathe(0, duration); got != 0 {
		t.Fatalf("alpha at start = %v, want 0", got)
	}
	if got := breathe(duration, duration); got != 0 {
		t.Fatalf("alpha at end = %v, want 0", got)
	}
	peak := 0.0
	for i := 0; i <= 300; i++ {
		v := breathe(duration*float64(i)/300, duration)
		if v < 0 || v > 1 {
			t.Fatalf("alpha out of range: %v", v)
		}
		if v > peak {
			peak = v
		}
	}
	if peak < 0.6 {
		t.Fatalf("peak alpha %v is too faint", peak)
	}
}

func TestBreathingFollowsTheKeyframes(t *testing.T) {
	const duration = 3.0
	for _, tc := range []struct {
		fraction float64
		want     float64
	}{
		{0.2, 1},
		{0.48, 0.48},
		{0.78, 1},
	} {
		if got := breathe(duration*tc.fraction, duration); math.Abs(got-tc.want) > 0.001 {
			t.Errorf("breathe at %v = %v, want %v", tc.fraction, got, tc.want)
		}
	}
}

func TestFlashAlternatesTheTwoColours(t *testing.T) {
	// The colours blink on a 950 ms cycle; the envelope is the cue's, fading in
	// over the first tenth of the three seconds and out over the last. At the
	// peak of a blink only one colour is lit, and the handover falls where
	// neither is, so taking a set off the screen is invisible.
	for _, tc := range []struct {
		name               string
		elapsed            time.Duration
		primary, secondary float64
	}{
		{"primary peak", 76 * time.Millisecond, 1, 0},
		{"handover", 475 * time.Millisecond, 0, 0},
		{"secondary peak", 551 * time.Millisecond, 0, 1},
		{"last secondary peak", 703 * time.Millisecond, 0, 1},
	} {
		_, primary, secondary := sample(AlternatingFlash, tc.elapsed)
		if math.Abs(primary-tc.primary) > 0.001 || math.Abs(secondary-tc.secondary) > 0.001 {
			t.Errorf("%s: colours %v/%v, want %v/%v", tc.name, primary, secondary, tc.primary, tc.secondary)
		}
	}
}

// TestFlashEnvelopeFollowsTheCue keeps a blink in the middle of the cue at
// full strength: the envelope belongs to the three seconds, not to the blink.
func TestFlashEnvelopeFollowsTheCue(t *testing.T) {
	for _, tc := range []struct {
		elapsed time.Duration
		want    float64
	}{
		{0, 0},
		{150 * time.Millisecond, 0.5},
		{900 * time.Millisecond, 1},
		{1500 * time.Millisecond, 1},
		{2850 * time.Millisecond, 0.5},
		{3 * time.Second, 0},
	} {
		envelope, _, _ := sample(AlternatingFlash, tc.elapsed)
		if math.Abs(envelope-tc.want) > 0.01 {
			t.Errorf("%v: envelope %v, want %v", tc.elapsed, envelope, tc.want)
		}
	}
}

func TestFlashFadesInAndOut(t *testing.T) {
	if got := sampleFrames(flashEnvelope, 0); got != 0 {
		t.Fatalf("flash starts at %v", got)
	}
	if got := sampleFrames(flashEnvelope, 1); got != 0 {
		t.Fatalf("flash ends at %v", got)
	}
	if got := sampleFrames(flashEnvelope, 0.5); got != 1 {
		t.Fatalf("flash holds at %v", got)
	}
}

func TestBreathingUsesOneColour(t *testing.T) {
	envelope, primary, secondary := sample(Breathing, 600*time.Millisecond)
	if primary != 1 || secondary != 0 {
		t.Fatalf("breathing shares light as %v/%v", primary, secondary)
	}
	if envelope <= 0 || envelope > 1 {
		t.Fatalf("envelope %v", envelope)
	}
}

// TestGlowWidthMatchesTheMacApp pins the sizing rule: 8.5% of the shorter side
// in points, clamped to 54-116. Two displays at different scales have to end up
// with the same visual width, which is what the point conversion is for.
func TestGlowWidthMatchesTheMacApp(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bounds rect
		dpi    uint32
		width  float64
	}{
		{"150% display", rect{0, 0, 2560, 1600}, 144, 136.0},
		{"100% display", rect{0, 0, 1920, 1080}, 96, 91.8},
		{"small display clamps up", rect{0, 0, 800, 600}, 96, 54},
		{"4K display clamps down", rect{0, 0, 3840, 2160}, 96, 116},
		{"unknown DPI", rect{0, 0, 1920, 1080}, 0, 91.8},
	} {
		got := metricsFor(tc.bounds, tc.dpi).width
		if math.Abs(got-tc.width) > 0.01 {
			t.Errorf("%s: width %v, want %v", tc.name, got, tc.width)
		}
	}
	// The hairline scales with the display too, so the stroke is the same
	// thickness on screen.
	got := metricsFor(rect{0, 0, 2560, 1600}, 144)
	if math.Abs(got.stroke-3.75) > 0.001 || math.Abs(got.radius-33) > 0.001 {
		t.Errorf("150%% hairline = %+v", got)
	}
}

func TestEdgeRectsTileTheDisplayWithoutOverlap(t *testing.T) {
	bounds := rect{0, 0, 1000, 800}
	top := edgeRect(bounds, EdgeTop, 40)
	bottom := edgeRect(bounds, EdgeBottom, 40)
	left := edgeRect(bounds, EdgeLeft, 40)
	right := edgeRect(bounds, EdgeRight, 40)

	if top.Bottom != left.Top || bottom.Top != left.Bottom {
		t.Fatalf("vertical bands do not meet the horizontal ones: top %v left %v bottom %v", top, left, bottom)
	}
	if left.Right > right.Left {
		t.Fatalf("horizontal bands overlap: left %v right %v", left, right)
	}
	if top.width() != 1000 || left.height() != 800-80 {
		t.Fatalf("unexpected band sizes: %v %v", top, left)
	}
}
