// Package events turns the files producers leave in the event directory into
// cue events, following PROTOCOL.md. Every field is metadata: prompt and
// response content never reaches this package.
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// Source says who finished. The protocol spells the names; they are also the
// JSON payload's values.
type Source string

const (
	Codex    Source = "codex"
	OpenCode Source = "opencode"
)

// Label is how the app names a source to the user.
func (s Source) Label() string {
	switch s {
	case Codex:
		return "Codex"
	case OpenCode:
		return "OpenCode"
	}
	return string(s)
}

// maxIdentityBytes is the protocol's ceiling for eventID and sessionID.
const maxIdentityBytes = 512

// MaxFileBytes is the protocol's ceiling for an event file. Anything larger
// is ignored without being read.
const MaxFileBytes = 16 << 10

// freshness is how old a completion may be and still cue (PROTOCOL.md).
const freshness = 60 * time.Second

// Event is one completion.
type Event struct {
	Source      Source
	EventID     string
	SessionID   string
	CompletedAt time.Time
}

// Identity is the protocol's dedup key.
func (e Event) Identity() string { return string(e.Source) + ":" + e.EventID }

// Fresh reports whether the completion is recent enough to cue: the protocol
// accepts only 0 <= now - completedAt < 60 seconds.
func (e Event) Fresh(now time.Time) bool {
	age := now.Sub(e.CompletedAt)
	return age >= 0 && age < freshness
}

// FreshFile is Fresh for a file's modification time, which is all a legacy
// empty turn-complete file says about when the turn ended.
func FreshFile(modified, now time.Time) bool {
	age := now.Sub(modified)
	return age >= 0 && age < freshness
}

var (
	// ErrNotAnEvent means the name is not one the protocol defines.
	ErrNotAnEvent = errors.New("events: not an event file")
	// ErrMalformed means the name is right but the payload is not.
	ErrMalformed = errors.New("events: malformed event")
)

// IsEventName reports whether a file name is one a producer writes.
func IsEventName(name string) bool {
	if strings.HasPrefix(name, "turn-complete.") {
		return true
	}
	return strings.HasPrefix(name, "cue-") && strings.HasSuffix(name, ".json")
}

// Parse reads one event file. modified is the file's modification time, which
// is the completion time of a legacy empty turn-complete file.
func Parse(name string, data []byte, modified time.Time) (Event, error) {
	switch {
	case strings.HasPrefix(name, "cue-") && strings.HasSuffix(name, ".json"):
		return decode(data)
	case strings.HasPrefix(name, "turn-complete."):
		// Helpers written before the payload existed wrote the file empty;
		// the protocol keeps them working by synthesizing the event.
		if len(data) == 0 {
			return Event{
				Source:      Codex,
				EventID:     fmt.Sprintf("legacy-%s-%d", name, modified.UnixNano()),
				SessionID:   "legacy",
				CompletedAt: modified,
			}, nil
		}
		return decode(data)
	}
	return Event{}, fmt.Errorf("%w: %q", ErrNotAnEvent, name)
}

// payload is the JSON body. json.Number accepts both an integer and a
// fractional completedAt, which the protocol allows.
type payload struct {
	Source      string      `json:"source"`
	EventID     string      `json:"eventID"`
	SessionID   string      `json:"sessionID"`
	CompletedAt json.Number `json:"completedAt"`
}

func decode(data []byte) (Event, error) {
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	source := Source(p.Source)
	if source != Codex && source != OpenCode {
		return Event{}, fmt.Errorf("%w: source %q", ErrMalformed, p.Source)
	}
	if err := checkIdentity("eventID", p.EventID); err != nil {
		return Event{}, err
	}
	if err := checkIdentity("sessionID", p.SessionID); err != nil {
		return Event{}, err
	}
	if p.CompletedAt == "" {
		return Event{}, fmt.Errorf("%w: no completedAt", ErrMalformed)
	}
	seconds, err := p.CompletedAt.Float64()
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return Event{}, fmt.Errorf("%w: completedAt %q", ErrMalformed, p.CompletedAt)
	}
	return Event{
		Source:      source,
		EventID:     p.EventID,
		SessionID:   p.SessionID,
		CompletedAt: time.Unix(0, int64(math.Round(seconds*float64(time.Second)))),
	}, nil
}

func checkIdentity(field, value string) error {
	switch n := len(value); {
	case n == 0:
		return fmt.Errorf("%w: empty %s", ErrMalformed, field)
	case n > maxIdentityBytes:
		return fmt.Errorf("%w: %s is %d bytes", ErrMalformed, field, n)
	}
	return nil
}
