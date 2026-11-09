package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/def-peter/cuedex-win/internal/update"
)

// The check's answer decides what the About page offers.
func TestUpdateAnswerOffersANewerRelease(t *testing.T) {
	a := &App{}
	a.state.version = "0.2.1"

	a.applyUpdateAnswer(update.Release{
		Version:   "0.3.0",
		Page:      "https://example/releases/tag/v0.3.0",
		Installer: update.Asset{Name: "cuedex-setup-0.3.0-amd64.exe"},
	}, nil)

	if a.state.updateState != updateAvailable || a.state.updateVersion != "0.3.0" {
		t.Fatalf("state %v version %q", a.state.updateState, a.state.updateVersion)
	}
	if a.state.updateRelease.Installer.Name == "" {
		t.Fatal("the release was not kept for the button")
	}
}

func TestUpdateAnswerSaysUpToDate(t *testing.T) {
	a := &App{}
	a.state.version = "0.2.1"
	a.applyUpdateAnswer(update.Release{Version: "0.2.1"}, nil)
	if a.state.updateState != updateUpToDate {
		t.Fatalf("state %v", a.state.updateState)
	}
}

func TestUpdateAnswerKeepsTheReasonItFailed(t *testing.T) {
	a := &App{}
	a.state.version = "0.2.1"
	a.applyUpdateAnswer(update.Release{}, errors.New("dial tcp: no network"))
	if a.state.updateState != updateFailed {
		t.Fatalf("state %v", a.state.updateState)
	}
	if got := a.state.updateProblem.text(chinese); !strings.Contains(got, "no network") {
		t.Fatalf("problem %q", got)
	}
}

func TestAboutPageOffersTheCheckOnlyInAPackagedBuild(t *testing.T) {
	packaged := settings{tab: tabAbout, version: "0.2.1"}
	tt := ui.NewTester(packaged.view, 820, 700)
	if !tt.HasText("检查更新") {
		t.Fatalf("the packaged build offers no check: %q", tt.Texts())
	}
	if !tt.HasText("版本 0.2.1") {
		t.Fatalf("the version line is wrong: %q", tt.Texts())
	}

	dev := settings{tab: tabAbout}
	tt = ui.NewTester(dev.view, 820, 700)
	if tt.HasText("检查更新") {
		t.Fatalf("a development build offers a check: %q", tt.Texts())
	}
}

func TestAboutPageInstallsTheReleaseItFound(t *testing.T) {
	s := settings{
		tab:           tabAbout,
		version:       "0.2.1",
		updateState:   updateAvailable,
		updateVersion: "0.3.0",
	}
	checked, installed := 0, 0
	s.onCheckUpdate = func() { checked++ }
	s.onInstallUpdate = func() { installed++ }
	tt := ui.NewTester(s.view, 820, 700)

	if !tt.HasText("下载 0.3.0") {
		t.Fatalf("the button does not name the release: %q", tt.Texts())
	}
	if err := tt.Click("下载 0.3.0"); err != nil {
		t.Fatal(err)
	}
	if installed != 1 || checked != 0 {
		t.Fatalf("installed %d checked %d", installed, checked)
	}
}

func TestAboutPageChecksWhenThereIsNothingToInstall(t *testing.T) {
	s := settings{tab: tabAbout, version: "0.2.1"}
	checked := 0
	s.onCheckUpdate = func() { checked++ }
	tt := ui.NewTester(s.view, 820, 700)
	if err := tt.Click("检查更新"); err != nil {
		t.Fatal(err)
	}
	if checked != 1 {
		t.Fatalf("checked %d", checked)
	}
}

func TestUpdateStatesWordTheHint(t *testing.T) {
	cases := []struct {
		state settings
		want  string
	}{
		{settings{updateState: updateChecking}, "正在检查…"},
		{settings{updateState: updateUpToDate}, "已是最新"},
		{settings{updateState: updateFailed, updateProblem: newProblem(updateProblemKind, "断网")}, "更新失败：断网"},
		{settings{updateState: updateIdle}, ""},
		{settings{updateState: updateAvailable, updateVersion: "0.3.0"}, ""},
	}
	for _, tc := range cases {
		if got := tc.state.updateHint(chinese); got != tc.want {
			t.Errorf("state %v: hint %q, want %q", tc.state.updateState, got, tc.want)
		}
	}
}

func TestUpdateButtonIsDisabledWhileBusy(t *testing.T) {
	for _, state := range []updateState{updateChecking, updateDownloading} {
		s := settings{updateState: state}
		if s.updateEnabled() {
			t.Errorf("state %v leaves the button enabled", state)
		}
	}
	idle := settings{updateState: updateIdle}
	if !idle.updateEnabled() {
		t.Error("the idle button is disabled")
	}
}
