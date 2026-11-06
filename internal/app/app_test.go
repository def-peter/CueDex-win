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
	"github.com/def-peter/cuedex-win/internal/recordings"
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
	tt := ui.NewTester(a.state.view, 820, 700)

	if tt.HasText("已暂停") {
		t.Fatalf("the idle page already says it is paused: %q", tt.Texts())
	}
	if err := tt.Click("暂停通知"); err != nil {
		t.Fatal(err)
	}
	if !a.state.paused || !tt.HasText("恢复提示") {
		t.Fatalf("paused %v, texts %q", a.state.paused, tt.Texts())
	}
}

// The page says nothing while the app simply waits: when the last cue came,
// and how many there have been, are not worth a line in the window.
func TestViewSaysNothingWhileItWaits(t *testing.T) {
	a := &App{}
	a.prefs = prefs.Default()
	a.state.applyGeneral(a.prefs.General, time.Now())
	a.state.cueCount = 2
	tt := ui.NewTester(a.state.view, 820, 700)

	if tt.HasText("最近一次") || tt.HasText("唤醒") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestCueStartCountsTheCue(t *testing.T) {
	a := &App{}
	a.cueStart(events.OpenCode)
	if a.state.cueCount != 1 {
		t.Fatalf("after one cue: %+v", a.state)
	}
}

func TestBothSourcesStartInTheSameMonochromeBreathing(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	codex, opencode := a.sourceCue(events.Codex), a.sourceCue(events.OpenCode)
	// Both start in the product blue breathing, as the mac app does; the user
	// tells them apart by giving one a colour or an effect of its own.
	if codex != opencode {
		t.Fatalf("defaults drifted apart: %+v and %+v", codex, opencode)
	}
	if codex.Animation != overlay.Breathing || codex.Primary != codex.Secondary {
		t.Fatalf("the default cue is %+v, want one breathing colour", codex)
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

// fakeClip records the bundled clips the app asked to play.
type fakeClip struct {
	plays   [][]byte
	volumes []int
	stops   int
	err     error
}

func (f *fakeClip) PlayBytes(data []byte, volume int) error {
	f.plays = append(f.plays, data)
	f.volumes = append(f.volumes, volume)
	return f.err
}

func (f *fakeClip) Stop() { f.stops++ }

func TestCuePlaysTheRecordedClipWithoutAVoice(t *testing.T) {
	clips := &fakeClip{}
	a := &App{clips: clips, prefs: prefs.Default()}
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Codex: true, CodexMode: prefs.SpeechRecording})

	a.cueStart(events.Codex)

	if len(clips.plays) != 1 || len(clips.plays[0]) == 0 {
		t.Fatalf("played %d clips", len(clips.plays))
	}
	if clips.volumes[0] != clipVolume {
		t.Fatalf("volume %d", clips.volumes[0])
	}
	if !a.state.speechProblem.empty() {
		t.Fatalf("a bundled clip was reported as a problem: %v", a.state.speechProblem)
	}
}

func TestCueUsesTheRecordingTheSourceNames(t *testing.T) {
	clips := &fakeClip{}
	a := &App{clips: clips, prefs: prefs.Default()}
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{
		OpenCode:          true,
		OpenCodeMode:      prefs.SpeechRecording,
		OpenCodeRecording: "check-your-messages-female",
	})

	a.cueStart(events.OpenCode)

	want, ok := recordings.Lookup("check-your-messages-female")
	if !ok {
		t.Fatal("the clip is not bundled")
	}
	bytes, err := want.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if len(clips.plays) != 1 || len(clips.plays[0]) != len(bytes) {
		t.Fatalf("played %d clips", len(clips.plays))
	}
}

func TestRecordingModeNeedsNoInstalledVoice(t *testing.T) {
	a := &App{clips: &fakeClip{}, voicesLoaded: true}
	a.prefs = prefs.Default().SetSpeech(prefs.SpeechPreferences{Codex: true, CodexMode: prefs.SpeechRecording})
	if !a.speechWanted(events.Codex) {
		t.Fatal("a bundled clip was refused because no system voice is installed")
	}
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Codex: true, CodexMode: prefs.SpeechSystem})
	if a.speechWanted(events.Codex) {
		t.Fatal("the system voice was used although none is installed")
	}
}

func TestPreviewPlaysTheRecordedClip(t *testing.T) {
	clips := &fakeClip{}
	a := &App{clips: clips, prefs: prefs.Default()}
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Codex: true, CodexMode: prefs.SpeechRecording})

	a.previewSpeech(events.Codex)

	if len(clips.plays) != 1 {
		t.Fatalf("played %d clips", len(clips.plays))
	}
	if a.state.cueCount != 0 {
		t.Fatalf("a preview counted as a cue: %d", a.state.cueCount)
	}
}

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

// TestPreviewGlowCuesNothing mirrors TestPreviewCuesNothing with the path that
// has no overlay in a test: the preview must not touch the cue counters.
func TestPreviewGlowCuesNothing(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	a.previewGlow(events.Codex)
	if a.state.cueCount != 0 {
		t.Fatalf("a glow preview counted as a cue: %+v", a.state)
	}
}

func TestPreviewSpeechUsesTheUsersPhrase(t *testing.T) {
	speaker := &fakeSpeaker{}
	a := &App{speech: speaker, prefs: prefs.Default()}
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{CodexMode: prefs.SpeechSystem, CodexText: "自定义短语"})

	a.previewSpeech(events.Codex)
	if len(speaker.said) != 1 || speaker.said[0] != "自定义短语" {
		t.Fatalf("said %v", speaker.said)
	}
	if a.speechPreviewTimer == nil {
		t.Fatal("the preview phrase has no stop timer")
	}
	if a.state.cueCount != 0 {
		t.Fatalf("a speech preview counted as a cue: %d", a.state.cueCount)
	}
	a.stopSpeechPreview()
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
	s := settings{tab: tabCues, pane: paneSound, source: string(events.Codex)}
	s.applySound(prefs.Default())
	previewed := []events.Source{}
	s.onPreviewSound = func(source events.Source) { previewed = append(previewed, source) }
	tt := ui.NewTester(s.view, 820, 700)

	if !tt.HasText("通知音") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if !tt.HasText(sound.Labels()[soundIndex(prefs.Default().Sound.Codex.ID)]) {
		t.Errorf("the chosen sound is not shown: %q", tt.Texts())
	}
	if err := tt.Click("测试声音"); err != nil {
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
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Voice: "voice", Codex: true, CodexMode: prefs.SpeechSystem})
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

func TestCueSpeaksTheUsersPhrase(t *testing.T) {
	speaker := &fakeSpeaker{voices: []speech.Voice{{ID: "voice", Name: "Voice", Language: "zh-CN"}}}
	a := &App{audio: &fakeAudio{}, speech: speaker, prefs: prefs.Default(), voicesLoaded: true, voices: speaker.voices}
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Voice: "voice", Codex: true, CodexMode: prefs.SpeechSystem, CodexText: "轮到你啦"})
	a.cueStart(events.Codex)

	if len(speaker.said) != 1 || speaker.said[0] != "轮到你啦" {
		t.Fatalf("said %v", speaker.said)
	}
}

func TestSpeechReplacesTheSoundOnlyWhenAsked(t *testing.T) {
	speaker := &fakeSpeaker{voices: []speech.Voice{{ID: "voice", Name: "Voice"}}}
	audio := &fakeAudio{}
	a := &App{audio: audio, speech: speaker, prefs: prefs.Default(), voicesLoaded: true, voices: speaker.voices}
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Codex: true, CodexMode: prefs.SpeechSystem, ReplaceSound: true})
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
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Codex: true, CodexMode: prefs.SpeechSystem, ReplaceSound: true})
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
	a.prefs = a.prefs.SetSpeech(prefs.SpeechPreferences{Codex: true, CodexMode: prefs.SpeechSystem})
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
	s := settings{tab: tabCues, pane: paneSound, source: string(events.Codex)}
	s.setVoices([]speech.Voice{{ID: "id-zh", Name: "Huihui", Language: "zh-CN"}})
	s.applySpeech(prefs.Default().SetSpeech(prefs.SpeechPreferences{
		CodexMode: prefs.SpeechSystem,
	}))
	changed := 0
	s.onChangeSpeech = func() { changed++ }
	tt := ui.NewTester(s.view, 820, 700)

	if !tt.HasText("通知文字") || !tt.HasText("说话人") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if !tt.HasText("系统朗读") || !tt.HasText("预录语音") {
		t.Fatalf("the mode picker is missing: %q", tt.Texts())
	}
	options := s.voiceOptions()
	if len(options) != 2 || options[0] != "系统默认" || options[1] != "Huihui（zh-CN）" {
		t.Errorf("voice options %v", options)
	}
	if !tt.HasText("系统默认") {
		t.Errorf("the picker does not show the default: %q", tt.Texts())
	}
	clickSwitchUnder(t, tt, "语音")
	if !s.speechCodex || changed != 1 {
		t.Fatalf("the switch did not reach the callback: %v, %d", s.speechCodex, changed)
	}
}

func TestSpeechSectionOffersRecordingsWithoutVoices(t *testing.T) {
	s := settings{tab: tabCues, pane: paneSound, source: string(events.Codex)}
	s.setVoices(nil)
	// The Mac greys the mode picker while speech is off, so a user turns it
	// on first; the test does the same.
	s.applySpeech(prefs.Default().SetSpeech(prefs.SpeechPreferences{Codex: true, CodexMode: prefs.SpeechSystem}))
	changed := 0
	s.onChangeSpeech = func() { changed++ }
	tt := ui.NewTester(s.view, 820, 700)
	if !tt.HasText("通知文字") {
		t.Fatalf("the bundled clips were hidden with the voices: %q", tt.Texts())
	}
	if !tt.HasText("通知音") {
		t.Fatal("the sound section disappeared too")
	}
	if !tt.HasText("没有可用的系统语音；可以改用预录语音。") {
		t.Fatalf("nothing explains the missing voice: %q", tt.Texts())
	}

	if err := tt.Click("预录语音"); err != nil {
		t.Fatal(err)
	}
	if got := s.speech().Mode(events.Codex); got != prefs.SpeechRecording {
		t.Fatalf("mode %q, want the bundled clip", got)
	}
	if changed != 1 {
		t.Fatalf("the change was not announced: %d", changed)
	}
	if tt.HasText("通知文字") {
		t.Fatalf("the phrase field stayed in recording mode: %q", tt.Texts())
	}
	if !tt.HasText("消息提醒（男声）") {
		t.Fatalf("the clip picker is missing: %q", tt.Texts())
	}
}

func TestViewSwitchesBetweenTheThreePages(t *testing.T) {
	s := settings{}
	s.applyGeneral(prefs.Default().General, time.Now())
	s.applyDisplay(prefs.Default())
	s.applySound(prefs.Default())
	tt := ui.NewTester(s.view, 820, 700)

	if !tt.HasText("启用提示") {
		t.Fatalf("the general page is not shown: %q", tt.Texts())
	}
	if err := tt.Click("提醒"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("提醒来源") || !tt.HasText("光效模式") {
		t.Fatalf("the cues page is not shown: %q", tt.Texts())
	}
	if err := tt.Click("关于"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("CueDex") || !tt.HasText("Codex 或 OpenCode 完成回复时") {
		t.Fatalf("the about page is not shown: %q", tt.Texts())
	}
	if err := tt.Click("通用"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("启用提示") || !tt.HasText("集成") {
		t.Fatalf("the general page did not return: %q", tt.Texts())
	}
}

func TestCuesEditTheSelectedSourceOnly(t *testing.T) {
	a := &App{prefs: prefs.Default(), prefsPath: filepath.Join(t.TempDir(), "preferences.json")}
	a.state.applyDisplay(a.prefs)
	a.state.applySound(a.prefs)
	a.state.onChangeDisplay = a.changeDisplay
	a.state.tab = tabCues
	tt := ui.NewTester(a.state.view, 820, 700)

	if err := tt.Click("OpenCode"); err != nil {
		t.Fatal(err)
	}
	clickSwitch(t, tt, "是否启用")
	if a.prefs.Display.OpenCode.Enabled {
		t.Fatal("the OpenCode glow switch did not turn off")
	}
	if !a.prefs.Display.Codex.Enabled {
		t.Fatal("editing OpenCode changed Codex")
	}
	if a.saveTimer != nil {
		a.saveTimer.Stop()
		a.saveTimer = nil
	}
}

func TestCuePaneSwitchesBetweenGlowAndSound(t *testing.T) {
	s := settings{tab: tabCues, source: string(events.Codex)}
	s.applyDisplay(prefs.Default())
	s.applySound(prefs.Default())
	tt := ui.NewTester(s.view, 820, 700)

	if !tt.HasText("光效模式") || tt.HasText("通知音") {
		t.Fatalf("the glow pane is not shown: %q", tt.Texts())
	}
	if err := tt.Click("声音"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("通知音") || tt.HasText("光效模式") {
		t.Fatalf("the sound pane is not shown: %q", tt.Texts())
	}
	if err := tt.Click("光效"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("光效模式") || tt.HasText("通知音") {
		t.Fatalf("the glow pane did not return: %q", tt.Texts())
	}
}

// TestTheGlowPaneShowsTheColoursTheEffectUses keeps the panel on the mac app's
// structure: 单色呼吸 offers one colour, 双色交替闪烁 offers the pair.
func TestTheGlowPaneShowsTheColoursTheEffectUses(t *testing.T) {
	s := settings{tab: tabCues, source: string(events.Codex)}
	s.applyDisplay(prefs.Default())
	tt := ui.NewTester(s.view, 820, 700)
	shows := func(text string) bool {
		_, ok := tt.Find(text)
		return ok
	}

	if !shows("光效颜色") || shows("主色") || shows("副色") {
		t.Fatalf("monochrome breathing does not show one colour: %q", tt.Texts())
	}
	if err := tt.Click("双色交替闪烁"); err != nil {
		t.Fatal(err)
	}
	if !shows("主色") || !shows("副色") || shows("光效颜色") {
		t.Fatalf("the two-color flash does not show the pair: %q", tt.Texts())
	}
	if err := tt.Click("单色呼吸"); err != nil {
		t.Fatal(err)
	}
	if !shows("光效颜色") || shows("主色") {
		t.Fatalf("the single colour did not come back: %q", tt.Texts())
	}
}

// TestAGreyedCueDoesNotPreview is the Mac's .disabled: with a cue switched
// off, the rest of its page is inert, so the test button does nothing rather
// than previewing an effect the app will not play.
func TestAGreyedCueDoesNotPreview(t *testing.T) {
	p := prefs.Default()
	display := p.Display.Codex
	display.Enabled = false
	p = p.SetDisplay(events.Codex, display).Set(events.Codex, prefs.SoundChoice{
		ID: p.Sound.Codex.ID, Volume: 0,
	})

	s := settings{tab: tabCues, source: string(events.Codex)}
	s.applyDisplay(p)
	s.applySound(p)
	s.setVoices([]speech.Voice{{ID: "id-zh", Name: "Huihui", Language: "zh-CN"}})
	s.applySpeech(p)
	previews := 0
	s.onPreviewGlow = func(events.Source) { previews++ }
	s.onPreviewSound = func(events.Source) { previews++ }
	s.onPreviewSpeech = func(events.Source) { previews++ }
	tt := ui.NewTester(s.view, 820, 700)

	for _, step := range []struct{ label, text string }{
		{"", "测试光效"},
		{"声音", "测试声音"},
		{"", "测试语音"},
	} {
		if step.label != "" {
			if err := tt.Click(step.label); err != nil {
				t.Fatal(err)
			}
		}
		if err := tt.Click(step.text); err != nil {
			t.Fatal(err)
		}
	}
	if previews != 0 {
		t.Fatalf("%d previews ran with every cue off", previews)
	}
}

func TestPreviewButtonsPreviewTheSelectedSource(t *testing.T) {
	s := settings{tab: tabCues, source: string(events.OpenCode)}
	s.applyDisplay(prefs.Default())
	s.applySound(prefs.Default())
	s.setVoices([]speech.Voice{{ID: "id-zh", Name: "Huihui", Language: "zh-CN"}})
	s.applySpeech(prefs.Default().SetSpeech(prefs.SpeechPreferences{OpenCode: true, OpenCodeMode: prefs.SpeechSystem}))

	var glow, sound, spoken []events.Source
	s.onPreviewGlow = func(source events.Source) { glow = append(glow, source) }
	s.onPreviewSound = func(source events.Source) { sound = append(sound, source) }
	s.onPreviewSpeech = func(source events.Source) { spoken = append(spoken, source) }
	tt := ui.NewTester(s.view, 820, 700)

	if err := tt.Click("测试光效"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("声音"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("测试声音"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("测试语音"); err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string][]events.Source{"glow": glow, "sound": sound, "speech": spoken} {
		if len(got) != 1 || got[0] != events.OpenCode {
			t.Errorf("%s previewed %v, want OpenCode once", name, got)
		}
	}
}

func TestSpeechTextWritesToTheSelectedSourceOnly(t *testing.T) {
	a := &App{
		prefs:     prefs.Default().SetSpeech(prefs.SpeechPreferences{OpenCode: true, OpenCodeMode: prefs.SpeechSystem}),
		prefsPath: filepath.Join(t.TempDir(), "preferences.json"),
	}
	a.state.applySound(a.prefs)
	a.state.setVoices([]speech.Voice{{ID: "id-zh", Name: "Huihui", Language: "zh-CN"}})
	a.state.applySpeech(a.prefs)
	a.state.tab = tabCues
	a.state.pane = paneSound
	a.state.source = string(events.OpenCode)
	a.state.onChangeSpeech = a.changeSpeech
	tt := ui.NewTester(a.state.view, 820, 700)

	// The row names the field, at the right edge of the group; the label
	// itself is not the field, so the click lands on the field's own rect.
	row, ok := tt.Find("通知文字")
	if !ok {
		t.Fatalf("no phrase row among %q", tt.Texts())
	}
	tt.ClickAt((820+pageWidth)/2-14-120, row.Y+row.H/2)
	tt.Type("OpenCode 完成了")

	if got := a.prefs.Speech.Text(events.OpenCode); got != "OpenCode 完成了" {
		t.Fatalf("OpenCode phrase %q", got)
	}
	if got := a.prefs.Speech.Text(events.Codex); got != "" {
		t.Fatalf("the Codex phrase changed too: %q", got)
	}
	if a.saveTimer != nil {
		a.saveTimer.Stop()
		a.saveTimer = nil
	}
}

func TestViewTogglesCodexIntegration(t *testing.T) {
	enabled := false
	s := settings{codexDetected: true}
	s.onToggleCodex = func() {
		enabled = !enabled
		s.codexInstalled = enabled
	}
	tt := ui.NewTester(s.view, 820, 1200)
	if !tt.HasText("未安装") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("启用集成"); err != nil {
		t.Fatal(err)
	}
	if !enabled || !tt.HasText("停用集成") {
		t.Fatalf("enabled %v, texts %q", enabled, tt.Texts())
	}
}

func TestCodexDetailSaysWhatIsMissing(t *testing.T) {
	for _, tc := range []struct {
		state settings
		want  string
	}{
		{settings{codexInstalled: true, codexDetected: true}, "已安装"},
		{settings{codexInstalled: true}, "已安装，未找到 Codex"},
		{settings{}, "未安装，未找到 Codex"},
		{settings{codexDetected: true}, "未安装"},
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
		{settings{opencodeInstalled: true, opencodeVersion: "2.2.5"}, "已安装"},
		{settings{opencodeVersion: "2.2.5"}, "OpenCode 2.2.5 版本不支持"},
		{settings{}, "未找到 OpenCode"},
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
	tt := ui.NewTester(a.state.view, 820, 1200)

	for _, want := range []string{"General", "Cue on completion", "Launch at login", "Integrations"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q; texts %q", want, tt.Texts())
		}
	}
	if tt.HasText("通知音") {
		t.Errorf("the Chinese section is still shown: %q", tt.Texts())
	}
}

func TestSoundNamesAreLocalized(t *testing.T) {
	if len(chinese.soundLabels()) != len(sound.All()) || len(english.soundLabels()) != len(sound.All()) {
		t.Fatalf("sound labels %q and %q do not name every sound", chinese.soundLabels(), english.soundLabels())
	}
	if chinese.soundLabels()[0] == english.soundLabels()[0] {
		t.Fatalf("both languages name the first sound %q", chinese.soundLabels()[0])
	}
}

// TestPanelDrawsTheSoundsInItsLanguage keeps the picker from showing Chinese
// names inside an English panel.
func TestPanelDrawsTheSoundsInItsLanguage(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	a.prefs = a.prefs.SetGeneral(prefs.General{Language: prefs.LanguageEnglish, Enabled: true})
	a.state.applyGeneral(a.prefs.General, time.Now())
	a.state.applySound(a.prefs)
	a.state.tab = tabCues
	a.state.pane = paneSound
	tt := ui.NewTester(a.state.view, 820, 700)

	if !tt.HasText("Rise") {
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
	tt := ui.NewTester(a.state.view, 820, 1200)

	for _, want := range []string{"通用", "启用提示", "语言", "暂停通知", "免打扰", "登录时启动", "集成"} {
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
	tt := ui.NewTester(a.state.view, 820, 1200)

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
	tt := ui.NewTester(a.state.view, 820, 1200)
	if !tt.HasText("General") {
		t.Fatalf("the panel did not switch: %q", tt.Texts())
	}
}

func TestSavedDisplayReachesTheCue(t *testing.T) {
	pink := color.NRGBA{R: 0xF4, G: 0x72, B: 0xB6, A: 255}
	violet := color.NRGBA{R: 0xA7, G: 0x8B, B: 0xFA, A: 255}
	a := &App{prefs: prefs.Default().SetDisplay(events.Codex, prefs.DisplayPreferences{
		Enabled:   true,
		Animation: overlay.AlternatingFlash,
		Color:     "#5EEAD4",
		Primary:   "#F472B6",
		Secondary: "#A78BFA",
	})}
	cue := a.sourceCue(events.Codex)
	if cue.Primary != pink || cue.Secondary != violet {
		t.Fatalf("the flash draws %v and %v, want the saved pair", cue.Primary, cue.Secondary)
	}
	if cue.Animation != overlay.AlternatingFlash {
		t.Fatalf("animation %v, want the saved one", cue.Animation)
	}

	// Monochrome breathing draws the single colour and ignores the pair.
	a.prefs = a.prefs.SetDisplay(events.Codex, prefs.DisplayPreferences{
		Enabled: true, Animation: overlay.Breathing,
		Color: prefs.DefaultColour, Primary: "#F472B6", Secondary: "#A78BFA",
	})
	cue = a.sourceCue(events.Codex)
	if cue.Primary != (color.NRGBA{R: 0x3D, G: 0x94, B: 0xFF, A: 255}) || cue.Secondary != cue.Primary {
		t.Fatalf("breathing draws %v and %v, want one blue", cue.Primary, cue.Secondary)
	}
}

// TestColourRowUsesMyGosPicker keeps the glow colour on the framework's own
// colour well and picker rather than a list of names: the well opens the
// picker, a swatch sets the colour, and the panel reports the change.
func TestColourRowUsesMyGosPicker(t *testing.T) {
	colour := ui.Hex(prefs.DefaultColour)
	changes := 0
	s := settings{onChangeDisplay: func(events.Source) { changes++ }}
	tt := ui.NewTester(func(c *ui.Context) {
		s.colourRow(c, paletteFor(c.Theme()), &colour, "Primary colour", events.Codex, true)
	}, 400, 700)

	if !tt.HasText("#3D94FF") {
		t.Fatalf("the row does not show the stored hex: %q", tt.Texts())
	}
	if err := tt.Click("Primary colour"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Green") {
		t.Fatalf("the colour well did not open a picker: %q", tt.Texts())
	}
	if err := tt.Click("Green"); err != nil {
		t.Fatal(err)
	}
	if got := prefs.FormatColour(nrgba(colour)); got != "#22C55E" {
		t.Fatalf("chose %q", got)
	}
	if changes != 1 {
		t.Fatalf("the change was reported %d times", changes)
	}
}

func TestPanelDisplayChoicesAreStored(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	a.state.applyGeneral(a.prefs.General, time.Now())
	a.state.applyDisplay(a.prefs)
	a.state.codexDisplay.enabled = false
	a.state.codexDisplay.animation = animationFlash
	a.state.codexDisplay.primary = ui.Hex("#F472B6")
	a.changeDisplay(events.Codex)

	got := a.prefs.Display.Codex
	if got.Enabled || got.Animation != overlay.AlternatingFlash || got.Primary != "#F472B6" {
		t.Fatalf("stored %+v", got)
	}
	if got.Color != prefs.DefaultColour {
		t.Fatalf("the single colour changed too: %q", got.Color)
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

// The panel keeps one line per integration and nothing else: versions and
// event times belong in the docs, not in the window.
func TestIntegrationsShowAOneLineState(t *testing.T) {
	a := &App{prefs: prefs.Default()}
	a.state.applyGeneral(prefs.Default().General, time.Now())
	a.state.codexDetected = true
	a.state.codexInstalled = true
	a.state.opencodeInstalled = true
	tt := ui.NewTester(a.state.view, 820, 1400)

	if !tt.HasText("已安装") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if tt.HasText("诊断") || tt.HasText("插件文件") || tt.HasText("最近一次事件") {
		t.Fatalf("the diagnostics block is still there: %q", tt.Texts())
	}
}

// clickSwitch clicks the switch of a switch row, which shows its label as
// plain text to the left and the switch at the right edge of the group: a
// click on the label text does nothing, so the switch itself is the target.
// The window is 820 DIPs wide and the page's column is 628 (pageWidth) wide,
// so the column ends at 724; a row is padded 14 from it, and a switch 36 wide
// sits centred 18 short of that edge.
func clickSwitch(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	row, ok := tt.Find(label)
	if !ok {
		t.Fatalf("no row labelled %q among %q", label, tt.Texts())
	}
	tt.ClickAt((820+pageWidth)/2-14-18, row.Y+row.H/2)
}

// clickSwitchUnder clicks the switch on the first row of the group titled
// section: the Mac labels the sound's toggle and the speech's "Enabled" alike,
// so the section, not the row, says which switch this is. A group's label
// carries its own padding, and a one-line row is 11 DIPs around 13px text.
func clickSwitchUnder(t *testing.T, tt *ui.Tester, section string) {
	t.Helper()
	label, ok := tt.Find(section)
	if !ok {
		t.Fatalf("no group labelled %q among %q", section, tt.Texts())
	}
	tt.ClickAt((820+pageWidth)/2-14-18, label.Y+label.H+19)
}
