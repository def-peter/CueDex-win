// Package paths says where CueDex keeps its per-user state on Windows.
package paths

import (
	"os"
	"path/filepath"
)

// Dir is the application's per-user directory, %APPDATA%\CueDex.
type Dir struct {
	Root string
}

// UserData locates the directory without creating it.
func UserData() (Dir, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return Dir{}, err
	}
	return Dir{Root: filepath.Join(base, "CueDex")}, nil
}

// Events is where producers drop one JSON file per completion.
func (d Dir) Events() string { return filepath.Join(d.Root, "Events") }

// OpenCodeGate is the file the OpenCode plugin checks before it emits. It
// carries the generation of the installed plugin, so an instance that is still
// loaded stops as soon as the gate is gone.
func (d Dir) OpenCodeGate() string { return filepath.Join(d.Root, "opencode-gate") }

// Prepare creates the directories the app needs on first run.
func (d Dir) Prepare() error {
	for _, dir := range []string{d.Root, d.Events()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}
