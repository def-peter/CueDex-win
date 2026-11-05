package speech

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestVoicesEnumeratesInstalledVoices(t *testing.T) {
	if testing.Short() {
		t.Skip("COM enumeration is part of the integration check")
	}
	engine := New()
	defer engine.Close()

	voices, err := engine.Voices()
	if errors.Is(err, ErrNoVoices) {
		t.Skip("this machine has no SAPI voice")
	}
	if err != nil {
		t.Fatalf("Voices: %v", err)
	}
	if len(voices) == 0 {
		t.Fatal("no voices and no error")
	}
	for _, v := range voices {
		if v.ID == "" {
			t.Errorf("voice with no id: %+v", v)
		}
		if v.Name == "" {
			t.Errorf("%s has no name", v.ID)
		}
		if v.Language != "" && !strings.Contains(v.Label(), v.Language) {
			t.Errorf("%s label %q omits the language", v.ID, v.Label())
		}
	}

	// A second call answers from the same thread without enumerating again.
	again, err := engine.Voices()
	if err != nil || len(again) != len(voices) {
		t.Fatalf("second call %v, %v", again, err)
	}
}

func TestConcurrentVoiceListingIsSafe(t *testing.T) {
	engine := New()
	defer engine.Close()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = engine.Voices()
		}()
	}
	wg.Wait()
}

func TestSpeakOfAnEmptyPhraseDoesNothing(t *testing.T) {
	engine := New()
	defer engine.Close()
	if err := engine.Speak("", "   "); err != nil {
		t.Fatalf("empty phrase: %v", err)
	}
}

func TestStopAndCloseAreSafeWithoutSpeech(t *testing.T) {
	engine := New()
	engine.Stop()
	engine.Close()
	engine.Close() // a second close is harmless
	if _, err := engine.Voices(); err == nil {
		t.Fatal("a closed engine still enumerates")
	}
}

func TestLanguageTag(t *testing.T) {
	for raw, want := range map[string]string{
		"804": "zh-CN",
		"409": "en-US",
		"0":   "0",
		"":    "",
	} {
		if got := languageTag(raw); got != want {
			t.Errorf("languageTag(%q) = %q, want %q", raw, got, want)
		}
	}
}
