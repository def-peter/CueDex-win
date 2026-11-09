package app

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/i18n"
	"github.com/def-peter/cuedex-win/internal/overlay"
	"github.com/def-peter/cuedex-win/internal/prefs"
	"github.com/def-peter/cuedex-win/internal/recordings"
	"github.com/def-peter/cuedex-win/internal/sound"
	"github.com/def-peter/cuedex-win/internal/speech"
	"github.com/def-peter/cuedex-win/internal/update"
)

// The animation picker's indexes.
const (
	animationBreathing = iota
	animationFlash
)

// The settings window's top-level pages.
const (
	tabGeneral = iota
	tabCues
	tabAbout
)

// The announcement mode picker's indexes, in the order the picker shows them:
// the recorded choice first, as the mac app does. applySpeech fills mode from
// the saved choice before the panel is drawn.
const (
	speechRecording = iota
	speechSystem
)

// The two halves of the cues page.
const (
	paneGlow = iota
	paneSound
)

// settings is the state the panel shows and the actions it offers. It has no
// window of its own, so a tester can drive it without one.
type settings struct {
	// language is what the panel is drawn in; empty means an uninitialised
	// panel, which draws Chinese, the product's first language.
	language        i18n.Language
	languageSetting string
	languageLabel   string

	// enabled is the master switch.
	enabled bool

	// tab is the page shown; source is the producer the cues page edits, as
	// an event source's name; sourcePick is the same choice as the picker's
	// index, which is what a segmented control binds to; pane chooses glow or
	// sound inside that page.
	tab        int
	source     string
	sourcePick int
	pane       int

	paused bool
	// pauseRemaining is how long a timed pause still has to run; zero with
	// paused set means until the user resumes.
	pauseRemaining time.Duration

	quietEnabled bool
	quietStart   string
	quietEnd     string

	openAtLogin bool
	// autostartProblem is why the Run entry could not be written.
	autostartProblem problem

	codexDisplay    displaySettings
	opencodeDisplay displaySettings

	// cueCount counts the cues the player has drawn. Nothing shows it; the
	// tests use it to prove a preview is not a cue.
	cueCount int
	// problem is why cues cannot arrive, empty when they can.
	problem problem
	// saveProblem is why the preferences could not be written.
	saveProblem problem

	codexInstalled bool
	codexDetected  bool
	codexProblem   problem

	opencodeInstalled bool
	opencodeVersion   string
	opencodeProblem   problem

	// updateState is what the About page says about a newer release; the
	// release itself is kept so the button can install it.
	updateState   updateState
	updateVersion string
	updateRelease update.Release
	updateProblem problem

	// version is what this build calls itself, empty in a development build.
	version string

	codexSound    soundSettings
	opencodeSound soundSettings
	soundProblem  problem

	speechVoices     []speech.Voice
	speechLoaded     bool
	speechProblem    problem
	speechCodex      bool
	speechOpenCode   bool
	speechReplace    bool
	speechVoiceID    string
	speechVoiceLabel string
	codexSpeech      speechSettings
	opencodeSpeech   speechSettings
	// The phrase each producer speaks. prefs normalizes them on save; the
	// panel keeps what is typed so the field does not jump back mid-edit.
	speechTextCodex    string
	speechTextOpenCode string

	onTogglePause    func()
	onClose          func()
	onToggleCodex    func()
	onToggleOpenCode func()

	onToggleEnabled     func()
	onChangeLanguage    func()
	onPauseFor          func(time.Duration)
	onResume            func()
	onChangeQuiet       func()
	onToggleOpenAtLogin func()
	onChangeDisplay     func(events.Source)

	onChangeSound   func(events.Source)
	onPreviewSound  func(events.Source)
	onPreviewGlow   func(events.Source)
	onPreviewSpeech func(events.Source)
	onChangeSpeech  func()

	onCheckUpdate   func()
	onInstallUpdate func()
}

// text is the panel's words in the current language.
func (s *settings) text() wording {
	if s.language == "" {
		return chinese
	}
	return copyFor(s.language)
}

// displaySettings is one producer's appearance controls. The colours are the
// widget's, because a colour well binds to one; prefs stores the hex. The
// single colour belongs to 单色呼吸 and the pair to 双色交替闪烁, which is why
// all three are held even though only some are on screen at a time.
type displaySettings struct {
	enabled   bool
	animation int
	colour    ui.Color
	primary   ui.Color
	secondary ui.Color
}

func (d *displaySettings) apply(p prefs.DisplayPreferences) {
	d.enabled = p.Enabled
	d.animation = animationBreathing
	if p.Animation == overlay.AlternatingFlash {
		d.animation = animationFlash
	}
	d.colour = uiColour(p.Colour())
	d.primary = uiColour(p.FlashPrimary())
	d.secondary = uiColour(p.FlashSecondary())
}

func (d *displaySettings) choice() prefs.DisplayPreferences {
	animation := overlay.Breathing
	if d.animation == animationFlash {
		animation = overlay.AlternatingFlash
	}
	return prefs.DisplayPreferences{
		Enabled:   d.enabled,
		Animation: animation,
		Color:     prefs.FormatColour(nrgba(d.colour)),
		Primary:   prefs.FormatColour(nrgba(d.primary)),
		Secondary: prefs.FormatColour(nrgba(d.secondary)),
	}
}

// uiColour converts a stored colour to the widget's, dropping alpha: the glow
// is always opaque, and the picker's opacity slider must not fade it away.
func uiColour(c color.NRGBA) ui.Color {
	return ui.Color{R: c.R, G: c.G, B: c.B, A: 255}
}

func nrgba(c ui.Color) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}
}

// applyDisplay fills the two appearance blocks from the saved preferences.
func (s *settings) applyDisplay(p prefs.Preferences) {
	s.codexDisplay.apply(p.Display.Codex)
	s.opencodeDisplay.apply(p.Display.OpenCode)
}

// displayChoice is what the panel currently says for one producer.
func (s *settings) displayChoice(source events.Source) prefs.DisplayPreferences {
	if source == events.OpenCode {
		return s.opencodeDisplay.choice()
	}
	return s.codexDisplay.choice()
}

// applyGeneral fills the application-wide controls. now decides the pause and
// the quiet-hours reading.
func (s *settings) applyGeneral(g prefs.General, now time.Time) {
	s.enabled = g.Enabled
	s.paused = g.PausedAt(now)
	s.pauseRemaining = g.Remaining(now)
	s.quietEnabled = g.QuietHours.Enabled
	s.quietStart = clockLabel(g.QuietHours.Start)
	s.quietEnd = clockLabel(g.QuietHours.End)
	s.openAtLogin = g.OpenAtLogin
	s.languageSetting = g.Language
	s.language = i18n.Resolve(g.Language)
	s.languageLabel = s.languageLabelFor(g.Language)
}

// general is what the panel currently says, as the file stores it.
func (s *settings) general(now time.Time) prefs.General {
	g := prefs.General{
		Language:    s.languageSetting,
		Enabled:     s.enabled,
		Paused:      s.paused,
		QuietHours:  prefs.QuietHours{Enabled: s.quietEnabled, Start: minuteOf(s.quietStart), End: minuteOf(s.quietEnd)},
		OpenAtLogin: s.openAtLogin,
	}
	if s.paused && s.pauseRemaining > 0 {
		g.PauseUntil = now.Add(s.pauseRemaining).Unix()
	}
	return g
}

func (s *settings) languageLabelFor(setting string) string {
	t := s.text()
	switch i18n.Language(setting) {
	case i18n.Chinese:
		return t.LangChinese
	case i18n.English:
		return t.LangEnglish
	}
	return t.LangSystem
}

// languageOptions is the picker, in the order the labels are read back.
func (s *settings) languageOptions() []string {
	t := s.text()
	return []string{t.LangSystem, t.LangChinese, t.LangEnglish}
}

// languageSettingFor maps the picker's label back to the stored value.
func (s *settings) languageSettingFor(label string) string {
	t := s.text()
	switch label {
	case t.LangChinese:
		return prefs.LanguageChinese
	case t.LangEnglish:
		return prefs.LanguageEnglish
	}
	return prefs.LanguageSystem
}

// quietAt reports whether the panel's quiet hours cover now.
func (s *settings) quietAt(now time.Time) bool {
	return prefs.QuietHours{
		Enabled: s.quietEnabled,
		Start:   minuteOf(s.quietStart),
		End:     minuteOf(s.quietEnd),
	}.Quiet(now)
}

// clockLabel writes minutes after midnight as a picker label.
func clockLabel(minute int) string {
	if minute < 0 {
		minute = 0
	}
	minute %= 24 * 60
	return fmt.Sprintf("%02d:%02d", minute/60, minute%60)
}

// minuteOf reads a picker label back to minutes after midnight.
func minuteOf(label string) int {
	var hour, minute int
	if _, err := fmt.Sscanf(label, "%d:%d", &hour, &minute); err != nil {
		return 0
	}
	return hour*60 + minute
}

// clockOptions is one label per half hour, which is the precision a quiet
// window needs.
func clockOptions() []string {
	options := make([]string, 0, 48)
	for minute := 0; minute < 24*60; minute += 30 {
		options = append(options, clockLabel(minute))
	}
	return options
}

// soundSettings is one producer's controls: which bundled sound, and how loud.
// The index lives in a field because a widget binds to an address.
type soundSettings struct {
	sound      int     // index into sound.All()
	soundLabel string  // the same entry, as the picker shows it
	volume     float64 // 0-100
	// enabled is the sound switch. The preferences store silence as volume
	// zero, so switching it off writes zero and remembers the level to come
	// back to.
	enabled    bool
	lastVolume float64
}

func (s *soundSettings) apply(choice prefs.SoundChoice, t wording) {
	s.sound = soundIndex(choice.ID)
	s.soundLabel = t.soundLabels()[s.sound]
	s.volume = float64(choice.Volume)
	s.enabled = choice.Volume > 0
	if s.volume > 0 {
		s.lastVolume = s.volume
	} else if s.lastVolume == 0 {
		s.lastVolume = 70
	}
}

// setSound maps the picker's label back to the entry it names.
func (s *soundSettings) setSound(t wording) {
	for i, label := range t.soundLabels() {
		if label == s.soundLabel {
			s.sound = i
			return
		}
	}
}

func (s *soundSettings) choice() prefs.SoundChoice {
	all := sound.All()
	index := s.sound
	if index < 0 || index >= len(all) {
		index = 0
	}
	volume := int(math.Round(s.volume))
	if !s.enabled {
		volume = 0
	}
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	return prefs.SoundChoice{ID: all[index].ID, Volume: volume}
}

// speechSettings is one producer's announcement: the system voice reads the
// stored phrase, or a bundled clip plays. The clip is a label because a Select
// binds to a string.
type speechSettings struct {
	mode      int // speechSystem or speechRecording
	recording string
}

func (s *speechSettings) apply(p prefs.SpeechPreferences, source events.Source, t wording) {
	s.mode = speechSystem
	if p.Mode(source) == prefs.SpeechRecording {
		s.mode = speechRecording
	}
	s.recording = recordingLabel(p.Recording(source), t)
}

func (s *speechSettings) modeValue() prefs.SpeechMode {
	if s.mode == speechRecording {
		return prefs.SpeechRecording
	}
	return prefs.SpeechSystem
}

func (s *speechSettings) recordingValue(t wording) string {
	return recordingValue(s.recording, t)
}

// recordingLabel names a clip in the panel's language. An unknown name shows
// the default clip rather than an empty row.
func recordingLabel(id string, t wording) string {
	labels := t.recordingLabels()
	for i, r := range recordings.All() {
		if r.ID == id && i < len(labels) {
			return labels[i]
		}
	}
	if len(labels) > 0 {
		return labels[0]
	}
	return id
}

func recordingValue(label string, t wording) string {
	all := recordings.All()
	for i, known := range t.recordingLabels() {
		if known == label && i < len(all) {
			return all[i].ID
		}
	}
	return recordings.DefaultID
}

// setEnabled keeps the switch and the stored level coherent: off writes zero
// volume but remembers the last level, and on restores it.
func (s *soundSettings) setEnabled() {
	if s.enabled {
		if s.volume <= 0 {
			s.volume = s.lastVolume
			if s.volume <= 0 {
				s.volume = 70
			}
		}
		return
	}
	if s.volume > 0 {
		s.lastVolume = s.volume
	}
	s.volume = 0
}

// applySound fills the panel from the saved choices.
func (s *settings) applySound(p prefs.Preferences) {
	t := s.text()
	s.codexSound.apply(p.Sound.Codex, t)
	s.opencodeSound.apply(p.Sound.OpenCode, t)
}

// choice is what the panel currently says for a producer.
func (s *settings) choice(source events.Source) prefs.SoundChoice {
	if source == events.OpenCode {
		return s.opencodeSound.choice()
	}
	return s.codexSound.choice()
}

func soundIndex(id string) int {
	for i, s := range sound.All() {
		if s.ID == id {
			return i
		}
	}
	return 0
}

// applySpeech fills the speech controls from the saved choices.
func (s *settings) applySpeech(p prefs.Preferences) {
	s.speechCodex = p.Speech.Codex
	s.speechOpenCode = p.Speech.OpenCode
	s.speechReplace = p.Speech.ReplaceSound
	s.speechVoiceID = p.Speech.Voice
	s.speechVoiceLabel = s.labelForVoiceID(p.Speech.Voice)
	s.speechTextCodex = p.Speech.Text(events.Codex)
	s.speechTextOpenCode = p.Speech.Text(events.OpenCode)
	s.codexSpeech.apply(p.Speech, events.Codex, s.text())
	s.opencodeSpeech.apply(p.Speech, events.OpenCode, s.text())
}

// setVoices records what SAPI reports and points the picker at the voice that
// is actually installed. A voice saved by another machine is not offered, and
// the next save drops it.
func (s *settings) setVoices(voices []speech.Voice) {
	s.speechVoices = voices
	s.speechLoaded = true
	if _, ok := s.voiceForID(s.speechVoiceID); !ok {
		s.speechVoiceID = ""
	}
	s.speechVoiceLabel = s.labelForVoiceID(s.speechVoiceID)
}

func (s *settings) voiceForID(id string) (speech.Voice, bool) {
	for _, v := range s.speechVoices {
		if v.ID == id {
			return v, true
		}
	}
	return speech.Voice{}, false
}

func (s *settings) labelForVoiceID(id string) string {
	if id == "" {
		return s.text().VoiceDefault
	}
	if v, ok := s.voiceForID(id); ok {
		return v.Label()
	}
	return s.text().VoiceDefault
}

// voiceOptions is the picker's list. The first choice is always the default.
func (s *settings) voiceOptions() []string {
	defaultLabel := s.text().VoiceDefault
	options := make([]string, 0, len(s.speechVoices)+1)
	options = append(options, defaultLabel)
	for _, v := range s.speechVoices {
		options = append(options, v.Label())
	}
	return options
}

// selectedVoiceID maps the picker's label back to the token id.
func (s *settings) selectedVoiceID() string {
	if s.speechVoiceLabel == "" || s.speechVoiceLabel == s.text().VoiceDefault {
		return ""
	}
	for _, v := range s.speechVoices {
		if v.Label() == s.speechVoiceLabel {
			return v.ID
		}
	}
	return ""
}

// speech is what the panel currently says about the announcement.
func (s *settings) speech() prefs.SpeechPreferences {
	return prefs.SpeechPreferences{
		Voice:             s.selectedVoiceID(),
		Codex:             s.speechCodex,
		OpenCode:          s.speechOpenCode,
		ReplaceSound:      s.speechReplace,
		CodexText:         s.speechTextCodex,
		OpenCodeText:      s.speechTextOpenCode,
		CodexMode:         s.codexSpeech.modeValue(),
		OpenCodeMode:      s.opencodeSpeech.modeValue(),
		CodexRecording:    s.codexSpeech.recordingValue(s.text()),
		OpenCodeRecording: s.opencodeSpeech.recordingValue(s.text()),
	}
}

// sourceEvent is the producer the cues page edits. An unset field means the
// first item, which is also what the sidebar shows.
func (s *settings) sourceEvent() events.Source {
	if events.Source(s.source) == events.OpenCode {
		return events.OpenCode
	}
	return events.Codex
}

// displayFor and soundFor point at the selected producer's controls.
func (s *settings) displayFor(source events.Source) *displaySettings {
	if source == events.OpenCode {
		return &s.opencodeDisplay
	}
	return &s.codexDisplay
}

func (s *settings) soundFor(source events.Source) *soundSettings {
	if source == events.OpenCode {
		return &s.opencodeSound
	}
	return &s.codexSound
}

// speechOn and speechText point at the selected producer's announcement.
func (s *settings) speechOn(source events.Source) *bool {
	if source == events.OpenCode {
		return &s.speechOpenCode
	}
	return &s.speechCodex
}

func (s *settings) speechText(source events.Source) *string {
	if source == events.OpenCode {
		return &s.speechTextOpenCode
	}
	return &s.speechTextCodex
}

// speechFor points at the selected producer's announcement mode and clip.
func (s *settings) speechFor(source events.Source) *speechSettings {
	if source == events.OpenCode {
		return &s.opencodeSpeech
	}
	return &s.codexSpeech
}

// status is the line under the title, and only when there is something to say:
// while the app simply waits, the page says nothing.
func (s *settings) status(t wording, now time.Time) string {
	switch {
	case !s.enabled:
		return t.Disabled
	case !s.problem.empty():
		return s.problem.text(t)
	case s.paused:
		return t.Paused
	case s.quietAt(now):
		return t.Quiet
	}
	return ""
}

func (s *settings) view(c *ui.Context) {
	p := paletteFor(c.Theme())
	c.SetTheme(p.theme)
	t := s.text()
	now := time.Now()
	if s.source == "" {
		s.source = string(events.Codex)
	}
	c.OnShortcut(ui.Cmd, ui.KeyW, func() {
		if s.onClose != nil {
			s.onClose()
		}
	})
	ui.Column(c).Fill().Background(p.page).Children(func() {
		c.Key("settings-tabs")
		ui.Row(c).FillWidth().Height(48).Padding(0, 28).AlignItems(ui.Center).
			Background(p.card).Children(func() {
			ui.Tabs(c, &s.tab, t.General, t.TabCues, t.TabAbout)
		})
		p.hairline(c)
		switch s.tab {
		case tabCues:
			s.cuesPage(c, p, t)
		case tabAbout:
			s.aboutPage(c, p, t)
		default:
			s.generalPage(c, p, t, now)
		}
	})
}

// generalPage holds the application-wide switches, the pause, the quiet hours
// and the two integrations: one group per subject.
func (s *settings) generalPage(c *ui.Context, p palette, t wording, now time.Time) {
	p.pageFrame(c, func() {
		p.pageTitle(c, t.General)
		if status := s.status(t, now); status != "" {
			ui.Box(c).Height(4)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(p.accent)
				ui.Text(c, status).FontSize(12).TextColor(p.muted)
			})
		}
		if !s.saveProblem.empty() {
			p.note(c, s.saveProblem.text(t))
		}
		ui.Box(c).Height(12)

		p.group(c, "", func() {
			p.row(c, t.Master, "", func() {
				ui.Switch(c, &s.enabled).Label(t.Master).OnChange(func() {
					if s.onToggleEnabled != nil {
						s.onToggleEnabled()
					}
				})
			})
			p.rule(c)
			p.row(c, t.Autostart, "", func() {
				ui.Switch(c, &s.openAtLogin).Label(t.Autostart).OnChange(func() {
					if s.onToggleOpenAtLogin != nil {
						s.onToggleOpenAtLogin()
					}
				})
			})
			p.rule(c)
			p.row(c, t.Language, "", func() {
				ui.Select(c, &s.languageLabel, s.languageOptions()).Label(t.Language).
					Width(200).OnChange(func() {
					if s.onChangeLanguage != nil {
						s.onChangeLanguage()
					}
				})
			})
		})
		if !s.autostartProblem.empty() {
			p.footnote(c, s.autostartProblem.text(t))
		}

		ui.Box(c).Height(12)
		p.group(c, "", func() {
			ui.Row(c).FillWidth().Padding(11, 14).Wrap().Gap(8).Children(func() {
				label := t.PauseCues
				if s.paused {
					label = t.Resume
				}
				if ui.Button(c, label).Clicked() && s.onTogglePause != nil {
					s.onTogglePause()
				}
				for _, choice := range []struct {
					label string
					span  time.Duration
				}{
					{t.Pause15, 15 * time.Minute},
					{t.Pause1h, time.Hour},
					{t.Pause4h, 4 * time.Hour},
				} {
					span := choice.span
					if ui.Button(c, choice.label).Clicked() && s.onPauseFor != nil {
						s.onPauseFor(span)
					}
				}
			})
		})
		if s.paused && s.pauseRemaining > 0 {
			p.footnote(c, fmt.Sprintf(t.PausedFor, t.shortDuration(s.pauseRemaining)))
		}

		ui.Box(c).Height(12)
		p.group(c, "", func() {
			p.row(c, t.QuietTitle, "", func() {
				ui.Switch(c, &s.quietEnabled).Label(t.QuietTitle).OnChange(func() {
					if s.onChangeQuiet != nil {
						s.onChangeQuiet()
					}
				})
			})
			p.rule(c)
			p.row(c, "", "", func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, t.QuietFrom).FontSize(12).TextColor(p.muted)
					ui.Select(c, &s.quietStart, clockOptions()).Label(t.QuietFrom).
						Width(104).OnChange(func() {
						if s.onChangeQuiet != nil {
							s.onChangeQuiet()
						}
					})
					ui.Text(c, t.QuietTo).FontSize(12).TextColor(p.faint)
					ui.Select(c, &s.quietEnd, clockOptions()).Label(t.QuietTo).
						Width(104).OnChange(func() {
						if s.onChangeQuiet != nil {
							s.onChangeQuiet()
						}
					})
				})
			})
		})

		ui.Box(c).Height(12)
		s.integrationSection(c, p, t)
	})
}

// cuesPage edits one producer's glow or sound. The producer is a picker at the
// top of the page rather than a rail beside it, so the settings keep the whole
// width and the two producers stay one click apart.
func (s *settings) cuesPage(c *ui.Context, p palette, t wording) {
	// The picker binds an index; source is what the rest of the panel reads.
	if s.sourceEvent() == events.OpenCode {
		s.sourcePick = 1
	} else {
		s.sourcePick = 0
	}
	p.pageFrame(c, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(16).Children(func() {
			p.pageTitle(c, t.TabCues).Grow(1)
			c.Key("cue-source")
			p.segmented(c, &s.sourcePick, t.Sources, 200, true, func() {
				if s.sourcePick == 1 {
					s.source = string(events.OpenCode)
				} else {
					s.source = string(events.Codex)
				}
			}, events.Codex.Label(), events.OpenCode.Label())
		})
		ui.Box(c).Height(14)
		c.Key("cue-pane")
		p.segmented(c, &s.pane, "", 240, true, nil, t.Glow, t.SoundPane)
		ui.Box(c).Height(12)
		source := s.sourceEvent()
		if s.pane == paneSound {
			c.Key("sound-pane")
			s.soundPane(c, p, t, source)
		} else {
			c.Key("glow-pane")
			s.glowPane(c, p, t, source)
		}
	})
}

// glowPane is the selected producer's appearance. During a preview the note
// says the integration is not part of what is tested.
func (s *settings) glowPane(c *ui.Context, p palette, t wording, source events.Source) {
	display := s.displayFor(source)
	changed := func() {
		if s.onChangeDisplay != nil {
			s.onChangeDisplay(source)
		}
	}
	flash := display.animation == animationFlash

	p.group(c, "", func() {
		p.row(c, t.EnableGlow, "", func() {
			ui.Switch(c, &display.enabled).Label(t.EnableGlow).OnChange(changed)
		})
		p.rule(c)
		c.Key("glow-animation")
		p.row(c, t.Effect, "", func() {
			// A disabled glow has nothing to preview or to choose about how
			// it looks, so the Mac greys the rest of the page; so do we.
			p.segmented(c, &display.animation, t.Effect, 300, false, changed,
				t.Breathing, t.Flash).Disabled(!display.enabled)
		})
		// The colour is the effect's own, so it sits right under it rather
		// than in a section of its own: 单色呼吸 takes one, 双色交替闪烁 two.
		p.rule(c)
		if flash {
			p.row(c, t.MainColour, "", func() {
				s.colourRow(c, p, &display.primary, t.MainColour, source,
					display.enabled)
			})
			p.rule(c)
			p.row(c, t.SecondaryColourLabel, "", func() {
				s.colourRow(c, p, &display.secondary, t.SecondaryColourLabel,
					source, display.enabled)
			})
		} else {
			p.row(c, t.Colour, "", func() {
				s.colourRow(c, p, &display.colour, t.Colour, source,
					display.enabled)
			})
		}
	})

	ui.Box(c).Height(16)
	ui.Row(c).Children(func() {
		// The button stands on the page, not in a row, so it sets its own
		// height; otherwise it collapses to its text.
		if ui.Button(c, t.TestGlow).Height(32).Padding(0, 14).
			Disabled(!display.enabled).Clicked() && s.onPreviewGlow != nil {
			s.onPreviewGlow(source)
		}
	})
}

// colourRow is a colour well and the hex it holds. The well opens MyGo's
// picker, where the colour is chosen; the hex repeats the stored value, so two
// swatches that look alike can still be told apart.
func (s *settings) colourRow(c *ui.Context, p palette, value *ui.Color, label string, source events.Source, enabled bool) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.ColorWell(c, value).Label(label).Disabled(!enabled).OnChange(func() {
			// The glow is opaque; the picker's opacity slider must not leave
			// an alpha this app would silently drop when it saves.
			value.A = 255
			if s.onChangeDisplay != nil {
				s.onChangeDisplay(source)
			}
		})
		hex := ui.Text(c, prefs.FormatColour(nrgba(*value))).FontSize(13).
			TextColor(p.ink)
		hex.Disabled(!enabled)
	})
}

// soundPane is the selected producer's sound and announcement.
func (s *settings) soundPane(c *ui.Context, p palette, t wording, source events.Source) {
	controls := s.soundFor(source)
	changed := func() {
		if s.onChangeSound != nil {
			s.onChangeSound(source)
		}
	}

	p.group(c, t.NotificationSound, func() {
		p.row(c, t.EnableSound, "", func() {
			ui.Switch(c, &controls.enabled).Label(t.EnableSound).OnChange(func() {
				controls.setEnabled()
				changed()
			})
		})
		p.rule(c)
		p.row(c, t.Sound, "", func() {
			ui.Select(c, &controls.soundLabel, t.soundLabels()).Label(t.Sound).
				Width(200).Disabled(!controls.enabled).OnChange(func() {
				controls.setSound(t)
				changed()
			})
			if ui.Button(c, t.TestSound).Disabled(!controls.enabled).Clicked() &&
				s.onPreviewSound != nil {
				s.onPreviewSound(source)
			}
		})
		if !s.soundProblem.empty() {
			p.rule(c)
			p.row(c, s.soundProblem.text(t), "", func() {})
		}
	})

	if s.speechLoaded {
		ui.Box(c).Height(12)
		s.speechPane(c, p, t, source)
	}
}

// speechPane edits the selected producer's announcement: a bundled clip, or
// the stored phrase read by a system voice. No event content reaches either.
func (s *settings) speechPane(c *ui.Context, p palette, t wording, source events.Source) {
	speech := s.speechFor(source)
	on := *s.speechOn(source)
	changed := func() {
		if s.onChangeSpeech != nil {
			s.onChangeSpeech()
		}
	}
	speak := func() {
		if s.onPreviewSpeech != nil {
			s.onPreviewSpeech(source)
		}
	}

	p.group(c, t.Speech, func() {
		p.row(c, t.SpeechEnable, "", func() {
			ui.Switch(c, s.speechOn(source)).Label(t.SpeechEnable).OnChange(changed)
		})
		p.rule(c)
		c.Key("speech-mode")
		p.row(c, t.Mode, "", func() {
			p.segmented(c, &speech.mode, t.Mode, 240, false, changed,
				t.SpeechModeRecording, t.SpeechModeSystem).Disabled(!on)
		})
		p.rule(c)
		if speech.mode == speechRecording {
			p.row(c, t.SpeechRecording, "", func() {
				ui.Select(c, &speech.recording, t.recordingLabels()).
					Label(t.SpeechRecording).Width(200).Disabled(!on).
					OnChange(changed)
				if ui.Button(c, t.TestSpeech).Disabled(!on).Clicked() {
					speak()
				}
			})
		} else {
			p.row(c, t.SpeechVoice, "", func() {
				ui.Select(c, &s.speechVoiceLabel, s.voiceOptions()).
					Label(t.SpeechVoice).Width(200).Disabled(!on).
					OnChange(changed)
			})
			p.rule(c)
			p.row(c, t.SpeechTextLabel, "", func() {
				ui.TextInput(c, s.speechText(source)).Label(t.SpeechText).
					Width(240).Disabled(!on).OnChange(changed)
				if ui.Button(c, t.TestSpeech).Disabled(!on).Clicked() {
					speak()
				}
			})
		}
	})
	if speech.mode == speechSystem && s.speechLoaded && len(s.speechVoices) == 0 {
		p.footnote(c, t.SpeechNoVoice)
	}
	if !s.speechProblem.empty() {
		p.footnote(c, s.speechProblem.text(t))
	}
}

// aboutPage names the product, its version and what it does.
func (s *settings) aboutPage(c *ui.Context, p palette, t wording) {
	// Nothing here scrolls: the whole page is one mark. The trailing spacer is
	// the tab bar's height, so the mark centres in the window rather than in
	// the page area under the tabs.
	ui.Column(c).Grow(1).FillWidth().Background(p.page).Justify(ui.Center).
		AlignItems(ui.Center).Padding(40, 0).Children(func() {
		// Wide enough for the one-line description at 13px, so it wraps on a
		// comma rather than mid-phrase.
		ui.Column(c).Width(520).AlignItems(ui.Center).Children(func() {
			if icon := aboutIcon(); icon != nil {
				ui.Image(c, icon).Size(64, 64)
			}
			ui.Box(c).Height(20)
			ui.Text(c, t.Product).FontSize(28).FontWeight(700).TextColor(p.ink).
				TextAlign(ui.Center)
			ui.Box(c).Height(2)
			ui.Text(c, s.versionLine(t)).FontSize(12).TextColor(p.muted).
				TextAlign(ui.Center)
			ui.Box(c).Height(18)
			ui.Text(c, t.AboutTagline).FontSize(13).TextColor(p.ink).
				TextAlign(ui.Center)
			if s.versionKnown() {
				ui.Box(c).Height(24)
				ui.Column(c).AlignItems(ui.Center).Children(func() {
					button := ui.Button(c, s.updateLabel(t)).Height(32).
						Padding(0, 14)
					button.Disabled(!s.updateEnabled())
					if button.Clicked() {
						if s.updateState == updateAvailable {
							if s.onInstallUpdate != nil {
								s.onInstallUpdate()
							}
						} else if s.onCheckUpdate != nil {
							s.onCheckUpdate()
						}
					}
					if hint := s.updateHint(t); hint != "" {
						ui.Box(c).Height(10)
						ui.Text(c, hint).FontSize(12).TextColor(p.faint).
							TextAlign(ui.Center)
					}
				})
			}
		})
		ui.Box(c).Height(48)
	})
}

// versionLine is the product version the packaged app carries, or a note for
// a development build.
func (s *settings) versionLine(t wording) string {
	if s.version == "" {
		return t.AboutDev
	}
	return fmt.Sprintf(t.AboutVersion, s.version)
}

// integrationSection is one group: each producer, what was found, and the
// button that installs or removes it.
func (s *settings) integrationSection(c *ui.Context, p palette, t wording) {
	p.group(c, t.Integrations, func() {
		p.row(c, t.CodexTitle, s.codexDetail(), func() {
			label := t.EnableCodex
			if s.codexInstalled {
				label = t.DisableCodex
			}
			if ui.Button(c, label).Clicked() && s.onToggleCodex != nil {
				s.onToggleCodex()
			}
		})
		p.rule(c)
		p.row(c, t.OpenCodeTitle, s.openCodeDetail(), func() {
			label := t.EnableOpen
			if s.opencodeInstalled {
				label = t.DisableOpen
			}
			if ui.Button(c, label).Clicked() && s.onToggleOpenCode != nil {
				s.onToggleOpenCode()
			}
		})
	})
}

// codexDetail is the line under the Codex button: what is installed, or why it
// could not be.
func (s *settings) codexDetail() string {
	t := s.text()
	switch {
	case !s.codexProblem.empty():
		return s.codexProblem.text(t)
	case s.codexInstalled && !s.codexDetected:
		return t.CodexNoBinary
	case s.codexInstalled:
		return t.CodexInstalled
	case !s.codexDetected:
		return t.CodexNotDetected
	default:
		return t.CodexNotInstalled
	}
}

// openCodeDetail is the line under the OpenCode button. A freshly installed
// plugin only takes effect the next time OpenCode starts, which the line says.
func (s *settings) openCodeDetail() string {
	t := s.text()
	switch {
	case !s.opencodeProblem.empty():
		return s.opencodeProblem.text(t)
	case s.opencodeInstalled:
		return t.OpenInstalled
	case s.opencodeVersion == "":
		return t.OpenNotDetected
	default:
		return fmt.Sprintf(t.OpenNotInstalled, s.opencodeVersion)
	}
}

// shortDuration writes a pause's remainder in minutes, or hours and minutes
// when it is long, in the panel's language.
func (t wording) shortDuration(d time.Duration) string {
	if d < time.Minute {
		d = time.Minute
	}
	minutes := int(d / time.Minute)
	if minutes < 60 {
		return fmt.Sprintf(t.SlowMinutes, minutes)
	}
	return fmt.Sprintf(t.SlowHours, minutes/60, minutes%60)
}
