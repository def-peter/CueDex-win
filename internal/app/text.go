package app

import (
	"fmt"
	"time"

	"github.com/def-peter/cuedex-win/internal/i18n"
	"github.com/def-peter/cuedex-win/internal/recordings"
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

	General       string
	GeneralHint   string
	SettingsTitle string
	Master        string
	MasterHint    string
	Language      string
	LangSystem    string
	LangChinese   string
	LangEnglish   string
	TabCues       string
	TabAbout      string

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

	Product      string
	AboutVersion string // fmt: version
	AboutDev     string
	AboutTagline string

	Sources              string
	CueFor               string // fmt: source label
	CueHint              string
	Glow                 string
	SoundPane            string
	NotificationSound    string
	EnableGlow           string
	GlowHint             string
	MainColour           string
	SecondaryColourLabel string
	FlashHint            string
	TestGlow             string
	EnableSound          string
	TestSound            string
	TestSpeech           string
	SpeechEnable         string
	SpeechText           string
	SpeechTextLabel      string
	PreviewNote          string

	Appearance string
	SourceOn   string
	Effect     string
	Breathing  string
	Flash      string
	Colour     string

	Sound     string
	SoundHint string
	Sounds    []string
	Preview   string
	Volume    string

	Speech              string
	SpeechHint          string
	Mode                string
	SpeechModeSystem    string
	SpeechModeRecording string
	SpeechRecording     string
	SpeechNoVoice       string
	SpeakCodex          string
	SpeakOpenCode       string
	SpeechVoice         string
	SpeechReplace       string
	SpeechBack          string
	VoiceDefault        string
	Recordings          []string

	Integrations  string
	CodexTitle    string
	OpenCodeTitle string
	EnableCodex   string
	DisableCodex  string
	EnableOpen    string
	DisableOpen   string

	Diagnostics              string
	DiagHint                 string
	DiagOpenVersion          string // fmt: version
	DiagOpenMissing          string
	DiagPluginOurs           string
	DiagPluginMissing        string
	DiagPluginForeign        string
	DiagPluginUnreadable     string
	DiagCodexMissing         string
	DiagCodexHooksOurs       string
	DiagCodexHooksMissing    string
	DiagCodexHooksInvalid    string
	DiagCodexHooksUnreadable string
	DiagLastEvent            string // fmt: time, source
	DiagNoEvent              string

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
	ProblemBusy             string
}

var chinese = wording{
	Idle:       "等待 Codex / OpenCode 的完成事件",
	Disabled:   "提示已关闭",
	Paused:     "已暂停：完成事件不会触发提示",
	Quiet:      "免打扰时段内：完成事件不会触发提示",
	LastCue:    "最近一次唤醒 %s%s，共 %d 次",
	FromSource: "（%s）",

	General:       "通用",
	GeneralHint:   "管理运行偏好，以及 Codex 和 OpenCode 的连接。",
	SettingsTitle: "CueDex 设置",
	Master:        "启用提示",
	MasterHint:    "关闭后既无光效也不发声",
	Language:      "语言",
	LangSystem:    "跟随系统",
	LangChinese:   "中文",
	LangEnglish:   "English",
	TabCues:       "提醒",
	TabAbout:      "关于",

	Pause:      "暂停",
	Pause15:    "15 分钟",
	Pause1h:    "1 小时",
	Pause4h:    "4 小时",
	PauseUntil: "直到恢复",
	Resume:     "恢复提示",
	PausedFor:  "还剩 %s 自动恢复",

	QuietTitle: "免打扰",
	QuietHint:  "这段时间内不提示；起止相同表示全天",
	QuietFrom:  "开始",
	QuietTo:    "结束",

	Autostart:     "登录时启动",
	AutostartHint: "登录后静默运行，只出现在托盘",

	Product:      "CueDex",
	AboutVersion: "版本 %s",
	AboutDev:     "开发版",
	AboutTagline: "当 Codex 或 OpenCode 完成回复时，CueDex 会通过屏幕边缘光效、提示音或语音提醒你。",

	Sources:              "提醒来源",
	CueFor:               "%s 提示",
	CueHint:              "主会话回复完成时，使用此来源的提示设置。",
	Glow:                 "光效",
	SoundPane:            "声音",
	NotificationSound:    "通知音",
	EnableGlow:           "是否启用",
	GlowHint:             "在屏幕边缘显示 3 秒光效。",
	Colour:               "颜色",
	MainColour:           "主色",
	SecondaryColourLabel: "副色",
	FlashHint:            "两种颜色交替闪烁，持续 3 秒。",
	TestGlow:             "测试光效",
	EnableSound:          "是否启用",
	TestSound:            "测试声音",
	TestSpeech:           "测试语音",
	SpeechEnable:         "是否启用",
	SpeechText:           "通知文字",
	SpeechTextLabel:      "通知文字",
	PreviewNote:          "测试仅预览当前来源的效果，不代表接入已验证。",

	Appearance: "外观",
	SourceOn:   "显示该来源的提示",
	Effect:     "光效模式",
	Breathing:  "单色呼吸",
	Flash:      "双色交替闪烁",

	Sound:     "声音",
	SoundHint: "每条提示最多响一次；音量 0 等同静音，不影响光效",
	Sounds:    []string{"上扬", "下沉", "清音", "双击"},
	Preview:   "试听",
	Volume:    "音量",

	Speech:              "语音",
	SpeechHint:          "播放预录语音，或朗读你为这个来源设置的短语；只播放固定内容，不读取任何回复",
	Mode:                "模式",
	SpeechModeSystem:    "系统朗读",
	SpeechModeRecording: "预录语音",
	SpeechRecording:     "录音",
	SpeechNoVoice:       "没有可用的系统语音；可以改用预录语音。",
	SpeakCodex:          "朗读 Codex",
	SpeakOpenCode:       "朗读 OpenCode",
	SpeechVoice:         "说话人",
	SpeechReplace:       "用语音替代提示音",
	SpeechBack:          "没有可用语音时自动回到提示音",
	VoiceDefault:        "系统默认",
	Recordings:          []string{"消息提醒（男声）", "消息提醒（女声）", "查看消息（男声）", "查看消息（女声）"},

	Integrations:  "集成",
	CodexTitle:    "Codex 集成",
	OpenCodeTitle: "OpenCode 集成",
	EnableCodex:   "启用集成",
	DisableCodex:  "停用集成",
	EnableOpen:    "启用集成",
	DisableOpen:   "停用集成",

	Diagnostics:              "诊断",
	DiagHint:                 "只显示状态与时间，不读取、不记录会话内容",
	DiagOpenVersion:          "OpenCode 版本：%s",
	DiagOpenMissing:          "OpenCode 版本：未检测到",
	DiagPluginOurs:           "插件文件：已安装",
	DiagPluginMissing:        "插件文件：未安装",
	DiagPluginForeign:        `插件文件：存在，但不是 CueDex 写的`,
	DiagPluginUnreadable:     "插件文件：读不到",
	DiagCodexMissing:         "Codex：未检测到",
	DiagCodexHooksOurs:       "Codex：目录已找到，Stop 钩子已安装",
	DiagCodexHooksMissing:    "Codex：目录已找到，未安装 Stop 钩子",
	DiagCodexHooksInvalid:    "Codex：hooks.json 结构异常",
	DiagCodexHooksUnreadable: "Codex：hooks.json 读不到",
	DiagLastEvent:            "最近一次事件：%s%s",
	DiagNoEvent:              "最近一次事件：还没有收到",

	Close:        "关闭",
	PauseCues:    "暂停通知",
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
	ProblemBusy:             "%s 正被另一个程序占用，关闭它后重试",
}

var english = wording{
	Idle:       "Waiting for a Codex or OpenCode turn to finish",
	Disabled:   "Cues are off",
	Paused:     "Paused: completions will not cue",
	Quiet:      "Quiet hours: completions will not cue",
	LastCue:    "Last cue at %s%s, %d in total",
	FromSource: " (%s)",

	General:       "General",
	GeneralHint:   "Runtime preferences and the Codex and OpenCode connections.",
	SettingsTitle: "CueDex Settings",
	Master:        "Cue on completion",
	MasterHint:    "Off means no glow and no sound",
	Language:      "Language",
	LangSystem:    "Follow Windows",
	LangChinese:   "中文",
	LangEnglish:   "English",
	TabCues:       "Cues",
	TabAbout:      "About",

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
	QuietTo:    "Until",

	Autostart:     "Launch at login",
	AutostartHint: "Runs quietly after sign-in, tray only",

	Product:      "CueDex",
	AboutVersion: "Version %s",
	AboutDev:     "Development build",
	AboutTagline: "CueDex routes Codex and OpenCode completion events to screen-edge glow, sound, or speech.",

	Sources:              "Attention Source",
	CueFor:               "%s cues",
	CueHint:              "Used when a main turn for this provider finishes.",
	Glow:                 "Glow",
	SoundPane:            "Sound",
	NotificationSound:    "Notification Sound",
	EnableGlow:           "Enabled",
	GlowHint:             "Draws a 3-second glow around the screen edge.",
	Colour:               "Color",
	MainColour:           "Primary Color",
	SecondaryColourLabel: "Secondary Color",
	FlashHint:            "The two colours alternate for 3 seconds.",
	TestGlow:             "Test Glow",
	EnableSound:          "Enabled",
	TestSound:            "Test Sound",
	TestSpeech:           "Test speech",
	SpeechEnable:         "Enabled",
	SpeechText:           "Message",
	SpeechTextLabel:      "Message",
	PreviewNote:          "Tests preview this source's effects only; they do not verify integration.",

	Appearance: "Appearance",
	SourceOn:   "Cue for this provider",
	Effect:     "Effect",
	Breathing:  "Monochrome Breathing",
	Flash:      "Two-Color Flash",

	Sound:     "Sound",
	SoundHint: "Plays once per cue; volume 0 is silent and leaves the glow alone",
	Sounds:    []string{"Rise", "Fall", "Chime", "Knock"},
	Preview:   "Preview",
	Volume:    "Volume",

	Speech:              "Speech",
	SpeechHint:          "Plays a bundled clip, or reads the phrase you set for this provider; only fixed text is ever played",
	Mode:                "Mode",
	SpeechModeSystem:    "System Speech",
	SpeechModeRecording: "Recorded",
	SpeechRecording:     "Recording",
	SpeechNoVoice:       "No system voice is installed; a recorded announcement still works.",
	SpeakCodex:          "Speak for Codex",
	SpeakOpenCode:       "Speak for OpenCode",
	SpeechVoice:         "Voice",
	SpeechReplace:       "Speak instead of the sound",
	SpeechBack:          "Falls back to the sound when no voice is installed",
	VoiceDefault:        "System default",
	Recordings:          []string{"Message Reminder (Male)", "Message Reminder (Female)", "Check Your Messages (Male)", "Check Your Messages (Female)"},

	Integrations:  "Integrations",
	CodexTitle:    "Codex integration",
	OpenCodeTitle: "OpenCode integration",
	EnableCodex:   "Enable Integration",
	DisableCodex:  "Disable Integration",
	EnableOpen:    "Enable Integration",
	DisableOpen:   "Disable Integration",

	Diagnostics:              "Diagnostics",
	DiagHint:                 "State and times only; the conversation is never read or recorded",
	DiagOpenVersion:          "OpenCode version: %s",
	DiagOpenMissing:          "OpenCode version: not detected",
	DiagPluginOurs:           "Plugin file: installed",
	DiagPluginMissing:        "Plugin file: not installed",
	DiagPluginForeign:        `Plugin file: present, but not written by CueDex`,
	DiagPluginUnreadable:     "Plugin file: cannot be read",
	DiagCodexMissing:         "Codex: not detected",
	DiagCodexHooksOurs:       "Codex: directory found, Stop hook installed",
	DiagCodexHooksMissing:    "Codex: directory found, Stop hook not installed",
	DiagCodexHooksInvalid:    "Codex: hooks.json is malformed",
	DiagCodexHooksUnreadable: "Codex: hooks.json cannot be read",
	DiagLastEvent:            "Latest event: %s%s",
	DiagNoEvent:              "Latest event: none received yet",

	Close:        "Close",
	PauseCues:    "Pause Notifications",
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
	ProblemBusy:             "%s is open in another program. Close it and try again",
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

// recordingLabels names the bundled clips in the order recordings.All returns
// them.
func (t wording) recordingLabels() []string {
	all := recordings.All()
	if len(t.Recordings) == len(all) {
		return t.Recordings
	}
	labels := make([]string, len(all))
	for i := range labels {
		if i < len(t.Recordings) {
			labels[i] = t.Recordings[i]
		}
	}
	return labels
}
