package app

import (
	"fmt"
	"time"

	"github.com/def-peter/cuedex-win/internal/i18n"
	"github.com/def-peter/cuedex-win/internal/prefs"
	"github.com/def-peter/cuedex-win/internal/sound"
)

// wording is every word the panel shows, in one language. The panel is small
// enough that a table of strings beats a lookup API: a missing string is a
// compile error, not a blank label.
type wording struct {
	Idle       string
	Disabled   string
	Paused     string
	Quiet      string
	LastCue    string // fmt: time, source suffix, count
	FromSource string // fmt: source label, " (OpenCode)"

	General     string
	Master      string
	MasterHint  string
	Language    string
	LangSystem  string
	LangChinese string
	LangEnglish string

	Pause      string
	Pause15    string
	Pause1h    string
	Pause4h    string
	PauseUntil string
	Resume     string
	PausedFor  string // fmt: duration

	QuietTitle string
	QuietHint  string
	QuietFrom  string
	QuietTo    string

	Autostart     string
	AutostartHint string

	Appearance string
	SourceOn   string
	Animation  string
	Breathing  string
	Flash      string
	Colour     string
	Colours    []string

	Sound     string
	SoundHint string
	Sounds    []string
	Preview   string
	Volume    string

	Speech        string
	SpeechHint    string
	SpeakCodex    string
	SpeakOpenCode string
	SpeechVoice   string
	SpeechReplace string
	SpeechBack    string
	VoiceDefault  string

	Integrations  string
	CodexTitle    string
	OpenCodeTitle string
	EnableCodex   string
	DisableCodex  string
	EnableOpen    string
	DisableOpen   string

	Close        string
	PauseCues    string
	OpenSettings string
	Quit         string

	CodexInstalled    string
	CodexNoBinary     string
	CodexNotInstalled string
	CodexNotDetected  string
	OpenInstalled     string
	OpenNotDetected   string
	OpenNotInstalled  string // fmt: version

	ProblemOverlay          string // fmt: detail
	ProblemWatch            string
	ProblemSound            string
	ProblemSpeech           string
	ProblemSave             string
	ProblemHooks            string
	ProblemCodex            string
	ProblemOpenNo           string
	ProblemOpenOld          string // fmt: version
	ProblemOpenOldNoVersion string
	ProblemOpenForeign      string
	ProblemOpenOther        string
	ProblemAutostart        string
}

var chinese = wording{
	Idle:       "等待 Codex / OpenCode 的完成事件",
	Disabled:   "提示已关闭",
	Paused:     "已暂停：完成事件不会触发提示",
	Quiet:      "免打扰时段内：完成事件不会触发提示",
	LastCue:    "最近一次唤醒 %s%s，共 %d 次",
	FromSource: "（%s）",

	General:     "通用",
	Master:      "启用提示",
	MasterHint:  "关闭后既不发光也不发声",
	Language:    "语言",
	LangSystem:  "跟随系统",
	LangChinese: "中文",
	LangEnglish: "English",

	Pause:      "暂停",
	Pause15:    "15 分钟",
	Pause1h:    "1 小时",
	Pause4h:    "4 小时",
	PauseUntil: "直到恢复",
	Resume:     "恢复提示",
	PausedFor:  "还剩 %s 自动恢复",

	QuietTitle: "免打扰",
	QuietHint:  "这段时间内不提示；起止相同表示全天",
	QuietFrom:  "从",
	QuietTo:    "到",

	Autostart:     "开机时启动",
	AutostartHint: "登录后静默运行，只出现在托盘",

	Appearance: "外观",
	SourceOn:   "显示该来源的提示",
	Animation:  "动画",
	Breathing:  "呼吸",
	Flash:      "交替闪烁",
	Colour:     "颜色",
	Colours:    []string{"青绿", "紫罗兰", "蓝", "粉", "琥珀", "绿", "珊瑚", "灰"},

	Sound:     "提示音",
	SoundHint: "每条提示最多响一次；音量 0 等同静音，不影响发光",
	Sounds:    []string{"上扬", "下沉", "清音", "双击"},
	Preview:   "试听",
	Volume:    "音量",

	Speech:        "语音播报",
	SpeechHint:    "只说固定短语（Codex 已完成 / OpenCode 已完成），不朗读任何回复内容",
	SpeakCodex:    "朗读 Codex",
	SpeakOpenCode: "朗读 OpenCode",
	SpeechVoice:   "声音",
	SpeechReplace: "用语音替代提示音",
	SpeechBack:    "没有可用语音时自动回到提示音",
	VoiceDefault:  "系统默认",

	Integrations:  "集成",
	CodexTitle:    "Codex 集成",
	OpenCodeTitle: "OpenCode 集成",
	EnableCodex:   "启用 Codex 集成",
	DisableCodex:  "停用 Codex 集成",
	EnableOpen:    "启用 OpenCode 集成",
	DisableOpen:   "停用 OpenCode 集成",

	Close:        "关闭",
	PauseCues:    "暂停提示",
	OpenSettings: "打开设置",
	Quit:         "退出",

	CodexInstalled:    "已安装：Codex 跑完一轮回复时会提示",
	CodexNoBinary:     "钩子已安装，但没有找到 Codex；装好 Codex 后自动生效",
	CodexNotInstalled: "未安装：Codex 跑完一轮回复时不会提示",
	CodexNotDetected:  "未安装，也没有找到 Codex",
	OpenInstalled:     "已安装：重启 OpenCode 后生效，主会话跑完一轮会提示",
	OpenNotDetected:   "未检测到 OpenCode",
	OpenNotInstalled:  "未安装（检测到 OpenCode %s）",

	ProblemOverlay:          "提示层不可用：%s",
	ProblemWatch:            "事件目录不可用：%s",
	ProblemSound:            "提示音播放失败：%s",
	ProblemSpeech:           "语音播报失败：%s",
	ProblemSave:             "偏好保存失败：%s",
	ProblemHooks:            "hooks.json 结构异常，已停止写入，你的配置没有被改动",
	ProblemCodex:            "Codex 集成操作失败：%s",
	ProblemOpenNo:           "没有检测到 OpenCode，未安装插件",
	ProblemOpenOld:          "OpenCode %s 不在支持范围（需要 2.1.1 及以上、3.0.0 以下），未安装插件",
	ProblemOpenOldNoVersion: "OpenCode 不在支持范围，未安装插件",
	ProblemOpenForeign:      `plugins\cuedex.js 不是 CueDex 写的，已停止操作，不会覆盖或删除`,
	ProblemOpenOther:        "OpenCode 集成操作失败：%s",
	ProblemAutostart:        "开机自启设置失败：%s",
}

var english = wording{
	Idle:       "Waiting for a Codex or OpenCode turn to finish",
	Disabled:   "Cues are off",
	Paused:     "Paused: completions will not cue",
	Quiet:      "Quiet hours: completions will not cue",
	LastCue:    "Last cue at %s%s, %d in total",
	FromSource: " (%s)",

	General:     "General",
	Master:      "Cue on completion",
	MasterHint:  "Off means no glow and no sound",
	Language:    "Language",
	LangSystem:  "Follow Windows",
	LangChinese: "中文",
	LangEnglish: "English",

	Pause:      "Pause",
	Pause15:    "15 minutes",
	Pause1h:    "1 hour",
	Pause4h:    "4 hours",
	PauseUntil: "Until resumed",
	Resume:     "Resume cues",
	PausedFor:  "Resumes in %s",

	QuietTitle: "Quiet hours",
	QuietHint:  "Nothing cues inside this window; equal ends mean all day",
	QuietFrom:  "From",
	QuietTo:    "To",

	Autostart:     "Start with Windows",
	AutostartHint: "Runs quietly after sign-in, tray only",

	Appearance: "Appearance",
	SourceOn:   "Cue for this provider",
	Animation:  "Animation",
	Breathing:  "Breathing",
	Flash:      "Alternating flash",
	Colour:     "Colour",
	Colours:    []string{"Teal", "Violet", "Blue", "Pink", "Amber", "Green", "Coral", "Slate"},

	Sound:     "Cue sound",
	SoundHint: "Plays once per cue; volume 0 is silent and leaves the glow alone",
	Sounds:    []string{"Rise", "Fall", "Chime", "Knock"},
	Preview:   "Preview",
	Volume:    "Volume",

	Speech:        "Spoken announcement",
	SpeechHint:    `Only a fixed phrase ("Codex is done" / "OpenCode is done"); no reply text is ever read`,
	SpeakCodex:    "Speak for Codex",
	SpeakOpenCode: "Speak for OpenCode",
	SpeechVoice:   "Voice",
	SpeechReplace: "Speak instead of the sound",
	SpeechBack:    "Falls back to the sound when no voice is installed",
	VoiceDefault:  "System default",

	Integrations:  "Integrations",
	CodexTitle:    "Codex integration",
	OpenCodeTitle: "OpenCode integration",
	EnableCodex:   "Enable Codex integration",
	DisableCodex:  "Disable Codex integration",
	EnableOpen:    "Enable OpenCode integration",
	DisableOpen:   "Disable OpenCode integration",

	Close:        "Close",
	PauseCues:    "Pause cues",
	OpenSettings: "Open settings",
	Quit:         "Quit",

	CodexInstalled:    "Installed: a finished Codex turn cues",
	CodexNoBinary:     "Hook installed, but Codex was not found; it works once Codex is installed",
	CodexNotInstalled: "Not installed: a finished Codex turn does not cue",
	CodexNotDetected:  "Not installed, and Codex was not found",
	OpenInstalled:     "Installed: takes effect when OpenCode restarts, then a finished main turn cues",
	OpenNotDetected:   "OpenCode not detected",
	OpenNotInstalled:  "Not installed (OpenCode %s detected)",

	ProblemOverlay:          "Overlay unavailable: %s",
	ProblemWatch:            "Event directory unavailable: %s",
	ProblemSound:            "Cue sound failed: %s",
	ProblemSpeech:           "Speech failed: %s",
	ProblemSave:             "Saving preferences failed: %s",
	ProblemHooks:            "hooks.json is malformed; nothing was written and your configuration is untouched",
	ProblemCodex:            "Codex integration failed: %s",
	ProblemOpenNo:           "OpenCode was not detected; the plugin was not installed",
	ProblemOpenOld:          "OpenCode %s is outside the supported range (2.1.1 or newer, below 3.0.0); the plugin was not installed",
	ProblemOpenOldNoVersion: "OpenCode is outside the supported range; the plugin was not installed",
	ProblemOpenForeign:      `plugins\cuedex.js was not written by CueDex; nothing was overwritten or deleted`,
	ProblemOpenOther:        "OpenCode integration failed: %s",
	ProblemAutostart:        "Startup entry failed: %s",
}

// copyFor is the table for a resolved language.
func copyFor(language i18n.Language) wording {
	if language == i18n.Chinese {
		return chinese
	}
	return english
}

// lastCue is the status line after a cue.
func (t wording) lastCue(at time.Time, source string, count int) string {
	from := ""
	if source != "" {
		from = fmt.Sprintf(t.FromSource, source)
	}
	return fmt.Sprintf(t.LastCue, at.Format("15:04:05"), from, count)
}

// colourLabels names each palette entry, in the palette's order.
func (t wording) colourLabels() []string {
	if len(t.Colours) == len(prefs.Palette) {
		return t.Colours
	}
	labels := make([]string, len(prefs.Palette))
	for i := range labels {
		if i < len(t.Colours) {
			labels[i] = t.Colours[i]
		}
	}
	return labels
}

// soundLabels names the bundled sounds in the order sound.All returns them.
func (t wording) soundLabels() []string {
	all := sound.All()
	if len(t.Sounds) == len(all) {
		return t.Sounds
	}
	labels := make([]string, len(all))
	for i := range labels {
		if i < len(t.Sounds) {
			labels[i] = t.Sounds[i]
		}
	}
	return labels
}
