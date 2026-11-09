package hook

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNotifyEventAcceptsAFinishedReply(t *testing.T) {
	now := time.Date(2026, 11, 9, 13, 0, 0, 0, time.UTC)
	event, ok := NotifyEvent([]byte(`{
		"type": "agent-turn-complete",
		"thread-id": "thread-1",
		"turn-id": "turn-1",
		"cwd": "C:\\work",
		"client": "Codex Desktop",
		"input-messages": ["你好"],
		"last-assistant-message": "你好！有什么我可以帮你的吗？"
	}`), now)
	if !ok {
		t.Fatal("a finished reply was ignored")
	}
	if event.Source != "codex" || event.SessionID != "thread-1" {
		t.Fatalf("event %+v", event)
	}
	if !event.CompletedAt.Equal(now) {
		t.Fatalf("completedAt %v", event.CompletedAt)
	}
	if len(event.EventID) != 64 {
		t.Fatalf("eventID %q", event.EventID)
	}
}

// The app finishes a turn to write the thread's title. That turn is not a
// reply, and cueing for it would double every notification.
func TestNotifyEventIgnoresTheTitleTurn(t *testing.T) {
	_, ok := NotifyEvent([]byte(`{
		"type": "agent-turn-complete",
		"thread-id": "t",
		"turn-id": "u",
		"input-messages": ["You are a helpful assistant. You will be presented with a user prompt"],
		"last-assistant-message": "{\"title\":\"你好\"}"
	}`), time.Now())
	if ok {
		t.Fatal("the title turn was accepted")
	}
}

func TestNotifyEventIgnoresAnEmptyReply(t *testing.T) {
	for _, message := range []string{`null`, `""`, `"   "`} {
		payload := `{"type":"agent-turn-complete","thread-id":"t","turn-id":"u","last-assistant-message":` + message + `}`
		if _, ok := NotifyEvent([]byte(payload), time.Now()); ok {
			t.Errorf("message %s was accepted", message)
		}
	}
	if _, ok := NotifyEvent([]byte(`{"type":"agent-turn-complete","thread-id":"t"}`), time.Now()); ok {
		t.Error("a payload without a message was accepted")
	}
}

func TestNotifyEventIgnoresAnotherNotificationType(t *testing.T) {
	payload := `{"type":"something-else","thread-id":"t","last-assistant-message":"done"}`
	if _, ok := NotifyEvent([]byte(payload), time.Now()); ok {
		t.Fatal("an unrelated notification was accepted")
	}
}

func TestNotifyEventIsStableForOneTurn(t *testing.T) {
	payload := []byte(`{"type":"agent-turn-complete","thread-id":"t","turn-id":"u","last-assistant-message":"done"}`)
	first, _ := NotifyEvent(payload, time.Unix(1, 0))
	second, _ := NotifyEvent(payload, time.Unix(2, 0))
	if first.EventID != second.EventID {
		t.Fatalf("ids %q and %q differ", first.EventID, second.EventID)
	}
}

func TestNotifyWritesOneEventFilePerTurn(t *testing.T) {
	dir := t.TempDir()
	payload := []byte(`{"type":"agent-turn-complete","thread-id":"t","turn-id":"u","last-assistant-message":"done"}`)
	wrote, err := Notify(payload, dir)
	if err != nil || !wrote {
		t.Fatalf("wrote %v, err %v", wrote, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("%d files", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("the event file is empty")
	}
}
