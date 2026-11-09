package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/def-peter/cuedex-win/internal/integrations"
	"github.com/def-peter/cuedex-win/internal/prefs"
)

// configWithChain is the config.toml the desktop app leaves behind once it has
// wrapped an earlier notify command in its own chain.
const configWithChain = `notify = [ "C:\\OpenAI\\codex-computer-use.exe", "turn-ended", "--previous-notify", "[]" ]
model = "x"
`

// The desktop app runs the notify line, not a user Stop hook, so enabling the
// integration has to claim that slot too.
func TestTogglingCodexPointsTheNotifyLineAtCueDex(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "config.toml")
	if err := os.WriteFile(config, []byte(configWithChain), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{prefs: prefs.Default()}
	a.codex = integrations.CodexIntegration{Home: home, Exe: `D:\APP\CueDex\CueDex.exe`}

	a.toggleCodex()

	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `D:\\\\APP\\\\CueDex\\\\CueDex.exe`) {
		t.Fatalf("the notify chain does not name CueDex:\n%s", got)
	}
	if !strings.Contains(got, "codex-computer-use.exe") {
		t.Fatalf("the app's own command was dropped:\n%s", got)
	}

	a.toggleCodex()
	data, err = os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "CueDex") {
		t.Fatalf("the notify line survived disabling:\n%s", string(data))
	}
}

// A notify line that belongs to another program is reported, not overwritten.
func TestCodexReportsATakenNotifyLine(t *testing.T) {
	home := t.TempDir()
	body := `notify = [ "C:\\OpenAI\\codex-computer-use.exe", "turn-ended", "--previous-notify", "[\"C:\\\\other\\\\tool.exe\"]" ]` + "\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{prefs: prefs.Default()}
	a.codex = integrations.CodexIntegration{Home: home, Exe: `D:\APP\CueDex\CueDex.exe`}

	a.toggleCodex()

	if got := a.state.codexProblem.text(chinese); !strings.Contains(got, "notify") {
		t.Fatalf("problem %q", got)
	}
}
