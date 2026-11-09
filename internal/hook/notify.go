package hook

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/def-peter/cuedex-win/internal/events"
)

// NotifyPayload is the legacy notify JSON Codex hands to the command in
// config.toml's notify line. The desktop app keeps no user Stop hooks running,
// but it does run this one: the app wraps an earlier notify in its own chain,
// so the channel survives the app rewriting the line.
type NotifyPayload struct {
	Type    string   `json:"type"`
	Thread  string   `json:"thread-id"`
	Turn    string   `json:"turn-id"`
	Inputs  []string `json:"input-messages"`
	Message *string  `json:"last-assistant-message"`
}

// titleMarks identify the app's own title-generation turns. They finish a turn
// too, but nothing the user is waiting for, so they must not cue.
var titleMarks = []string{
	"You will be presented with a user prompt",
	"Generate a concise UI title",
}

// NotifyEvent returns the completion a notify payload describes. It reports
// false for anything that is not a finished reply.
func NotifyEvent(data []byte, now time.Time) (events.Event, bool) {
	var payload NotifyPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return events.Event{}, false
	}
	if payload.Type != "" && payload.Type != "agent-turn-complete" {
		return events.Event{}, false
	}
	if payload.Message == nil || strings.TrimSpace(*payload.Message) == "" {
		return events.Event{}, false
	}
	for _, input := range payload.Inputs {
		for _, mark := range titleMarks {
			if strings.Contains(input, mark) {
				return events.Event{}, false
			}
		}
	}
	session := clamp(payload.Thread, "legacy")
	id, ok := identity(session, clamp(payload.Turn, ""))
	if !ok {
		return events.Event{}, false
	}
	return events.Event{
		Source:      events.Codex,
		EventID:     id,
		SessionID:   session,
		CompletedAt: now,
	}, true
}

// Notify reads one notify payload and leaves an event file when a turn really
// finished. It reports whether it did, so the caller knows whether to wake the
// app.
func Notify(data []byte, eventsDir string) (bool, error) {
	event, ok := NotifyEvent(data, time.Now())
	if !ok {
		return false, nil
	}
	if err := writeEvent(eventsDir, event); err != nil {
		return false, err
	}
	return true, nil
}

// identity is the protocol's dedup key: the turn when the payload names one,
// a random id when it does not.
func identity(session, turn string) (string, bool) {
	if turn != "" {
		sum := sha256.Sum256([]byte(session + "\n" + turn))
		return hex.EncodeToString(sum[:]), true
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", false
	}
	return hex.EncodeToString(random[:]), true
}
