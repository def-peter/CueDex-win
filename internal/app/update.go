package app

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/def-peter/cuedex-win/internal/update"
	"github.com/egoist/mygo"
)

// updateState is what the About page says about a newer release.
type updateState int

const (
	updateIdle updateState = iota
	updateChecking
	updateUpToDate
	updateAvailable
	updateDownloading
	updateFailed
)

// The network calls and the installer hand-off are variables so a test can
// drive the panel's states without a network and without launching anything.
var (
	updateFetch = func(ctx context.Context) (update.Release, error) {
		return update.Fetch(ctx, nil, update.LatestURL)
	}
	updateDownload = func(ctx context.Context, release update.Release, dir string) (string, error) {
		return update.Download(ctx, nil, release.Installer, dir)
	}
	updateRun  = func(path string) error { return exec.Command(path).Start() }
	updateQuit = func() { mygo.App.Quit() }
)

// checkForUpdate asks GitHub for the newest release. The answer only lands in
// the About page: the check runs at launch, and a panel that is closed has
// nothing to interrupt.
func (a *App) checkForUpdate() {
	if a.checking {
		return
	}
	a.checking = true
	a.state.updateState = updateChecking
	a.state.updateProblem = problem{}
	a.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		release, err := updateFetch(ctx)
		mygo.RunOnMain(func() { a.applyUpdateAnswer(release, err) })
	}()
}

// applyUpdateAnswer records what a check found. It is separate so the states
// can be tested without the network.
func (a *App) applyUpdateAnswer(release update.Release, err error) {
	a.checking = false
	switch {
	case err != nil:
		a.state.updateState = updateFailed
		a.state.updateProblem = newProblem(updateProblemKind, err.Error())
	case update.IsNewer(release.Version, a.state.version):
		a.state.updateState = updateAvailable
		a.state.updateVersion = release.Version
		a.state.updateRelease = release
	default:
		a.state.updateState = updateUpToDate
	}
	a.invalidate()
}

// installUpdate downloads the installer the release publishes, checks it
// against the published digest, runs it, and quits so it can replace the
// running executable.
func (a *App) installUpdate() {
	release := a.state.updateRelease
	if a.checking {
		return
	}
	if release.Installer.Name == "" {
		// Nothing to install by itself: hand the page to the browser.
		openPage(release.Page)
		return
	}
	a.checking = true
	a.state.updateState = updateDownloading
	a.state.updateProblem = problem{}
	a.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		path, err := updateDownload(ctx, release, a.dir.Updates())
		mygo.RunOnMain(func() {
			a.checking = false
			if err == nil {
				err = updateRun(path)
			}
			if err != nil {
				a.state.updateState = updateFailed
				a.state.updateProblem = newProblem(updateProblemKind, err.Error())
				a.invalidate()
				return
			}
			a.state.updateState = updateIdle
			a.invalidate()
			// The installer replaces this executable; stepping aside first is
			// what lets it write where the old one sits.
			go func() {
				time.Sleep(400 * time.Millisecond)
				mygo.RunOnMain(updateQuit)
			}()
		})
	}()
}

// openPage shows a URL in the user's browser.
func openPage(url string) {
	if url == "" {
		return
	}
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

// updateLabel is the update button's wording.
func (s *settings) updateLabel(t wording) string {
	switch s.updateState {
	case updateDownloading:
		return t.UpdateDownloading
	case updateAvailable:
		return fmt.Sprintf(t.UpdateInstall, s.updateVersion)
	}
	return t.CheckUpdate
}

// updateHint is the line beside the button. It stays empty when the button
// already says the same thing.
func (s *settings) updateHint(t wording) string {
	switch s.updateState {
	case updateChecking:
		return t.UpdateChecking
	case updateUpToDate:
		return t.UpdateUpToDate
	case updateFailed:
		if !s.updateProblem.empty() {
			return s.updateProblem.text(t)
		}
	}
	return ""
}

// updateEnabled reports whether the button can be pressed.
func (s *settings) updateEnabled() bool {
	return s.updateState != updateChecking && s.updateState != updateDownloading
}

// versionKnown reports whether this build carries a version to compare with.
// A development build says 开发版 and offers no check.
func (s *settings) versionKnown() bool { return s.version != "" }
