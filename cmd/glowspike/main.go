// Glowspike draws CueDex's screen-edge cue on every display for three
// seconds and quits: the spike that decides whether the product works on
// Windows at all.
//
//	go run ./cmd/glowspike -anim breathing
//	go run ./cmd/glowspike -anim flash -dur 5s -delay 2s
//
// Each display gets one frameless, transparent, topmost window the size of
// its bounds, held out of the taskbar and Alt+Tab, which the pointer goes
// through. Nothing is drawn between cues, and the windows never take focus.
//
// The windows are created first and stay idle for -delay, so the numbers the
// spike prints are those of a running app that is asked to cue: the latency
// from the cue to its first frame, the idle cost before it, and how many
// frames it drew.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// cue is one display's overlay window and what it draws.
type cue struct {
	window *mygo.Window
	glow   *glow
}

func main() {
	dur := flag.Duration("dur", 3*time.Second, "how long the cue glows")
	delay := flag.Duration("delay", time.Second, "how long the windows stay idle before the cue")
	stay := flag.Duration("stay", 0, "how long the windows stay alive, painting nothing, after the cue")
	destroy := flag.Bool("destroy", false, "destroy the overlay windows when the cue ends")
	hide := flag.Bool("hide", false, "keep the overlay windows hidden while there is no cue")
	anim := flag.String("anim", "breathing", "animation: breathing or flash")
	flag.Parse()

	created := time.Now()
	cueAt := created.Add(*delay)
	cues := []*cue{}

	mygo.App.WhenReady(func() {
		rects := monitors()
		fmt.Fprintf(os.Stderr, "glowspike: %d display(s) by Win32, %d by MyGo, animation %s, cue at +%s for %s\n",
			len(rects), len(mygo.Screen.Displays()), *anim, *delay, *dur)
		for i, r := range rects {
			g := newGlow(r, cueAt, *dur, *anim == "flash")
			w := mygo.NewWindow(mygo.WindowOptions{
				Title:             "CueDex Glow",
				X:                 int(r.Left),
				Y:                 int(r.Top),
				Width:             int(r.width()),
				Height:            int(r.height()),
				Transparent:       true,
				Frameless:         true,
				AlwaysOnTop:       true,
				SkipTaskbar:       true,
				Panel:             true,
				DisableShadow:     true,
				DisableResize:     true,
				DisableMinimize:   true,
				DisableMaximize:   true,
				DisableClose:      true,
				DisableFullScreen: true,
				// DisableMove must stay false: MyGo answers every
				// WM_WINDOWPOSCHANGING with SWP_NOMOVE while a window is
				// not movable, its first placement included, which leaves
				// it at 0,0. A cue is click-through, so nobody can drag it
				// anyway.
				Content: ui.View(g.view),
			})
			if w == nil {
				fmt.Fprintf(os.Stderr, "glowspike: no window on display %d at %v\n", i, r)
				continue
			}
			// MyGo's options place a window in DIPs of the primary display,
			// which misses a display of another scaling: set the geometry
			// it has to cover, in pixels, above every other window.
			placed := setWindowRect(w.NativeHandle(), r, true)
			// Every click, wheel and hover belongs to what is behind.
			w.SetIgnoreMouseEvents(true)
			cues = append(cues, &cue{window: w, glow: g})
			got, _ := windowRect(w.NativeHandle())
			fmt.Fprintf(os.Stderr, "glowspike: display %d want %v placed=%v got %v dpi=%d\n",
				i, r, placed, got, windowDPI(w.NativeHandle()))
		}
		if *hide {
			for _, c := range cues {
				c.window.Hide()
			}
			// A hidden window paints nothing, so the cue shows it just
			// before it starts and lets the first frame follow.
			time.AfterFunc(max(*delay-50*time.Millisecond, 0), func() {
				for _, c := range cues {
					c.window.ShowInactive()
				}
			})
		}

		// What the windows settle to once Windows has placed them, which
		// is where they are when the cue arrives.
		time.AfterFunc(300*time.Millisecond, func() {
			for i, c := range cues {
				r, _ := windowRect(c.window.NativeHandle())
				fmt.Fprintf(os.Stderr, "glowspike: cue %d settled %v\n", i, r)
			}
		})

		time.AfterFunc(*delay+*dur, func() {
			for i, c := range cues {
				if c.glow.first.IsZero() {
					fmt.Fprintf(os.Stderr, "glowspike: cue %d never drew\n", i)
					continue
				}
				fmt.Fprintf(os.Stderr, "glowspike: cue %d first frame %v after the cue, %d frames, gpu=%v\n",
					i, c.glow.first.Sub(cueAt).Round(time.Millisecond), c.glow.frames, c.glow.gpu)
			}
			if *destroy {
				for _, c := range cues {
					c.window.Destroy()
				}
				cues = nil
			}
			if *hide {
				for _, c := range cues {
					c.window.Hide()
				}
			}
			time.AfterFunc(*stay, func() { mygo.App.Quit() })
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
