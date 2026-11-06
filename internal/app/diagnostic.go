package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/integrations"
)

// pluginFileState is what plugins\cuedex.js is, as far as the panel can tell.
type pluginFileState int

const (
	pluginMissing pluginFileState = iota
	pluginOurs
	pluginForeign
	pluginUnreadable
)

// hooksFileState is what the Codex Stop entry looks like.
type hooksFileState int

const (
	hooksMissing hooksFileState = iota
	hooksOurs
	hooksInvalid
	hooksUnreadable
)

// diagnostic is the panel's last section: what the two integrations look like
// and the newest completion the app has seen. Every field is metadata; no
// prompt, reply, or session content reaches it.
type diagnostic struct {
	openCodeVersion string
	plugin          pluginFileState
	codexDetected   bool
	hooks           hooksFileState
	lastEvent       time.Time
	lastSource      events.Source
}

// versionLine is the version the probe read from the installed OpenCode.
func (d diagnostic) versionLine(t wording) string {
	if d.openCodeVersion == "" {
		return t.DiagOpenMissing
	}
	return fmt.Sprintf(t.DiagOpenVersion, d.openCodeVersion)
}

// pluginLine is the state of the plugin file, whoever wrote it.
func (d diagnostic) pluginLine(t wording) string {
	switch d.plugin {
	case pluginOurs:
		return t.DiagPluginOurs
	case pluginForeign:
		return t.DiagPluginForeign
	case pluginUnreadable:
		return t.DiagPluginUnreadable
	}
	return t.DiagPluginMissing
}

// codexLine is the state of Codex's directory and Stop entry.
func (d diagnostic) codexLine(t wording) string {
	switch {
	case !d.codexDetected:
		return t.DiagCodexMissing
	case d.hooks == hooksOurs:
		return t.DiagCodexHooksOurs
	case d.hooks == hooksInvalid:
		return t.DiagCodexHooksInvalid
	case d.hooks == hooksUnreadable:
		return t.DiagCodexHooksUnreadable
	}
	return t.DiagCodexHooksMissing
}

// lastEventLine is when the newest completion arrived and from which producer.
func (d diagnostic) lastEventLine(t wording) string {
	if d.lastEvent.IsZero() {
		return t.DiagNoEvent
	}
	source := ""
	if d.lastSource != "" {
		source = fmt.Sprintf(t.FromSource, d.lastSource.Label())
	}
	return fmt.Sprintf(t.DiagLastEvent, d.lastEvent.Format("15:04:05"), source)
}

// refreshDiagnostics reads the two integrations' files and keeps the newest
// completion the watcher has reported. It never opens an event file.
func (a *App) refreshDiagnostics() {
	d := a.state.diag
	d.openCodeVersion = a.opencode.Version
	exists, owned, err := a.opencode.PluginFile()
	switch {
	case err != nil:
		d.plugin = pluginUnreadable
	case !exists:
		d.plugin = pluginMissing
	case owned:
		d.plugin = pluginOurs
	default:
		d.plugin = pluginForeign
	}
	d.codexDetected = a.codex.Home != "" && a.codex.Detected()
	installed, hooksErr := a.codex.Installed()
	switch {
	case hooksErr == nil && installed:
		d.hooks = hooksOurs
	case hooksErr == nil:
		d.hooks = hooksMissing
	case errors.Is(hooksErr, integrations.ErrInvalidHooks):
		d.hooks = hooksInvalid
	default:
		d.hooks = hooksUnreadable
	}
	a.state.diag = d
}
