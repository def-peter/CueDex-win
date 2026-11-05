package speech

import (
	"strings"
	"testing"

	"github.com/def-peter/cuedex-win/internal/events"
)

func TestPhraseDependsOnlyOnTheSource(t *testing.T) {
	// The signature takes no event payload, but pin the text down so a later
	// change cannot quietly interpolate a session id or a reply.
	if got := Phrase(events.Codex); got != "Codex 已完成" {
		t.Errorf("codex phrase %q", got)
	}
	if got := Phrase(events.OpenCode); got != "OpenCode 已完成" {
		t.Errorf("opencode phrase %q", got)
	}
	for _, phrase := range []string{codexPhrase, openCodePhrase} {
		if strings.ContainsAny(phrase, "{}%") {
			t.Errorf("phrase %q looks like a format string", phrase)
		}
	}
}
