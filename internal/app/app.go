// Package app is the tray application: one icon, one settings panel, and the
// cue entry point that producers reach through "CueDex.exe --cue".
package app

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/def-peter/cuedex-win/internal/paths"
)

// App owns the tray icon, the settings panel, and everything the panels show.
// Its state is read and changed on the main thread only.
type App struct {
	dir   paths.Dir
	tray  *mygo.Tray
	win   *mygo.Window
	state settings
}

// New returns an app that keeps its files in dir.
func New(dir paths.Dir) *App {
	a := &App{dir: dir}
	a.state.onTogglePause = a.togglePause
	a.state.onClose = a.hide
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

	// A tray app outlives every window it has: closing the panel must not
	// end the process.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
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
		Height:            280,
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

// Cue is what a producer asks for: show the attention cue now. It is called
// on the main thread, either from the first launch's arguments or from a
// second launch that handed its arguments to this instance.
func (a *App) Cue() {
	if a.state.paused {
		return
	}
	a.state.cueCount++
	a.state.lastCue = time.Now()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) setPaused(paused bool) {
	a.state.paused = paused
	if a.win != nil {
		a.win.Invalidate()
	}
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
