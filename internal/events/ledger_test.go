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

func TestLedgerKeepsSourcesApart(t *testing.T) {
	l := NewLedger()
	codex := Event{Source: Codex, EventID: "same", SessionID: "s", CompletedAt: at(0)}
	opencode := Event{Source: OpenCode, EventID: "same", SessionID: "s", CompletedAt: at(0)}
	if !l.Accept(codex, at(1)) {
		t.Fatal("the Codex completion was dropped")
	}
	// The identity is source:eventID, so another producer's event with the
	// same eventID is news.
	if !l.Accept(opencode, at(1)) {
		t.Fatal("the same eventID from another source was dropped")
	}
}
