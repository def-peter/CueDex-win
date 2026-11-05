package main

import (
	"strconv"
	"syscall"
	"unsafe"
)

// rect is a rectangle in physical pixels, as Win32 reports it: the one
// number set that says where an overlay window really is, whatever MyGo's
// display and bounds APIs convert between.
type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) String() string {
	return strconv.Itoa(int(r.Left)) + "," + strconv.Itoa(int(r.Top)) +
		" " + strconv.Itoa(int(r.Right-r.Left)) + "x" + strconv.Itoa(int(r.Bottom-r.Top))
}

func (r rect) width() int32  { return r.Right - r.Left }
func (r rect) height() int32 { return r.Bottom - r.Top }

// monitorInfo mirrors MONITORINFO.
type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procGetDpiForWindow     = user32.NewProc("GetDpiForWindow")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
)

const (
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
)

func windowRect(hwnd uintptr) (rect, bool) {
	var r rect
	ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r, ok != 0
}

// windowDPI is the window's DPI: 96 at 100% scaling, 144 at 150%.
func windowDPI(hwnd uintptr) uint32 {
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	return uint32(dpi)
}

// monitors enumerates every display's full rectangle, in physical pixels,
// which is what an overlay has to cover.
func monitors() []rect {
	var out []rect
	cb := syscall.NewCallback(func(hmon, _, _, _ uintptr) uintptr {
		var mi monitorInfo
		mi.Size = uint32(unsafe.Sizeof(mi))
		if ok, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); ok != 0 {
			out = append(out, mi.Monitor)
		}
		return 1
	})
	procEnumDisplayMonitors.Call(0, 0, cb, 0)
	return out
}

// setWindowRect puts a window exactly at r, in physical pixels, above other
// windows when topmost: the placement MyGo's own options get wrong on a
// display whose scaling differs from the primary one's.
func setWindowRect(hwnd uintptr, r rect, topmost bool) bool {
	after, flags := uintptr(0), uintptr(swpNoZOrder|swpNoActivate)
	if topmost {
		after = ^uintptr(0) // HWND_TOPMOST
		flags = swpNoActivate
	}
	ok, _, _ := procSetWindowPos.Call(hwnd, after,
		uintptr(r.Left), uintptr(r.Top), uintptr(r.width()), uintptr(r.height()), flags)
	return ok != 0
}
