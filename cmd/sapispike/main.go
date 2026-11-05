// Command sapispike checks what SAPI offers on this machine: the installed
// voices, their languages, and whether a phrase is actually heard. It writes
// no CueDex state; it exists to record the spike result in HANDOFF.md.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/speech"
)

func main() {
	engine := speech.New()
	defer engine.Close()

	voices, err := engine.Voices()
	fmt.Printf("voices: %d, err: %v\n", len(voices), err)
	for _, v := range voices {
		fmt.Printf("  %s | %s | %s\n", v.ID, v.Name, v.Language)
	}
	if len(os.Args) > 1 && os.Args[1] == "speak" {
		phrase := speech.Phrase(events.Codex)
		fmt.Printf("speaking %q\n", phrase)
		if err := engine.Speak("", phrase); err != nil {
			fmt.Println("speak:", err)
			return
		}
		time.Sleep(3 * time.Second)
	}
}
