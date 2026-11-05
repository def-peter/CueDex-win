// Package sound plays the cue sounds that ship with the app. The four WAVs are
// embedded, so a cue sounds the same on every machine and never depends on a
// system sound happening to be installed.
package sound

import (
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"
)

// The four sounds are short on purpose: a cue is three seconds long, and the
// sound must end inside it.
//
//go:embed wav/*.wav
var assets embed.FS

// Sound is one bundled cue sound. ID is what preferences store; Label is what
// the panel shows.
type Sound struct {
	ID    string
	Label string
}

// catalog is in the order the panel shows. IDs never change, so a preference
// saved by an older build still names the same sound.
var catalog = []Sound{
	{ID: "rise", Label: "上扬"},
	{ID: "fall", Label: "下沉"},
	{ID: "ping", Label: "清音"},
	{ID: "knock", Label: "双击"},
}

// All returns the bundled sounds in panel order.
func All() []Sound { return append([]Sound(nil), catalog...) }

// Labels returns the panel's names, in the same order as All.
func Labels() []string {
	labels := make([]string, len(catalog))
	for i, s := range catalog {
		labels[i] = s.Label
	}
	return labels
}

// Lookup finds a sound by ID.
func Lookup(id string) (Sound, bool) {
	for _, s := range catalog {
		if s.ID == id {
			return s, true
		}
	}
	return Sound{}, false
}

// Bytes returns the embedded WAV.
func (s Sound) Bytes() ([]byte, error) {
	return assets.ReadFile("wav/" + s.ID + ".wav")
}

// ErrUnsupported means the file is not a PCM 16-bit WAV, which is the only
// shape the bundled sounds have. A file of another shape is refused rather
// than played at the wrong speed.
var ErrUnsupported = errors.New("sound: not a PCM 16-bit WAV")

// Player plays one sound at a time: a new cue stops whatever was still
// sounding. It is safe from any goroutine.
type Player struct {
	mu sync.Mutex
	// keep holds the bytes the device is reading from; the memory has to
	// outlive the asynchronous playback.
	keep []byte
}

// Play sounds id at volume (0-100). Zero is silence, which is not an error.
// A sound that cannot be resolved or decoded is reported and nothing is heard,
// so a broken asset never turns into a missing cue.
func (p *Player) Play(id string, volume int) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
	if volume <= 0 {
		return nil
	}
	s, ok := Lookup(id)
	if !ok {
		return fmt.Errorf("sound: unknown sound %q", id)
	}
	data, err := s.Bytes()
	if err != nil {
		return fmt.Errorf("sound: %s: %w", id, err)
	}
	data, err = Scaled(data, volume)
	if err != nil {
		return fmt.Errorf("sound: %s: %w", id, err)
	}
	p.keep = data
	if !play(&data[0], len(data)) {
		p.keep = nil
		return fmt.Errorf("sound: %s could not be played", id)
	}
	return nil
}

// Stop silences whatever is sounding.
func (p *Player) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *Player) stopLocked() {
	if p.keep == nil {
		return
	}
	silence()
	p.keep = nil
}

// wav says where the samples are and how loud to make them. Only the data
// chunk is touched, so a WAV with other chunks keeps them as they were.
type wav struct {
	data       int
	length     int
	byteRate   int
	blockAlign int
}

func parseWAV(b []byte) (wav, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return wav{}, ErrUnsupported
	}
	var (
		found   wav
		haveFmt bool
		haveDat bool
	)
	for offset := 12; offset+8 <= len(b); {
		id := string(b[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(b[offset+4:]))
		body := offset + 8
		if size < 0 || body+size > len(b) {
			return wav{}, ErrUnsupported
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return wav{}, ErrUnsupported
			}
			format := binary.LittleEndian.Uint16(b[body:])
			channels := int(binary.LittleEndian.Uint16(b[body+2:]))
			rate := int(binary.LittleEndian.Uint32(b[body+4:]))
			byteRate := int(binary.LittleEndian.Uint32(b[body+8:]))
			blockAlign := int(binary.LittleEndian.Uint16(b[body+12:]))
			bits := int(binary.LittleEndian.Uint16(b[body+14:]))
			if format != 1 || channels < 1 || rate <= 0 || bits != 16 || blockAlign < 2 {
				return wav{}, ErrUnsupported
			}
			found.byteRate, found.blockAlign = byteRate, blockAlign
			haveFmt = true
		case "data":
			found.data, found.length = body, size
			haveDat = true
		}
		offset = body + size
		if size%2 == 1 {
			offset++
		}
	}
	if !haveFmt || !haveDat || found.byteRate <= 0 {
		return wav{}, ErrUnsupported
	}
	return found, nil
}

// Scaled returns a copy of wav at the given volume, 0 being silent and 100 the
// file unchanged. Volume is clamped, so an out-of-range preference still plays.
func Scaled(wavBytes []byte, volume int) ([]byte, error) {
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	info, err := parseWAV(wavBytes)
	if err != nil {
		return nil, err
	}
	out := append([]byte(nil), wavBytes...)
	if volume == 100 {
		return out, nil
	}
	for i := info.data; i+1 < info.data+info.length; i += 2 {
		sample := int32(int16(binary.LittleEndian.Uint16(out[i:])))
		binary.LittleEndian.PutUint16(out[i:], uint16(int16(sample*int32(volume)/100)))
	}
	return out, nil
}

// Duration reports how long a WAV sounds, so a test can check the bundled
// sounds fit inside a cue.
func Duration(wavBytes []byte) (time.Duration, error) {
	info, err := parseWAV(wavBytes)
	if err != nil {
		return 0, err
	}
	return time.Duration(int64(info.length) * int64(time.Second) / int64(info.byteRate)), nil
}
