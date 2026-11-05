package integrations

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const command = `"C:\Program Files\CueDex\CueDex.exe" --hook codex`

// decode parses a hooks file for comparison. Numbers keep their text, so a
// timeout the user wrote as 3 is still 3.
func decode(t *testing.T, data []byte) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return value
}

func TestAddStopHookStartsAnEmptyFile(t *testing.T) {
	data, err := AddStopHook(nil, command)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "command": "\"C:\\Program Files\\CueDex\\CueDex.exe\" --hook codex",
            "commandWindows": "\"C:\\Program Files\\CueDex\\CueDex.exe\" --hook codex",
            "timeout": 3,
            "type": "command"
          }
        ]
      }
    ]
  }
}
`
	if string(data) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", data, want)
	}
	if present, err := HasStopHook(data, command); err != nil || !present {
		t.Fatalf("present %v, err %v", present, err)
	}
}

func TestAddStopHookIsIdempotent(t *testing.T) {
	once, err := AddStopHook(nil, command)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := AddStopHook(once, command)
	if err != nil {
		t.Fatal(err)
	}
	// A repeated install must not even reformat the file.
	if !bytes.Equal(once, twice) {
		t.Fatalf("second install changed the file:\n%s\nthen\n%s", once, twice)
	}
}

func TestAddStopHookKeepsEverythingElse(t *testing.T) {
	user := []byte(`{
  "description": "my hooks",
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "echo hi"}]}
    ],
    "Stop": [
      {"hooks": [{"type": "command", "command": "say done", "timeout": 5}]}
    ]
  },
  "somethingElse": {"nested": [1, 2.5, true]}
}`)
	updated, err := AddStopHook(user, command)
	if err != nil {
		t.Fatal(err)
	}
	root := decode(t, updated).(map[string]any)
	if root["description"] != "my hooks" {
		t.Fatalf("description lost: %v", root["description"])
	}
	if !reflect.DeepEqual(root["somethingElse"], decode(t, []byte(`{"nested": [1, 2.5, true]}`))) {
		t.Fatalf("unknown fields changed: %v", root["somethingElse"])
	}
	hooks := root["hooks"].(map[string]any)
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Fatal("the user's PreToolUse hook was lost")
	}
	groups := hooks["Stop"].([]any)
	if len(groups) != 2 {
		t.Fatalf("Stop has %d groups, want the user's and ours", len(groups))
	}
	first := groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if first["command"] != "say done" {
		t.Fatalf("the user's Stop hook changed: %v", first)
	}
}

func TestRemoveStopHookRestoresTheFile(t *testing.T) {
	user := []byte(`{
  "description": "my hooks",
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "echo hi"}]}
    ],
    "Stop": [
      {"hooks": [{"type": "command", "command": "say done", "timeout": 5}]}
    ]
  }
}
`)
	installed, err := AddStopHook(user, command)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveStopHook(installed, command)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decode(t, removed), decode(t, user)) {
		t.Fatalf("removal did not restore the file:\n%s\nwant\n%s", removed, user)
	}
}

func TestRemoveStopHookLeavesAnEmptyFileAsNil(t *testing.T) {
	installed, err := AddStopHook(nil, command)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveStopHook(installed, command)
	if err != nil {
		t.Fatal(err)
	}
	if removed != nil {
		t.Fatalf("removed = %q, want nil so the file is deleted", removed)
	}
}

func TestRemoveStopHookKeepsAGroupThatStillHasHandlers(t *testing.T) {
	data := []byte(`{"hooks":{"Stop":[{"matcher":"x","hooks":[
		{"type":"command","command":"say done"},
		{"type":"command","command":"` + escape(command) + `"}
	]}]}}`)
	removed, err := RemoveStopHook(data, command)
	if err != nil {
		t.Fatal(err)
	}
	root := decode(t, removed).(map[string]any)
	group := root["hooks"].(map[string]any)["Stop"].([]any)[0].(map[string]any)
	if group["matcher"] != "x" {
		t.Fatalf("the group lost its matcher: %v", group)
	}
	handlers := group["hooks"].([]any)
	if len(handlers) != 1 || handlers[0].(map[string]any)["command"] != "say done" {
		t.Fatalf("handlers after removal: %v", handlers)
	}
}

func TestRemoveStopHookIgnoresAnotherAppsEntry(t *testing.T) {
	other := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"\"C:\\Other\\other.exe\" --hook codex"}]}]}}`)
	removed, err := RemoveStopHook(other, command)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decode(t, removed), decode(t, other)) {
		t.Fatalf("someone else's hook was removed: %s", removed)
	}
}

func TestStopHookRejectsAFileItDoesNotUnderstand(t *testing.T) {
	for name, data := range map[string]string{
		"not json":            `{"hooks":`,
		"hooks not an object": `{"hooks": []}`,
		"stop not a list":     `{"hooks": {"Stop": {}}}`,
		"group without hooks": `{"hooks": {"Stop": [{"matcher": "x"}]}}`,
		"handler not object":  `{"hooks": {"Stop": [{"hooks": [1]}]}}`,
		"top level array":     `[]`,
	} {
		if _, err := AddStopHook([]byte(data), command); !errors.Is(err, ErrInvalidHooks) {
			t.Errorf("%s: err %v, want ErrInvalidHooks", name, err)
		}
		if _, err := RemoveStopHook([]byte(data), command); !errors.Is(err, ErrInvalidHooks) {
			t.Errorf("%s: remove err %v, want ErrInvalidHooks", name, err)
		}
	}
}

func TestInstallAndUninstallOnDisk(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".codex")
	integration := CodexIntegration{Home: home, Exe: `C:\Apps\CueDex.exe`}
	if integration.Detected() {
		t.Fatal("an empty home was reported as a Codex install")
	}
	if err := integration.Install(); err != nil {
		t.Fatal(err)
	}
	if !integration.Detected() {
		t.Fatal("install did not create the Codex home")
	}
	if installed, err := integration.Installed(); err != nil || !installed {
		t.Fatalf("installed %v, err %v", installed, err)
	}
	if err := integration.Install(); err != nil {
		t.Fatal(err)
	}
	if installed, err := integration.Installed(); err != nil || !installed {
		t.Fatalf("after a second install: installed %v, err %v", installed, err)
	}
	if err := integration.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if installed, err := integration.Installed(); err != nil || installed {
		t.Fatalf("after uninstall: installed %v, err %v", installed, err)
	}
	if _, err := os.Stat(integration.HooksPath()); !os.IsNotExist(err) {
		t.Fatalf("the file we created was left behind: %v", err)
	}
}

func TestInstallKeepsTheOriginalAsABackup(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "hooks.json")
	original := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"say done"}]}]}}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	integration := CodexIntegration{Home: home, Exe: `C:\Apps\CueDex.exe`}
	if err := integration.Install(); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".cuedex-backup")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatalf("backup %s, want the original %s", backup, original)
	}
	if err := integration.Uninstall(); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decode(t, restored), decode(t, original)) {
		t.Fatalf("restored %s, want %s", restored, original)
	}
}

func TestInstallRefusesAFileItCannotParse(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "hooks.json")
	broken := []byte(`{"hooks": "nope"}`)
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	integration := CodexIntegration{Home: home, Exe: `C:\Apps\CueDex.exe`}
	if err := integration.Install(); !errors.Is(err, ErrInvalidHooks) {
		t.Fatalf("err %v, want ErrInvalidHooks", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, broken) {
		t.Fatalf("the unreadable file was overwritten: %s", after)
	}
}

func TestCodexHomeFollowsTheEnvironment(t *testing.T) {
	t.Setenv("CODEX_HOME", `D:\custom\codex`)
	if got, err := CodexHome(); err != nil || got != `D:\custom\codex` {
		t.Fatalf("CodexHome() = %q, %v", got, err)
	}

	// An empty CODEX_HOME is ignored, the way codex-rs ignores it.
	t.Setenv("CODEX_HOME", "   ")
	got, err := CodexHome()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".codex"); got != want {
		t.Fatalf("CodexHome() = %q, want %q", got, want)
	}
}

// escape makes a string safe to embed in the JSON literal above.
func escape(s string) string {
	data, _ := json.Marshal(s)
	return string(data[1 : len(data)-1])
}
