// Package app is the tray application: one icon, one settings panel, and the
// cue entry point that producers reach through "CueDex.exe --cue".
package app

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/integrations"
	"github.com/def-peter/cuedex-win/internal/overlay"
	"github.com/def-peter/cuedex-win/internal/paths"
)

// App owns the tray icon, the settings panel, and everything the panels show.
// Its state is read and changed on the main thread only.
type App struct {
	dir     paths.Dir
	tray    *mygo.Tray
	win     *mygo.Window
	overlay *overlay.Overlay
	watch   *events.Watcher
	codex   integrations.CodexIntegration
	state   settings
}

// New returns an app that keeps its files in dir and installs hooks that run
// exe.
func New(dir paths.Dir, exe string) *App {
	home, err := integrations.CodexHome()
	if err != nil {
		home = ""
	}
	a := &App{dir: dir, codex: integrations.CodexIntegration{Home: home, Exe: exe}}
	a.state.onTogglePause = a.togglePause
	a.state.onClose = a.hide
	a.state.onToggleCodex = a.toggleCodex
	return a
}

// Ready builds the tray and the panel. It runs once, on the main thread.
func (a *App) Ready() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "cuedex",
		Icon:    trayIcon(),
		ToolTip: "CueDex",
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			{Label: "打开设置", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
			{
				Label: "暂停提示",
				Type:  mygo.MenuItemCheckbox,
				Click: func(item *mygo.MenuItem, _ *mygo.Window) { a.setPaused(item.Checked) },
			},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}),
	})
	if err != nil {
		// Without a tray the app has no way to be operated; failing loudly
		// beats a process that is running and unreachable.
		panic(err)
	}
	a.tray = tray

	// The overlay is four layered windows on the primary display. It costs a
	// few hundred kilobytes idle, unlike a MyGo window.
	cue, err := overlay.Start()
	if err != nil {
		log.Printf("cuedex: %v", err)
		a.state.problem = "提示层不可用：" + err.Error()
	} else {
		a.overlay = cue
	}

	watch, err := events.Watch(a.dir.Events(), func(e events.Event) {
		// The watcher has its own goroutine; the app is read and changed on
		// the main thread only.
		mygo.RunOnMain(func() { a.pulse(e.Source) })
	})
	if err != nil {
		log.Printf("cuedex: watch %s: %v", a.dir.Events(), err)
		a.state.problem = "事件目录不可用：" + err.Error()
	} else {
		a.watch = watch
	}
	a.refreshCodex()

	// A tray app outlives every window it has: closing the panel must not
	// end the process.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.watch != nil {
			a.watch.Close()
		}
		if a.overlay != nil {
			a.overlay.Close()
		}
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
}

// newWindow builds the settings panel. A MyGo window costs about 80 MB
// whether or not it is visible, so the panel exists only while the user has
// it open: the app sits near the bare event loop otherwise.
func (a *App) newWindow() *mygo.Window {
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:             "CueDex",
		Width:             420,
		Height:            360,
		Frameless:         true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		Content:           ui.View(a.state.view),
	})
	// Losing focus closes the panel, as a tray popover does, and gives its
	// memory back.
	w.OnBlur(func() {
		w.Close()
	})
	w.OnClosed(func() {
		if a.win == w {
			a.win = nil
		}
	})
	return w
}

// Cue is the wake a producer asks for with "CueDex.exe --cue". The file that
// came with it is read by the watcher, whose own cue restarts in the
// producer's colour a moment later.
func (a *App) Cue() { a.pulse("") }

// pulse shows one cue. An empty source is a bare wake, from a producer that
// asked the app to cue without leaving a file the app could read.
func (a *App) pulse(source events.Source) {
	if a.state.paused {
		return
	}
	a.state.cueCount++
	a.state.lastCue = time.Now()
	if source != "" {
		a.state.lastSource = source
	}
	if a.overlay != nil {
		a.overlay.Show(sourceColour(source))
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) setPaused(paused bool) {
	a.state.paused = paused
	a.invalidate()
}

func (a *App) togglePause() { a.setPaused(!a.state.paused) }

func (a *App) show() {
	if a.win == nil {
		a.win = a.newWindow()
	}
	a.win.Show()
}

func (a *App) hide() {
	if a.win != nil {
		a.win.Close()
	}
}

// refreshCodex reads the hooks file so the panel says what is actually there.
func (a *App) refreshCodex() {
	a.state.codexDetected = a.codex.Home != "" && a.codex.Detected()
	installed, err := a.codex.Installed()
	a.state.codexInstalled = installed
	a.state.codexProblem = ""
	if err != nil {
		a.state.codexProblem = codexProblem(err)
	}
}

// toggleCodex installs or removes the Stop hook the way the panel's button
// asks. Both operations rewrite one file and never touch anything else in it.
func (a *App) toggleCodex() {
	var err error
	if a.state.codexInstalled {
		err = a.codex.Uninstall()
	} else {
		err = a.codex.Install()
	}
	a.refreshCodex()
	if err != nil {
		a.state.codexProblem = codexProblem(err)
	}
	a.invalidate()
}

// codexProblem says why the integration could not be changed, in the words the
// panel shows.
func codexProblem(err error) string {
	if errors.Is(err, integrations.ErrInvalidHooks) {
		return "hooks.json 结构异常，已停止写入，你的配置没有被改动"
	}
	return "Codex 集成操作失败：" + err.Error()
}

func (a *App) invalidate() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

// sourceColour is each producer's colour: Codex is the product's own teal,
// OpenCode a violet that stays legible on a light and a dark desktop.
func sourceColour(source events.Source) color.NRGBA {
	if source == events.OpenCode {
		return color.NRGBA{R: 167, G: 139, B: 250, A: 255}
	}
	return color.NRGBA{R: 94, G: 234, B: 212, A: 255}
}

// trayIcon draws the product: four glowing edges around an empty middle.
func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	glow := color.NRGBA{R: 94, G: 234, B: 212, A: 255}
	for _, r := range []image.Rectangle{
		image.Rect(2, 2, 30, 5),
		image.Rect(2, 27, 30, 30),
		image.Rect(2, 5, 5, 27),
		image.Rect(27, 5, 30, 27),
	} {
		draw.Draw(img, r, image.NewUniform(glow), image.Point{}, draw.Src)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil
	}
	return b.Bytes()
}
