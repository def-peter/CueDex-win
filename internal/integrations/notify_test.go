package integrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) (CodexIntegration, string) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return CodexIntegration{Home: home, Exe: `D:\APP\CueDex\CueDex.exe`}, path
}

func readConfig(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstallNotifyAddsATopLevelLine(t *testing.T) {
	c, path := writeConfig(t, "model = \"x\"\n\n[mcp_servers.a]\ncommand = \"a\"\n")

	if err := c.InstallNotify(); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, path)
	if !strings.Contains(got, `notify = [ "D:\\APP\\CueDex\\CueDex.exe", "--notify" ]`) {
		t.Fatalf("config:\n%s", got)
	}
	// The line has to sit before the first table, or Codex reads it as a key of
	// that table.
	if strings.Index(got, "notify") > strings.Index(got, "[mcp_servers.a]") {
		t.Fatalf("notify was written inside a table:\n%s", got)
	}

	installed, err := c.NotifyInstalled()
	if err != nil || !installed {
		t.Fatalf("installed %v, err %v", installed, err)
	}
}

func TestInstallNotifyTakesTheAppsChainSlot(t *testing.T) {
	body := `notify = [ "C:\\OpenAI\\codex-computer-use.exe", "turn-ended", "--previous-notify", "[]" ]` + "\n"
	c, path := writeConfig(t, body)

	if err := c.InstallNotify(); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, path)
	if !strings.Contains(got, `"--previous-notify", "[\"D:\\\\APP\\\\CueDex\\\\CueDex.exe\",\"--notify\"]"`) {
		t.Fatalf("config:\n%s", got)
	}
	if !strings.Contains(got, "codex-computer-use.exe") {
		t.Fatalf("the app's own command was dropped:\n%s", got)
	}
	installed, err := c.NotifyInstalled()
	if err != nil || !installed {
		t.Fatalf("installed %v, err %v", installed, err)
	}
}

func TestInstallNotifyLeavesAnotherProgramAlone(t *testing.T) {
	body := `notify = [ "C:\\OpenAI\\codex-computer-use.exe", "turn-ended", "--previous-notify", "[\"C:\\\\other\\\\notifier.exe\"]" ]` + "\n"
	c, path := writeConfig(t, body)

	if err := c.InstallNotify(); err != ErrNotifyTaken {
		t.Fatalf("err %v, want ErrNotifyTaken", err)
	}
	if got := readConfig(t, path); got != body {
		t.Fatalf("the file changed:\n%s", got)
	}
}

func TestInstallNotifyIsIdempotent(t *testing.T) {
	c, path := writeConfig(t, "model = \"x\"\n")
	if err := c.InstallNotify(); err != nil {
		t.Fatal(err)
	}
	first := readConfig(t, path)
	if err := c.InstallNotify(); err != nil {
		t.Fatal(err)
	}
	if got := readConfig(t, path); got != first {
		t.Fatalf("a second install changed the file:\n%s", got)
	}
}

func TestUninstallNotifyRemovesOurLine(t *testing.T) {
	c, path := writeConfig(t, "model = \"x\"\n")
	if err := c.InstallNotify(); err != nil {
		t.Fatal(err)
	}
	if err := c.UninstallNotify(); err != nil {
		t.Fatal(err)
	}
	if got := readConfig(t, path); strings.Contains(got, "notify") {
		t.Fatalf("the line survived:\n%s", got)
	}
	if !strings.Contains(readConfig(t, path), "model") {
		t.Fatal("the rest of the file was lost")
	}
}

func TestUninstallNotifyEmptiesTheChainSlot(t *testing.T) {
	body := `notify = [ "C:\\OpenAI\\codex-computer-use.exe", "turn-ended", "--previous-notify", "[]" ]` + "\n"
	c, path := writeConfig(t, body)
	if err := c.InstallNotify(); err != nil {
		t.Fatal(err)
	}
	if err := c.UninstallNotify(); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, path)
	if !strings.Contains(got, `"--previous-notify", "[]"`) {
		t.Fatalf("the slot was not restored:\n%s", got)
	}
}

func TestParseArrayHandlesEscapedPaths(t *testing.T) {
	argv, err := parseArray(`[ "D:\\APP\\CueDex\\CueDex.exe", "--notify" ]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) != 2 || argv[0] != `D:\APP\CueDex\CueDex.exe` || argv[1] != "--notify" {
		t.Fatalf("argv %q", argv)
	}
}

func TestFindNotifyIgnoresATableKey(t *testing.T) {
	lines := []string{
		"model = \"x\"",
		"[hooks.state.'a']",
		"notify = \"inside a table\"",
	}
	if _, _, ok, err := findNotify(lines); err != nil || ok {
		t.Fatalf("found %v, err %v", ok, err)
	}
}
