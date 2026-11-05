package main

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestAlphaBreathesFromAndToNothing(t *testing.T) {
	if a := alpha(0); math.Abs(a) > 1e-9 {
		t.Errorf("alpha at the start is %v, want 0", a)
	}
	if a := alpha(1); math.Abs(a) > 1e-9 {
		t.Errorf("alpha at the end is %v, want 0", a)
	}
	if a := alpha(0.5); math.Abs(a-1) > 1e-9 {
		t.Errorf("alpha in the middle is %v, want 1", a)
	}
}

func TestProgressEndsTheCue(t *testing.T) {
	start := time.Now()
	g := newGlow(rect{Right: 100, Bottom: 100}, start, time.Second, false)
	for _, tc := range []struct {
		at   time.Duration
		want bool
	}{
		{-time.Millisecond, false},
		{0, true},
		{500 * time.Millisecond, true},
		{time.Second, false},
		{2 * time.Second, false},
	} {
		if _, live := g.progress(start.Add(tc.at)); live != tc.want {
			t.Errorf("live %v at %v, want %v", live, tc.at, tc.want)
		}
	}
}

func TestStopsSwapColorsOnTheFlash(t *testing.T) {
	g := newGlow(rect{}, time.Now(), time.Second, true)
	// A second holds four flashes: the first half of each second starts on
	// the first color, the second half on the second.
	first := g.stops(0.1)
	second := g.stops(0.4)
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("no stops")
	}
	if first[0].Color == second[0].Color {
		t.Errorf("both halves of the flash start with %v", first[0].Color)
	}
}

// The view runs without a window: it draws the cue while it runs, and
// nothing before it starts or once it is over, without touching a display.
func TestViewDrawsOnlyWhileCueing(t *testing.T) {
	now := time.Now()
	live := newGlow(rect{Right: 400, Bottom: 300}, now.Add(-time.Millisecond), time.Minute, false)
	ui.NewTester(live.view, 400, 300)
	if live.frames == 0 {
		t.Fatal("the live cue drew no frames")
	}

	// A cue that has not arrived yet, and one that is over, paint nothing.
	for _, tc := range []struct {
		name string
		cue  *glow
	}{
		{"before", newGlow(rect{Right: 400, Bottom: 300}, now.Add(time.Second), time.Minute, false)},
		{"after", newGlow(rect{Right: 400, Bottom: 300}, now.Add(-2*time.Minute), time.Minute, false)},
	} {
		ui.NewTester(tc.cue.view, 400, 300)
		if tc.cue.frames != 0 {
			t.Errorf("a cue %s its time drew %d frames, want 0", tc.name, tc.cue.frames)
		}
	}
}
