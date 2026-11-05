package speech

import "github.com/def-peter/cuedex-win/internal/events"

// The phrases are constants on purpose. Nothing from an event, a session or a
// reply may ever be spoken, so the only input is which producer finished.
const (
	codexPhrase    = "Codex 已完成"
	openCodePhrase = "OpenCode 已完成"
)

// Phrase is what a completion is announced as.
func Phrase(source events.Source) string {
	if source == events.OpenCode {
		return openCodePhrase
	}
	return codexPhrase
}
