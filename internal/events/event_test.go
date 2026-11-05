package events

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParsePayload(t *testing.T) {
	modified := time.Date(2026, 11, 5, 14, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		data string
		want Event
	}{
		{
			name: "cue-codex-abc.json",
			data: `{"source":"codex","eventID":"e1","sessionID":"s1","completedAt":1793491200}`,
			want: Event{Source: Codex, EventID: "e1", SessionID: "s1", CompletedAt: time.Unix(1793491200, 0)},
		},
		{
			name: "cue-opencode-def.json",
			data: `{"source":"opencode","eventID":"e2","sessionID":"s2","completedAt":1793491200.5}`,
			want: Event{Source: OpenCode, EventID: "e2", SessionID: "s2", CompletedAt: time.Unix(1793491200, 500000000)},
		},
		{
			// The Codex hook writes JSON to the same name a legacy helper
			// used for an empty marker.
			name: "turn-complete.4242",
			data: `{"source":"codex","eventID":"e3","sessionID":"s3","completedAt":1793491201}`,
			want: Event{Source: Codex, EventID: "e3", SessionID: "s3", CompletedAt: time.Unix(1793491201, 0)},
		},
	} {
		got, err := Parse(tc.name, []byte(tc.data), modified)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.Source != tc.want.Source || got.EventID != tc.want.EventID || got.SessionID != tc.want.SessionID {
			t.Fatalf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
		// Fractional seconds come to us through a float64, so the last
		// microsecond is the precision the protocol actually has.
		if d := got.CompletedAt.Sub(tc.want.CompletedAt); d < -time.Microsecond || d > time.Microsecond {
			t.Fatalf("%s: completedAt %v, want %v", tc.name, got.CompletedAt, tc.want.CompletedAt)
		}
	}
}

func TestParseLegacyEmptyTurnComplete(t *testing.T) {
	modified := time.Date(2026, 11, 5, 14, 0, 0, 0, time.UTC)
	got, err := Parse("turn-complete.4242", nil, modified)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != Codex || got.SessionID != "legacy" {
		t.Fatalf("got %+v, want a synthesized Codex event", got)
	}
	if !got.CompletedAt.Equal(modified) {
		t.Fatalf("completedAt %v, want the file's modification time %v", got.CompletedAt, modified)
	}
	// Reading the same file twice must not look like two completions.
	again, err := Parse("turn-complete.4242", nil, modified)
	if err != nil || again.Identity() != got.Identity() {
		t.Fatalf("identity changed between reads: %q then %q", got.Identity(), again.Identity())
	}
}

func TestParseRejectsMalformedPayloads(t *testing.T) {
	modified := time.Now()
	payload := func(body string) []byte { return []byte(body) }
	long := strings.Repeat("x", maxIdentityBytes+1)
	for name, data := range map[string][]byte{
		"not json":                 payload(`{"source":`),
		"unknown source":           payload(`{"source":"claude","eventID":"e","sessionID":"s","completedAt":1}`),
		"empty event id":           payload(`{"source":"codex","eventID":"","sessionID":"s","completedAt":1}`),
		"empty session id":         payload(`{"source":"codex","eventID":"e","sessionID":"","completedAt":1}`),
		"long event id":            payload(`{"source":"codex","eventID":"` + long + `","sessionID":"s","completedAt":1}`),
		"long session id":          payload(`{"source":"codex","eventID":"e","sessionID":"` + long + `","completedAt":1}`),
		"missing completedAt":      payload(`{"source":"codex","eventID":"e","sessionID":"s"}`),
		"completedAt not a number": payload(`{"source":"codex","eventID":"e","sessionID":"s","completedAt":"now"}`),
	} {
		if _, err := Parse("cue-codex-x.json", data, modified); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: err %v, want ErrMalformed", name, err)
		}
	}
}

func TestParseIgnoresOtherNames(t *testing.T) {
	for _, name := range []string{
		"notes.txt",
		"cue-codex-x.txt",
		"turn-complete",
		"Cue-codex-x.json",
		// A producer's half-written file, which it renames into place.
		"turn-complete.123.tmp",
		"cue-codex-x.json.TMP",
	} {
		if _, err := Parse(name, nil, time.Now()); !errors.Is(err, ErrNotAnEvent) {
			t.Errorf("%s: err %v, want ErrNotAnEvent", name, err)
		}
		if IsEventName(name) {
			t.Errorf("%s is not an event name", name)
		}
	}
}

func TestFreshHasTheProtocolsWindow(t *testing.T) {
	now := time.Date(2026, 11, 5, 14, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		age  time.Duration
		want bool
	}{
		{0, true},
		{59 * time.Second, true},
		{60 * time.Second, false},
		{2 * time.Minute, false},
		{-time.Second, false}, // from the future
	} {
		e := Event{CompletedAt: now.Add(-tc.age)}
		if got := e.Fresh(now); got != tc.want {
			t.Errorf("age %v: fresh %v, want %v", tc.age, got, tc.want)
		}
	}
}
