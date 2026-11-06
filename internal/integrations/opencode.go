package integrations

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// OpenCodeOwnership is the plugin file's first line. A file without it belongs
// to someone else, and this app neither replaces nor removes it.
const OpenCodeOwnership = "// CueDex managed OpenCode V2 bridge."

// ErrForeignPlugin means plugins\cuedex.js is not this app's file.
var ErrForeignPlugin = errors.New("integrations: cuedex.js was not written by CueDex")

// UnsupportedVersionError says why the plugin was not installed. An empty
// Version means OpenCode was not found at all.
type UnsupportedVersionError struct {
	Version string
}

func (e UnsupportedVersionError) Error() string {
	if e.Version == "" {
		return "integrations: OpenCode was not detected"
	}
	return "integrations: OpenCode " + e.Version + " is outside the supported range 2.1.1-3.0.0"
}

//go:embed cuedex-opencode.js
var openCodeTemplate string

// OpenCodeIntegration is the local plugin that tells this app a main session
// finished.
type OpenCodeIntegration struct {
	// ConfigDir is <config root>\opencode, where OpenCode looks for plugins.
	ConfigDir string
	// EventsDir is the directory the protocol fixes.
	EventsDir string
	// GatePath is the file the plugin checks before emitting. Removing it
	// stops every already-loaded instance.
	GatePath string
	// Exe is the app the plugin starts.
	Exe string
	// Version is what `opencode --version` said, detected before installing.
	Version string
}

// OpenCodeConfigDir is where OpenCode keeps its configuration: XDG_CONFIG_HOME
// when it is set, %USERPROFILE%\.config otherwise, then opencode.
func OpenCodeConfigDir() (string, error) {
	base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "opencode"), nil
}

// PluginPath is the file OpenCode loads.
func (o OpenCodeIntegration) PluginPath() string {
	return filepath.Join(o.ConfigDir, "plugins", "cuedex.js")
}

// Installed reports whether this app owns the plugin file and the gate that
// keeps it live.
func (o OpenCodeIntegration) Installed() (bool, error) {
	data, err := os.ReadFile(o.PluginPath())
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !HasOwnership(data) {
		return false, nil
	}
	gate, err := os.ReadFile(o.GatePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(gate)) != "", nil
}

// PluginFile reports whether plugins\cuedex.js exists and whether it is this
// app's file. A file someone else wrote is reported, not treated as a failure:
// the panel has to say whose it is, and nothing is changed either way.
func (o OpenCodeIntegration) PluginFile() (exists, owned bool, err error) {
	data, err := os.ReadFile(o.PluginPath())
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, HasOwnership(data), nil
}

// Install writes the rendered plugin and a fresh gate. A second install leaves
// both files exactly as they were.
func (o OpenCodeIntegration) Install() error {
	if !SupportsVersion(o.Version) {
		return UnsupportedVersionError{Version: o.Version}
	}
	existing, err := os.ReadFile(o.PluginPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(existing) > 0 && !HasOwnership(existing) {
		return ErrForeignPlugin
	}
	generation, err := o.generation()
	if err != nil {
		return err
	}
	rendered, err := RenderPlugin(openCodeTemplate, PluginValues{
		Events:     o.EventsDir,
		Gate:       o.GatePath,
		Generation: generation,
		App:        o.Exe,
	})
	if err != nil {
		return err
	}
	if bytes.Equal(existing, []byte(rendered)) {
		if gate, err := os.ReadFile(o.GatePath); err == nil && strings.TrimSpace(string(gate)) == generation {
			return nil
		}
	}
	for _, dir := range []string{filepath.Dir(o.PluginPath()), filepath.Dir(o.GatePath)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	if err := writeAtomic(o.PluginPath(), []byte(rendered)); err != nil {
		return err
	}
	return writeAtomic(o.GatePath, []byte(generation))
}

// Uninstall removes the gate first, so an OpenCode that already loaded the
// plugin stops emitting before its file disappears.
func (o OpenCodeIntegration) Uninstall() error {
	if err := os.Remove(o.GatePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	data, err := os.ReadFile(o.PluginPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !HasOwnership(data) {
		return ErrForeignPlugin
	}
	return os.Remove(o.PluginPath())
}

// HasOwnership reports whether the plugin file is this app's.
func HasOwnership(data []byte) bool {
	return bytes.HasPrefix(data, []byte(OpenCodeOwnership))
}

// PluginValues are the four values the template carries as placeholders.
type PluginValues struct {
	Events     string
	Gate       string
	Generation string
	App        string
}

// RenderPlugin fills the template in. Every value becomes a JSON string
// literal, so a Windows path never has to be escaped by hand.
func RenderPlugin(template string, values PluginValues) (string, error) {
	pairs := [][2]string{
		{"__CUEDEX_EVENTS__", values.Events},
		{"__CUEDEX_GATE__", values.Gate},
		{"__CUEDEX_GENERATION__", values.Generation},
		{"__CUEDEX_APP__", values.App},
	}
	rendered := template
	for _, pair := range pairs {
		literal, err := json.Marshal(pair[1])
		if err != nil {
			return "", err
		}
		rendered = strings.ReplaceAll(rendered, `"`+pair[0]+`"`, string(literal))
	}
	if strings.Contains(rendered, "__CUEDEX_") {
		return "", errors.New("integrations: the plugin template still has placeholders")
	}
	return rendered, nil
}

var versionPattern = regexp.MustCompile(
	`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`,
)

// SupportsVersion reports whether a version is one the plugin was written and
// tested for: stable 2.x from 2.1.1, below 3.0.0.
func SupportsVersion(version string) bool {
	core := strings.TrimPrefix(strings.TrimSpace(version), "v")
	match := versionPattern.FindStringSubmatch(core)
	if match == nil {
		return false
	}
	parts := make([]int, 3)
	for i, field := range match[1:4] {
		n, err := strconv.Atoi(field)
		if err != nil {
			return false
		}
		parts[i] = n
	}
	major, minor, patch := parts[0], parts[1], parts[2]
	if major != 2 {
		return false
	}
	return minor > 1 || (minor == 1 && patch >= 1)
}

// DetectVersion asks the installed OpenCode what version it is, taking the last
// word it prints ("opencode v2.2.5" is "v2.2.5").
func DetectVersion() (string, error) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return "", UnsupportedVersionError{}
	}
	command := commandFor(path, "--version")
	var output bytes.Buffer
	command.Stdout = &output
	if err := command.Start(); err != nil {
		return "", err
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	select {
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		return "", errors.New("integrations: opencode --version timed out")
	case err := <-waited:
		if err != nil {
			return "", err
		}
	}
	fields := strings.Fields(output.String())
	if len(fields) == 0 {
		return "", errors.New("integrations: opencode printed no version")
	}
	return fields[len(fields)-1], nil
}

// commandFor runs a command-line tool. npm and Volta install .cmd shims, which
// Windows cannot start directly, so those go through cmd.exe.
func commandFor(path string, args ...string) *exec.Cmd {
	hidden := &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cmd", ".bat":
		comspec := os.Getenv("COMSPEC")
		if comspec == "" {
			comspec = `C:\Windows\System32\cmd.exe`
		}
		// cmd /s /c takes the whole line as one string, with the quotes it
		// expects; Go's own argument quoting would escape them.
		line := comspec + ` /d /s /c ""` + path + `"`
		for _, arg := range args {
			line += " " + arg
		}
		line += `"`
		command := exec.Command(comspec)
		hidden.CmdLine = line
		command.SysProcAttr = hidden
		return command
	default:
		command := exec.Command(path, args...)
		command.SysProcAttr = hidden
		return command
	}
}

// generation reuses the gate's value, so a second install leaves the plugin
// byte for byte as it was; only the first install makes a new one.
func (o OpenCodeIntegration) generation() (string, error) {
	if gate, err := os.ReadFile(o.GatePath); err == nil {
		if value := strings.TrimSpace(string(gate)); value != "" {
			return value, nil
		}
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}
