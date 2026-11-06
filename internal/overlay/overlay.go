// Package overlay draws the cue: four glowing edges on every display, each a
// Win32 layered window carrying a pre-rendered bitmap.
//
// MyGo's own windows cost tens of megabytes each and take hundreds of
// milliseconds to paint after being hidden, so the overlay talks to user32
// directly: a cue paints each band once, and every frame only changes one
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
	// bandThickness is the glow's width at 96 DPI, in physical pixels. A
	// display at another scale gets it multiplied by its DPI, so the glow
	// looks the same width everywhere.
	bandThickness = 64
	// frameInterval is 33 ms: every frame re-blends the whole band, and at
	// 30 frames a second the cue is smooth while staying inside the CPU
	// budget in HANDOFF.md. Measured on this machine, 60 fps costs about
	// twice as much for no visible gain.
	frameInterval = 33 // ms
)

// CueDuration is how long one cue's animation lasts, fade out included. The
// player keeps its queue in step with it.
const CueDuration = 3 * time.Second

// Animation is how a cue moves.
type Animation int

const (
	// Breathing pulses one colour twice and fades out.
	Breathing Animation = iota
	// AlternatingFlash blinks the primary colour twice, then the secondary
	// twice, staying lit in between.
	AlternatingFlash
)

// String is what a log or a test failure reads.
func (a Animation) String() string {
	if a == AlternatingFlash {
		return "alternating-flash"
	}
	return "breathing"
}

// Valid reports whether the value is one this package draws.
func (a Animation) Valid() bool { return a == Breathing || a == AlternatingFlash }

// MarshalText writes the animation as its name, so a preferences file and a log
// use the same word for it.
func (a Animation) MarshalText() ([]byte, error) {
	if !a.Valid() {
		return []byte(Breathing.String()), nil
	}
	return []byte(a.String()), nil
}

// UnmarshalText reads an animation by name. A name this build does not know
// falls back to breathing rather than failing the whole preferences file.
func (a *Animation) UnmarshalText(text []byte) error {
	switch string(text) {
	case AlternatingFlash.String(), "flash", "alternatingFlash":
		*a = AlternatingFlash
	default:
		*a = Breathing
	}
	return nil
}

// Cue is one completion's presentation: the primary colour, the second colour
// the flash alternates with, and the animation.
type Cue struct {
	Primary   color.NRGBA
	Secondary color.NRGBA
	Animation Animation
}

// band is one edge window: one colour, one window. The bitmap is made when a
// cue paints it and freed as soon as the window has it, because the layered
// window keeps its own copy: a 3-second cue then holds a few megabytes of
// surfaces instead of the tens a resident bitmap per band would cost.
type band struct {
	hwnd      uintptr
	width     int32
	height    int32
	edge      Edge
	thickness int32
}

// display is one monitor's bands. Each edge has two windows, one per cue
// colour; the flash alternates them by alpha, so a handover never uploads a
// bitmap.
type display struct {
	bounds    rect
	dpi       uint32
	primary   []*band
	secondary []*band
}

// bands returns the set holding one of the two cue colours.
func (d display) bands(second bool) []*band {
	if second {
		return d.secondary
	}
	return d.primary
}

// Overlay owns the windows and the animation timer on one OS thread.
type Overlay struct {
	ready chan struct{}
	err   error

	// messageWindow receives the overlay's messages: the first band's
	// handle. It is zero until the windows exist and after a failed start.
	messageWindow uintptr

	// mu guards the fields a caller on another goroutine touches.
	mu      sync.Mutex
	pending Cue
	queued  bool

	// Owned by the overlay thread once it is running.
	displays  []display
	signature string
	screenDC  uintptr
	started   time.Time
	playing   bool
	current   Cue
	// secondary says which of the two colours is being shown.
	secondary bool

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

// Show plays one cue, on every display. It is safe from any goroutine and
// restarts the animation if one is already running.
func (o *Overlay) Show(c Cue) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.pending = c
	o.queued = true
	window := o.messageWindow
	o.mu.Unlock()
	if window == 0 {
		return
	}
	procPostMessageW.Call(window, messageCue, 0, 0)
}

// Hide takes the cue off the screen at once, as when the user pauses while one
// is playing. It is safe from any goroutine.
func (o *Overlay) Hide() {
	if o == nil {
		return
	}
	o.mu.Lock()
	window := o.messageWindow
	o.mu.Unlock()
	if window == 0 {
		return
	}
	procPostMessageW.Call(window, messageHide, 0, 0)
}

// Close destroys the windows and ends the overlay thread.
func (o *Overlay) Close() {
	if o == nil {
		return
	}
	o.closeOnce.Do(func() {
		o.mu.Lock()
		window := o.messageWindow
		o.mu.Unlock()
		if window != 0 {
			procPostMessageW.Call(window, messageQuit, 0, 0)
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

	o.screenDC, _, _ = procGetDC.Call(0)
	if o.screenDC == 0 {
		o.err = fmt.Errorf("overlay: no screen device context")
		close(o.ready)
		return
	}

	o.buildDisplays()
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

// buildDisplays makes the bands for the current display configuration. Every
// monitor gets its own rectangles, in its own physical pixels.
func (o *Overlay) buildDisplays() {
	o.destroyDisplays()
	o.signature = ""
	for _, m := range monitors() {
		t := thicknessFor(m.dpi)
		d := display{bounds: m.bounds, dpi: m.dpi}
		for _, edge := range []Edge{EdgeTop, EdgeBottom, EdgeLeft, EdgeRight} {
			r := edgeRect(m.bounds, edge, t)
			d.primary = append(d.primary, o.newBand(r, edge, t))
			d.secondary = append(d.secondary, o.newBand(r, edge, t))
		}
		o.displays = append(o.displays, d)
		o.signature += signaturePart(m)
	}
	o.mu.Lock()
	if len(o.displays) > 0 && len(o.displays[0].primary) > 0 {
		o.messageWindow = o.displays[0].primary[0].hwnd
	} else {
		o.messageWindow = 0
	}
	o.mu.Unlock()
}

// thicknessFor reads the band width at a display's scale.
func thicknessFor(dpi uint32) int32 {
	if dpi == 0 {
		dpi = 96
	}
	t := int32(bandThickness * int(dpi) / 96)
	if t < bandThickness/2 {
		t = bandThickness / 2
	}
	return t
}

func signaturePart(m monitor) string {
	b := m.bounds
	return fmt.Sprintf("%d,%d,%d,%d@%d;", b.Left, b.Top, b.Right, b.Bottom, m.dpi)
}

// syncDisplays rebuilds the bands when the display configuration differs from
// what is on screen. It is cheap when nothing changed, which is the common
// case: two EnumDisplayMonitors calls per cue.
func (o *Overlay) syncDisplays() {
	monitorList := monitors()
	sig := ""
	for _, m := range monitorList {
		sig += signaturePart(m)
	}
	if sig == o.signature && len(o.displays) == len(monitorList) {
		return
	}
	o.buildDisplays()
}

func (o *Overlay) newBand(r rect, edge Edge, thickness int32) *band {
	hwnd, _, _ := procCreateWindowExW.Call(
		wsExLayered|wsExTransparent|wsExToolWindow|wsExTopmost|wsExNoActivate,
		uintptr(unsafe.Pointer(utf16("CueDexOverlay"))),
		uintptr(unsafe.Pointer(utf16("CueDex"))),
		wsPopup,
		uintptr(r.Left), uintptr(r.Top), uintptr(r.width()), uintptr(r.height()),
		0, 0, 0, 0,
	)
	if hwnd == 0 {
		return nil
	}
	procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)

	return &band{
		hwnd:      hwnd,
		width:     r.width(),
		height:    r.height(),
		edge:      edge,
		thickness: thickness,
	}
}

// surface paints one band into a fresh bitmap. The bitmap is discarded once the
// window has it.
func (o *Overlay) surface(b *band, c color.NRGBA) (hdc, bitmap uintptr) {
	hdc, _, _ = procCreateCompatibleDC.Call(o.screenDC)
	if hdc == 0 {
		return 0, 0
	}

	header := bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       b.width,
		Height:      -b.height, // top-down rows
		Planes:      1,
		BitCount:    32,
		Compression: biRGB,
	}
	var bits unsafe.Pointer
	bitmap, _, _ = procCreateDIBSection.Call(
		hdc,
		uintptr(unsafe.Pointer(&header)),
		biRGB,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if bitmap == 0 || bits == nil {
		procDeleteDC.Call(hdc)
		return 0, 0
	}
	procSelectObject.Call(hdc, bitmap)

	fillBand(unsafe.Slice((*byte)(bits), int(b.width)*int(b.height)*4),
		int(b.width), int(b.height), b.edge, c, float64(b.thickness))
	return hdc, bitmap
}

// release frees a surface made by surface.
func (o *Overlay) release(hdc, bitmap uintptr) {
	if hdc == 0 {
		return
	}
	procDeleteDC.Call(hdc)
	if bitmap != 0 {
		procDeleteObject.Call(bitmap)
	}
}

// beginCue starts the animation. Every display shows the same colours.
func (o *Overlay) beginCue(c Cue) {
	if !c.Animation.Valid() {
		c.Animation = Breathing
	}
	o.stopCue()
	o.syncDisplays()
	if len(o.displays) == 0 {
		return
	}
	flash := c.Animation == AlternatingFlash
	for _, d := range o.displays {
		for _, b := range d.primary {
			o.show(b, c.Primary)
		}
		for _, b := range d.secondary {
			if flash {
				o.show(b, c.Secondary)
			} else {
				procShowWindow.Call(b.hwnd, swHide)
			}
		}
	}
	o.current = c
	o.started = time.Now()
	o.playing = true
	o.secondary = false
	o.setTimer()
}

// show paints a band with its colour and puts it on screen at zero alpha, so
// the first frame never reveals a previous cue's pixels.
func (o *Overlay) show(b *band, c color.NRGBA) {
	if b == nil {
		return
	}
	hdc, bitmap := o.surface(b, c)
	if hdc == 0 {
		return
	}
	o.upload(b, hdc, 0)
	o.release(hdc, bitmap)
	procShowWindow.Call(b.hwnd, swShowNoActivate)
}

func (o *Overlay) setTimer() {
	o.mu.Lock()
	window := o.messageWindow
	o.mu.Unlock()
	if window != 0 {
		procSetTimer.Call(window, timerID, frameInterval, 0)
	}
}

func (o *Overlay) killTimer() {
	o.mu.Lock()
	window := o.messageWindow
	o.mu.Unlock()
	if window != 0 {
		procKillTimer.Call(window, timerID)
	}
}

// tick advances the animation one frame and ends it after CueDuration.
func (o *Overlay) tick() {
	if !o.playing {
		return
	}
	elapsed := time.Since(o.started)
	if elapsed >= CueDuration {
		o.finishCue()
		return
	}
	envelope, primary, secondary := sample(o.current.Animation, elapsed)
	// One colour is lit at a time. The handover falls where the outgoing
	// share is zero, so taking that set off the screen is invisible.
	second := o.current.Animation == AlternatingFlash && flashPhase(elapsed) >= 0.5
	if second != o.secondary {
		o.secondary = second
		o.dim(!second)
	}
	share := primary
	if second {
		share = secondary
	}
	alpha := byte(envelope * share * 255)
	for _, d := range o.displays {
		for _, b := range d.bands(second) {
			o.commit(b, alpha)
		}
	}
}

// dim takes one colour set off the screen.
func (o *Overlay) dim(second bool) {
	for _, d := range o.displays {
		for _, b := range d.bands(second) {
			o.commit(b, 0)
		}
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
	o.killTimer()
	for _, d := range o.displays {
		for _, b := range d.primary {
			procShowWindow.Call(b.hwnd, swHide)
		}
		for _, b := range d.secondary {
			procShowWindow.Call(b.hwnd, swHide)
		}
	}
}

// upload gives a band's window the bitmap in srcDC, at the given alpha.
func (o *Overlay) upload(b *band, srcDC uintptr, alpha byte) {
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
		srcDC, uintptr(unsafe.Pointer(&src)),
		0, uintptr(unsafe.Pointer(&blend)),
		ulwAlpha,
	)
}

// commit changes one band's global alpha. The window already holds its pixels,
// so no source is given; only a single pixel is marked dirty, so the frame costs
// a call rather than a copy of the whole band.
func (o *Overlay) commit(b *band, alpha byte) {
	sz := size{CX: b.width, CY: b.height}
	blend := blendFunction{
		BlendOp:             acSrcOver,
		SourceConstantAlpha: alpha,
		AlphaFormat:         acSrcAlpha,
	}
	dirty := rect{0, 0, 1, 1}
	info := updateLayeredWindowInfo{
		Size:    uint32(unsafe.Sizeof(updateLayeredWindowInfo{})),
		DstDC:   o.screenDC,
		WndSize: &sz,
		Blend:   &blend,
		Flags:   ulwAlpha,
		Dirty:   &dirty,
	}
	procUpdateLayeredWindowIndirect.Call(b.hwnd, uintptr(unsafe.Pointer(&info)))
}

// displayChanged recomputes the geometry after a resolution or scale change or
// a monitor being plugged in or out. A cue that is playing restarts, because
// its bitmaps belong to the old geometry.
func (o *Overlay) displayChanged() {
	cue := o.current
	playing := o.playing
	o.stopCue()
	o.buildDisplays()
	if playing {
		o.beginCue(cue)
	}
}

func (o *Overlay) destroyDisplays() {
	for _, d := range o.displays {
		for _, b := range append(append([]*band{}, d.primary...), d.secondary...) {
			if b == nil {
				continue
			}
			procDestroyWindow.Call(b.hwnd)
		}
	}
	o.displays = nil
}

func (o *Overlay) shutdown() {
	o.stopCue()
	o.destroyDisplays()
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
		o.mu.Lock()
		cue, queued := o.pending, o.queued
		o.queued = false
		o.mu.Unlock()
		if queued {
			o.beginCue(cue)
		}
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
	case wmDisplayChange:
		o.displayChanged()
		return 0
	case wmNcHitTest:
		return htTransparent
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wparam, lparam)
	return r
}
