// Command overlayspike draws the cue animations on every display, so the
// geometry and the cost of an animation can be measured without a producer. It
// writes no CueDex state.
//
//	go run ./cmd/overlayspike
//	go run ./cmd/overlayspike -animation flash -repeat 10s
package main

import (
	"flag"
	"fmt"
	"image/color"
	"time"

	"github.com/def-peter/cuedex-win/internal/overlay"
)

func main() {
	animation := flag.String("animation", "both", "breathing, flash, or both in turn")
	repeat := flag.Duration("repeat", 0, "keep cueing for this long; 0 shows each animation once")
	flag.Parse()

	o, err := overlay.Start()
	if err != nil {
		fmt.Println("start:", err)
		return
	}
	defer o.Close()

	cues := map[string]overlay.Cue{
		"breathing": {
			Primary:   color.NRGBA{R: 94, G: 234, B: 212, A: 255},
			Secondary: color.NRGBA{R: 96, G: 165, B: 250, A: 255},
			Animation: overlay.Breathing,
		},
		"flash": {
			Primary:   color.NRGBA{R: 167, G: 139, B: 250, A: 255},
			Secondary: color.NRGBA{R: 244, G: 114, B: 182, A: 255},
			Animation: overlay.AlternatingFlash,
		},
	}

	var order []string
	switch *animation {
	case "breathing", "flash":
		order = []string{*animation}
	case "both":
		order = []string{"breathing", "flash"}
	default:
		fmt.Println("unknown animation:", *animation)
		return
	}

	deadline := time.Now().Add(*repeat)
	for {
		for _, name := range order {
			fmt.Println("showing", name)
			o.Show(cues[name])
			if *repeat == 0 {
				time.Sleep(overlay.CueDuration + 300*time.Millisecond)
				continue
			}
			time.Sleep(overlay.CueDuration)
			if time.Now().After(deadline) {
				return
			}
		}
		if *repeat == 0 {
			return
		}
	}
}
