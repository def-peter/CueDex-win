package events

import (
	"testing"
	"time"
)

func at(seconds int64) time.Time { return time.Unix(1793491200+seconds, 0) }

func TestLedgerDropsRepeatsWithinAMinute(t *testing.T) {
	l := NewLedger()
	e := Event{Source: Codex, EventID: "same", SessionID: "s", CompletedAt: at(0)}
	if !l.Accept(e, at(0)) {
		t.Fatal("first event was dropped")
	}
	// The same completion arriving from a second file: the replay is dropped
	// but a later completion with the same identity would also be dropped
	// only while the identity is remembered.
	if l.Accept(e, at(30)) {
		t.Fatal("repeat within 60 seconds was accepted")
	}
	replay := e
	replay.CompletedAt = at(61)
	if !l.Accept(replay, at(61)) {
		t.Fatal("identity was remembered past 60 seconds")
	}
}

func TestLedgerDropsStaleEvents(t *testing.T) {
	l := NewLedger()
	e := Event{Source: Codex, EventID: "old", SessionID: "s", CompletedAt: at(0)}
	if l.Accept(e, at(60)) {
		t.Fatal("an event 60 seconds old was accepted")
	}
	future := Event{Source: Codex, EventID: "future", SessionID: "s", CompletedAt: at(10)}
	if l.Accept(future, at(0)) {
		t.Fatal("an event from the future was accepted")
	}
}

func TestLedgerKeepsOnePendingPerSource(t *testing.T) {
	l := NewLedger()
	older := Event{Source: Codex, EventID: "a", SessionID: "s", CompletedAt: at(0)}
	newer := Event{Source: Codex, EventID: "b", SessionID: "s", CompletedAt: at(1)}
	other := Event{Source: OpenCode, EventID: "c", SessionID: "s", CompletedAt: at(0)}
	for _, e := range []Event{older, newer, other} {
		if !l.Accept(e, at(2)) {
			t.Fatalf("%s was dropped", e.EventID)
		}
	}
	if !l.Pending(Codex) || !l.Pending(OpenCode) {
		t.Fatal("a source lost its pending cue")
	}
	first, ok := l.Next(at(2))
	if !ok || first.Identity() != other.Identity() {
		t.Fatalf("next = %+v, want the oldest pending cue (opencode)", first)
	}
	second, ok := l.Next(at(2))
	if !ok || second.Identity() != newer.Identity() {
		t.Fatalf("next = %+v, want the newer codex cue", second)
	}
	if _, ok := l.Next(at(2)); ok {
		t.Fatal("a cue was delivered twice")
	}
}

func TestLedgerSkipsCuesThatWentStaleWhileWaiting(t *testing.T) {
	l := NewLedger()
	e := Event{Source: Codex, EventID: "a", SessionID: "s", CompletedAt: at(0)}
	if !l.Accept(e, at(1)) {
		t.Fatal("fresh event was dropped")
	}
	if _, ok := l.Next(at(120)); ok {
		t.Fatal("a cue that aged out while pending was delivered")
	}
	if l.Pending(Codex) {
		t.Fatal("the stale cue is still pending")
	}
}
