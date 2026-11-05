package overlay

import (
	"image/color"
	"testing"
)

func TestFalloffIsOpaqueAtTheEdgeAndClearInside(t *testing.T) {
	if got := falloff(0, 64); got != 1 {
		t.Fatalf("falloff at the edge = %v, want 1", got)
	}
	if got := falloff(64, 64); got != 0 {
		t.Fatalf("falloff at the inner edge = %v, want 0", got)
	}
	prev := 1.0
	for d := 1.0; d < 64; d++ {
		got := falloff(d, 64)
		if got < 0 || got > prev {
			t.Fatalf("falloff(%v) = %v, not decreasing from %v", d, got, prev)
		}
		prev = got
	}
}

func TestBandPixelsFadeInward(t *testing.T) {
	c := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	const w, h = 4, 8
	pix := bandPixels(w, h, EdgeTop, c, 8)
	at := func(x, y int) byte { return pix[(y*w+x)*4+3] }

	if at(0, 0) != 255 {
		t.Fatalf("outer row alpha = %d, want 255", at(0, 0))
	}
	// The band is exactly as thick as the fade, so its last row is the
	// faintest one rather than nothing at all.
	if at(0, h-1) > 12 {
		t.Fatalf("inner row alpha = %d, want under 12", at(0, h-1))
	}
	if at(0, 1) >= at(0, 0) {
		t.Fatalf("alpha did not decrease inward: %d then %d", at(0, 0), at(0, 1))
	}
}

func TestBandPixelsArePremultiplied(t *testing.T) {
	c := color.NRGBA{R: 200, G: 100, B: 50, A: 255}
	pix := bandPixels(1, 64, EdgeTop, c, 64)
	// Row 1 is partly transparent, so every colour channel must have been
	// scaled by the same alpha as the alpha channel.
	a := pix[(1*1+0)*4+3]
	b := pix[(1*1+0)*4+0]
	if a == 0 || a == 255 {
		t.Fatalf("expected a partial alpha on row 1, got %d", a)
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
