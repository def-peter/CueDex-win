package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/def-peter/cuedex-win/internal/integrations"
	"github.com/def-peter/cuedex-win/internal/prefs"
)

// A plugin written by an older copy carries that copy's path. On the next
// start the panel rewrites it, so a cue does not start a program that is no
// longer there.
func TestRefreshOpenCodeRepointsAStalePlugin(t *testing.T) {
	config := t.TempDir()
	integration := integrations.OpenCodeIntegration{
		ConfigDir: config,
		EventsDir: filepath.Join(config, "Events"),
		GatePath:  filepath.Join(config, "gate"),
		Exe:       `D:\APP\CueDex\CueDex.exe`,
		Version:   "2.2.5",
	}
	// The same plugin, rendered for a copy that has since moved.
	stale := integration
	stale.Exe = `D:\code\me\go\CueDex-win\build\windows-amd64\CueDex.exe`
	if err := stale.Install(); err != nil {
		t.Fatal(err)
	}

	a := &App{prefs: prefs.Default(), opencode: integration}
	a.refreshOpenCode()

	if !a.state.opencodeInstalled {
		t.Fatalf("the plugin is not reported as installed: %+v", a.state.opencodeProblem)
	}
	data, err := os.ReadFile(filepath.Join(config, "plugins", "cuedex.js"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, `build-windows-amd64`) || strings.Contains(got, `build\\windows-amd64`) {
		t.Fatalf("the plugin still names the old copy:\n%s", got)
	}
	if !strings.Contains(got, `D:\\APP\\CueDex\\CueDex.exe`) {
		t.Fatalf("the plugin does not name this copy:\n%s", got)
	}
}

// An unchanged plugin is left exactly as it is.
func TestRefreshOpenCodeLeavesAMatchingPlugin(t *testing.T) {
	config := t.TempDir()
	integration := integrations.OpenCodeIntegration{
		ConfigDir: config,
		EventsDir: filepath.Join(config, "Events"),
		GatePath:  filepath.Join(config, "gate"),
		Exe:       `D:\APP\CueDex\CueDex.exe`,
		Version:   "2.2.5",
	}
	if err := integration.Install(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config, "plugins", "cuedex.js")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	a := &App{prefs: prefs.Default(), opencode: integration}
	a.refreshOpenCode()

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("a matching plugin was rewritten")
	}
}
