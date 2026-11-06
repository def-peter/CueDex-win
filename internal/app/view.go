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
	"github.com/def-peter/cuedex-win/internal/sound"
	"github.com/def-peter/cuedex-win/internal/speech"
)

// The animation picker's indexes.
const (
	animationBreathing = iota
	animationFlash
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

	cueCount   int
	lastCue    time.Time
	lastSource events.Source
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

	// diag is the last section's snapshot of both integrations and the event
	// stream.
	diag diagnostic

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

	onChangeSound  func(events.Source)
	onPreviewSound func(events.Source)
	onChangeSpeech func()
}

// text is the panel's words in the current language.
func (s *settings) text() wording {
	if s.language == "" {
		return chinese
	}
	return copyFor(s.language)
}

// displaySettings is one producer's appearance controls. The colour is a label
// because a Select binds to a string; apply turns the saved hex into the label
// and choice turns it back.
type displaySettings struct {
	enabled   bool
	animation int
	colour    string
}

func (d *displaySettings) apply(p prefs.DisplayPreferences, t wording) {
	d.enabled = p.Enabled
	d.animation = animationBreathing
	if p.Animation == overlay.AlternatingFlash {
		d.animation = animationFlash
	}
	d.colour = colourLabel(p.Colour(), t)
}

func (d *displaySettings) choice(t wording) prefs.DisplayPreferences {
	animation := overlay.Breathing
	if d.animation == animationFlash {
		animation = overlay.AlternatingFlash
	}
	return prefs.DisplayPreferences{
		Enabled:   d.enabled,
		Animation: animation,
		Color:     colourValue(d.colour, t),
	}
}

func colourLabel(c color.NRGBA, t wording) string {
	hex := prefs.FormatColour(c)
	labels := t.colourLabels()
	for i, known := range prefs.Palette {
		if known == hex {
			return labels[i]
		}
	}
	return labels[0]
}

func colourValue(label string, t wording) string {
	for i, known := range t.colourLabels() {
		if known == label && i < len(prefs.Palette) {
			return prefs.Palette[i]
		}
	}
	return prefs.Palette[0]
}

// applyDisplay fills the two appearance blocks from the saved preferences.
//
// It is re-run when the language changes, because the colour labels differ.
func (s *settings) applyDisplay(p prefs.Preferences) {
	t := s.text()
	s.codexDisplay.apply(p.Display.Codex, t)
	s.opencodeDisplay.apply(p.Display.OpenCode, t)
}

// displayChoice is what the panel currently says for one producer.
func (s *settings) displayChoice(source events.Source) prefs.DisplayPreferences {
	t := s.text()
	if source == events.OpenCode {
		return s.opencodeDisplay.choice(t)
	}
	return s.codexDisplay.choice(t)
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
	sound  int     // index into sound.All()
	volume float64 // 0-100
}

func (s *soundSettings) apply(choice prefs.SoundChoice) {
	s.sound = soundIndex(choice.ID)
	s.volume = float64(choice.Volume)
}

func (s *soundSettings) choice() prefs.SoundChoice {
	all := sound.All()
	index := s.sound
	if index < 0 || index >= len(all) {
		index = 0
	}
	volume := int(math.Round(s.volume))
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	return prefs.SoundChoice{ID: all[index].ID, Volume: volume}
}

// applySound fills the panel from the saved choices.
func (s *settings) applySound(p prefs.Preferences) {
	s.codexSound.apply(p.Sound.Codex)
	s.opencodeSound.apply(p.Sound.OpenCode)
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
		Voice:        s.selectedVoiceID(),
		Codex:        s.speechCodex,
		OpenCode:     s.speechOpenCode,
		ReplaceSound: s.speechReplace,
	}
}

// status is the line under the title: what the app is doing right now.
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
	case s.cueCount > 0:
		source := ""
		if s.lastSource != "" {
			source = s.lastSource.Label()
		}
		return t.lastCue(s.lastCue, source, s.cueCount)
	}
	return t.Idle
}

func (s *settings) view(c *ui.Context) {
	theme := c.Theme()
	t := s.text()
	now := time.Now()
	c.OnShortcut(ui.Cmd, ui.KeyW, func() {
		if s.onClose != nil {
			s.onClose()
		}
	})
	ui.Column(c).Fill().Padding(20).Gap(10).Children(func() {
		ui.Text(c, "CueDex").FontSize(22).Bold()
		ui.Text(c, s.status(t, now)).FontSize(12).TextColor(theme.TextMuted)
		if !s.saveProblem.empty() {
			ui.Text(c, s.saveProblem.text(t)).FontSize(12).TextColor(theme.TextMuted)
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				s.generalSection(c, theme, t)
				ui.Divider(c)
				s.appearanceSection(c, theme, t)
				ui.Divider(c)
				s.soundSection(c, theme, t)
				if s.speechLoaded && len(s.speechVoices) > 0 {
					ui.Divider(c)
					s.speechSection(c, theme, t)
				}
				ui.Divider(c)
				s.integrationSection(c, theme, t)
				ui.Divider(c)
				s.diagnosticSection(c, theme, t)
			})
		})
		ui.Row(c).Gap(8).Children(func() {
			label := t.PauseCues
			if s.paused {
				label = t.Resume
			}
			if ui.Button(c, label).Clicked() && s.onTogglePause != nil {
				s.onTogglePause()
			}
			ui.Spacer(c)
			if ui.Button(c, t.Close).Clicked() && s.onClose != nil {
				s.onClose()
			}
		})
	})
}

func (s *settings) generalSection(c *ui.Context, theme *ui.Theme, t wording) {
	ui.Text(c, t.General).FontSize(14).Bold()
	ui.Row(c).Gap(8).Children(func() {
		ui.Switch(c, &s.enabled).OnChange(func() {
			if s.onToggleEnabled != nil {
				s.onToggleEnabled()
			}
		})
		ui.Text(c, t.Master).FontSize(12)
		ui.Spacer(c)
	})
	ui.Text(c, t.MasterHint).FontSize(12).TextColor(theme.TextMuted)

	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, t.Language).FontSize(12).Width(64)
		ui.Select(c, &s.languageLabel, s.languageOptions()).Grow(1).OnChange(func() {
			if s.onChangeLanguage != nil {
				s.onChangeLanguage()
			}
		})
	})

	ui.Text(c, t.Pause).FontSize(12)
	ui.Row(c).Gap(8).Children(func() {
		for _, choice := range []struct {
			label string
			span  time.Duration
		}{
			{t.Pause15, 15 * time.Minute},
			{t.Pause1h, time.Hour},
			{t.Pause4h, 4 * time.Hour},
			{t.PauseUntil, 0},
		} {
			span := choice.span
			if ui.Button(c, choice.label).Clicked() && s.onPauseFor != nil {
				s.onPauseFor(span)
			}
		}
		ui.Spacer(c)
	})
	if s.paused && s.pauseRemaining > 0 {
		ui.Text(c, fmt.Sprintf(t.PausedFor, shortDuration(s.pauseRemaining))).FontSize(12).TextColor(theme.TextMuted)
	}

	ui.Row(c).Gap(8).Children(func() {
		ui.Switch(c, &s.quietEnabled).OnChange(func() {
			if s.onChangeQuiet != nil {
				s.onChangeQuiet()
			}
		})
		ui.Text(c, t.QuietTitle).FontSize(12)
		ui.Spacer(c)
	})
	ui.Text(c, t.QuietHint).FontSize(12).TextColor(theme.TextMuted)
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, t.QuietFrom).FontSize(12).Width(32)
		ui.Select(c, &s.quietStart, clockOptions()).Grow(1).OnChange(func() {
			if s.onChangeQuiet != nil {
				s.onChangeQuiet()
			}
		})
		ui.Text(c, t.QuietTo).FontSize(12).Width(32)
		ui.Select(c, &s.quietEnd, clockOptions()).Grow(1).OnChange(func() {
			if s.onChangeQuiet != nil {
				s.onChangeQuiet()
			}
		})
	})

	ui.Row(c).Gap(8).Children(func() {
		ui.Switch(c, &s.openAtLogin).OnChange(func() {
			if s.onToggleOpenAtLogin != nil {
				s.onToggleOpenAtLogin()
			}
		})
		ui.Text(c, t.Autostart).FontSize(12)
		ui.Spacer(c)
	})
	ui.Text(c, t.AutostartHint).FontSize(12).TextColor(theme.TextMuted)
	if !s.autostartProblem.empty() {
		ui.Text(c, s.autostartProblem.text(t)).FontSize(12).TextColor(theme.TextMuted)
	}
}

func (s *settings) appearanceSection(c *ui.Context, theme *ui.Theme, t wording) {
	ui.Text(c, t.Appearance).FontSize(14).Bold()
	s.appearanceControls(c, theme, t, events.Codex, events.Codex.Label(), &s.codexDisplay)
	s.appearanceControls(c, theme, t, events.OpenCode, events.OpenCode.Label(), &s.opencodeDisplay)
}

func (s *settings) appearanceControls(c *ui.Context, theme *ui.Theme, t wording, source events.Source, label string, controls *displaySettings) {
	ui.Row(c).Gap(8).Children(func() {
		ui.Switch(c, &controls.enabled).OnChange(func() {
			if s.onChangeDisplay != nil {
				s.onChangeDisplay(source)
			}
		})
		ui.Text(c, label+" · "+t.SourceOn).FontSize(12)
		ui.Spacer(c)
	})
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, t.Animation).FontSize(12).Width(64)
		ui.Segmented(c, &controls.animation, t.Breathing, t.Flash).OnChange(func() {
			if s.onChangeDisplay != nil {
				s.onChangeDisplay(source)
			}
		})
		ui.Spacer(c)
	})
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, t.Colour).FontSize(12).Width(64)
		ui.Select(c, &controls.colour, t.colourLabels()).Grow(1).OnChange(func() {
			if s.onChangeDisplay != nil {
				s.onChangeDisplay(source)
			}
		})
	})
}

func (s *settings) soundSection(c *ui.Context, theme *ui.Theme, t wording) {
	ui.Text(c, t.Sound).FontSize(14).Bold()
	ui.Text(c, t.SoundHint).FontSize(12).TextColor(theme.TextMuted)
	s.soundControls(c, t, events.Codex, events.Codex.Label(), &s.codexSound)
	s.soundControls(c, t, events.OpenCode, events.OpenCode.Label(), &s.opencodeSound)
	if !s.soundProblem.empty() {
		ui.Text(c, s.soundProblem.text(t)).FontSize(12).TextColor(theme.TextMuted)
	}
}

// soundControls is one producer's two rows: the sound itself, then its volume.
// The preview plays the choice right away, without a completion.
func (s *settings) soundControls(c *ui.Context, t wording, source events.Source, label string, controls *soundSettings) {
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, label).FontSize(12).Width(64)
		ui.Segmented(c, &controls.sound, t.soundLabels()...).OnChange(func() {
			if s.onChangeSound != nil {
				s.onChangeSound(source)
			}
		})
		ui.Spacer(c)
		ui.Button(c, t.Preview).OnClick(func() {
			if s.onPreviewSound != nil {
				s.onPreviewSound(source)
			}
		})
	})
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, t.Volume).FontSize(12).Width(64)
		ui.Slider(c, &controls.volume, 0, 100).Grow(1).OnChange(func() {
			if s.onChangeSound != nil {
				s.onChangeSound(source)
			}
		})
		ui.Text(c, fmt.Sprintf("%d%%", int(math.Round(controls.volume)))).FontSize(12).Width(44)
	})
}

func (s *settings) speechSection(c *ui.Context, theme *ui.Theme, t wording) {
	ui.Text(c, t.Speech).FontSize(14).Bold()
	ui.Text(c, t.SpeechHint).FontSize(12).TextColor(theme.TextMuted)
	ui.Row(c).Gap(16).Children(func() {
		ui.Checkbox(c, &s.speechCodex, t.SpeakCodex).OnChange(func() {
			if s.onChangeSpeech != nil {
				s.onChangeSpeech()
			}
		})
		ui.Checkbox(c, &s.speechOpenCode, t.SpeakOpenCode).OnChange(func() {
			if s.onChangeSpeech != nil {
				s.onChangeSpeech()
			}
		})
		ui.Spacer(c)
	})
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, t.SpeechVoice).FontSize(12).Width(64)
		ui.Select(c, &s.speechVoiceLabel, s.voiceOptions()).Grow(1).OnChange(func() {
			if s.onChangeSpeech != nil {
				s.onChangeSpeech()
			}
		})
	})
	ui.Checkbox(c, &s.speechReplace, t.SpeechReplace).OnChange(func() {
		if s.onChangeSpeech != nil {
			s.onChangeSpeech()
		}
	})
	ui.Text(c, t.SpeechBack).FontSize(12).TextColor(theme.TextMuted)
	if !s.speechProblem.empty() {
		ui.Text(c, s.speechProblem.text(t)).FontSize(12).TextColor(theme.TextMuted)
	}
}

func (s *settings) integrationSection(c *ui.Context, theme *ui.Theme, t wording) {
	ui.Text(c, t.Integrations).FontSize(14).Bold()
	ui.Text(c, t.CodexTitle).FontSize(14).Bold()
	ui.Text(c, s.codexDetail()).FontSize(12).TextColor(theme.TextMuted)
	ui.Row(c).Gap(8).Children(func() {
		label := t.EnableCodex
		if s.codexInstalled {
			label = t.DisableCodex
		}
		if ui.Button(c, label).Clicked() && s.onToggleCodex != nil {
			s.onToggleCodex()
		}
		ui.Spacer(c)
	})
	ui.Text(c, t.OpenCodeTitle).FontSize(14).Bold()
	ui.Text(c, s.openCodeDetail()).FontSize(12).TextColor(theme.TextMuted)
	ui.Row(c).Gap(8).Children(func() {
		label := t.EnableOpen
		if s.opencodeInstalled {
			label = t.DisableOpen
		}
		if ui.Button(c, label).Clicked() && s.onToggleOpenCode != nil {
			s.onToggleOpenCode()
		}
		ui.Spacer(c)
	})
}

// diagnosticSection says what the app found, so a failing install has somewhere
// to be looked at after the notification is gone.
func (s *settings) diagnosticSection(c *ui.Context, theme *ui.Theme, t wording) {
	ui.Text(c, t.Diagnostics).FontSize(14).Bold()
	for _, line := range []string{
		s.diag.versionLine(t),
		s.diag.pluginLine(t),
		s.diag.codexLine(t),
		s.diag.lastEventLine(t),
	} {
		ui.Text(c, line).FontSize(12).TextColor(theme.TextMuted)
	}
	ui.Text(c, t.DiagHint).FontSize(12).TextColor(theme.TextMuted)
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
// when it is long, so a countdown reads the same in both languages.
func shortDuration(d time.Duration) string {
	if d < time.Minute {
		d = time.Minute
	}
	minutes := int(d / time.Minute)
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	return fmt.Sprintf("%d h %02d min", minutes/60, minutes%60)
}
