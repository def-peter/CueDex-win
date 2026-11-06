package app

import (
	"github.com/def-peter/cuedex-win/internal/i18n"
	"github.com/def-peter/cuedex-win/internal/recordings"
	"github.com/def-peter/cuedex-win/internal/sound"
)

// wording is every word the panel shows, in one language. The panel is small
// enough that a table of strings beats a lookup API: a missing string is a
// compile error, not a blank label.
type wording struct {
	Disabled string
	Paused   string
	Quiet    string

	General       string
	SettingsTitle string
	Master        string
	Language      string
	LangSystem    string
	LangChinese   string
	LangEnglish   string
	TabCues       string
	TabAbout      string

	Pause15   string
	Pause1h   string
	Pause4h   string
	Resume    string
	PausedFor string // fmt: duration

	QuietTitle string
	QuietFrom  string
	QuietTo    string

	Autostart string

	Product      string
	AboutVersion string // fmt: version
	AboutDev     string
	AboutTagline string

	Sources              string
	Glow                 string
	SoundPane            string
	NotificationSound    string
	EnableGlow           string
	MainColour           string
	SecondaryColourLabel string
	TestGlow             string
	EnableSound          string
	TestSound            string
	TestSpeech           string
	SpeechEnable         string
	SpeechText           string
	SpeechTextLabel      string

	Appearance string
	Effect     string
	Breathing  string
	Flash      string
	Colour     string

	Sound  string
	Sounds []string

	Speech              string
	Mode                string
	SpeechModeSystem    string
	SpeechModeRecording string
	SpeechRecording     string
	SpeechNoVoice       string
	SpeechVoice         string
	VoiceDefault        string
	Recordings          []string

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
	ProblemBusy             string
}

var chinese = wording{
	Disabled: "提示已关闭",
	Paused:   "已暂停：完成事件不会触发提示",
	Quiet:    "免打扰时段内：完成事件不会触发提示",

	General:       "通用",
	SettingsTitle: "CueDex 设置",
	Master:        "启用提示",
	Language:      "语言",
	LangSystem:    "跟随系统",
	LangChinese:   "中文",
	LangEnglish:   "English",
	TabCues:       "提醒",
	TabAbout:      "关于",

	Pause15:   "15 分钟",
	Pause1h:   "1 小时",
	Pause4h:   "4 小时",
	Resume:    "恢复提示",
	PausedFor: "%s 后恢复",

	QuietTitle: "免打扰",
	QuietFrom:  "开始",
	QuietTo:    "结束",

	Autostart: "登录时启动",

	Product:      "CueDex",
	AboutVersion: "版本 %s",
	AboutDev:     "开发版",
	AboutTagline: "Codex 或 OpenCode 完成回复时，用光效、声音或语音提醒你。",

	Sources:              "提醒来源",
	Glow:                 "光效",
	SoundPane:            "声音",
	NotificationSound:    "通知音",
	EnableGlow:           "是否启用",
	Colour:               "颜色",
	MainColour:           "主色",
	SecondaryColourLabel: "副色",
	TestGlow:             "测试光效",
	EnableSound:          "是否启用",
	TestSound:            "测试声音",
	TestSpeech:           "测试语音",
	SpeechEnable:         "是否启用",
	SpeechText:           "通知文字",
	SpeechTextLabel:      "通知文字",

	Appearance: "外观",
	Effect:     "光效模式",
	Breathing:  "单色呼吸",
	Flash:      "双色交替闪烁",

	Sound:  "声音",
	Sounds: []string{"上扬", "下沉", "清音", "双击"},

	Speech:              "语音",
	Mode:                "模式",
	SpeechModeSystem:    "系统朗读",
	SpeechModeRecording: "预录语音",
	SpeechRecording:     "录音",
	SpeechNoVoice:       "没有可用的系统语音；可以改用预录语音。",
	SpeechVoice:         "说话人",
	VoiceDefault:        "系统默认",
	Recordings:          []string{"消息提醒（男声）", "消息提醒（女声）", "查看消息（男声）", "查看消息（女声）"},

	Integrations:  "集成",
	CodexTitle:    "Codex 集成",
	OpenCodeTitle: "OpenCode 集成",
	EnableCodex:   "启用集成",
	DisableCodex:  "停用集成",
	EnableOpen:    "启用集成",
	DisableOpen:   "停用集成",

	Close:        "关闭",
	PauseCues:    "暂停通知",
	OpenSettings: "打开设置",
	Quit:         "退出",

	CodexInstalled:    "已安装",
	CodexNoBinary:     "已安装，未找到 Codex",
	CodexNotInstalled: "未安装",
	CodexNotDetected:  "未安装，未找到 Codex",
	OpenInstalled:     "已安装",
	OpenNotDetected:   "未找到 OpenCode",
	OpenNotInstalled:  "OpenCode %s 版本不支持",

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
	Disabled: "Cues are off",
	Paused:   "Paused: completions will not cue",
	Quiet:    "Quiet hours: completions will not cue",

	General:       "General",
	SettingsTitle: "CueDex Settings",
	Master:        "Cue on completion",
	Language:      "Language",
	LangSystem:    "Follow Windows",
	LangChinese:   "中文",
	LangEnglish:   "English",
	TabCues:       "Cues",
	TabAbout:      "About",

	Pause15:   "15 minutes",
	Pause1h:   "1 hour",
	Pause4h:   "4 hours",
	Resume:    "Resume cues",
	PausedFor: "Resumes in %s",

	QuietTitle: "Quiet hours",
	QuietFrom:  "From",
	QuietTo:    "Until",

	Autostart: "Launch at login",

	Product:      "CueDex",
	AboutVersion: "Version %s",
	AboutDev:     "Development build",
	AboutTagline: "Glow, sound or speech when Codex or OpenCode finishes a reply.",

	Sources:              "Attention Source",
	Glow:                 "Glow",
	SoundPane:            "Sound",
	NotificationSound:    "Notification Sound",
	EnableGlow:           "Enabled",
	Colour:               "Color",
	MainColour:           "Primary Color",
	SecondaryColourLabel: "Secondary Color",
	TestGlow:             "Test Glow",
	EnableSound:          "Enabled",
	TestSound:            "Test Sound",
	TestSpeech:           "Test speech",
	SpeechEnable:         "Enabled",
	SpeechText:           "Message",
	SpeechTextLabel:      "Message",

	Appearance: "Appearance",
	Effect:     "Effect",
	Breathing:  "Monochrome Breathing",
	Flash:      "Two-Color Flash",

	Sound:  "Sound",
	Sounds: []string{"Rise", "Fall", "Chime", "Knock"},

	Speech:              "Speech",
	Mode:                "Mode",
	SpeechModeSystem:    "System Speech",
	SpeechModeRecording: "Recorded",
	SpeechRecording:     "Recording",
	SpeechNoVoice:       "No system voice is installed; a recorded announcement still works.",
	SpeechVoice:         "Voice",
	VoiceDefault:        "System default",
	Recordings:          []string{"Message Reminder (Male)", "Message Reminder (Female)", "Check Your Messages (Male)", "Check Your Messages (Female)"},

	Integrations:  "Integrations",
	CodexTitle:    "Codex integration",
	OpenCodeTitle: "OpenCode integration",
	EnableCodex:   "Enable Integration",
	DisableCodex:  "Disable Integration",
	EnableOpen:    "Enable Integration",
	DisableOpen:   "Disable Integration",

	Close:        "Close",
	PauseCues:    "Pause Notifications",
	OpenSettings: "Open settings",
	Quit:         "Quit",

	CodexInstalled:    "Installed",
	CodexNoBinary:     "Installed, Codex missing",
	CodexNotInstalled: "Not installed",
	CodexNotDetected:  "Not installed, Codex missing",
	OpenInstalled:     "Installed",
	OpenNotDetected:   "OpenCode not found",
	OpenNotInstalled:  "OpenCode %s is unsupported",

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
