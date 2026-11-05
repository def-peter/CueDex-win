package sound

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

func TestCatalogHasFourSounds(t *testing.T) {
	all := All()
	if len(all) < 4 {
		t.Fatalf("%d bundled sounds, want at least four", len(all))
	}
	seen := map[string]bool{}
	for _, s := range all {
		if s.ID == "" || s.Label == "" {
			t.Fatalf("sound with no id or label: %+v", s)
		}
		if seen[s.ID] {
			t.Fatalf("duplicate sound %q", s.ID)
		}
		seen[s.ID] = true
		if _, ok := Lookup(s.ID); !ok {
			t.Errorf("%q cannot be looked up", s.ID)
		}
	}
	if len(Labels()) != len(all) {
		t.Fatalf("%d labels for %d sounds", len(Labels()), len(all))
	}
	if _, ok := Lookup("nope"); ok {
		t.Fatal("an unknown id was found")
	}
}

func TestBundledSoundsFitInsideACue(t *testing.T) {
	// A cue is three seconds; a sound that ran past it would still be
	// sounding while the next cue starts, since a new cue stops it.
	for _, s := range All() {
		data, err := s.Bytes()
		if err != nil {
			t.Fatalf("%s: %v", s.ID, err)
		}
		duration, err := Duration(data)
		if err != nil {
			t.Fatalf("%s: %v", s.ID, err)
		}
		if duration <= 0 || duration >= 3*time.Second {
			t.Errorf("%s lasts %v, want under three seconds", s.ID, duration)
		}
	}
}

func TestScaledLeavesVolumeHundredAlone(t *testing.T) {
	wav := testWAV([]int16{1000, -1000, 32767, -32768})
	scaled, err := Scaled(wav, 100)
	if err != nil {
		t.Fatal(err)
	}
	if string(scaled) != string(wav) {
		t.Fatalf("volume 100 changed the file")
	}
}

func TestScaledMultipliesSamples(t *testing.T) {
	wav := testWAV([]int16{1000, -1000, 100, -100})
	scaled, err := Scaled(wav, 50)
	if err != nil {
		t.Fatal(err)
	}
	want := []int16{500, -500, 50, -50}
	got := samplesOf(t, scaled)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
	// Everything outside the data chunk is untouched.
	if string(scaled[:44]) != string(wav[:44]) {
		t.Fatal("the header changed")
	}
}

func TestScaledZeroIsSilent(t *testing.T) {
	wav := testWAV([]int16{1000, -1000, 20000})
	scaled, err := Scaled(wav, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range samplesOf(t, scaled) {
		if sample != 0 {
			t.Fatalf("silence has %d in it", sample)
		}
	}
}

func TestScaledClampsTheVolume(t *testing.T) {
	wav := testWAV([]int16{1000, -1000})
	quiet, err := Scaled(wav, -20)
	if err != nil {
		t.Fatal(err)
	}
	loud, err := Scaled(wav, 400)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range samplesOf(t, quiet) {
		if sample != 0 {
			t.Fatalf("a negative volume is not silent: %d", sample)
		}
	}
	if string(loud) != string(wav) {
		t.Fatal("a volume above 100 is not the original")
	}
}

func TestScaledKeepsOtherChunks(t *testing.T) {
	wav := testWAV([]int16{1000})
	// Append a chunk after the data one; it has to survive untouched.
	extra := append([]byte{}, []byte("LIST\x04\x00\x00\x00abcd")...)
	withExtra := append(append([]byte{}, wav...), extra...)
	binary.LittleEndian.PutUint32(withExtra[4:], uint32(len(withExtra)-8))
	scaled, err := Scaled(withExtra, 50)
	if err != nil {
		t.Fatal(err)
	}
	if string(scaled[len(scaled)-len(extra):]) != string(extra) {
		t.Fatal("the trailing chunk changed")
	}
}

func TestScaledRefusesAnotherShape(t *testing.T) {
	eightBit := testWAV([]int16{1000})
	binary.LittleEndian.PutUint16(eightBit[34:], 8) // claim 8-bit samples
	for name, data := range map[string][]byte{
		"empty":     nil,
		"not a wav": []byte("plain text, not a sound"),
		"8-bit":     eightBit,
		"truncated": testWAV([]int16{1, 2})[:20],
	} {
		if _, err := Scaled(data, 50); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err %v, want ErrUnsupported", name, err)
		}
	}
}

func TestPlayerRefusesAnUnknownSound(t *testing.T) {
	// Play stops before it looks the sound up, so a headless test touches no
	// device until the WAV is valid.
	p := &Player{}
	if err := p.Play("nope", 50); err == nil {
		t.Fatal("an unknown sound was accepted")
	}
	if err := p.Play("rise", 0); err != nil {
		t.Fatalf("volume zero: %v", err)
	}
}

// testWAV builds a small PCM 16-bit mono file.
func testWAV(samples []int16) []byte {
	const rate = 44100
	data := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(s))
	}
	header := make([]byte, 44)
	copy(header[0:], "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(36+len(data)))
	copy(header[8:], "WAVE")
	copy(header[12:], "fmt ")
	binary.LittleEndian.PutUint32(header[16:], 16)
	binary.LittleEndian.PutUint16(header[20:], 1)
	binary.LittleEndian.PutUint16(header[22:], 1)
	binary.LittleEndian.PutUint32(header[24:], rate)
	binary.LittleEndian.PutUint32(header[28:], rate*2)
	binary.LittleEndian.PutUint16(header[32:], 2)
	binary.LittleEndian.PutUint16(header[34:], 16)
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], uint32(len(data)))
	return append(header, data...)
}

func samplesOf(t *testing.T, wav []byte) []int16 {
	t.Helper()
	info, err := parseWAV(wav)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]int16, info.length/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(wav[info.data+i*2:]))
	}
	return out
}
