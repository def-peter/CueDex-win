//go:build windows

package sound

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// PlaySound stops whatever it was playing when it is called again, so it can
// only ever carry one stream. A spoken clip has to sound beside the cue chime,
// the way the mac app's NSSound does, so the clip uses waveOut instead: each
// Recorder owns one output handle, and the driver mixes the two.

const (
	waveMapper        = 0xFFFFFFFF // WAVE_MAPPER: the default output device
	waveCallbackEvent = 0x00050000 // CALLBACK_EVENT: signal an event when done
	waveFormatPCM     = 1
	waveInfinite      = 0xFFFFFFFF
)

// waveFormat is WAVEFORMATEX for uncompressed PCM.
type waveFormat struct {
	format         uint16
	channels       uint16
	samplesPerSec  uint32
	avgBytesPerSec uint32
	blockAlign     uint16
	bitsPerSample  uint16
	extraSize      uint16
}

// waveHeader is WAVEHDR, the buffer description waveOut plays from.
type waveHeader struct {
	data          uintptr
	bufferLength  uint32
	bytesRecorded uint32
	user          uintptr
	flags         uint32
	loops         uint32
	next          uintptr
	reserved      uintptr
}

var (
	procWaveOutOpen            = winmm.NewProc("waveOutOpen")
	procWaveOutPrepareHeader   = winmm.NewProc("waveOutPrepareHeader")
	procWaveOutUnprepareHeader = winmm.NewProc("waveOutUnprepareHeader")
	procWaveOutWrite           = winmm.NewProc("waveOutWrite")
	procWaveOutReset           = winmm.NewProc("waveOutReset")
	procWaveOutClose           = winmm.NewProc("waveOutClose")

	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procCreateEvent         = kernel32.NewProc("CreateEventW")
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	procCloseHandle         = kernel32.NewProc("CloseHandle")
)

// Recorder plays one WAV held in memory through its own output handle, so it
// never cuts off the cue chime. It is safe from any goroutine.
type Recorder struct {
	mu     sync.Mutex
	keep   []byte
	device uintptr
	header *waveHeader
	event  uintptr
	// serial invalidates the cleanup of a playback that Stop already ended.
	serial uint64
}

// PlayBytes plays a WAV at volume (0-100), replacing whatever this Recorder was
// playing. Zero is silence, which is not an error. A file that cannot be
// decoded, or a device that will not open, is reported and nothing is heard.
func (r *Recorder) PlayBytes(data []byte, volume int) error {
	if r == nil {
		return nil
	}
	info, err := parseWAV(data)
	if err != nil {
		return err
	}
	if volume < 0 {
		volume = 0
	}
	if volume == 0 {
		r.Stop()
		return nil
	}
	scaled, err := Scaled(data, volume)
	if err != nil {
		return err
	}
	if len(scaled) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// One Recorder carries one stream: a new clip replaces the old one, the
	// same way a new cue replaces the last.
	r.stopLocked()

	event, _, callErr := procCreateEvent.Call(0, 0, 0, 0) // auto-reset, unsignaled
	if event == 0 {
		return fmt.Errorf("sound: create event: %v", callErr)
	}
	format := waveFormat{
		format:         waveFormatPCM,
		channels:       uint16(info.channels),
		samplesPerSec:  uint32(info.rate),
		avgBytesPerSec: uint32(info.byteRate),
		blockAlign:     uint16(info.blockAlign),
		bitsPerSample:  uint16(info.bits),
	}
	var device uintptr
	if code, _, _ := procWaveOutOpen.Call(
		uintptr(unsafe.Pointer(&device)),
		waveMapper,
		uintptr(unsafe.Pointer(&format)),
		event,
		0,
		waveCallbackEvent,
	); code != 0 {
		procCloseHandle.Call(event)
		return fmt.Errorf("sound: open device: waveOut error %d", code)
	}
	header := &waveHeader{
		data:         uintptr(unsafe.Pointer(&scaled[0])),
		bufferLength: uint32(len(scaled)),
	}
	if code, _, _ := procWaveOutPrepareHeader.Call(
		device,
		uintptr(unsafe.Pointer(header)),
		unsafe.Sizeof(*header),
	); code != 0 {
		procWaveOutClose.Call(device)
		procCloseHandle.Call(event)
		return fmt.Errorf("sound: prepare buffer: waveOut error %d", code)
	}
	if code, _, _ := procWaveOutWrite.Call(
		device,
		uintptr(unsafe.Pointer(header)),
		unsafe.Sizeof(*header),
	); code != 0 {
		procWaveOutUnprepareHeader.Call(device, uintptr(unsafe.Pointer(header)), unsafe.Sizeof(*header))
		procWaveOutClose.Call(device)
		procCloseHandle.Call(event)
		return fmt.Errorf("sound: write buffer: waveOut error %d", code)
	}

	r.keep, r.device, r.header, r.event = scaled, device, header, event
	r.serial++
	serial := r.serial
	go r.release(serial, device, event, header)
	return nil
}

// release returns the device once the driver has finished the buffer, or does
// nothing when Stop already took it away.
func (r *Recorder) release(serial uint64, device, event uintptr, header *waveHeader) {
	procWaitForSingleObject.Call(event, waveInfinite)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.serial != serial {
		return
	}
	procWaveOutUnprepareHeader.Call(device, uintptr(unsafe.Pointer(header)), unsafe.Sizeof(*header))
	procWaveOutClose.Call(device)
	procCloseHandle.Call(event)
	r.keep, r.device, r.header, r.event = nil, 0, nil, 0
}

// Stop ends whatever this Recorder is playing.
func (r *Recorder) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopLocked()
}

func (r *Recorder) stopLocked() {
	if r.device == 0 {
		return
	}
	// Reset marks the queued buffer as done, so it can be unprepared at once.
	procWaveOutReset.Call(r.device)
	procWaveOutUnprepareHeader.Call(r.device, uintptr(unsafe.Pointer(r.header)), unsafe.Sizeof(*r.header))
	procWaveOutClose.Call(r.device)
	procCloseHandle.Call(r.event)
	r.serial++
	r.keep, r.device, r.header, r.event = nil, 0, nil, 0
}
