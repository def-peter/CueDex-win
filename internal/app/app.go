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
	"github.com/def-peter/cuedex-win/internal/playback"
	"github.com/def-peter/cuedex-win/internal/prefs"
	"github.com/def-peter/cuedex-win/internal/sound"
)

// audio is the part of the sound player the app uses, so a test can hear what
// it was asked to play without a device.
type audio interface {
	Play(id string, volume int) error
	Stop()
}

// App owns the tray icon, the settings panel, and everything the panels show.
// Its state is read and changed on the main thread only.
type App struct {
	dir      paths.Dir
	tray     *mygo.Tray
	win      *mygo.Window
	overlay  *overlay.Overlay
	watch    *events.Watcher
	player   *playback.Player
	audio    audio
	codex    integrations.CodexIntegration
	opencode integrations.OpenCodeIntegration
	prefs    prefs.Preferences
	// prefsPath is where the choices are saved.
	prefsPath string
	// saveTimer debounces the save while a slider is being dragged.
	saveTimer *time.Timer
	state     settings
}

// New returns an app that keeps its files in dir and installs hooks that run
// exe.
func New(dir paths.Dir, exe string) *App {
	home, err := integrations.CodexHome()
	if err != nil {
		home = ""
	}
	configDir, err := integrations.OpenCodeConfigDir()
	if err != nil {
		configDir = ""
	}
	stored, err := prefs.Load(dir.Preferences())
	if err != nil {
		log.Printf("cuedex: %v", err)
	}
	a := &App{
		dir:   dir,
		codex: integrations.CodexIntegration{Home: home, Exe: exe},
		opencode: integrations.OpenCodeIntegration{
			ConfigDir: configDir,
			EventsDir: dir.Events(),
			GatePath:  dir.OpenCodeGate(),
			Exe:       exe,
		},
		audio:     &sound.Player{},
		prefs:     stored,
		prefsPath: dir.Preferences(),
	}
	a.state.applySound(stored)
	// The player decides when a cue runs; the drawing and the counters are the
	// main thread's, so its callbacks marshal there.
	a.player = playback.New(
		func(source events.Source) { mygo.RunOnMain(func() { a.cueStart(source) }) },
		func() { mygo.RunOnMain(a.cueStop) },
	)
	a.state.onTogglePause = a.togglePause
	a.state.onClose = a.hide
	a.state.onToggleCodex = a.toggleCodex
	a.state.onToggleOpenCode = a.toggleOpenCode
	a.state.onChangeSound = a.changeSound
	a.state.onPreviewSound = a.previewSound
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

	watch, err := events.Watch(a.dir.Events(), func(e events.Event) { a.player.Submit(e) })
	if err != nil {
		log.Printf("cuedex: watch %s: %v", a.dir.Events(), err)
		a.state.problem = "事件目录不可用：" + err.Error()
	} else {
		a.watch = watch
	}
	a.refreshCodex()
	a.refreshOpenCode()

	// Asking OpenCode for its version starts a process, so it happens off the
	// main thread; the answer arrives before the user can press the button in
	// almost every case.
	go func() {
		version, _ := integrations.DetectVersion()
		mygo.RunOnMain(func() {
			a.opencode.Version = version
			a.refreshOpenCode()
			a.invalidate()
		})
	}()

	// A tray app outlives every window it has: closing the panel must not
	// end the process.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.watch != nil {
			a.watch.Close()
		}
		a.player.Close()
		if a.overlay != nil {
			a.overlay.Close()
		}
		a.audio.Stop()
		if a.saveTimer != nil {
			a.saveTimer.Stop()
			a.saveTimer = nil
		}
		a.savePrefs()
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
		Height:            660,
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

// Cue is the wake a producer asks for with "CueDex.exe --cue". The cue itself
// comes from the file the producer left, so the wake only asks the watcher to
// look again: a notification that was missed must not lose a completion.
func (a *App) Cue() { a.watch.Nudge() }

// cueStart draws one cue and records it. The player calls it on the main
// thread, once per cue, never while another one is running.
func (a *App) cueStart(source events.Source) {
	a.state.cueCount++
	a.state.lastCue = time.Now()
	a.state.lastSource = source
	if a.overlay != nil {
		a.overlay.Show(sourceColour(source))
	}
	choice := a.prefs.For(source)
	if a.audio != nil {
		if err := a.audio.Play(choice.ID, choice.Volume); err != nil {
			log.Printf("cuedex: %v", err)
			a.state.soundProblem = "提示音播放失败：" + err.Error()
		}
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

// cueStop takes a cue off the screen when the user pauses.
func (a *App) cueStop() {
	if a.overlay != nil {
		a.overlay.Hide()
	}
	if a.audio != nil {
		a.audio.Stop()
	}
}

// changeSound stores the choice the panel just made. The next cue uses it; the
// file is written a moment later, so dragging a slider is one save.
func (a *App) changeSound(source events.Source) {
	a.prefs = a.prefs.Set(source, a.state.choice(source))
	if a.saveTimer != nil {
		a.saveTimer.Stop()
	}
	a.saveTimer = time.AfterFunc(400*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			a.saveTimer = nil
			a.savePrefs()
		})
	})
}

// previewSound plays the chosen sound without waiting for a completion.
func (a *App) previewSound(source events.Source) {
	choice := a.prefs.For(source)
	if a.audio == nil {
		return
	}
	if err := a.audio.Play(choice.ID, choice.Volume); err != nil {
		log.Printf("cuedex: %v", err)
		a.state.soundProblem = "提示音播放失败：" + err.Error()
		a.invalidate()
		return
	}
	a.state.soundProblem = ""
	a.invalidate()
}

// savePrefs writes the choices out. A failure is shown in the panel rather than
// ending the app: cueing still works from memory.
func (a *App) savePrefs() {
	if a.prefsPath == "" {
		return
	}
	if err := prefs.Save(a.prefsPath, a.prefs); err != nil {
		log.Printf("cuedex: save preferences: %v", err)
		a.state.soundProblem = "偏好保存失败：" + err.Error()
		a.invalidate()
	}
}

func (a *App) setPaused(paused bool) {
	a.state.paused = paused
	if paused {
		a.player.Pause()
	} else {
		a.player.Resume()
	}
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

// refreshOpenCode reads the plugin and gate so the panel says what is there.
func (a *App) refreshOpenCode() {
	installed, err := a.opencode.Installed()
	a.state.opencodeInstalled = installed
	a.state.opencodeVersion = a.opencode.Version
	a.state.opencodeProblem = ""
	if err != nil {
		a.state.opencodeProblem = openCodeProblem(err)
	}
}

// toggleOpenCode installs or removes the local plugin the way the panel's
// button asks.
func (a *App) toggleOpenCode() {
	var err error
	if a.state.opencodeInstalled {
		err = a.opencode.Uninstall()
	} else {
		err = a.opencode.Install()
	}
	a.refreshOpenCode()
	if err != nil {
		a.state.opencodeProblem = openCodeProblem(err)
	}
	a.invalidate()
}

// openCodeProblem says why the plugin could not be changed, in the words the
// panel shows.
func openCodeProblem(err error) string {
	var unsupported integrations.UnsupportedVersionError
	switch {
	case errors.As(err, &unsupported):
		if unsupported.Version == "" {
			return "没有检测到 OpenCode，未安装插件"
		}
		return "OpenCode " + unsupported.Version + " 不在支持范围（需要 2.1.1 及以上、3.0.0 以下），未安装插件"
	case errors.Is(err, integrations.ErrForeignPlugin):
		return `plugins\cuedex.js 不是 CueDex 写的，已停止操作，不会覆盖或删除`
	default:
		return "OpenCode 集成操作失败：" + err.Error()
	}
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
