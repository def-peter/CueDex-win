package events

import "time"

// rememberFor is how long a seen identity is remembered (PROTOCOL.md rule 3).
const rememberFor = 60 * time.Second

// Ledger decides which completions cue. It remembers identities so a repeat
// within a minute is dropped, and keeps at most one pending cue per source so
// a burst from one producer does not stack up (rules 3 and 4).
type Ledger struct {
	seen    map[string]time.Time
	pending map[Source]Event
}

// NewLedger returns an empty ledger.
func NewLedger() *Ledger {
	return &Ledger{seen: map[string]time.Time{}, pending: map[Source]Event{}}
}

// Accept records e when it is fresh and has not been seen, and queues it for
// its source. It reports whether the event was new.
func (l *Ledger) Accept(e Event, now time.Time) bool {
	for identity, at := range l.seen {
		if now.Sub(at) >= rememberFor {
			delete(l.seen, identity)
		}
	}
	if !e.Fresh(now) {
		return false
	}
	identity := e.Identity()
	if _, seen := l.seen[identity]; seen {
		return false
	}
	l.seen[identity] = now

	if current, ok := l.pending[e.Source]; !ok || !e.CompletedAt.Before(current.CompletedAt) {
		l.pending[e.Source] = e
	}
	return true
}

// Next removes and returns the oldest pending event that is still fresh.
func (l *Ledger) Next(now time.Time) (Event, bool) {
	var oldest Event
	found := false
	for source, e := range l.pending {
		if !e.Fresh(now) {
			delete(l.pending, source)
			continue
		}
		if !found || e.CompletedAt.Before(oldest.CompletedAt) {
			oldest, found = e, true
		}
	}
	if found {
		delete(l.pending, oldest.Source)
	}
	return oldest, found
}

// Pending reports whether a source is waiting for its cue.
func (l *Ledger) Pending(source Source) bool {
	_, ok := l.pending[source]
	return ok
}

// Drop forgets a source's pending cue, as when the user dismisses it.
func (l *Ledger) Drop(source Source) { delete(l.pending, source) }
