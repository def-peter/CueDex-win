// Package autostart reads and writes the per-user Run entry that makes Windows
// start CueDex with the session. It writes nothing outside HKEY_CURRENT_USER and
// needs no elevation.
package autostart

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	// DefaultKey is the per-user Run key every Windows session reads.
	DefaultKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	// DefaultValue is the entry's name, so a user can find and delete it by
	// hand.
	DefaultValue = "CueDex"
	// backgroundFlag is how the entry starts the app: tray only.
	backgroundFlag = "--background"
)

var (
	advapi32             = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	procRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")
	procRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	regKeyQueryValue     = uintptr(0x0001)
	regKeySetValue       = uintptr(0x0002)
	regSZ                = uintptr(1)
	errorFileNotFound    = syscall.Errno(2)
	hkeyCurrentUser      = uintptr(0x80000001)
)

// Entry is one Run value: which key it lives in and under what name. The two
// are fields rather than constants so a test can use a scratch key and never
// touch the real Run entry.
type Entry struct {
	Key   string
	Value string
}

// Default is the entry a real install writes.
func Default() Entry {
	return Entry{Key: DefaultKey, Value: DefaultValue}
}

// Command is what the Run entry holds: the executable path in quotes, followed
// by the flag that keeps a session start quiet.
func Command(exe string) string {
	return `"` + exe + `" ` + backgroundFlag
}

// Enabled reports whether the entry exists and still points at this executable.
// An entry that points elsewhere counts as off, so copying or moving the app
// does not leave a stale command starting the old copy.
func (e Entry) Enabled(exe string) (bool, error) {
	command, err := e.read()
	if err != nil {
		return false, err
	}
	if command == "" {
		return false, nil
	}
	return sameCommand(command, exe), nil
}

// Enable writes the entry, replacing whatever it held.
func (e Entry) Enable(exe string) error {
	key, err := e.create()
	if err != nil {
		return err
	}
	defer procRegCloseKey.Call(key)

	name, err := syscall.UTF16PtrFromString(e.Value)
	if err != nil {
		return err
	}
	command, err := syscall.UTF16FromString(Command(exe))
	if err != nil {
		return err
	}
	result, _, _ := procRegSetValueExW.Call(
		key,
		uintptr(unsafe.Pointer(name)),
		0,
		regSZ,
		uintptr(unsafe.Pointer(&command[0])),
		uintptr(len(command)*2),
	)
	if result != 0 {
		return fmt.Errorf("autostart: write %s\\%s: %w", e.Key, e.Value, syscall.Errno(result))
	}
	return nil
}

// Disable removes the entry. Removing one that is already gone is not an error,
// so turning the switch off twice is harmless.
func (e Entry) Disable() error {
	key, err := e.open(regKeySetValue)
	if err != nil {
		if errors.Is(err, errorFileNotFound) {
			return nil
		}
		return err
	}
	defer procRegCloseKey.Call(key)

	name, err := syscall.UTF16PtrFromString(e.Value)
	if err != nil {
		return err
	}
	result, _, _ := procRegDeleteValueW.Call(key, uintptr(unsafe.Pointer(name)))
	if result != 0 && syscall.Errno(result) != errorFileNotFound {
		return fmt.Errorf("autostart: delete %s\\%s: %w", e.Key, e.Value, syscall.Errno(result))
	}
	return nil
}

// read returns the entry's command, or "" when it is absent.
func (e Entry) read() (string, error) {
	key, err := e.open(regKeyQueryValue)
	if err != nil {
		if errors.Is(err, errorFileNotFound) {
			return "", nil
		}
		return "", err
	}
	defer procRegCloseKey.Call(key)

	name, err := syscall.UTF16PtrFromString(e.Value)
	if err != nil {
		return "", err
	}
	var kind uint32
	var size uint32
	result, _, _ := procRegQueryValueExW.Call(
		key,
		uintptr(unsafe.Pointer(name)),
		0,
		uintptr(unsafe.Pointer(&kind)),
		0,
		uintptr(unsafe.Pointer(&size)),
	)
	if syscall.Errno(result) == errorFileNotFound {
		return "", nil
	}
	if result != 0 {
		return "", fmt.Errorf("autostart: read %s\\%s: %w", e.Key, e.Value, syscall.Errno(result))
	}
	if size == 0 {
		return "", nil
	}
	buffer := make([]uint16, (size+1)/2)
	result, _, _ = procRegQueryValueExW.Call(
		key,
		uintptr(unsafe.Pointer(name)),
		0,
		uintptr(unsafe.Pointer(&kind)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if syscall.Errno(result) == errorFileNotFound {
		return "", nil
	}
	if result != 0 {
		return "", fmt.Errorf("autostart: read %s\\%s: %w", e.Key, e.Value, syscall.Errno(result))
	}
	if kind != uint32(regSZ) {
		// A value of another type was not written by this app.
		return "", nil
	}
	return syscall.UTF16ToString(buffer), nil
}

func (e Entry) open(access uintptr) (uintptr, error) {
	path, err := syscall.UTF16PtrFromString(e.Key)
	if err != nil {
		return 0, err
	}
	var key uintptr
	result, _, _ := procRegOpenKeyExW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(path)),
		0,
		access,
		uintptr(unsafe.Pointer(&key)),
	)
	if result != 0 {
		return 0, syscall.Errno(result)
	}
	return key, nil
}

// create opens the key for writing, making it when the tree is not there yet.
// The real Run key always exists; a scratch key in a test does not.
func (e Entry) create() (uintptr, error) {
	path, err := syscall.UTF16PtrFromString(e.Key)
	if err != nil {
		return 0, err
	}
	var key uintptr
	var disposition uint32
	result, _, _ := procRegCreateKeyExW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(path)),
		0,
		0,
		0,
		regKeySetValue,
		0,
		uintptr(unsafe.Pointer(&key)),
		uintptr(unsafe.Pointer(&disposition)),
	)
	if result != 0 {
		return 0, fmt.Errorf("autostart: open %s: %w", e.Key, syscall.Errno(result))
	}
	return key, nil
}

// sameCommand compares two Run commands by executable path, ignoring the quotes
// and the flag, so a path spelled with a different case still matches.
func sameCommand(command, exe string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	first := command
	if strings.HasPrefix(command, `"`) {
		if end := strings.Index(command[1:], `"`); end >= 0 {
			first = command[1 : end+1]
		}
	} else if end := strings.IndexAny(command, " \t"); end >= 0 {
		first = command[:end]
	}
	return strings.EqualFold(filepath.Clean(first), filepath.Clean(exe))
}
