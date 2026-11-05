package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexEventAcceptsAFinishedReply(t *testing.T) {
	now := time.Date(2026, 11, 5, 14, 0, 0, 0, time.UTC)
	event, ok := CodexEvent([]byte(`{
		"hook_event_name": "Stop",
		"session_id": "sess-1",
		"turn_id": "turn-9",
		"last_assistant_message": "All done.",
		"transcript_path": "C:\\Users\\me\\.codex\\sessions\\x.jsonl"
	}`), now)
	if !ok {
		t.Fatal("a finished reply was ignored")
	}
	if event.Source != "codex" || event.SessionID != "sess-1" {
		t.Fatalf("event %+v", event)
	}
	if !event.CompletedAt.Equal(now) {
		t.Fatalf("completedAt %v, want %v", event.CompletedAt, now)
	}
	if len(event.EventID) != 64 {
		t.Fatalf("eventID %q, want a sha256 hex digest", event.EventID)
	}
}

func TestCodexEventIsStableForOneTurn(t *testing.T) {
	payload := []byte(`{"hook_event_name":"Stop","session_id":"s","turn_id":"t","last_assistant_message":"hi"}`)
	first, ok := CodexEvent(payload, time.Unix(1, 0))
	if !ok {
		t.Fatal("ignored")
	}
	second, ok := CodexEvent(payload, time.Unix(2, 0))
	if !ok {
		t.Fatal("ignored")
	}
	if first.EventID != second.EventID {
		t.Fatalf("the same turn produced %q then %q", first.EventID, second.EventID)
	}
}

func TestCodexEventIgnoresTurnsWithoutAReply(t *testing.T) {
	now := time.Unix(1793491200, 0)
	for name, payload := range map[string]string{
		"another event": `{"hook_event_name":"PreToolUse","session_id":"s","last_assistant_message":"hi"}`,
		"empty message": `{"hook_event_name":"Stop","session_id":"s","last_assistant_message":""}`,
		"blank message": `{"hook_event_name":"Stop","session_id":"s","last_assistant_message":" \n\t "}`,
		"no message":    `{"hook_event_name":"Stop","session_id":"s"}`,
		"null message":  `{"hook_event_name":"Stop","session_id":"s","last_assistant_message":null}`,
		"not json":      `{"hook_event_name":`,
		"not an object": `"Stop"`,
	} {
		if event, ok := CodexEvent([]byte(payload), now); ok {
			t.Errorf("%s: cued %+v", name, event)
		}
	}
}

func TestCodexEventFallsBackWithoutSessionOrTurn(t *testing.T) {
	now := time.Unix(1793491200, 0)
	event, ok := CodexEvent([]byte(`{"hook_event_name":"Stop","last_assistant_message":"hi"}`), now)
	if !ok {
		t.Fatal("ignored")
	}
	if event.SessionID != "legacy" {
		t.Fatalf("sessionID %q, want the protocol's legacy", event.SessionID)
	}
	if event.EventID == "" {
		t.Fatal("a turn without a turn_id got no eventID")
	}
}

func TestCodexWritesOneEventFile(t *testing.T) {
	dir := t.TempDir()
	stdin := strings.NewReader(`{"hook_event_name":"Stop","session_id":"s","turn_id":"t","last_assistant_message":"done"}`)
	wrote, err := Codex(stdin, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("nothing was written")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("files %v, want exactly one event", entries)
	}
	name := entries[0].Name()
	if !strings.HasPrefix(name, "turn-complete.") {
		t.Fatalf("name %q, want the protocol's turn-complete.<pid>", name)
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"source":"codex"`) || !strings.Contains(string(data), `"sessionID":"s"`) {
		t.Fatalf("payload %s", data)
	}
	// The reply itself never reaches the event file.
	if strings.Contains(string(data), "done") {
		t.Fatalf("payload carries the reply: %s", data)
	}
}

func TestCodexWritesNothingForAToolOnlyTurn(t *testing.T) {
	dir := t.TempDir()
	stdin := strings.NewReader(`{"hook_event_name":"Stop","session_id":"s","last_assistant_message":""}`)
	wrote, err := Codex(stdin, dir)
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Fatal("a turn without a reply wrote an event")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("files %v, want none", entries)
	}
}

func TestClampKeepsTheProtocolCeiling(t *testing.T) {
	long := strings.Repeat("x", 600)
	if got := clamp(long, ""); len(got) != 512 {
		t.Fatalf("clamped to %d bytes, want 512", len(got))
	}
	if got := clamp("  ", "legacy"); got != "legacy" {
		t.Fatalf("blank became %q, want the fallback", got)
	}
	if got := clamp(" id ", ""); got != "id" {
		t.Fatalf("clamped %q, want it trimmed", got)
	}
}
