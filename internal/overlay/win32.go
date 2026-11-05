package overlay

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procSetTimer            = user32.NewProc("SetTimer")
	procKillTimer           = user32.NewProc("KillTimer")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wsPopup         = 0x80000000
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	wsExToolWindow  = 0x00000080
	wsExTopmost     = 0x00000008
	wsExNoActivate  = 0x08000000

	swShowNoActivate = 4
	swHide           = 0

	hwndTopmost = ^uintptr(0) // HWND_TOPMOST

	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoActivate = 0x0010

	ulwAlpha = 0x00000002

	biRGB = 0

	htTransparent = ^uintptr(0) // HTTRANSPARENT

	wmDestroy   = 0x0002
	wmNcHitTest = 0x0084
	wmTimer     = 0x0113

	messageCue  = 0x8001 // WM_APP + 1
	messageQuit = 0x8002 // WM_APP + 2

	timerID = 1

	monitorInfoPrimary = 0x00000001
)

type rect struct {
	Left, Top, Right, Bottom int32
}

func (r rect) width() int32  { return r.Right - r.Left }
func (r rect) height() int32 { return r.Bottom - r.Top }

type point struct{ X, Y int32 }

type size struct{ CX, CY int32 }

type msg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

const acSrcOver = 0
const acSrcAlpha = 1

// primaryMonitor returns the bounds of the primary display in physical
// pixels, which is what a layered window's bitmap has to match.
func primaryMonitor() (rect, bool) {
	var found rect
	var ok bool
	cb := syscall.NewCallback(func(hMonitor, _ /*hdc*/, _ /*rect*/, _ uintptr) uintptr {
		mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
		if r, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi))); r == 0 {
			return 1
		}
		if mi.Flags&monitorInfoPrimary != 0 {
			found, ok = mi.Monitor, true
		}
		return 1
	})
	procEnumDisplayMonitors.Call(0, 0, cb, 0)
	return found, ok
}

func utf16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	return p
}
