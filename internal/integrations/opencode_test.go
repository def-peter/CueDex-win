package integrations

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupportsVersion(t *testing.T) {
	for version, want := range map[string]bool{
		"2.1.1":         true,
		"2.2.5":         true,
		"v2.2.5":        true,
		"2.10.0":        true,
		"2.99.99":       true,
		"2.2.5+build.7": true,
		"2.1.0":         false,
		"2.0.9":         false,
		"3.0.0":         false,
		"3.1.0":         false,
		"1.9.9":         false,
		"2.2":           false,
		"2.2.5.1":       false,
		"2.2.5-rc.1":    false,
		"02.2.5":        false,
		"vv2.2.5":       false,
		"":              false,
		"latest":        false,
	} {
		if got := SupportsVersion(version); got != want {
			t.Errorf("SupportsVersion(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestRenderPluginReplacesEveryPlaceholder(t *testing.T) {
	rendered, err := RenderPlugin(openCodeTemplate, PluginValues{
		Events:     `C:\Users\me\AppData\Roaming\CueDex\Events`,
		Gate:       `C:\Users\me\AppData\Roaming\CueDex\opencode-gate`,
		Generation: "3f2b",
		App:        `C:\Program Files\CueDex\CueDex.exe`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rendered, OpenCodeOwnership) {
		t.Fatalf("rendered plugin starts with %q", rendered[:40])
	}
	for _, want := range []string{
		`"C:\\Users\\me\\AppData\\Roaming\\CueDex\\Events"`,
		`"C:\\Users\\me\\AppData\\Roaming\\CueDex\\opencode-gate"`,
		`"3f2b"`,
		`"C:\\Program Files\\CueDex\\CueDex.exe"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered plugin has no %s", want)
		}
	}
	if strings.Contains(rendered, "__CUEDEX_") {
		t.Error("a placeholder survived")
	}
	// The plugin launches the app the Windows way, not through open(1).
	if !strings.Contains(rendered, `execFile(appPath, ["--cue"], { windowsHide: true }`) {
		t.Error("the launch line is not the Windows one")
	}
}

func TestRenderPluginRefusesALeftoverPlaceholder(t *testing.T) {
	if _, err := RenderPlugin(`const x = "__CUEDEX_OTHER__";`, PluginValues{}); err == nil {
		t.Fatal("a template with an unknown placeholder was accepted")
	}
}

func TestInstallWritesThePluginAndTheGate(t *testing.T) {
	config, gate := filepath.Join(t.TempDir(), "opencode"), filepath.Join(t.TempDir(), "gate")
	integration := OpenCodeIntegration{
		ConfigDir: config,
		EventsDir: `C:\Users\me\AppData\Roaming\CueDex\Events`,
		GatePath:  gate,
		Exe:       `C:\Apps\CueDex.exe`,
		Version:   "2.2.5",
	}
	if err := integration.Install(); err != nil {
		t.Fatal(err)
	}
	plugin := integration.PluginPath()
	if plugin != filepath.Join(config, "plugins", "cuedex.js") {
		t.Fatalf("plugin path %q", plugin)
	}
	first, err := os.ReadFile(plugin)
	if err != nil {
		t.Fatal(err)
	}
	if !HasOwnership(first) {
		t.Fatal("the plugin has no ownership header")
	}
	if installed, err := integration.Installed(); err != nil || !installed {
		t.Fatalf("installed %v, err %v", installed, err)
	}
	// A second install changes neither file.
	if err := integration.Install(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(plugin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("a repeated install rewrote the plugin")
	}
	if err := integration.Uninstall(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{plugin, gate} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%q survived the uninstall: %v", path, err)
		}
	}
}

func TestInstallRefusesAForeignPlugin(t *testing.T) {
	config := filepath.Join(t.TempDir(), "opencode")
	plugin := filepath.Join(config, "plugins", "cuedex.js")
	if err := os.MkdirAll(filepath.Dir(plugin), 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("// somebody else's plugin\nexport default {};\n")
	if err := os.WriteFile(plugin, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	integration := OpenCodeIntegration{
		ConfigDir: config,
		GatePath:  filepath.Join(t.TempDir(), "gate"),
		Version:   "2.2.5",
	}
	if err := integration.Install(); !errors.Is(err, ErrForeignPlugin) {
		t.Fatalf("err %v, want ErrForeignPlugin", err)
	}
	after, err := os.ReadFile(plugin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, foreign) {
		t.Fatal("the foreign plugin was overwritten")
	}
	if err := integration.Uninstall(); !errors.Is(err, ErrForeignPlugin) {
		t.Fatalf("uninstall err %v, want ErrForeignPlugin", err)
	}
	if _, err := os.Stat(plugin); err != nil {
		t.Fatal("the foreign plugin was removed")
	}
}

func TestInstallRefusesAnUnsupportedVersion(t *testing.T) {
	integration := OpenCodeIntegration{
		ConfigDir: filepath.Join(t.TempDir(), "opencode"),
		GatePath:  filepath.Join(t.TempDir(), "gate"),
		Version:   "3.0.0",
	}
	err := integration.Install()
	var unsupported UnsupportedVersionError
	if !errors.As(err, &unsupported) || unsupported.Version != "3.0.0" {
		t.Fatalf("err %v, want UnsupportedVersionError for 3.0.0", err)
	}
	if _, err := os.Stat(integration.PluginPath()); !os.IsNotExist(err) {
		t.Fatal("a plugin was written for an unsupported version")
	}
	if err := integration.Uninstall(); err != nil {
		t.Fatalf("uninstall %v", err)
	}
}

func TestUninstallRemovesTheGateBeforeThePlugin(t *testing.T) {
	config, gate := filepath.Join(t.TempDir(), "opencode"), filepath.Join(t.TempDir(), "gate")
	integration := OpenCodeIntegration{
		ConfigDir: config,
		GatePath:  gate,
		Exe:       `C:\Apps\CueDex.exe`,
		Version:   "2.2.5",
	}
	if err := integration.Install(); err != nil {
		t.Fatal(err)
	}
	// A gate that disagrees with the plugin still means "not ours to keep".
	if err := os.WriteFile(gate, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := integration.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gate); !os.IsNotExist(err) {
		t.Fatal("the gate survived")
	}
	if _, err := os.Stat(integration.PluginPath()); !os.IsNotExist(err) {
		t.Fatal("the plugin survived")
	}
}

func TestOpenCodeConfigDirFollowsTheEnvironment(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", `D:\config`)
	got, err := OpenCodeConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(`D:\config`, "opencode"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	t.Setenv("XDG_CONFIG_HOME", "  ")
	got, err = OpenCodeConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "opencode"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
