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

	"github.com/def-peter/cuedex-win/internal/autostart"
	"github.com/def-peter/cuedex-win/internal/events"
	"github.com/def-peter/cuedex-win/internal/i18n"
	"github.com/def-peter/cuedex-win/internal/integrations"
	"github.com/def-peter/cuedex-win/internal/overlay"
	"github.com/def-peter/cuedex-win/internal/paths"
	"github.com/def-peter/cuedex-win/internal/playback"
	"github.com/def-peter/cuedex-win/internal/prefs"
	"github.com/def-peter/cuedex-win/internal/sound"
	"github.com/def-peter/cuedex-win/internal/speech"
)

// audio is the part of the sound player the app uses, so a test can hear what
// it was asked to play without a device.
type audio interface {
	Play(id string, volume int) error
	Stop()
}

// speaker is the part of the speech engine the app uses, so a test can hear
// what would have been said without a voice.
type speaker interface {
	Voices() ([]speech.Voice, error)
	Speak(voiceID, text string) error
	Stop()
	Close()
}

// App owns the tray icon, the settings panel, and everything the panels show.
// Its state is read and changed on the main thread only.
type App struct {
	dir      paths.Dir
	exe      string
	tray     *mygo.Tray
	win      *mygo.Window
	overlay  *overlay.Overlay
	watch    *events.Watcher
	player   *playback.Player
	audio    audio
	speech   speaker
	codex    integrations.CodexIntegration
	opencode integrations.OpenCodeIntegration
	prefs    prefs.Preferences
	// prefsPath is where the choices are saved.
	prefsPath string
	// saveTimer debounces the save while a slider is being dragged.
	saveTimer *time.Timer
	// pauseTimer ends a timed pause, so the panel and the tray agree with the
	// clock even when no cue arrives.
	pauseTimer *time.Timer
	// autostart is the Run entry the panel's switch writes.
	autostart autostart.Entry
	// voices are the installed SAPI voices. They are read once in the
	// background; until then the panel hides the speech section.
	voices        []speech.Voice
	voicesLoaded  bool
	loadingVoices bool
	state         settings
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
	// The registry, not the file, says whether the app starts with Windows:
	// the user can remove the entry from Task Manager, and the panel must not
	// claim otherwise.
	runEntry := autostart.Default()
	if on, err := runEntry.Enabled(exe); err == nil {
		stored.General.OpenAtLogin = on
	} else {
		log.Printf("cuedex: autostart: %v", err)
	}
	a := &App{
		dir:       dir,
		exe:       exe,
		autostart: runEntry,
		codex:     integrations.CodexIntegration{Home: home, Exe: exe},
		opencode: integrations.OpenCodeIntegration{
			ConfigDir: configDir,
			EventsDir: dir.Events(),
			GatePath:  dir.OpenCodeGate(),
			Exe:       exe,
		},
		audio:     &sound.Player{},
		speech:    speech.New(),
		prefs:     stored,
		prefsPath: dir.Preferences(),
	}
	a.state.applyGeneral(stored.General, time.Now())
	a.state.applyDisplay(stored)
	a.state.applySound(stored)
	a.state.applySpeech(stored)
	// The player decides when a cue runs; the drawing and the counters are the
	// main thread's, so its callbacks marshal there.
	a.player = playback.New(
		func(source events.Source) { mygo.RunOnMain(func() { a.cueStart(source) }) },
		func() { mygo.RunOnMain(a.cueStop) },
	)
	// A pause survives a restart, so the player starts in the state the file
	// recorded rather than cueing until the panel is opened.
	a.syncPause()
	a.state.onTogglePause = a.togglePause
	a.state.onClose = a.hide
	a.state.onToggleCodex = a.toggleCodex
	a.state.onToggleOpenCode = a.toggleOpenCode
	a.state.onChangeSound = a.changeSound
	a.state.onPreviewSound = a.previewSound
	a.state.onChangeSpeech = a.changeSpeech
	a.state.onToggleEnabled = a.changeGeneral
	a.state.onChangeLanguage = a.changeLanguage
	a.state.onPauseFor = a.pauseFor
	a.state.onResume = a.resume
	a.state.onChangeQuiet = a.changeGeneral
	a.state.onToggleOpenAtLogin = a.toggleOpenAtLogin
	a.state.onChangeDisplay = a.changeDisplay
	return a
}

// Ready builds the tray and the panel. It runs once, on the main thread.
func (a *App) Ready() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "cuedex",
		Icon:    trayIcon(),
		ToolTip: "CueDex",
		Menu:    a.trayMenu(),
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
		a.state.problem = newProblem(overlayProblem, err.Error())
	} else {
		a.overlay = cue
	}

	// The preferences decide whether a completion may cue at all, and they
	// are the main thread's, so the decision is taken there. Submitting an
	// event the user switched off would hold the queue for three seconds for
	// nothing.
	watch, err := events.Watch(a.dir.Events(), func(e events.Event) {
		mygo.RunOnMain(func() {
			if a.prefs.Cues(e.Source, time.Now()) {
				a.player.Submit(e)
			}
		})
	})
	if err != nil {
		log.Printf("cuedex: watch %s: %v", a.dir.Events(), err)
		a.state.problem = newProblem(watchProblem, err.Error())
	} else {
		a.watch = watch
	}
	a.refreshCodex()
	a.refreshOpenCode()
	a.loadVoices()

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
		a.speech.Close()
		if a.saveTimer != nil {
			a.saveTimer.Stop()
			a.saveTimer = nil
		}
		if a.pauseTimer != nil {
			a.pauseTimer.Stop()
			a.pauseTimer = nil
		}
		a.savePrefs()
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
}

// trayMenu is the tray's menu in the interface language. It is rebuilt when
// the language changes, because a menu cannot relabel itself.
func (a *App) trayMenu() *mygo.Menu {
	t := a.state.text()
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: t.OpenSettings, Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
		{
			Label:   t.PauseCues,
			Type:    mygo.MenuItemCheckbox,
			Checked: a.prefs.General.PausedAt(time.Now()),
			Click:   func(item *mygo.MenuItem, _ *mygo.Window) { a.setPaused(item.Checked) },
		},
		mygo.Separator(),
		// RoleQuit names itself in MyGo's own language, which does not
		// follow this panel's switch.
		{Label: t.Quit, Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

// rebuildTray swaps the tray menu for one in the current language.
func (a *App) rebuildTray() {
	if a.tray != nil {
		a.tray.SetMenu(a.trayMenu())
	}
}

// newWindow builds the settings panel. A MyGo window costs about 80 MB
// whether or not it is visible, so the panel exists only while the user has
// it open: the app sits near the bare event loop otherwise.
func (a *App) newWindow() *mygo.Window {
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:             "CueDex",
		Width:             420,
		Height:            790,
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
		a.overlay.Show(a.sourceCue(source))
	}
	spoken := a.speechWanted(source)
	if spoken {
		if err := a.speech.Speak(a.prefs.Speech.Voice, speech.Phrase(source)); err != nil {
			log.Printf("cuedex: speech: %v", err)
			a.state.speechProblem = newProblem(speechProblem, err.Error())
		}
	}
	// Speech replaces the sound only when the user asked for that and a voice
	// is actually installed; a machine without voices always keeps the chime.
	if !spoken || !a.prefs.Speech.ReplaceSound {
		a.playSound(source)
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

// speechWanted says whether this completion should be announced. A machine
// with no installed voice never tries, so a half-configured preference cannot
// leave a cue silent.
func (a *App) speechWanted(source events.Source) bool {
	if a.speech == nil || !a.prefs.Speech.On(source) {
		return false
	}
	return !a.voicesLoaded || len(a.voices) > 0
}

// playSound plays the producer's bundled cue sound.
func (a *App) playSound(source events.Source) {
	choice := a.prefs.For(source)
	if a.audio == nil {
		return
	}
	if err := a.audio.Play(choice.ID, choice.Volume); err != nil {
		log.Printf("cuedex: %v", err)
		a.state.soundProblem = newProblem(soundProblem, err.Error())
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
	if a.speech != nil {
		a.speech.Stop()
	}
}

// changeSound stores the choice the panel just made. The next cue uses it; the
// file is written a moment later, so dragging a slider is one save.
func (a *App) changeSound(source events.Source) {
	a.prefs = a.prefs.Set(source, a.state.choice(source))
	a.scheduleSave()
}

// changeSpeech stores what the panel currently says about the announcement.
func (a *App) changeSpeech() {
	a.prefs = a.prefs.SetSpeech(a.state.speech())
	a.scheduleSave()
}

// changeDisplay stores one producer's appearance.
func (a *App) changeDisplay(source events.Source) {
	a.prefs = a.prefs.SetDisplay(source, a.state.displayChoice(source))
	a.scheduleSave()
}

// changeGeneral stores the application-wide switches the panel just moved.
func (a *App) changeGeneral() {
	a.prefs = a.prefs.SetGeneral(a.state.general(time.Now()))
	a.syncPause()
	a.scheduleSave()
	a.invalidate()
}

// changeLanguage switches the interface language, which relabels the panel and
// the tray menu.
func (a *App) changeLanguage() {
	a.state.languageSetting = a.state.languageSettingFor(a.state.languageLabel)
	a.state.language = i18n.Resolve(a.state.languageSetting)
	a.state.languageLabel = a.state.languageLabelFor(a.state.languageSetting)
	// The colour picker and the voice picker hold labels, not values, so they
	// are refilled in the new language.
	a.state.applyDisplay(a.prefs)
	a.state.applySpeech(a.prefs)
	a.prefs = a.prefs.SetGeneral(a.state.general(time.Now()))
	a.rebuildTray()
	a.scheduleSave()
	a.invalidate()
}

// pauseFor suppresses cues for d, or until the user resumes when d is zero.
func (a *App) pauseFor(d time.Duration) {
	a.prefs = a.prefs.SetGeneral(a.prefs.General.PauseFor(time.Now(), d))
	a.syncPause()
	a.scheduleSave()
	a.invalidate()
}

// resume ends a pause, timed or not.
func (a *App) resume() {
	a.prefs = a.prefs.SetGeneral(a.prefs.General.Resume())
	a.syncPause()
	a.scheduleSave()
	a.invalidate()
}

// syncPause pushes the pause into the player and schedules its end, so a timed
// pause ends even when no completion arrives.
func (a *App) syncPause() {
	now := time.Now()
	before := a.state.paused
	paused := a.prefs.General.PausedAt(now)
	a.state.paused = paused
	a.state.pauseRemaining = a.prefs.General.Remaining(now)
	if a.player != nil {
		if paused {
			a.player.Pause()
		} else {
			a.player.Resume()
		}
	}
	if a.pauseTimer != nil {
		a.pauseTimer.Stop()
		a.pauseTimer = nil
	}
	if remaining := a.state.pauseRemaining; remaining > 0 {
		a.pauseTimer = time.AfterFunc(remaining, func() {
			mygo.RunOnMain(a.pauseExpired)
		})
	}
	// The tray's checkbox is a copy, so it is rebuilt when the answer changes.
	if before != paused {
		a.rebuildTray()
	}
}

// pauseExpired clears a pause whose deadline passed.
func (a *App) pauseExpired() {
	a.pauseTimer = nil
	if a.prefs.General.PausedAt(time.Now()) {
		a.syncPause()
		return
	}
	a.prefs = a.prefs.SetGeneral(a.prefs.General.Resume())
	a.syncPause()
	a.scheduleSave()
	a.invalidate()
}

// toggleOpenAtLogin writes or removes the Run entry, then reads it back: the
// switch must show what the registry holds, not what was asked for.
func (a *App) toggleOpenAtLogin() {
	var err error
	if a.state.openAtLogin {
		err = a.autostart.Enable(a.exe)
	} else {
		err = a.autostart.Disable()
	}
	if err != nil {
		log.Printf("cuedex: autostart: %v", err)
		a.state.autostartProblem = newProblem(autostartProblem, err.Error())
	} else {
		a.state.autostartProblem = problem{}
	}
	if on, readErr := a.autostart.Enabled(a.exe); readErr == nil {
		a.state.openAtLogin = on
	}
	a.prefs = a.prefs.SetGeneral(a.state.general(time.Now()))
	a.scheduleSave()
	a.invalidate()
}

// scheduleSave writes the preferences once the user stops moving things. A
// slider drag and a row of toggles become one file write.
func (a *App) scheduleSave() {
	if a.prefsPath == "" {
		return
	}
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

// loadVoices reads the installed SAPI voices in the background. The panel
// hides the speech section until the answer arrives, and a machine with no
// voices simply keeps it hidden.
func (a *App) loadVoices() {
	if a.speech == nil || a.voicesLoaded || a.loadingVoices {
		return
	}
	a.loadingVoices = true
	go func() {
		voices, err := a.speech.Voices()
		mygo.RunOnMain(func() {
			a.loadingVoices = false
			a.voicesLoaded = true
			a.voices = voices
			if err != nil {
				log.Printf("cuedex: speech: %v", err)
				a.state.speechProblem = newProblem(speechProblem, err.Error())
			}
			a.state.setVoices(voices)
			a.invalidate()
		})
	}()
}

// previewSound plays the chosen sound without waiting for a completion.
func (a *App) previewSound(source events.Source) {
	choice := a.prefs.For(source)
	if a.audio == nil {
		return
	}
	if err := a.audio.Play(choice.ID, choice.Volume); err != nil {
		log.Printf("cuedex: %v", err)
		a.state.soundProblem = newProblem(soundProblem, err.Error())
		a.invalidate()
		return
	}
	a.state.soundProblem = problem{}
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
		a.state.saveProblem = newProblem(saveProblem, err.Error())
		a.invalidate()
		return
	}
	a.state.saveProblem = problem{}
}

func (a *App) setPaused(paused bool) {
	if paused {
		a.pauseFor(0)
	} else {
		a.resume()
	}
}

func (a *App) togglePause() { a.setPaused(!a.state.paused) }

func (a *App) show() {
	if a.win == nil {
		a.win = a.newWindow()
	}
	a.loadVoices()
	a.win.Show()
}

// Open shows the settings panel. It is what a launch without --background does,
// and what the tray's first item does.
func (a *App) Open() { a.show() }

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
	a.state.codexProblem = problem{}
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
func codexProblem(err error) problem {
	if errors.Is(err, integrations.ErrInvalidHooks) {
		return problem{kind: hooksProblem}
	}
	return newProblem(codexProblemKind, err.Error())
}

// refreshOpenCode reads the plugin and gate so the panel says what is there.
func (a *App) refreshOpenCode() {
	installed, err := a.opencode.Installed()
	a.state.opencodeInstalled = installed
	a.state.opencodeVersion = a.opencode.Version
	a.state.opencodeProblem = problem{}
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
func openCodeProblem(err error) problem {
	var unsupported integrations.UnsupportedVersionError
	switch {
	case errors.As(err, &unsupported):
		if unsupported.Version == "" {
			return problem{kind: openCodeMissing}
		}
		return newProblem(openCodeUnsupported, unsupported.Version)
	case errors.Is(err, integrations.ErrForeignPlugin):
		return problem{kind: openCodeForeign}
	default:
		return newProblem(openCodeOther, err.Error())
	}
}

func (a *App) invalidate() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

// sourceCue is each producer's presentation, as the panel last stored it: the
// colour the user picked, the animation, and a second colour derived from the
// first so the flash never clashes.
func (a *App) sourceCue(source events.Source) overlay.Cue {
	display := a.prefs.Display.For(source)
	return overlay.Cue{
		Primary:   display.Colour(),
		Secondary: display.Secondary(),
		Animation: display.Animation,
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
