//go:build windows

package events

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The watcher is the part that has to work with Windows itself, so the test
// watches a real directory and lets a real file arrive.
func TestWatchDeliversAndDeletesAnEvent(t *testing.T) {
	dir := t.TempDir()
	got := make(chan Event, 4)
	w, err := Watch(dir, func(e Event) { got <- e })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	now := time.Now().Unix()
	body := fmt.Sprintf(`{"source":"opencode","eventID":"live-1","sessionID":"s1","completedAt":%d}`, now)
	writeAtomically(t, dir, "cue-opencode-live.json", body)

	select {
	case e := <-got:
		if e.Source != OpenCode || e.EventID != "live-1" {
			t.Fatalf("got %+v", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event arrived")
	}
	if _, err := os.Stat(filepath.Join(dir, "cue-opencode-live.json")); !os.IsNotExist(err) {
		t.Fatalf("the file was not deleted after it was read: %v", err)
	}
}

func TestWatchReadsWhatArrivedWhileTheAppWasNotRunning(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Unix()
	body := fmt.Sprintf(`{"source":"codex","eventID":"early","sessionID":"s","completedAt":%d}`, now)
	if err := os.WriteFile(filepath.Join(dir, "turn-complete.77"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got := make(chan Event, 1)
	w, err := Watch(dir, func(e Event) { got <- e })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	select {
	case e := <-got:
		if e.EventID != "early" {
			t.Fatalf("got %+v", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the file present at startup was not consumed")
	}
}

func TestWatchForgetsStaleFilesWithoutCueing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cue-codex-old.json")
	if err := os.WriteFile(path, []byte(`{"source":"codex","eventID":"old","sessionID":"s","completedAt":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	got := make(chan Event, 1)
	w, err := Watch(dir, func(e Event) { got <- e })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	select {
	case e := <-got:
		t.Fatalf("stale file cued: %+v", e)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale file left behind: %v", err)
	}
}

// writeAtomically does what the protocol asks a producer to do: write a
// temporary name, then rename into place, so a reader never sees half a file.
func writeAtomically(t *testing.T, dir, name, body string) {
	t.Helper()
	tmp := filepath.Join(dir, name+".tmp")
	if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}
