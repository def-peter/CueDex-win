// Package hook is the entry a host's hook runs: it reads the host's payload,
// decides whether a turn really finished, leaves one event file, and wakes the
// app. It reads metadata only and never stores the reply itself.
package hook

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/def-peter/cuedex-win/internal/events"
)

// maxPayload caps what a hook reads from stdin. Codex sends the reply with the
// payload, so the ceiling has to be generous; nothing past it is a metadata
// check anyway.
const maxPayload = 32 << 20

// createNoWindow keeps a console from flashing when the hook starts the app.
const createNoWindow = 0x08000000

// CodexPayload is the part of a Codex Stop payload this app reads. Everything
// else in the payload, including the reply, is ignored.
type CodexPayload struct {
	Event   string  `json:"hook_event_name"`
	Session string  `json:"session_id"`
	Turn    string  `json:"turn_id"`
	Message *string `json:"last_assistant_message"`
}

// CodexEvent returns the completion a Stop payload describes. It reports false
// for every other event, and for a turn that produced no reply: a tool-only
// turn is not something the user is waiting for.
func CodexEvent(data []byte, now time.Time) (events.Event, bool) {
	var payload CodexPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return events.Event{}, false
	}
	if payload.Event != "Stop" {
		return events.Event{}, false
	}
	if payload.Message == nil || strings.TrimSpace(*payload.Message) == "" {
		return events.Event{}, false
	}
	session := clamp(payload.Session, "legacy")
	id := ""
	if turn := clamp(payload.Turn, ""); turn != "" {
		sum := sha256.Sum256([]byte(session + "\n" + turn))
		id = hex.EncodeToString(sum[:])
	} else {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return events.Event{}, false
		}
		id = hex.EncodeToString(random[:])
	}
	return events.Event{
		Source:      events.Codex,
		EventID:     id,
		SessionID:   session,
		CompletedAt: now,
	}, true
}

// Codex reads the payload Codex sends when a turn stops and leaves one event
// file when the turn really produced a reply. It reports whether it did, so the
// caller knows whether the app has to be woken.
func Codex(stdin io.Reader, eventsDir string) (bool, error) {
	data, err := io.ReadAll(io.LimitReader(stdin, maxPayload))
	if err != nil {
		return false, err
	}
	event, ok := CodexEvent(data, time.Now())
	if !ok {
		return false, nil
	}
	if err := writeEvent(eventsDir, event); err != nil {
		return false, err
	}
	return true, nil
}

// writeEvent writes one turn-complete file, then renames it into place so the
// app never reads half of it (PROTOCOL.md).
func writeEvent(dir string, event events.Event) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := events.Marshal(event)
	if err != nil {
		return err
	}
	name := "turn-complete." + strconv.Itoa(os.Getpid())
	path := filepath.Join(dir, name)
	temp := filepath.Join(dir, "."+name+".tmp")
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		// A file left by a reused pid: this completion replaces it.
		os.Remove(path)
		if err := os.Rename(temp, path); err != nil {
			return err
		}
	}
	return nil
}

// Wake starts the app with --cue. A second process hands the argument to the
// running instance and exits, so this works whether or not the app is up.
func Wake(exe string) error {
	command := exec.Command(exe, "--cue")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

// clamp keeps a string inside the protocol's 512-byte ceiling, and uses fallback
// when it is empty.
func clamp(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len(value) > 512 {
		value = value[:512]
		for len(value) > 0 && value[len(value)-1]&0xC0 == 0x80 {
			value = value[:len(value)-1]
		}
	}
	return value
}
