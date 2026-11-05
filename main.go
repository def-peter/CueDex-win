package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"

	"github.com/def-peter/cuedex-win/internal/app"
	"github.com/def-peter/cuedex-win/internal/paths"
)

func main() {
	opts := parseArgs(os.Args[1:])

	mygo.App.SetName("CueDex")
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)

	// A second launch hands its arguments to the running instance and ends
	// there, so only one tray icon and one event consumer ever exist.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}

	dir, err := paths.UserData()
	if err != nil {
		log.Fatal(err)
	}
	if err := dir.Prepare(); err != nil {
		log.Fatal(err)
	}

	a := app.New(dir)
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if parseArgs(args).cue {
			a.Cue()
		}
	})

	mygo.App.WhenReady(func() {
		a.Ready()
		if opts.cue {
			a.Cue()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// options are the launch arguments the app understands.
type options struct {
	// cue asks the running app to show a cue now (PROTOCOL.md).
	cue bool
	// background is how the startup entry launches the app: tray only, no
	// panel, nothing that looks like a launch.
	background bool
}

func parseArgs(args []string) options {
	var o options
	for _, arg := range args {
		switch arg {
		case "--cue":
			o.cue = true
		case "--background":
			o.background = true
		}
	}
	return o
}
