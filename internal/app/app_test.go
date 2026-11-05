package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
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
	tt := ui.NewTester(a.state.view, 420, 280)

	if !tt.HasText("最近一次唤醒 14:32:07，共 2 次") {
		t.Fatalf("texts %q", tt.Texts())
	}
}
