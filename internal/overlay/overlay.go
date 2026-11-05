// Package overlay draws the cue: four glowing edges on the primary display,
// each a Win32 layered window carrying a pre-rendered bitmap.
//
// MyGo's own windows cost tens of megabytes each and take hundreds of
// milliseconds to paint after being hidden, so the overlay talks to user32
// directly: the bitmaps are made once, and every frame only changes one
// alpha byte.
package overlay

import (
	"fmt"
	"image/color"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	// bandThickness is in physical pixels; at 150% it reads as about 43
	// logical pixels.
	bandThickness = 64
	frameInterval = 16 // ms
)

// CueDuration is how long one cue's animation lasts, fade out included. The
// player keeps its queue in step with it.
const CueDuration = 3 * time.Second

// band is one edge window and the bitmap it shows.
type band struct {
	hwnd   uintptr
	hdc    uintptr
	bitmap uintptr
	bits   unsafe.Pointer
	width  int32
	height int32
	edge   Edge
}

// Overlay owns the windows and the animation timer on one OS thread.
type Overlay struct {
	ready chan struct{}
	err   error

	hwnds []uintptr // set before ready is closed

	// Owned by the overlay thread once it is running.
	bands    []*band
	screenDC uintptr
	started  time.Time
	playing  bool
	alpha    byte

	closeOnce sync.Once
	done      chan struct{}
}

// current is the overlay whose thread is pumping messages. There is one.
var current *Overlay

// Start creates the overlay windows on their own thread. Nothing is visible
// until Show is called, and an idle overlay wakes for no timer at all.
func Start() (*Overlay, error) {
	o := &Overlay{ready: make(chan struct{}), done: make(chan struct{})}
	go o.run()
	<-o.ready
	if o.err != nil {
		return nil, o.err
	}
	return o, nil
}

// Show plays one cue in the given colour. It is safe from any goroutine and
// restarts the animation if one is already running.
func (o *Overlay) Show(c color.NRGBA) {
	if o == nil || len(o.hwnds) == 0 {
		return
	}
	rgb := uintptr(uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B))
	procPostMessageW.Call(o.hwnds[0], messageCue, rgb, 0)
}

// Hide takes the cue off the screen at once, as when the user pauses while one
// is playing. It is safe from any goroutine.
func (o *Overlay) Hide() {
	if o == nil || len(o.hwnds) == 0 {
		return
	}
	procPostMessageW.Call(o.hwnds[0], messageHide, 0, 0)
}

// Close destroys the windows and ends the overlay thread.
func (o *Overlay) Close() {
	if o == nil {
		return
	}
	o.closeOnce.Do(func() {
		if len(o.hwnds) > 0 {
			procPostMessageW.Call(o.hwnds[0], messageQuit, 0, 0)
		}
		<-o.done
	})
}

func (o *Overlay) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(o.done)

	current = o
	defer func() { current = nil }()

	className := utf16("CueDexOverlay")
	instance, _, _ := procGetModuleHandleW.Call(0)
	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   syscall.NewCallback(wndProc),
		Instance:  instance,
		ClassName: className,
	}
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		// A second overlay in the same process finds the class registered
		// and uses it; anything else is a real failure.
		if errno, ok := err.(syscall.Errno); !ok || errno != 1410 { // ERROR_CLASS_ALREADY_EXISTS
			o.err = fmt.Errorf("overlay: register class: %w", err)
			close(o.ready)
			return
		}
	}

	bounds, ok := primaryMonitor()
	if !ok {
		o.err = fmt.Errorf("overlay: no primary monitor")
		close(o.ready)
		return
	}
	o.screenDC, _, _ = procGetDC.Call(0)
	if o.screenDC == 0 {
		o.err = fmt.Errorf("overlay: no screen device context")
		close(o.ready)
		return
	}

	edges := []Edge{EdgeTop, EdgeBottom, EdgeLeft, EdgeRight}
	for _, edge := range edges {
		r := edgeRect(bounds, edge, bandThickness)
		b, err := o.newBand(r, edge)
		if err != nil {
			o.err = err
			close(o.ready)
			return
		}
		o.bands = append(o.bands, b)
		o.hwnds = append(o.hwnds, b.hwnd)
	}
	close(o.ready)

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { // 0 is WM_QUIT, -1 an error
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (o *Overlay) newBand(r rect, edge Edge) (*band, error) {
	hwnd, _, err := procCreateWindowExW.Call(
		wsExLayered|wsExTransparent|wsExToolWindow|wsExTopmost|wsExNoActivate,
		uintptr(unsafe.Pointer(utf16("CueDexOverlay"))),
		uintptr(unsafe.Pointer(utf16("CueDex"))),
		wsPopup,
		uintptr(r.Left), uintptr(r.Top), uintptr(r.width()), uintptr(r.height()),
		0, 0, 0, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("overlay: create window: %w", err)
	}
	procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)

	hdc, _, _ := procCreateCompatibleDC.Call(o.screenDC)
	if hdc == 0 {
		return nil, fmt.Errorf("overlay: create device context")
	}

	header := bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       r.width(),
		Height:      -r.height(), // top-down rows
		Planes:      1,
		BitCount:    32,
		Compression: biRGB,
	}
	var bits unsafe.Pointer
	bitmap, _, _ := procCreateDIBSection.Call(
		hdc,
		uintptr(unsafe.Pointer(&header)),
		biRGB,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if bitmap == 0 || bits == nil {
		return nil, fmt.Errorf("overlay: create bitmap")
	}
	procSelectObject.Call(hdc, bitmap)

	return &band{
		hwnd:   hwnd,
		hdc:    hdc,
		bitmap: bitmap,
		bits:   bits,
		width:  r.width(),
		height: r.height(),
		edge:   edge,
	}, nil
}

// beginCue starts the animation in a colour packed as 0xRRGGBB.
func (o *Overlay) beginCue(rgb uint32) {
	c := color.NRGBA{R: byte(rgb >> 16), G: byte(rgb >> 8), B: byte(rgb), A: 255}
	for _, b := range o.bands {
		pixels := bandPixels(int(b.width), int(b.height), b.edge, c, bandThickness)
		copy(unsafe.Slice((*byte)(b.bits), len(pixels)), pixels)
		o.commit(b, 0)
		procShowWindow.Call(b.hwnd, swShowNoActivate)
	}
	o.started = time.Now()
	o.playing = true
	procSetTimer.Call(o.hwnds[0], timerID, frameInterval, 0)
}

// tick advances the animation one frame and ends it after cueDuration.
func (o *Overlay) tick() {
	if !o.playing {
		return
	}
	elapsed := time.Since(o.started)
	if elapsed >= CueDuration {
		o.finishCue()
		return
	}
	alpha := breathe(elapsed.Seconds(), CueDuration.Seconds())
	for _, b := range o.bands {
		o.commit(b, byte(alpha*255))
	}
}

func (o *Overlay) finishCue() {
	o.stopCue()
}

// stopCue ends the animation, whether it ran its course or was cut short.
func (o *Overlay) stopCue() {
	if !o.playing {
		return
	}
	o.playing = false
	procKillTimer.Call(o.hwnds[0], timerID)
	for _, b := range o.bands {
		procShowWindow.Call(b.hwnd, swHide)
	}
}

// commit pushes one band's bitmap to its window with the given global alpha.
func (o *Overlay) commit(b *band, alpha byte) {
	sz := size{CX: b.width, CY: b.height}
	src := point{}
	blend := blendFunction{
		BlendOp:             acSrcOver,
		SourceConstantAlpha: alpha,
		AlphaFormat:         acSrcAlpha,
	}
	procUpdateLayeredWindow.Call(
		b.hwnd, o.screenDC,
		0, uintptr(unsafe.Pointer(&sz)),
		b.hdc, uintptr(unsafe.Pointer(&src)),
		0, uintptr(unsafe.Pointer(&blend)),
		ulwAlpha,
	)
}

func (o *Overlay) shutdown() {
	procKillTimer.Call(o.hwnds[0], timerID)
	for _, b := range o.bands {
		procDestroyWindow.Call(b.hwnd)
		procDeleteObject.Call(b.bitmap)
		procDeleteDC.Call(b.hdc)
	}
	if o.screenDC != 0 {
		procReleaseDC.Call(0, o.screenDC)
		o.screenDC = 0
	}
	procPostQuitMessage.Call(0)
}

// wndProc runs on the overlay thread for every message any band receives.
func wndProc(hwnd, message, wparam, lparam uintptr) uintptr {
	o := current
	if o == nil {
		r, _, _ := procDefWindowProcW.Call(hwnd, message, wparam, lparam)
		return r
	}
	switch message {
	case messageCue:
		o.beginCue(uint32(wparam))
		return 0
	case messageHide:
		o.stopCue()
		return 0
	case messageQuit:
		o.shutdown()
		return 0
	case wmTimer:
		o.tick()
		return 0
	case wmNcHitTest:
		return htTransparent
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wparam, lparam)
	return r
}
