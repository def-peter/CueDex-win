package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/integrations"
	"github.com/def-peter/cuedex-win/internal/playback"
	"github.com/def-peter/cuedex-win/internal/prefs"
	"github.com/def-peter/cuedex-win/internal/sound"
)

func TestPauseReachesThePlayer(t *testing.T) {
	a := &App{}
	a.player = playback.New(func(events.Source) {}, func() {})
	defer a.player.Close()
	event := events.Event{Source: events.Codex, EventID: "e", SessionID: "s", CompletedAt: time.Now()}

	a.setPaused(true)
	if a.player.Submit(event) || a.player.Playing() {
		t.Fatal("a completion was taken while paused")
	}

	a.setPaused(false)
	if !a.player.Submit(event) || !a.player.Playing() {
		t.Fatal("cueing did not resume")
	}
}

func TestCueWithoutAWatcherIsHarmless(t *testing.T) {
	// A second launch can reach the app before the watcher exists; the wake
	// has nothing to nudge and must not panic.
	(&App{}).Cue()
}

// The panel runs without a window in tests, which click as a user would.
func TestViewTogglesPause(t *testing.T) {
	a := &App{}
	a.state.onTogglePause = a.togglePause
	tt := ui.NewTester(a.state.view, 420, 660)

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
	tt := ui.NewTester(a.state.view, 420, 660)

	if !tt.HasText("最近一次唤醒 14:32:07（OpenCode），共 2 次") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestCueStartRecordsTheSource(t *testing.T) {
	a := &App{}
	a.cueStart(events.OpenCode)
	if a.state.cueCount != 1 || a.state.lastSource != events.OpenCode {
		t.Fatalf("after one cue: %+v", a.state)
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

// fakeAudio records what the app asked to hear.
type fakeAudio struct {
	plays   []string
	volumes []int
	stops   int
	err     error
}

func (f *fakeAudio) Play(id string, volume int) error {
	f.plays = append(f.plays, id)
	f.volumes = append(f.volumes, volume)
	return f.err
}

func (f *fakeAudio) Stop() { f.stops++ }

func TestCuePlaysEachProducersSound(t *testing.T) {
	audio := &fakeAudio{}
	a := &App{audio: audio, prefs: prefs.Default()}
	a.cueStart(events.Codex)
	a.cueStart(events.OpenCode)

	if len(audio.plays) != 2 {
		t.Fatalf("played %v, want one sound per cue", audio.plays)
	}
	if audio.plays[0] != prefs.Default().Sound.Codex.ID {
		t.Errorf("codex played %q", audio.plays[0])
	}
	if audio.plays[1] != prefs.Default().Sound.OpenCode.ID {
		t.Errorf("opencode played %q", audio.plays[1])
	}
	if audio.volumes[0] != prefs.Default().Sound.Codex.Volume {
		t.Errorf("codex volume %d", audio.volumes[0])
	}
}

func TestVolumeZeroStillCues(t *testing.T) {
	audio := &fakeAudio{}
	a := &App{audio: audio, prefs: prefs.Default()}
	a.prefs = a.prefs.Set(events.Codex, prefs.SoundChoice{ID: "rise", Volume: 0})
	a.cueStart(events.Codex)

	if a.state.cueCount != 1 {
		t.Fatalf("count %d, want the cue counted", a.state.cueCount)
	}
	if len(audio.plays) != 1 || audio.volumes[0] != 0 {
		t.Fatalf("plays %v volumes %v, want silence requested", audio.plays, audio.volumes)
	}
	if a.state.soundProblem != "" {
		t.Fatalf("silence was reported as a problem: %q", a.state.soundProblem)
	}
}

func TestCueStopsTheSound(t *testing.T) {
	audio := &fakeAudio{}
	a := &App{audio: audio}
	a.cueStop()
	if audio.stops != 1 {
		t.Fatalf("stops %d, want the sound silenced", audio.stops)
	}
}

func TestPreviewCuesNothing(t *testing.T) {
	audio := &fakeAudio{}
	a := &App{audio: audio, prefs: prefs.Default()}
	a.previewSound(events.OpenCode)
	if len(audio.plays) != 1 || audio.plays[0] != prefs.Default().Sound.OpenCode.ID {
		t.Fatalf("plays %v", audio.plays)
	}
	if a.state.cueCount != 0 {
		t.Fatalf("a preview counted as a cue: %d", a.state.cueCount)
	}
}

func TestSoundFailureIsReportedNotFatal(t *testing.T) {
	audio := &fakeAudio{err: errors.New("no device")}
	a := &App{audio: audio, prefs: prefs.Default()}
	a.cueStart(events.Codex)
	if a.state.cueCount != 1 {
		t.Fatalf("a failed sound stopped the cue: %d", a.state.cueCount)
	}
	if !strings.Contains(a.state.soundProblem, "no device") {
		t.Fatalf("problem %q, want the cause shown", a.state.soundProblem)
	}
}

func TestPanelChoiceReachesThePreferences(t *testing.T) {
	a := &App{prefs: prefs.Default(), prefsPath: filepath.Join(t.TempDir(), "preferences.json")}
	a.state.applySound(a.prefs)
	a.state.codexSound.sound = soundIndex("knock")
	a.state.codexSound.volume = 33
	a.changeSound(events.Codex)

	if a.prefs.Sound.Codex.ID != "knock" || a.prefs.Sound.Codex.Volume != 33 {
		t.Fatalf("codex choice %+v", a.prefs.Sound.Codex)
	}
	if a.prefs.Sound.OpenCode != prefs.Default().Sound.OpenCode {
		t.Fatalf("the other producer changed: %+v", a.prefs.Sound.OpenCode)
	}
	// Instead of waiting for the debounce, flush the way quitting does.
	if a.saveTimer != nil {
		a.saveTimer.Stop()
		a.saveTimer = nil
	}
	a.savePrefs()
	stored, err := prefs.Load(a.prefsPath)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Sound.Codex != (prefs.SoundChoice{ID: "knock", Volume: 33}) {
		t.Fatalf("saved %+v", stored.Sound.Codex)
	}
}

func TestAChangeAppliesToTheNextCueAtOnce(t *testing.T) {
	audio := &fakeAudio{}
	a := &App{audio: audio, prefs: prefs.Default()}
	a.state.applySound(a.prefs)
	a.state.codexSound.sound = soundIndex("knock")
	a.state.codexSound.volume = 12
	a.changeSound(events.Codex)
	if a.saveTimer != nil {
		a.saveTimer.Stop()
		a.saveTimer = nil
	}

	a.cueStart(events.Codex)
	if len(audio.plays) != 1 || audio.plays[0] != "knock" || audio.volumes[0] != 12 {
		t.Fatalf("played %v at %v, want the new choice", audio.plays, audio.volumes)
	}
}

func TestViewShowsTheSoundControls(t *testing.T) {
	s := settings{}
	s.applySound(prefs.Default())
	previewed := []events.Source{}
	s.onPreviewSound = func(source events.Source) { previewed = append(previewed, source) }
	tt := ui.NewTester(s.view, 420, 660)

	if !tt.HasText("提示音") {
		t.Fatalf("texts %q", tt.Texts())
	}
	for _, label := range sound.Labels() {
		if !tt.HasText(label) {
			t.Errorf("missing the %q choice", label)
		}
	}
	if !tt.HasText("70%") {
		t.Errorf("the default volume is not shown: %q", tt.Texts())
	}
	// The preview belongs to a producer row, and the Codex row comes first.
	if err := tt.Click("试听"); err != nil {
		t.Fatal(err)
	}
	if len(previewed) != 1 || previewed[0] != events.Codex {
		t.Fatalf("previewed %v, want Codex", previewed)
	}
}

func TestSoundChoiceFollowsThePanelIndex(t *testing.T) {
	s := settings{}
	s.applySound(prefs.Default())
	all := sound.All()
	for i, entry := range all {
		s.codexSound.sound = i
		if got := s.choice(events.Codex); got.ID != entry.ID {
			t.Errorf("index %d chose %q, want %q", i, got.ID, entry.ID)
		}
	}
	// An index from a file written by a build with more sounds still lands
	// on a real sound, and a fractional volume is rounded.
	s.codexSound.sound = len(all) + 3
	s.codexSound.volume = 42.6
	if got := s.choice(events.Codex); got.ID != all[0].ID || got.Volume != 43 {
		t.Fatalf("out-of-range choice %+v", got)
	}
}

func TestViewTogglesCodexIntegration(t *testing.T) {
	enabled := false
	s := settings{codexDetected: true}
	s.onToggleCodex = func() {
		enabled = !enabled
		s.codexInstalled = enabled
	}
	tt := ui.NewTester(s.view, 420, 660)
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

func TestOpenCodeDetailSaysWhatIsMissing(t *testing.T) {
	for _, tc := range []struct {
		state settings
		want  string
	}{
		{settings{opencodeInstalled: true, opencodeVersion: "2.2.5"}, "已安装：重启 OpenCode 后生效，主会话跑完一轮会提示"},
		{settings{opencodeVersion: "2.2.5"}, "未安装（检测到 OpenCode 2.2.5）"},
		{settings{}, "未检测到 OpenCode"},
		{settings{opencodeProblem: "坏了"}, "坏了"},
	} {
		if got := tc.state.openCodeDetail(); got != tc.want {
			t.Errorf("%+v: got %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestOpenCodeProblemExplainsTheVersionGate(t *testing.T) {
	if got := openCodeProblem(integrations.UnsupportedVersionError{Version: "3.0.0"}); !strings.Contains(got, "3.0.0") {
		t.Fatalf("got %q, want the detected version", got)
	}
	if got := openCodeProblem(integrations.UnsupportedVersionError{}); !strings.Contains(got, "没有检测到") {
		t.Fatalf("got %q, want it to say OpenCode is missing", got)
	}
	if got := openCodeProblem(integrations.ErrForeignPlugin); !strings.Contains(got, "cuedex.js") {
		t.Fatalf("got %q, want the file named", got)
	}
}
