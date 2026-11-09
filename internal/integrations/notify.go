package integrations

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrNotifyTaken means config.toml's notify line already names another
// program. CueDex does not take such a line over: the user may depend on it.
var ErrNotifyTaken = errors.New("integrations: the notify line already names another program")

// ConfigPath is <CODEX_HOME>/config.toml, where Codex keeps the legacy notify
// line. The desktop app runs that command on every finished turn, which is the
// only completion channel it executes for a user configuration.
func (c CodexIntegration) ConfigPath() string {
	return filepath.Join(c.Home, "config.toml")
}

// NotifyArgv is the argv CueDex needs in the notify line: the payload arrives
// as the argument after these.
func (c CodexIntegration) NotifyArgv() []string {
	return []string{c.Exe, "--notify"}
}

// NotifyInstalled reports whether the notify line reaches this executable,
// either directly or through the app's own --previous-notify chain.
func (c CodexIntegration) NotifyInstalled() (bool, error) {
	data, err := os.ReadFile(c.ConfigPath())
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(data), "\n")
	_, argv, ok, err := findNotify(lines)
	if err != nil || !ok {
		return false, err
	}
	if equalArgv(argv, c.NotifyArgv()) {
		return true, nil
	}
	previous, _, ok, err := previousNotify(argv)
	if err != nil {
		return false, err
	}
	return ok && equalArgv(previous, c.NotifyArgv()), nil
}

// InstallNotify points the notify line at CueDex. When the app has already
// wrapped an earlier command in its chain, CueDex takes the slot the earlier
// command held, so computer-use keeps calling the rest of the chain. A line
// that belongs to another program is left alone.
func (c CodexIntegration) InstallNotify() error {
	path := c.ConfigPath()
	lines, err := readLines(path)
	if err != nil {
		return err
	}
	want := c.NotifyArgv()
	index, argv, ok, err := findNotify(lines)
	if err != nil {
		return err
	}
	if ok {
		if equalArgv(argv, want) {
			return nil
		}
		previous, at, hasPrevious, err := previousNotify(argv)
		if err != nil {
			return err
		}
		if !hasPrevious || (len(previous) > 0 && !equalArgv(previous, want)) {
			return ErrNotifyTaken
		}
		encoded, err := json.Marshal(want)
		if err != nil {
			return err
		}
		argv[at] = string(encoded)
		lines[index] = formatNotify(argv)
		return writeLines(path, lines)
	}
	lines = insertNotify(lines, want)
	return writeLines(path, lines)
}

// UninstallNotify removes CueDex from the notify line, restoring the slot the
// app's chain expects when the app had wrapped an earlier command.
func (c CodexIntegration) UninstallNotify() error {
	path := c.ConfigPath()
	lines, err := readLines(path)
	if err != nil {
		return err
	}
	index, argv, ok, err := findNotify(lines)
	if err != nil || !ok {
		return err
	}
	want := c.NotifyArgv()
	switch {
	case equalArgv(argv, want):
		lines = append(lines[:index], lines[index+1:]...)
	case true:
		previous, at, hasPrevious, err := previousNotify(argv)
		if err != nil || !hasPrevious || !equalArgv(previous, want) {
			return err
		}
		argv[at] = "[]"
		lines[index] = formatNotify(argv)
	}
	return writeLines(path, lines)
}

// findNotify returns the top-level notify line: its index, its argv, and
// whether there is one. A notify line inside a section is not ours to touch.
func findNotify(lines []string) (int, []string, bool, error) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[[") {
			// The first table header ends the top-level block.
			break
		}
		if !strings.HasPrefix(trimmed, "notify") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "notify"))
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		argv, err := parseArray(strings.TrimSpace(strings.TrimPrefix(rest, "=")))
		if err != nil {
			return 0, nil, false, err
		}
		return i, argv, true, nil
	}
	return 0, nil, false, nil
}

// previousNotify returns the argv the app's chain stores in
// --previous-notify, the index of that value, and whether it was found.
func previousNotify(argv []string) ([]string, int, bool, error) {
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] != "--previous-notify" {
			continue
		}
		var previous []string
		if err := json.Unmarshal([]byte(argv[i+1]), &previous); err != nil {
			return nil, 0, false, ErrInvalidHooks
		}
		return previous, i + 1, true, nil
	}
	return nil, 0, false, nil
}

// parseArray reads the TOML string array of a notify line.
func parseArray(value string) ([]string, error) {
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, ErrInvalidHooks
	}
	body := strings.TrimSpace(value[1 : len(value)-1])
	var argv []string
	for body != "" {
		body = strings.TrimLeft(body, " \t,")
		if body == "" {
			break
		}
		if body[0] != '"' {
			return nil, ErrInvalidHooks
		}
		end := 1
		for end < len(body) {
			if body[end] == '\\' {
				end += 2
				continue
			}
			if body[end] == '"' {
				break
			}
			end++
		}
		if end >= len(body) {
			return nil, ErrInvalidHooks
		}
		text, err := strconv.Unquote(body[:end+1])
		if err != nil {
			return nil, ErrInvalidHooks
		}
		argv = append(argv, text)
		body = strings.TrimLeft(body[end+1:], " \t")
		if strings.HasPrefix(body, ",") {
			body = body[1:]
		}
	}
	return argv, nil
}

func formatNotify(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		quoted[i] = strconv.Quote(arg)
	}
	return "notify = [ " + strings.Join(quoted, ", ") + " ]"
}

// insertNotify puts the line after the last top-level key, before the first
// table, so it stays where Codex reads it as a top-level setting.
func insertNotify(lines []string, argv []string) []string {
	line := formatNotify(argv)
	at := len(lines)
	for i, existing := range lines {
		if strings.HasPrefix(strings.TrimSpace(existing), "[") {
			at = i
			break
		}
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:at]...)
	out = append(out, line)
	out = append(out, lines[at:]...)
	return out
}

func equalArgv(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\n"), nil
}

func writeLines(path string, lines []string) error {
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}
