package events

import "time"

// rememberFor is how long a seen identity is remembered (PROTOCOL.md rule 3).
const rememberFor = 60 * time.Second

// Ledger decides which completions are news. It remembers identities so a
// repeat within a minute is dropped and refuses anything that is already too
// old to cue (rules 2 and 3). What waits to be played, one per source, belongs
// to playback.
type Ledger struct {
	seen map[string]time.Time
}

// NewLedger returns an empty ledger.
func NewLedger() *Ledger {
	return &Ledger{seen: map[string]time.Time{}}
}

// Accept records e when it is fresh and has not been seen. It reports whether
// the event is new, which is what the caller hands to the player.
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
	return true
}
