package playback

import (
	"testing"
	"time"

	"github.com/def-peter/cuedex-win/internal/events"
)

// fakeClock stands in for the wall clock: nothing happens until a test moves
// time forward, so a cue's length is checked exactly rather than approximately.
type fakeClock struct {
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	at   time.Time
	fn   func()
	done bool
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 5, 14, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) After(d time.Duration, fn func()) func() {
	timer := &fakeTimer{at: c.now.Add(d), fn: fn}
	c.timers = append(c.timers, timer)
	return func() { timer.done = true }
}

// Advance moves time forward and runs whatever fell due, in order.
func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
	for {
		var due *fakeTimer
		for _, timer := range c.timers {
			if timer.done || timer.at.After(c.now) {
				continue
			}
			if due == nil || timer.at.Before(due.at) {
				due = timer
			}
		}
		if due == nil {
			return
		}
		due.done = true
		due.fn()
	}
}

// harness records what the player asked to show, in order.
type harness struct {
	t       *testing.T
	clock   *fakeClock
	player  *Player
	shows   []events.Source
	hidden  int
	dropped int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, clock: newFakeClock()}
	h.player = NewWithClock(
		func(source events.Source) { h.shows = append(h.shows, source) },
		func() { h.hidden++ },
		h.clock,
		Duration,
	)
	t.Cleanup(h.player.Close)
	return h
}

// submit offers an event and records a drop, so a test can tell a refused
// completion from an accepted one.
func (h *harness) submit(source events.Source, id string, age time.Duration) {
	h.t.Helper()
	e := events.Event{
		Source:      source,
		EventID:     id,
		SessionID:   "session",
		CompletedAt: h.clock.Now().Add(-age),
	}
	if !h.player.Submit(e) {
		h.dropped++
	}
}

func (h *harness) finishCue() {
	h.t.Helper()
	h.clock.Advance(Duration)
}

func TestAburstKeepsOnePerSourceAndPlaysOne(t *testing.T) {
	h := newHarness(t)
	// Five completions at once: three from Codex, two from OpenCode.
	h.submit(events.Codex, "c1", 3*time.Second)
	h.submit(events.OpenCode, "o1", 2*time.Second)
	h.submit(events.Codex, "c2", time.Second)
	h.submit(events.OpenCode, "o2", 500*time.Millisecond)
	h.submit(events.Codex, "c3", 0)

	if len(h.shows) != 1 || h.shows[0] != events.Codex {
		t.Fatalf("shows %v, want the first arrival only", h.shows)
	}
	if got := h.player.Pending(); got != 2 {
		t.Fatalf("%d cues waiting, want one per source", got)
	}
	if !h.player.Playing() {
		t.Fatal("nothing is playing")
	}

	// OpenCode's first completion was the next to arrive, so its slot plays
	// next, with the newest completion in it; Codex's slot follows.
	h.finishCue()
	if len(h.shows) != 2 || h.shows[1] != events.OpenCode {
		t.Fatalf("shows %v, want the OpenCode cue second", h.shows)
	}
	h.finishCue()
	if len(h.shows) != 3 || h.shows[2] != events.Codex {
		t.Fatalf("shows %v, want the Codex cue third", h.shows)
	}
	h.finishCue()
	if len(h.shows) != 3 {
		t.Fatalf("shows %v, want three cues in total", h.shows)
	}
	if h.player.Pending() != 0 {
		t.Fatalf("%d cues left waiting", h.player.Pending())
	}
}

func TestBurstFromOneSourceLeavesTheNewestWaiting(t *testing.T) {
	h := newHarness(t)
	for i, id := range []string{"a", "b", "c", "d", "e"} {
		h.submit(events.Codex, id, time.Duration(4-i)*time.Second)
	}
	if len(h.shows) != 1 {
		t.Fatalf("shows %v, want only the first", h.shows)
	}
	if got := h.player.Pending(); got != 1 {
		t.Fatalf("%d waiting, want one slot", got)
	}
	h.finishCue()
	if len(h.shows) != 2 {
		t.Fatalf("shows %v, want the waiting cue second", h.shows)
	}
	h.finishCue()
	if len(h.shows) != 2 {
		t.Fatalf("shows %v, want the other four completions dropped", h.shows)
	}
}

func TestNewerCompletionDoesNotInterruptTheCueOnScreen(t *testing.T) {
	h := newHarness(t)
	h.submit(events.OpenCode, "first", 0)
	h.clock.Advance(2 * time.Second)
	h.submit(events.OpenCode, "second", 0)
	if len(h.shows) != 1 {
		t.Fatalf("shows %v, want the running cue left alone", h.shows)
	}
	// The running cue still gets its full three seconds.
	h.clock.Advance(999 * time.Millisecond)
	if len(h.shows) != 1 {
		t.Fatalf("shows %v, want the cue to last its full duration", h.shows)
	}
	h.clock.Advance(time.Millisecond)
	if len(h.shows) != 2 || h.shows[1] != events.OpenCode {
		t.Fatalf("shows %v, want the newer completion after it", h.shows)
	}
}

func TestAnOlderCompletionDoesNotReplaceTheWaitingOne(t *testing.T) {
	h := newHarness(t)
	h.submit(events.Codex, "playing", 0)
	h.submit(events.Codex, "newer", 0)
	h.submit(events.Codex, "older", 10*time.Second)
	h.finishCue()
	if len(h.shows) != 2 {
		t.Fatalf("shows %v, want two cues", h.shows)
	}
	// The waiting slot still holds "newer"; an older replay changed nothing.
	if got := h.player.Pending(); got != 0 {
		t.Fatalf("%d cues left waiting", got)
	}
}

func TestBothSourcesPlayInFullOneAfterTheOther(t *testing.T) {
	h := newHarness(t)
	h.submit(events.Codex, "c", 0)
	h.submit(events.OpenCode, "o", 0)
	if want := []events.Source{events.Codex}; !equal(h.shows, want) {
		t.Fatalf("shows %v, want %v", h.shows, want)
	}
	h.clock.Advance(Duration - time.Millisecond)
	if len(h.shows) != 1 {
		t.Fatalf("shows %v, want the second cue only after the first", h.shows)
	}
	h.clock.Advance(time.Millisecond)
	if want := []events.Source{events.Codex, events.OpenCode}; !equal(h.shows, want) {
		t.Fatalf("shows %v, want %v", h.shows, want)
	}
	h.clock.Advance(Duration - time.Millisecond)
	if len(h.shows) != 2 {
		t.Fatalf("shows %v, want the last cue to run its full length", h.shows)
	}
	h.clock.Advance(time.Millisecond)
	if h.player.Playing() || h.player.Pending() != 0 {
		t.Fatalf("playing %v pending %d, want a quiet player", h.player.Playing(), h.player.Pending())
	}
}

func TestPauseDropsWhatArrivesAndWhatWasWaiting(t *testing.T) {
	h := newHarness(t)
	h.submit(events.Codex, "playing", 0)
	h.submit(events.OpenCode, "waiting", 0)
	h.player.Pause()

	if h.hidden != 1 || h.player.Playing() {
		t.Fatalf("hidden %d playing %v, want the cue stopped", h.hidden, h.player.Playing())
	}
	if h.player.Pending() != 0 {
		t.Fatalf("%d cues kept while paused", h.player.Pending())
	}
	h.submit(events.Codex, "during-pause", 0)
	if h.dropped != 1 || len(h.shows) != 1 {
		t.Fatalf("dropped %d shows %v, want the paused completion refused", h.dropped, h.shows)
	}

	h.player.Resume()
	h.clock.Advance(10 * Duration)
	if len(h.shows) != 1 {
		t.Fatalf("shows %v, want nothing replayed after the pause", h.shows)
	}
	// Cueing works again after the resume.
	h.submit(events.OpenCode, "after", 0)
	if len(h.shows) != 2 || h.shows[1] != events.OpenCode {
		t.Fatalf("shows %v, want the completion after the pause", h.shows)
	}
}

func TestACueThatWentStaleWhileWaitingIsSkipped(t *testing.T) {
	h := newHarness(t)
	h.submit(events.Codex, "playing", 0)
	// Fifty-eight seconds old when it arrived, three more while it waited:
	// past the protocol's minute, so it is no longer news.
	h.submit(events.OpenCode, "almost-old", 58*time.Second)
	h.finishCue()
	if len(h.shows) != 1 {
		t.Fatalf("shows %v, want the stale waiting cue skipped", h.shows)
	}
	if h.player.Pending() != 0 {
		t.Fatalf("%d cues left waiting", h.player.Pending())
	}
}

func TestCloseStopsTheTimer(t *testing.T) {
	h := newHarness(t)
	h.submit(events.Codex, "c", 0)
	h.player.Close()
	h.clock.Advance(Duration)
	if len(h.shows) != 1 || h.player.Playing() {
		t.Fatalf("shows %v playing %v, want nothing after Close", h.shows, h.player.Playing())
	}
}

func equal(a, b []events.Source) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
