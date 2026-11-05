package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/integrations"
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
	tt := ui.NewTester(a.state.view, 420, 360)

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
	tt := ui.NewTester(a.state.view, 420, 360)

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

func TestViewTogglesCodexIntegration(t *testing.T) {
	enabled := false
	s := settings{codexDetected: true}
	s.onToggleCodex = func() {
		enabled = !enabled
		s.codexInstalled = enabled
	}
	tt := ui.NewTester(s.view, 420, 360)
	if !tt.HasText("未安装：Codex 跑完一轮回复时不会提示") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("启用 Codex 集成"); err != nil {
		t.Fatal(err)
	}
	if !enabled || !tt.HasText("停用 Codex 集成") {
		t.Fatalf("enabled %v, texts %q", enabled, tt.Texts())
	}
}

func TestCodexDetailSaysWhatIsMissing(t *testing.T) {
	for _, tc := range []struct {
		state settings
		want  string
	}{
		{settings{codexInstalled: true, codexDetected: true}, "已安装：Codex 跑完一轮回复时会提示"},
		{settings{codexInstalled: true}, "钩子已安装，但没有找到 Codex；装好 Codex 后自动生效"},
		{settings{}, "未安装，也没有找到 Codex"},
		{settings{codexDetected: true}, "未安装：Codex 跑完一轮回复时不会提示"},
		{settings{codexProblem: "坏了"}, "坏了"},
	} {
		if got := tc.state.codexDetail(); got != tc.want {
			t.Errorf("%+v: got %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestCodexProblemNamesTheBrokenFile(t *testing.T) {
	if got := codexProblem(integrations.ErrInvalidHooks); !strings.Contains(got, "hooks.json") {
		t.Fatalf("got %q, want the file named", got)
	}
	if got := codexProblem(errors.New("access denied")); !strings.Contains(got, "access denied") {
		t.Fatalf("got %q, want the cause shown", got)
	}
}
