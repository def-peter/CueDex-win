//go:build windows

package sound

import (
	"syscall"
	"unsafe"
)

const (
	sndAsync     = 0x0001
	sndNoDefault = 0x0002 // do not fall back to the system beep
	sndMemory    = 0x0004
)

var (
	winmm         = syscall.NewLazyDLL("winmm.dll")
	procPlaySound = winmm.NewProc("PlaySoundW")
)

// play asks the device to run the WAV in memory. The call returns at once and
// the bytes must stay valid until playback ends, which Player guarantees.
func play(data *byte, size int) bool {
	r, _, _ := procPlaySound.Call(
		uintptr(unsafe.Pointer(data)),
		0,
		sndMemory|sndAsync|sndNoDefault,
	)
	return r != 0
}

// silence stops playback.
func silence() { procPlaySound.Call(0, 0, 0) }
