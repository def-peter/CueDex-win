package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/def-peter/cuedex-win/internal/events"
)

func TestCueIsDroppedWhilePaused(t *testing.T) {
	a := &App{}
	a.setPaused(true)
	a.Cue()
	if a.state.cueCount != 0 || !a.state.lastCue.IsZero() {
		t.Fatalf("cue counted while paused: count %d at %v", a.state.cueCount, a.state.lastCue)
	}

	a.setPaused(false)
	a.Cue()
	if a.state.cueCount != 1 || a.state.lastCue.IsZero() {
		t.Fatalf("cue not counted after resuming: count %d", a.state.cueCount)
	}
}

// The panel runs without a window in tests, which click as a user would.
func TestViewTogglesPause(t *testing.T) {
	a := &App{}
	a.state.onTogglePause = a.togglePause
	tt := ui.NewTester(a.state.view, 420, 280)

	if !tt.HasText("等待 Codex / OpenCode 的完成事件") {
		t.Fatalf("idle panel texts %q", tt.Texts())
	}
	if err := tt.Click("暂停提示"); err != nil {
		t.Fatal(err)
	}
	if !a.state.paused || !tt.HasText("恢复提示") {
		t.Fatalf("paused %v, texts %q", a.state.paused, tt.Texts())
	}
}

func TestViewShowsLastCue(t *testing.T) {
	a := &App{}
	a.state.cueCount = 2
	a.state.lastCue = time.Date(2026, 11, 5, 14, 32, 7, 0, time.Local)
	a.state.lastSource = events.OpenCode
	tt := ui.NewTester(a.state.view, 420, 280)

	if !tt.HasText("最近一次唤醒 14:32:07（OpenCode），共 2 次") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestPulseRemembersTheSource(t *testing.T) {
	a := &App{}
	a.pulse(events.OpenCode)
	if a.state.cueCount != 1 || a.state.lastSource != events.OpenCode {
		t.Fatalf("after a pulse: %+v", a.state)
	}
	// A bare wake leaves the last known source alone.
	a.Cue()
	if a.state.cueCount != 2 || a.state.lastSource != events.OpenCode {
		t.Fatalf("after a bare wake: %+v", a.state)
	}
}

func TestSourceColoursDiffer(t *testing.T) {
	codex, opencode := sourceColour(events.Codex), sourceColour(events.OpenCode)
	if codex == opencode {
		t.Fatalf("both sources cue in %v", codex)
	}
	if got := sourceColour(""); got != codex {
		t.Fatalf("a bare wake cues in %v, want the product colour %v", got, codex)
	}
}
