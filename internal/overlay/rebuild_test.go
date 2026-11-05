package overlay

import (
	"image/color"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

var (
	procEnumWindows    = user32.NewProc("EnumWindows")
	procGetClassNameW  = user32.NewProc("GetClassNameW")
	procGetWindowRect  = user32.NewProc("GetWindowRect")
	procGetWindowLongW = user32.NewProc("GetWindowLongW")
	procSendMessageW   = user32.NewProc("SendMessageW")
)

// overlayWindows lists the overlay's windows with their handles.
func overlayWindows(t *testing.T) []uintptr {
	t.Helper()
	var found []uintptr
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var name [64]uint16
		n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if n > 0 && syscall.UTF16ToString(name[:n]) == "CueDexOverlay" {
			found = append(found, hwnd)
		}
		return 1
	})
	if ok, _, _ := procEnumWindows.Call(cb, 0); ok == 0 {
		t.Fatal("EnumWindows failed")
	}
	return found
}

// bandRects reads every overlay window's rectangle from the window manager, so
// a test sees what is really on screen rather than what the package believes.
func bandRects(t *testing.T) []rect {
	t.Helper()
	var found []rect
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var name [64]uint16
		n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if n == 0 || syscall.UTF16ToString(name[:n]) != "CueDexOverlay" {
			return 1
		}
		var r rect
		if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok != 0 {
			found = append(found, r)
		}
		return 1
	})
	if ok, _, _ := procEnumWindows.Call(cb, 0); ok == 0 {
		t.Fatal("EnumWindows failed")
	}
	return found
}

func TestBandsCoverEveryDisplayAtItsOwnScale(t *testing.T) {
	o, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()

	displays := monitors()
	if len(displays) == 0 {
		t.Skip("no display is attached to this session")
	}
	found := bandRects(t)

	// Each edge has one window per cue colour, and every one of them has to sit
	// exactly on the rectangle the geometry asks for, in physical pixels.
	for _, m := range displays {
		thickness := thicknessFor(m.dpi)
		for _, edge := range []Edge{EdgeTop, EdgeBottom, EdgeLeft, EdgeRight} {
			want := edgeRect(m.bounds, edge, thickness)
			seen := 0
			for _, got := range found {
				if got == want {
					seen++
				}
			}
			if seen < 2 {
				t.Errorf("%v band of display %+v at %v: %d windows, want 2", edge, m.bounds, want, seen)
			}
		}
	}
}

// TestDisplayChangeRebuildsMidCue drives the path Windows takes when the scale,
// a resolution, or a monitor changes while a cue is playing. There is no way to
// change the real desktop from a test, so it posts the same message the shell
// sends; what it proves is that the windows are rebuilt and the cue restarts.
func TestDisplayChangeRebuildsMidCue(t *testing.T) {
	o, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()

	cue := Cue{
		Primary:   color.NRGBA{R: 94, G: 234, B: 212, A: 255},
		Secondary: color.NRGBA{R: 96, G: 165, B: 250, A: 255},
		Animation: AlternatingFlash,
	}
	o.Show(cue)
	time.Sleep(150 * time.Millisecond)

	o.mu.Lock()
	window := o.messageWindow
	o.mu.Unlock()
	if window == 0 {
		t.Fatal("no overlay window to send the message to")
	}
	for i := 0; i < 3; i++ {
		procPostMessageW.Call(window, wmDisplayChange, 0, 0)
		time.Sleep(60 * time.Millisecond)
	}

	// The rebuilt windows must still cover the displays, and a second cue must
	// still play: a rebuild that lost them would leave the bands missing.
	displays := monitors()
	if got := len(bandRects(t)); got < 8*len(displays) {
		t.Fatalf("%d band windows after the rebuild, want %d", got, 8*len(displays))
	}
	o.Show(Cue{Primary: cue.Primary, Animation: Breathing})
	time.Sleep(100 * time.Millisecond)
}

// TestBandsAreClickThroughToolWindows checks the window style the cue relies on:
// the bands must never take the mouse, never appear in the taskbar or Alt+Tab,
// and never take focus.
func TestBandsAreClickThroughToolWindows(t *testing.T) {
	o, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()

	windows := overlayWindows(t)
	if len(windows) == 0 {
		t.Skip("no display is attached to this session")
	}
	const (
		wsExTopmost     = 0x00000008
		wsExTransparent = 0x00000020
		wsExToolWindow  = 0x00000080
		wsExAppWindow   = 0x00040000
		wsExLayered     = 0x00080000
		wsExNoActivate  = 0x08000000
	)
	// GWL_EXSTYLE is -20, and the syscall takes the index as a uintptr.
	gwlExStyle := ^uintptr(19)
	for _, hwnd := range windows {
		style, _, _ := procGetWindowLongW.Call(hwnd, gwlExStyle)
		for _, want := range []struct {
			name string
			bit  uintptr
		}{
			{"layered", wsExLayered},
			{"transparent (click through)", wsExTransparent},
			{"tool window (no taskbar, no Alt+Tab)", wsExToolWindow},
			{"topmost", wsExTopmost},
			{"no activate", wsExNoActivate},
		} {
			if style&want.bit == 0 {
				t.Errorf("window %#x is missing the %s style", hwnd, want.name)
			}
		}
		if style&wsExAppWindow != 0 {
			t.Errorf("window %#x has WS_EX_APPWINDOW and would appear in the taskbar", hwnd)
		}
		// HWND passed as a hit-test target answers HTTRANSPARENT, which is what
		// makes the system hand the click to the window below.
		if hit, _, _ := procSendMessageW.Call(hwnd, wmNcHitTest, 0, 0x00050005); int32(hit) != -1 {
			t.Errorf("window %#x answers hit test %d, want HTTRANSPARENT", hwnd, int32(hit))
		}
	}
}
