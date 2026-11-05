package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/def-peter/cuedex-win/internal/events"
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

	onTogglePause func()
	onClose       func()
	onToggleCodex func()
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
