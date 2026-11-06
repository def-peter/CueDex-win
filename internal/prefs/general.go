package prefs

import "time"

// Language values a preferences file may hold. An empty one follows Windows.
const (
	LanguageSystem  = ""
	LanguageChinese = "zh"
	LanguageEnglish = "en"
)

// General is the application-wide half of the preferences.
type General struct {
	// Language is the interface language; empty follows Windows.
	Language string `json:"language"`
	// Enabled is the master switch: off means no cue at all.
	Enabled bool `json:"enabled"`
	// Paused suppresses cues; PauseUntil ends that, and zero means until the
	// user resumes.
	Paused     bool       `json:"paused"`
	PauseUntil int64      `json:"pauseUntil"` // unix seconds
	QuietHours QuietHours `json:"quietHours"`
	// OpenAtLogin mirrors the Run entry, so the panel can show the truth.
	OpenAtLogin bool `json:"openAtLogin"`
}

// QuietHours is a daily window in which nothing is shown. The window may cross
// midnight, and equal ends mean the whole day, as on the mac build.
type QuietHours struct {
	Enabled bool `json:"enabled"`
	Start   int  `json:"start"` // minutes after midnight
	End     int  `json:"end"`
}

// Quiet reports whether at falls inside the window.
func (q QuietHours) Quiet(at time.Time) bool {
	if !q.Enabled {
		return false
	}
	if q.Start == q.End {
		return true
	}
	minute := at.Hour()*60 + at.Minute()
	if q.Start < q.End {
		return minute >= q.Start && minute < q.End
	}
	return minute >= q.Start || minute < q.End
}

// PausedAt reports whether cues are suppressed at. A pause with no end lasts
// until the user resumes, and an elapsed end is not a pause any more.
func (g General) PausedAt(at time.Time) bool {
	if !g.Paused {
		return false
	}
	if g.PauseUntil == 0 {
		return true
	}
	return at.Unix() < g.PauseUntil
}

// PauseFor starts a pause that ends after d, and returns the settings. A d of
// zero pauses until the user resumes.
func (g General) PauseFor(at time.Time, d time.Duration) General {
	g.Paused = true
	if d <= 0 {
		g.PauseUntil = 0
		return g
	}
	g.PauseUntil = at.Add(d).Unix()
	return g
}

// Resume ends any pause.
func (g General) Resume() General {
	g.Paused = false
	g.PauseUntil = 0
	return g
}

// Remaining is how long the pause still has to run; zero means no timed pause
// is in effect (either not paused, or paused until resumed).
func (g General) Remaining(at time.Time) time.Duration {
	if !g.PausedAt(at) || g.PauseUntil == 0 {
		return 0
	}
	return time.Duration(g.PauseUntil-at.Unix()) * time.Second
}

func normalizeGeneral(g General) General {
	switch g.Language {
	case LanguageChinese, LanguageEnglish:
	default:
		g.Language = LanguageSystem
	}
	g.QuietHours.Start = clampMinute(g.QuietHours.Start)
	g.QuietHours.End = clampMinute(g.QuietHours.End)
	if g.PauseUntil < 0 {
		g.PauseUntil = 0
	}
	return g
}

func clampMinute(minute int) int {
	if minute < 0 {
		return 0
	}
	if minute > 24*60-1 {
		return 24*60 - 1
	}
	return minute
}
