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

	onTogglePause func()
	onClose       func()
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
