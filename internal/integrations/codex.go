// Package integrations installs and removes the hooks that let Codex and
// OpenCode tell CueDex a turn finished. Nothing here reads a prompt or a
// response: an integration only has to point the host at this executable.
package integrations

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrInvalidHooks means the file is not the shape Codex reads. The app never
// rewrites such a file: a wrong guess would damage a working configuration.
var ErrInvalidHooks = errors.New("integrations: hooks.json structure is not what Codex expects")

// CodexIntegration is the Stop hook in <CODEX_HOME>\hooks.json.
type CodexIntegration struct {
	// Home is <CODEX_HOME>, the directory Codex reads its configuration from.
	Home string
	// Exe is the CueDex executable the hook runs.
	Exe string
}

// CodexHome is where Codex keeps its configuration: CODEX_HOME when it is set
// and non-empty, %USERPROFILE%\.codex otherwise (the rule codex-rs uses).
func CodexHome() (string, error) {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

// HooksPath is the file the hook goes into.
func (c CodexIntegration) HooksPath() string {
	return filepath.Join(c.Home, "hooks.json")
}

// Command is the one line cmd.exe runs. It carries no shell metacharacters,
// prints nothing, and exits 0.
func (c CodexIntegration) Command() string {
	return `"` + c.Exe + `" --hook codex`
}

// Detected reports whether Codex looks installed on this machine. Installing
// without it is allowed, so the answer only changes what the settings panel
// says.
func (c CodexIntegration) Detected() bool {
	info, err := os.Stat(c.Home)
	return err == nil && info.IsDir()
}

// Installed reports whether the Stop hook is present.
func (c CodexIntegration) Installed() (bool, error) {
	data, err := os.ReadFile(c.HooksPath())
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return HasStopHook(data, c.Command())
}

// Install adds the Stop hook, keeping everything else in the file as it was.
func (c CodexIntegration) Install() error {
	original, err := os.ReadFile(c.HooksPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updated, err := AddStopHook(original, c.Command())
	if err != nil {
		return err
	}
	if bytes.Equal(updated, original) {
		return nil
	}
	if err := os.MkdirAll(c.Home, 0o700); err != nil {
		return err
	}
	if len(original) > 0 {
		if err := backupOnce(c.HooksPath(), original); err != nil {
			return err
		}
	}
	return writeAtomic(c.HooksPath(), updated)
}

// Uninstall removes only this app's Stop handler, leaving the user's own
// handlers and every other event type alone.
func (c CodexIntegration) Uninstall() error {
	original, err := os.ReadFile(c.HooksPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	updated, err := RemoveStopHook(original, c.Command())
	if err != nil {
		return err
	}
	if bytes.Equal(updated, original) {
		return nil
	}
	if updated == nil {
		return os.Remove(c.HooksPath())
	}
	return writeAtomic(c.HooksPath(), updated)
}

// HasStopHook reports whether data already contains this app's Stop handler.
func HasStopHook(data []byte, command string) (bool, error) {
	root, err := parseHooks(data)
	if err != nil {
		return false, err
	}
	table, err := hooksTable(root)
	if err != nil {
		return false, err
	}
	groups, err := stopGroups(table)
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		handlers, err := groupHandlers(group)
		if err != nil {
			return false, err
		}
		for _, handler := range handlers {
			if isManagedHandler(handler, command) {
				return true, nil
			}
		}
	}
	return false, nil
}

// AddStopHook returns data with one Stop handler for command. When the handler
// is already there, data is returned unchanged, so a second install leaves the
// file byte for byte as it was.
func AddStopHook(data []byte, command string) ([]byte, error) {
	root, err := parseHooks(data)
	if err != nil {
		return nil, err
	}
	table, err := hooksTable(root)
	if err != nil {
		return nil, err
	}
	groups, err := stopGroups(table)
	if err != nil {
		return nil, err
	}
	present, err := HasStopHook(data, command)
	if err != nil {
		return nil, err
	}
	if present {
		return data, nil
	}
	handler := map[string]any{
		"type":           "command",
		"command":        command,
		"commandWindows": command,
		"timeout":        json.Number("3"),
	}
	groups = append(groups, map[string]any{"hooks": []any{handler}})
	table["Stop"] = groups
	root["hooks"] = table
	return marshalHooks(root)
}

// RemoveStopHook returns data without this app's Stop handler, or nil when
// nothing else is left in the file.
func RemoveStopHook(data []byte, command string) ([]byte, error) {
	root, err := parseHooks(data)
	if err != nil {
		return nil, err
	}
	table, err := hooksTable(root)
	if err != nil {
		return nil, err
	}
	groups, err := stopGroups(table)
	if err != nil {
		return nil, err
	}
	var kept []any
	for _, group := range groups {
		handlers, err := groupHandlers(group)
		if err != nil {
			return nil, err
		}
		var keptHandlers []any
		for _, handler := range handlers {
			if !isManagedHandler(handler, command) {
				keptHandlers = append(keptHandlers, handler)
			}
		}
		if len(keptHandlers) > 0 {
			group["hooks"] = keptHandlers
			kept = append(kept, group)
		}
	}
	if len(kept) == 0 {
		delete(table, "Stop")
	} else {
		table["Stop"] = kept
	}
	if len(table) == 0 {
		delete(root, "hooks")
	} else {
		root["hooks"] = table
	}
	if len(root) == 0 {
		return nil, nil
	}
	return marshalHooks(root)
}

// parseHooks decodes the file without losing anything it does not understand:
// numbers keep their text and unknown keys keep their values.
func parseHooks(data []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidHooks, err)
	}
	if root == nil {
		return nil, ErrInvalidHooks
	}
	return root, nil
}

func hooksTable(root map[string]any) (map[string]any, error) {
	raw, ok := root["hooks"]
	if !ok {
		return map[string]any{}, nil
	}
	table, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: hooks is not an object", ErrInvalidHooks)
	}
	return table, nil
}

func stopGroups(table map[string]any) ([]map[string]any, error) {
	raw, ok := table["Stop"]
	if !ok {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: hooks.Stop is not a list", ErrInvalidHooks)
	}
	groups := make([]map[string]any, 0, len(list))
	for _, item := range list {
		group, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: a Stop entry is not an object", ErrInvalidHooks)
		}
		if _, err := groupHandlers(group); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func groupHandlers(group map[string]any) ([]map[string]any, error) {
	raw, ok := group["hooks"]
	if !ok {
		return nil, fmt.Errorf("%w: a Stop entry has no hooks", ErrInvalidHooks)
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: a Stop entry's hooks is not a list", ErrInvalidHooks)
	}
	handlers := make([]map[string]any, 0, len(list))
	for _, item := range list {
		handler, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: a handler is not an object", ErrInvalidHooks)
		}
		handlers = append(handlers, handler)
	}
	return handlers, nil
}

// isManagedHandler says a handler is this app's: the exact line this build
// writes, or one left by another CueDex install for the same entry point.
func isManagedHandler(handler map[string]any, command string) bool {
	for _, field := range []string{"command", "commandWindows"} {
		value, ok := handler[field].(string)
		if !ok {
			continue
		}
		if value == command {
			return true
		}
		lower := strings.ToLower(value)
		if strings.Contains(lower, "cuedex") && strings.HasSuffix(strings.TrimSpace(lower), "--hook codex") {
			return true
		}
	}
	return false
}

func marshalHooks(root map[string]any) ([]byte, error) {
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidHooks, err)
	}
	return append(data, '\n'), nil
}

// backupOnce keeps the user's file as it was before the first install. It is
// written once and never overwritten, so a later install cannot bury the
// original.
func backupOnce(path string, original []byte) error {
	backup := path + ".cuedex-backup"
	if _, err := os.Stat(backup); err == nil {
		return nil
	}
	return writeAtomic(backup, original)
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".cuedex-*.tmp")
	if err != nil {
		return err
	}
	name := file.Name()
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(name)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
