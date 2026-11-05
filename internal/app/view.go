package app

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/prefs"
	"github.com/def-peter/cuedex-win/internal/sound"
)

// settings is the state the panel shows and the two actions it offers. It has
// no window of its own, so a tester can drive it without one.
type settings struct {
	paused     bool
	cueCount   int
	lastCue    time.Time
	lastSource events.Source
	// problem is why cues cannot arrive, empty when they can.
	problem string

	codexInstalled bool
	codexDetected  bool
	codexProblem   string

	opencodeInstalled bool
	opencodeVersion   string
	opencodeProblem   string

	codexSound    soundSettings
	opencodeSound soundSettings
	soundProblem  string

	onTogglePause func()
	onClose       func()
	onToggleCodex func()

	onToggleOpenCode func()
	onChangeSound    func(events.Source)
	onPreviewSound   func(events.Source)
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

func (s *settings) view(c *ui.Context) {
	t := c.Theme()
	c.OnShortcut(ui.Cmd, ui.KeyW, func() {
		if s.onClose != nil {
			s.onClose()
		}
	})
	ui.Column(c).Fill().Padding(20).Gap(10).Children(func() {
		ui.Text(c, "CueDex").FontSize(22).Bold()
		status := "等待 Codex / OpenCode 的完成事件"
		switch {
		case s.paused:
			status = "已暂停：完成事件不会触发提示"
		case s.problem != "":
			status = s.problem
		case s.cueCount > 0:
			from := ""
			if s.lastSource != "" {
				from = "（" + s.lastSource.Label() + "）"
			}
			status = fmt.Sprintf("最近一次唤醒 %s%s，共 %d 次", s.lastCue.Format("15:04:05"), from, s.cueCount)
		}
		ui.Text(c, status).FontSize(12).TextColor(t.TextMuted)
		ui.Spacer(c)
		ui.Text(c, "提示音").FontSize(14).Bold()
		ui.Text(c, "每条提示最多响一次；音量 0 等同静音，不影响发光").FontSize(12).TextColor(t.TextMuted)
		s.soundControls(c, events.Codex, "Codex", &s.codexSound)
		s.soundControls(c, events.OpenCode, "OpenCode", &s.opencodeSound)
		if s.soundProblem != "" {
			ui.Text(c, s.soundProblem).FontSize(12).TextColor(t.TextMuted)
		}
		ui.Spacer(c)
		ui.Text(c, "Codex 集成").FontSize(14).Bold()
		ui.Text(c, s.codexDetail()).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			label := "启用 Codex 集成"
			if s.codexInstalled {
				label = "停用 Codex 集成"
			}
			if ui.Button(c, label).Clicked() && s.onToggleCodex != nil {
				s.onToggleCodex()
			}
			ui.Spacer(c)
		})
		ui.Text(c, "OpenCode 集成").FontSize(14).Bold()
		ui.Text(c, s.openCodeDetail()).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			label := "启用 OpenCode 集成"
			if s.opencodeInstalled {
				label = "停用 OpenCode 集成"
			}
			if ui.Button(c, label).Clicked() && s.onToggleOpenCode != nil {
				s.onToggleOpenCode()
			}
			ui.Spacer(c)
		})
		ui.Spacer(c)
		ui.Row(c).Gap(8).Children(func() {
			label := "暂停提示"
			if s.paused {
				label = "恢复提示"
			}
			if ui.Button(c, label).Clicked() && s.onTogglePause != nil {
				s.onTogglePause()
			}
			ui.Spacer(c)
			if ui.Button(c, "关闭").Clicked() && s.onClose != nil {
				s.onClose()
			}
		})
	})
}

// soundControls is one producer's two rows: the sound itself, then its volume.
// The preview plays the choice right away, without a completion.
func (s *settings) soundControls(c *ui.Context, source events.Source, label string, controls *soundSettings) {
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, label).FontSize(12).Width(64)
		ui.Segmented(c, &controls.sound, sound.Labels()...).OnChange(func() {
			if s.onChangeSound != nil {
				s.onChangeSound(source)
			}
		})
		ui.Spacer(c)
		ui.Button(c, "试听").OnClick(func() {
			if s.onPreviewSound != nil {
				s.onPreviewSound(source)
			}
		})
	})
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, "音量").FontSize(12).Width(64)
		ui.Slider(c, &controls.volume, 0, 100).Grow(1).OnChange(func() {
			if s.onChangeSound != nil {
				s.onChangeSound(source)
			}
		})
		ui.Text(c, fmt.Sprintf("%d%%", int(math.Round(controls.volume)))).FontSize(12).Width(44)
	})
}

// codexDetail is the line under the Codex button: what is installed, or why it
// could not be.
func (s *settings) codexDetail() string {
	switch {
	case s.codexProblem != "":
		return s.codexProblem
	case s.codexInstalled && !s.codexDetected:
		return "钩子已安装，但没有找到 Codex；装好 Codex 后自动生效"
	case s.codexInstalled:
		return "已安装：Codex 跑完一轮回复时会提示"
	case !s.codexDetected:
		return "未安装，也没有找到 Codex"
	default:
		return "未安装：Codex 跑完一轮回复时不会提示"
	}
}

// openCodeDetail is the line under the OpenCode button. A freshly installed
// plugin only takes effect the next time OpenCode starts, which the line says.
func (s *settings) openCodeDetail() string {
	switch {
	case s.opencodeProblem != "":
		return s.opencodeProblem
	case s.opencodeInstalled:
		return "已安装：重启 OpenCode 后生效，主会话跑完一轮会提示"
	case s.opencodeVersion == "":
		return "未检测到 OpenCode"
	default:
		return "未安装（检测到 OpenCode " + s.opencodeVersion + "）"
	}
}
