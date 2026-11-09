package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"

	"github.com/def-peter/cuedex-win/internal/app"
	"github.com/def-peter/cuedex-win/internal/hook"
	"github.com/def-peter/cuedex-win/internal/paths"
)

func main() {
	opts := parseArgs(os.Args[1:])

	// A host's hook runs and ends here: no tray, no single-instance lock, and
	// nothing on stdout or stderr that could disturb the host.
	if opts.hook != "" {
		runHook(opts.hook)
		return
	}

	// The desktop app's notify line runs this: the payload is the argument,
	// not stdin, and no tray or window may appear.
	if opts.notify != "" {
		runNotify(opts.notify)
		return
	}

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

	a := app.New(dir, executable())
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
		// A launch by the user opens the panel. The startup entry's
		// --background launch stays in the tray, and a --cue wake from a
		// producer never flashes a window: both only cue.
		if !opts.background && !opts.cue {
			a.Open()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// executable is this process's own path. The hook line and the wake both spell
// it out, so a copied or moved app still points at itself.
func executable() string {
	exe, err := os.Executable()
	if err != nil {
		return os.Args[0]
	}
	return exe
}

// runHook runs the entry a producer's hook calls. It never reports failure: a
// hook that writes to stderr or exits nonzero can disturb the host that ran it,
// and the next turn will try again.
func runHook(producer string) {
	dir, err := paths.UserData()
	if err != nil {
		return
	}
	if err := dir.Prepare(); err != nil {
		return
	}
	switch producer {
	case "codex":
		wrote, err := hook.Codex(os.Stdin, dir.Events())
		if err == nil && wrote {
			_ = hook.Wake(executable())
		}
	}
}

// runNotify runs the entry Codex's legacy notify line calls: the payload is the
// argument, and the same silence applies.
func runNotify(payload string) {
	dir, err := paths.UserData()
	if err != nil {
		return
	}
	if err := dir.Prepare(); err != nil {
		return
	}
	wrote, err := hook.Notify([]byte(payload), dir.Events())
	if err == nil && wrote {
		_ = hook.Wake(executable())
	}
}

// options are the launch arguments the app understands.
type options struct {
	// cue asks the running app to show a cue now (PROTOCOL.md).
	cue bool
	// background is how the startup entry launches the app: tray only, no
	// panel, nothing that looks like a launch.
	background bool
	// hook names the producer whose hook this process is. Empty means this is
	// the app itself.
	hook string
	// notify carries Codex's legacy notify payload, which arrives as an
	// argument rather than on stdin.
	notify string
}

func parseArgs(args []string) options {
	var o options
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cue":
			o.cue = true
		case "--background":
			o.background = true
		case "--hook":
			if i+1 < len(args) {
				o.hook = args[i+1]
				i++
			}
		case "--notify":
			if i+1 < len(args) {
				o.notify = args[i+1]
				i++
			}
		}
	}
	return o
}
