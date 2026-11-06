package app

import (
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/i18n"
	"github.com/def-peter/cuedex-win/internal/integrations"
	"github.com/def-peter/cuedex-win/internal/overlay"
	"github.com/def-peter/cuedex-win/internal/playback"
	"github.com/def-peter/cuedex-win/internal/prefs"
	"github.com/def-peter/cuedex-win/internal/sound"
	"github.com/def-peter/cuedex-win/internal/speech"
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
	a.prefs = prefs.Default()
	a.state.applyGeneral(a.prefs.General, time.Now())
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
	a.prefs = prefs.Default()
	a.state.applyGeneral(a.prefs.General, time.Now())
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
	a := &App{prefs: prefs.Default()}
	codex, opencode := a.sourceCue(events.Codex), a.sourceCue(events.OpenCode)
	if codex.Primary == opencode.Primary {
		t.Fatalf("both sources cue in %v", codex.Primary)
	}
	if codex.Secondary == opencode.Secondary {
		t.Fatalf("both sources flash the same second colour %v", codex.Secondary)
	}
	if !codex.Animation.Valid() || !opencode.Animation.Valid() {
		t.Fatalf("animations %v and %v", codex.Animation, opencode.Animation)
	}
	if got := a.sourceCue(""); got != codex {
		t.Fatalf("a bare wake cues %+v, want the product cue %+v", got, codex)
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
	if !a.state.soundProblem.empty() {
		t.Fatalf("silence was reported as a problem: %v", a.state.soundProblem)
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
	if !strings.Contains(a.state.soundProblem.detail, "no device") {
		t.Fatalf("problem %+v, want the cause shown", a.state.soundProblem)
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
	tt := ui.NewTester(s.view, 420, 1200)

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

// fakeSpeaker records what the app asked to say.
type fakeSpeaker struct {
	voices   []speech.Voice
	voiceErr error
	speakErr error
	said     []string
	voiceIDs []string
	stops    int
	closed   bool
}

func (f *fakeSpeaker) Voices() ([]speech.Voice, error) { return f.voices, f.voiceErr }

func (f *fakeSpeaker) Speak(voiceID, text string) error {
	f.voiceIDs = append(f.voiceIDs, voiceID)
	f.said = append(f.said, text)
	return f.speakErr
}

func (f *fakeSpeaker) Stop()  { f.stops++ }
func (f *fakeSpeaker) Close() { f.closed = true }

func TestCueSpeaksTheFixedPhrase(t *testing.T) {
	speaker := &fakeSpeaker{voices: []speech.Voice{{ID: "voice", Name: "Voice", Language: "zh-CN"}}}
	audio := &fakeAudio{}
	a := &App{audio: audio, speech: speaker, prefs: prefs.Default(), voicesLoaded: true, voices: speaker.voices}
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Voice: "voice", Codex: true})
	a.cueStart(events.Codex)

	if len(speaker.said) != 1 || speaker.said[0] != "Codex 已完成" {
		t.Fatalf("said %v", speaker.said)
	}
	if speaker.voiceIDs[0] != "voice" {
		t.Fatalf("voice %q", speaker.voiceIDs[0])
	}
	// Speech is additive by default.
	if len(audio.plays) != 1 {
		t.Fatalf("plays %v, want the sound too", audio.plays)
	}
}

func TestSpeechReplacesTheSoundOnlyWhenAsked(t *testing.T) {
	speaker := &fakeSpeaker{voices: []speech.Voice{{ID: "voice", Name: "Voice"}}}
	audio := &fakeAudio{}
	a := &App{audio: audio, speech: speaker, prefs: prefs.Default(), voicesLoaded: true, voices: speaker.voices}
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Codex: true, ReplaceSound: true})
	a.cueStart(events.Codex)
	if len(speaker.said) != 1 || len(audio.plays) != 0 {
		t.Fatalf("said %v plays %v, want speech instead of sound", speaker.said, audio.plays)
	}
}

func TestNoVoicesFallsBackToTheSound(t *testing.T) {
	// The preference was saved on a machine with voices; this one has none.
	speaker := &fakeSpeaker{}
	audio := &fakeAudio{}
	a := &App{audio: audio, speech: speaker, prefs: prefs.Default(), voicesLoaded: true}
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Codex: true, ReplaceSound: true})
	a.cueStart(events.Codex)
	if len(speaker.said) != 0 {
		t.Fatalf("said %v without a voice", speaker.said)
	}
	if len(audio.plays) != 1 {
		t.Fatalf("plays %v, want the chime back", audio.plays)
	}
}

func TestSpeechIsOffByDefault(t *testing.T) {
	speaker := &fakeSpeaker{voices: []speech.Voice{{ID: "voice"}}}
	a := &App{audio: &fakeAudio{}, speech: speaker, prefs: prefs.Default(), voicesLoaded: true, voices: speaker.voices}
	a.cueStart(events.Codex)
	if len(speaker.said) != 0 {
		t.Fatalf("said %v with speech off", speaker.said)
	}
}

func TestSpeechFailureDoesNotStopTheCue(t *testing.T) {
	speaker := &fakeSpeaker{speakErr: errors.New("no output")}
	a := &App{audio: &fakeAudio{}, speech: speaker, prefs: prefs.Default()}
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Codex: true})
	a.cueStart(events.Codex)
	if a.state.cueCount != 1 {
		t.Fatalf("count %d, want the cue counted", a.state.cueCount)
	}
	if !strings.Contains(a.state.speechProblem.detail, "no output") {
		t.Fatalf("problem %+v", a.state.speechProblem)
	}
}

func TestCueStopSilencesTheSpeech(t *testing.T) {
	speaker := &fakeSpeaker{}
	a := &App{speech: speaker}
	a.cueStop()
	if speaker.stops != 1 {
		t.Fatalf("stops %d", speaker.stops)
	}
}

func TestVoiceChoiceMapsLabelsToTokens(t *testing.T) {
	s := settings{}
	huihui := speech.Voice{ID: "id-zh", Name: "Huihui", Language: "zh-CN"}
	zira := speech.Voice{ID: "id-en", Name: "Zira", Language: "en-US"}
	s.setVoices([]speech.Voice{huihui, zira})
	s.applySpeech(prefs.Default().SetSpeech(prefs.SpeechPreferences{Voice: "id-en", Codex: true}))
	if s.speechVoiceLabel != zira.Label() {
		t.Fatalf("label %q, want %q", s.speechVoiceLabel, zira.Label())
	}
	if got := s.selectedVoiceID(); got != "id-en" {
		t.Fatalf("selected %q", got)
	}
	s.speechVoiceLabel = huihui.Label()
	if got := s.selectedVoiceID(); got != "id-zh" {
		t.Fatalf("selected %q", got)
	}
	s.speechVoiceLabel = "系统默认"
	if got := s.selectedVoiceID(); got != "" {
		t.Fatalf("default selected %q", got)
	}
}

func TestVoiceThatIsNotInstalledFallsBack(t *testing.T) {
	s := settings{}
	s.setVoices([]speech.Voice{{ID: "id-zh", Name: "Huihui", Language: "zh-CN"}})
	s.applySpeech(prefs.Default().SetSpeech(prefs.SpeechPreferences{Voice: "id-from-another-machine", Codex: true}))
	if s.speechVoiceLabel != "系统默认" || s.selectedVoiceID() != "" {
		t.Fatalf("label %q id %q, want the default", s.speechVoiceLabel, s.selectedVoiceID())
	}
	if !s.speechCodex {
		t.Fatal("the toggle was lost with the voice")
	}
}

func TestSpeechSectionAppearsWithVoices(t *testing.T) {
	s := settings{}
	s.setVoices([]speech.Voice{{ID: "id-zh", Name: "Huihui", Language: "zh-CN"}})
	s.applySpeech(prefs.Default())
	changed := 0
	s.onChangeSpeech = func() { changed++ }
	tt := ui.NewTester(s.view, 420, 1200)

	if !tt.HasText("语音播报") || !tt.HasText("不朗读任何回复内容") {
		t.Fatalf("texts %q", tt.Texts())
	}
	options := s.voiceOptions()
	if len(options) != 2 || options[0] != "系统默认" || options[1] != "Huihui（zh-CN）" {
		t.Errorf("voice options %v", options)
	}
	if !tt.HasText("系统默认") {
		t.Errorf("the picker does not show the default: %q", tt.Texts())
	}
	if err := tt.Click("朗读 Codex"); err != nil {
		t.Fatal(err)
	}
	if !s.speechCodex || changed != 1 {
		t.Fatalf("toggle %v changes %d", s.speechCodex, changed)
	}
}

func TestSpeechSectionIsHiddenWithoutVoices(t *testing.T) {
	s := settings{}
	s.setVoices(nil)
	s.applySpeech(prefs.Default())
	tt := ui.NewTester(s.view, 420, 1200)
	if tt.HasText("语音播报") {
		t.Fatalf("speech is offered without a voice: %q", tt.Texts())
	}
	if !tt.HasText("提示音") {
		t.Fatal("the sound section disappeared too")
	}
}

func TestViewTogglesCodexIntegration(t *testing.T) {
	enabled := false
	s := settings{codexDetected: true}
	s.onToggleCodex = func() {
		enabled = !enabled
		s.codexInstalled = enabled
	}
	tt := ui.NewTester(s.view, 420, 1200)
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
		{settings{codexProblem: newProblem(codexProblemKind, "坏了")}, "Codex 集成操作失败：坏了"},
	} {
		if got := tc.state.codexDetail(); got != tc.want {
			t.Errorf("%+v: got %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestCodexProblemNamesTheBrokenFile(t *testing.T) {
	if got := codexProblem(integrations.ErrInvalidHooks).text(chinese); !strings.Contains(got, "hooks.json") {
		t.Fatalf("got %q, want the file named", got)
	}
	if got := codexProblem(errors.New("access denied")).text(chinese); !strings.Contains(got, "access denied") {
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
		{settings{opencodeProblem: newProblem(openCodeOther, "坏了")}, "OpenCode 集成操作失败：坏了"},
	} {
		if got := tc.state.openCodeDetail(); got != tc.want {
			t.Errorf("%+v: got %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestOpenCodeProblemExplainsTheVersionGate(t *testing.T) {
	if got := openCodeProblem(integrations.UnsupportedVersionError{Version: "3.0.0"}).text(chinese); !strings.Contains(got, "3.0.0") {
		t.Fatalf("got %q, want the detected version", got)
	}
	if got := openCodeProblem(integrations.UnsupportedVersionError{}).text(chinese); !strings.Contains(got, "没有检测到") {
		t.Fatalf("got %q, want it to say OpenCode is missing", got)
	}
	if got := openCodeProblem(integrations.ErrForeignPlugin).text(chinese); !strings.Contains(got, "cuedex.js") {
		t.Fatalf("got %q, want the file named", got)
	}
}

func TestPanelIsDrawnInTheStoredLanguage(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	a.prefs = a.prefs.SetGeneral(prefs.General{Language: prefs.LanguageEnglish, Enabled: true})
	a.state.applyGeneral(a.prefs.General, time.Now())
	a.state.applyDisplay(a.prefs)
	tt := ui.NewTester(a.state.view, 420, 1200)

	for _, want := range []string{"General", "Cue on completion", "Appearance", "Cue sound", "Integrations", "Close"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q; texts %q", want, tt.Texts())
		}
	}
	if tt.HasText("提示音") {
		t.Errorf("the Chinese section is still shown: %q", tt.Texts())
	}
}

func TestSoundAndColourNamesAreLocalized(t *testing.T) {
	if len(chinese.soundLabels()) != len(sound.All()) || len(english.soundLabels()) != len(sound.All()) {
		t.Fatalf("sound labels %q and %q do not name every sound", chinese.soundLabels(), english.soundLabels())
	}
	if chinese.soundLabels()[0] == english.soundLabels()[0] {
		t.Fatalf("both languages name the first sound %q", chinese.soundLabels()[0])
	}
	if len(chinese.colourLabels()) != len(prefs.Palette) {
		t.Fatalf("%d colour names for %d palette entries", len(chinese.colourLabels()), len(prefs.Palette))
	}
	if chinese.colourLabels()[0] == english.colourLabels()[0] {
		t.Fatalf("both languages name the first colour %q", chinese.colourLabels()[0])
	}
}

// TestPanelDrawsTheSoundsInItsLanguage keeps the picker from showing Chinese
// names inside an English panel.
func TestPanelDrawsTheSoundsInItsLanguage(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	a.prefs = a.prefs.SetGeneral(prefs.General{Language: prefs.LanguageEnglish, Enabled: true})
	a.state.applyGeneral(a.prefs.General, time.Now())
	a.state.applySound(a.prefs)
	tt := ui.NewTester(a.state.view, 420, 1200)

	if !tt.HasText("Rise") || !tt.HasText("Knock") {
		t.Fatalf("English sound names missing: %q", tt.Texts())
	}
	if tt.HasText("上扬") {
		t.Fatalf("a Chinese sound name is still shown: %q", tt.Texts())
	}
}

func TestPanelShowsTheGeneralControls(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	a.state.applyGeneral(a.prefs.General, time.Now())
	a.state.applyDisplay(a.prefs)
	tt := ui.NewTester(a.state.view, 420, 1200)

	for _, want := range []string{"通用", "启用提示", "语言", "暂停", "免打扰", "开机时启动", "外观", "交替闪烁"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q; texts %q", want, tt.Texts())
		}
	}
}

func TestPanelPauseChoicesStoreADeadline(t *testing.T) {
	a := &App{prefs: prefs.Default(), player: playback.New(func(events.Source) {}, func() {})}
	defer a.player.Close()
	a.state.applyGeneral(a.prefs.General, time.Now())
	a.state.onPauseFor = a.pauseFor
	tt := ui.NewTester(a.state.view, 420, 1200)

	if err := tt.Click("1 小时"); err != nil {
		t.Fatal(err)
	}
	if !a.state.paused || !a.prefs.General.Paused {
		t.Fatalf("the panel is not paused: %v %+v", a.state.paused, a.prefs.General)
	}
	remaining := a.prefs.General.Remaining(time.Now())
	if remaining < 59*time.Minute || remaining > time.Hour {
		t.Fatalf("remaining %v, want about an hour", remaining)
	}
	if a.player.Submit(events.Event{Source: events.Codex, CompletedAt: time.Now()}) {
		t.Fatal("the player took a cue while paused")
	}
}

func TestPanelLanguageSwitchIsStored(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	a.state.applyGeneral(a.prefs.General, time.Now())
	a.state.applyDisplay(a.prefs)
	// The picker binds to a label, so this is what a click on "English" leaves
	// behind before the change handler runs.
	a.state.languageLabel = a.state.languageOptions()[2]
	a.changeLanguage()

	if a.prefs.General.Language != prefs.LanguageEnglish {
		t.Fatalf("stored language %q", a.prefs.General.Language)
	}
	if a.state.language != i18n.English {
		t.Fatalf("panel language %q", a.state.language)
	}
	tt := ui.NewTester(a.state.view, 420, 1200)
	if !tt.HasText("General") {
		t.Fatalf("the panel did not switch: %q", tt.Texts())
	}
}

func TestSavedDisplayReachesTheCue(t *testing.T) {
	a := &App{prefs: prefs.Default().SetDisplay(events.Codex, prefs.DisplayPreferences{
		Enabled:   true,
		Animation: overlay.AlternatingFlash,
		Color:     "#F472B6",
	})}
	cue := a.sourceCue(events.Codex)
	if cue.Primary != (color.NRGBA{R: 0xF4, G: 0x72, B: 0xB6, A: 255}) {
		t.Fatalf("primary %v, want the saved colour", cue.Primary)
	}
	if cue.Animation != overlay.AlternatingFlash {
		t.Fatalf("animation %v, want the saved one", cue.Animation)
	}
	if cue.Secondary == cue.Primary {
		t.Fatal("the flash alternates with the same colour")
	}
}

func TestPanelDisplayChoicesAreStored(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	a.state.applyGeneral(a.prefs.General, time.Now())
	a.state.applyDisplay(a.prefs)
	a.state.codexDisplay.enabled = false
	a.state.codexDisplay.animation = animationFlash
	a.state.codexDisplay.colour = "粉"
	a.changeDisplay(events.Codex)

	got := a.prefs.Display.Codex
	if got.Enabled || got.Animation != overlay.AlternatingFlash || got.Color != "#F472B6" {
		t.Fatalf("stored %+v", got)
	}
	if a.prefs.Display.OpenCode != prefs.Default().Display.OpenCode {
		t.Fatalf("the other producer changed: %+v", a.prefs.Display.OpenCode)
	}
}

func TestPanelQuietHoursAreStored(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	a.state.applyGeneral(a.prefs.General, time.Now())
	a.state.quietEnabled = true
	a.state.quietStart = "21:30"
	a.state.quietEnd = "08:00"
	a.changeGeneral()

	hours := a.prefs.General.QuietHours
	if !hours.Enabled || hours.Start != 21*60+30 || hours.End != 8*60 {
		t.Fatalf("stored %+v", hours)
	}
}

func TestClockLabelsRoundTrip(t *testing.T) {
	for _, minute := range []int{0, 30, 90, 23*60 + 30} {
		if got := minuteOf(clockLabel(minute)); got != minute {
			t.Errorf("%d became %q and back to %d", minute, clockLabel(minute), got)
		}
	}
	if got := len(clockOptions()); got != 48 {
		t.Fatalf("%d half-hour options", got)
	}
}

// fakeNotifier records the notifications the app tried to show.
type fakeNotifier struct {
	shown chan [2]string
}

func newFakeNotifier() *fakeNotifier {
	return &fakeNotifier{shown: make(chan [2]string, 4)}
}

func (f *fakeNotifier) Show(title, body string) error {
	f.shown <- [2]string{title, body}
	return nil
}

func (f *fakeNotifier) next(t *testing.T) string {
	t.Helper()
	select {
	case message := <-f.shown:
		if message[0] != "CueDex" {
			t.Fatalf("notification title %q", message[0])
		}
		return message[1]
	case <-time.After(2 * time.Second):
		t.Fatal("no notification was shown")
		return ""
	}
}

// The three ways an install can fail each have to say why when the user is not
// looking at the panel.
func TestToastExplainsAnUnsupportedOpenCode(t *testing.T) {
	dir := t.TempDir()
	toasts := newFakeNotifier()
	a := &App{
		prefs: prefs.Default(),
		opencode: integrations.OpenCodeIntegration{
			ConfigDir: dir,
			EventsDir: filepath.Join(dir, "events"),
			GatePath:  filepath.Join(dir, "gate"),
			Version:   "3.0.0",
		},
		toasts: toasts,
	}

	a.toggleOpenCode()

	if body := toasts.next(t); !strings.Contains(body, "3.0.0") || !strings.Contains(body, "支持范围") {
		t.Fatalf("notification %q", body)
	}
}

func TestToastExplainsAForeignPlugin(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plugins", "cuedex.js")
	if err := os.WriteFile(path, []byte("// someone else's plugin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	toasts := newFakeNotifier()
	a := &App{
		prefs: prefs.Default(),
		opencode: integrations.OpenCodeIntegration{
			ConfigDir: dir,
			EventsDir: filepath.Join(dir, "events"),
			GatePath:  filepath.Join(dir, "gate"),
			Version:   "2.2.5",
		},
		toasts: toasts,
	}

	a.toggleOpenCode()

	if body := toasts.next(t); !strings.Contains(body, "cuedex.js") || !strings.Contains(body, "不是 CueDex") {
		t.Fatalf("notification %q", body)
	}
}

func TestToastExplainsAFileInUse(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "hooks.json")
	if err := os.WriteFile(path, []byte(`{"hooks":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	held, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(held)
	toasts := newFakeNotifier()
	a := &App{
		prefs:  prefs.Default(),
		codex:  integrations.CodexIntegration{Home: home, Exe: "CueDex.exe"},
		toasts: toasts,
	}

	a.toggleCodex()

	if body := toasts.next(t); !strings.Contains(body, "hooks.json") || !strings.Contains(body, "占用") {
		t.Fatalf("notification %q", body)
	}
}

// TestNotificationsAreQuietWhenNothingFailed keeps a successful install from
// telling the user about it.
func TestNotificationsAreQuietWhenNothingFailed(t *testing.T) {
	toasts := newFakeNotifier()
	a := &App{prefs: prefs.Default(), toasts: toasts}
	a.notify(problem{})
	select {
	case message := <-toasts.shown:
		t.Fatalf("notified %q", message[1])
	case <-time.After(100 * time.Millisecond):
	}
}

func TestDiagnosticsReportBothIntegrations(t *testing.T) {
	home := t.TempDir()
	config := t.TempDir()
	if err := os.MkdirAll(filepath.Join(config, "plugins"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "plugins", "cuedex.js"), []byte("// stranger\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{
		prefs:    prefs.Default(),
		codex:    integrations.CodexIntegration{Home: home, Exe: "CueDex.exe"},
		opencode: integrations.OpenCodeIntegration{ConfigDir: config, Version: "2.2.5"},
	}
	a.state.diag.lastEvent = time.Date(2026, 11, 6, 14, 32, 7, 0, time.Local)
	a.state.diag.lastSource = events.OpenCode

	a.refreshDiagnostics()

	if a.state.diag.plugin != pluginForeign {
		t.Errorf("plugin state %v", a.state.diag.plugin)
	}
	if a.state.diag.hooks != hooksMissing {
		t.Errorf("hooks state %v", a.state.diag.hooks)
	}
	if !a.state.diag.codexDetected {
		t.Error("the Codex directory was not detected")
	}
	lines := strings.Join([]string{
		a.state.diag.versionLine(chinese),
		a.state.diag.pluginLine(chinese),
		a.state.diag.codexLine(chinese),
		a.state.diag.lastEventLine(chinese),
	}, "\n")
	for _, want := range []string{
		"OpenCode 版本：2.2.5",
		"插件文件：存在，但不是 CueDex 写的",
		"Codex：目录已找到，未安装 Stop 钩子",
		"最近一次事件：14:32:07（OpenCode）",
	} {
		if !strings.Contains(lines, want) {
			t.Errorf("diagnostics do not show %q:\n%s", want, lines)
		}
	}
}

func TestPanelShowsDiagnosticsInItsLanguage(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	a.state.applyGeneral(prefs.General{Language: prefs.LanguageEnglish, Enabled: true}, time.Now())
	a.state.diag.plugin = pluginOurs
	a.state.diag.codexDetected = true
	a.state.diag.hooks = hooksOurs
	tt := ui.NewTester(a.state.view, 420, 1400)

	for _, want := range []string{"Diagnostics", "Plugin file: installed", "Codex: directory found, Stop hook installed"} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing from %q", want, tt.Texts())
		}
	}
	if tt.HasText("诊断") {
		t.Fatalf("Chinese diagnostics are still shown: %q", tt.Texts())
	}
}
