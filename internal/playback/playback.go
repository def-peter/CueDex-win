// Package playback turns accepted completions into cues the user can follow:
// one at a time, at most one waiting per producer, never overlapping.
package playback

import (
	"sync"
	"time"

	"github.com/def-peter/cuedex-win/internal/events"
)

// Duration is how long one cue lasts, fade out included. It matches the
// overlay's animation, so the next cue starts exactly when the screen is quiet
// again.
const Duration = 3 * time.Second

// Clock is the player's notion of time. Production uses the wall clock; a test
// drives it by hand, so the queue is checked without a window or a sleep.
type Clock interface {
	// Now is the current time, for freshness.
	Now() time.Time
	// After runs fn once d has passed and returns a cancel for it.
	After(d time.Duration, fn func()) (cancel func())
}

// Player plays one cue at a time. A completion that arrives while a cue is
// playing waits in its producer's slot, and a newer completion from the same
// producer replaces the waiting one. The playing cue is never cut short.
//
// Every method is safe from any goroutine. The callbacks run on whichever
// goroutine moved the player on, so a caller that draws has to marshal them.
type Player struct {
	mu sync.Mutex

	show     func(events.Source)
	hide     func()
	clock    Clock
	duration time.Duration

	paused  bool
	playing bool
	current events.Source
	cancel  func()
	pending []events.Event
}

// New returns a player that cues for Duration on the wall clock.
func New(show func(events.Source), hide func()) *Player {
	return NewWithClock(show, hide, wallClock{}, Duration)
}

// NewWithClock is New with the timing supplied, which is what a test does.
func NewWithClock(show func(events.Source), hide func(), clock Clock, duration time.Duration) *Player {
	return &Player{show: show, hide: hide, clock: clock, duration: duration}
}

// Submit offers one completion. It reports whether the completion was taken: a
// paused player drops it without keeping a backlog.
func (p *Player) Submit(e events.Event) bool {
	if p == nil {
		return false
	}
	var start *events.Event
	p.mu.Lock()
	switch {
	case p.paused:
		p.mu.Unlock()
		return false
	case p.playing:
		p.keepLocked(e)
	default:
		start = p.startLocked(e)
	}
	p.mu.Unlock()
	p.announce(start)
	return true
}

// Pause stops the cue on screen and forgets everything that was waiting. A user
// who comes back from a pause does not want the completions from it.
func (p *Player) Pause() {
	if p == nil {
		return
	}
	var stop bool
	p.mu.Lock()
	if !p.paused {
		p.paused = true
		p.pending = nil
		if p.playing {
			p.playing = false
			p.current = ""
			if p.cancel != nil {
				p.cancel()
				p.cancel = nil
			}
			stop = true
		}
	}
	p.mu.Unlock()
	if stop && p.hide != nil {
		p.hide()
	}
}

// Resume takes cueing up again. What arrived while paused is gone for good.
func (p *Player) Resume() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.paused = false
	p.mu.Unlock()
}

// Playing reports whether a cue is on screen.
func (p *Player) Playing() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

// Pending reports how many cues are waiting, one per producer at most.
func (p *Player) Pending() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pending)
}

// Close stops the timer and forgets everything. The cue on screen belongs to
// whoever shows it and is not hidden here.
func (p *Player) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.pending = nil
	p.playing = false
	p.current = ""
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.mu.Unlock()
}

// startLocked begins a cue and leaves the end of it to the clock.
func (p *Player) startLocked(e events.Event) *events.Event {
	p.playing = true
	p.current = e.Source
	p.cancel = p.clock.After(p.duration, p.finish)
	return &e
}

// keepLocked puts e in its producer's waiting slot. A source that is already
// waiting keeps its place in the queue and takes the newer completion.
func (p *Player) keepLocked(e events.Event) {
	for i := range p.pending {
		if p.pending[i].Source != e.Source {
			continue
		}
		if !e.CompletedAt.Before(p.pending[i].CompletedAt) {
			p.pending[i] = e
		}
		return
	}
	p.pending = append(p.pending, e)
}

// finish ends the cue that was playing and starts the next one that is still
// worth cueing. It runs when the cue's duration has passed.
func (p *Player) finish() {
	var next *events.Event
	p.mu.Lock()
	p.playing = false
	p.current = ""
	p.cancel = nil
	now := p.clock.Now()
	for len(p.pending) > 0 {
		e := p.pending[0]
		p.pending[0] = events.Event{}
		p.pending = p.pending[1:]
		// A cue that waited too long is no longer news.
		if e.Fresh(now) {
			next = p.startLocked(e)
			break
		}
	}
	p.mu.Unlock()
	p.announce(next)
}

func (p *Player) announce(e *events.Event) {
	if e != nil && p.show != nil {
		p.show(e.Source)
	}
}

// wallClock is the real clock.
type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

func (wallClock) After(d time.Duration, fn func()) func() {
	timer := time.AfterFunc(d, fn)
	return func() { timer.Stop() }
}
