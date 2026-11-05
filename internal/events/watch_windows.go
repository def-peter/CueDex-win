//go:build windows

package events

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procReadDirectoryChangesW  = kernel32.NewProc("ReadDirectoryChangesW")
	procCreateEventW           = kernel32.NewProc("CreateEventW")
	procSetEvent               = kernel32.NewProc("SetEvent")
	procResetEvent             = kernel32.NewProc("ResetEvent")
	procWaitForMultipleObjects = kernel32.NewProc("WaitForMultipleObjects")
	procCancelIoEx             = kernel32.NewProc("CancelIoEx")
	procGetOverlappedResult    = kernel32.NewProc("GetOverlappedResult")
	procCloseHandle            = kernel32.NewProc("CloseHandle")
)

const (
	fileListDirectory   = 0x0001
	fileShareAll        = 0x0007 // read, write, delete
	fileFlagBackup      = 0x02000000
	fileFlagOverlapped  = 0x40000000
	notifyFileName      = 0x0001
	notifyDirName       = 0x0002
	notifySize          = 0x0008
	notifyLastWrite     = 0x0010
	notifyFilter        = notifyFileName | notifyDirName | notifySize | notifyLastWrite
	waitObject0         = 0
	waitTimeoutInfinite = 0xFFFFFFFF
)

// Watcher consumes the event directory without polling: Windows tells it when
// the directory changes, and it then reads whatever is there.
type Watcher struct {
	dir     string
	deliver func(Event)

	handle syscall.Handle
	change syscall.Handle // manual reset: a change completed
	quit   syscall.Handle // manual reset: stop the loop

	ledger *Ledger
	buffer []byte
	done   chan struct{}
	once   sync.Once
}

// Watch starts consuming dir. deliver runs on the watcher's own goroutine, in
// completion order, for every completion that deserves a cue.
func Watch(dir string, deliver func(Event)) (*Watcher, error) {
	path, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(path,
		fileListDirectory,
		fileShareAll,
		nil,
		syscall.OPEN_EXISTING,
		fileFlagBackup|fileFlagOverlapped,
		0,
	)
	if err != nil {
		return nil, err
	}
	change, err := createEvent()
	if err != nil {
		procCloseHandle.Call(uintptr(handle))
		return nil, err
	}
	quit, err := createEvent()
	if err != nil {
		procCloseHandle.Call(uintptr(handle))
		procCloseHandle.Call(uintptr(change))
		return nil, err
	}
	w := &Watcher{
		dir:     dir,
		deliver: deliver,
		handle:  handle,
		change:  change,
		quit:    quit,
		ledger:  NewLedger(),
		buffer:  make([]byte, 4096),
		done:    make(chan struct{}),
	}
	go w.run()
	return w, nil
}

// Close asks the watcher to stop. It does not wait: the watcher may be
// handing an event to the UI thread, which is where Close is usually called.
func (w *Watcher) Close() {
	w.once.Do(func() { procSetEvent.Call(uintptr(w.quit)) })
}

// Done is closed when the watcher's goroutine has stopped.
func (w *Watcher) Done() <-chan struct{} { return w.done }

func (w *Watcher) run() {
	defer close(w.done)
	defer procCloseHandle.Call(uintptr(w.handle))
	defer procCloseHandle.Call(uintptr(w.change))
	defer procCloseHandle.Call(uintptr(w.quit))

	// Arm first, then read what is there: files a producer wrote while the
	// app was not running still cue, and a file written during this first
	// scan still raises the notification the loop below waits for.
	overlapped := new(syscall.Overlapped)
	overlapped.HEvent = w.change
	if !w.arm(overlapped) {
		return
	}
	w.scan()

	handles := []syscall.Handle{w.change, w.quit}
	for {
		wait, _, _ := procWaitForMultipleObjects.Call(
			2,
			uintptr(unsafe.Pointer(&handles[0])),
			0,
			waitTimeoutInfinite,
		)
		switch wait {
		case waitObject0:
			runtime.KeepAlive(w.buffer)
			w.scan()
			overlapped = new(syscall.Overlapped)
			overlapped.HEvent = w.change
			if !w.arm(overlapped) {
				return
			}
		case waitObject0 + 1:
			procCancelIoEx.Call(uintptr(w.handle), uintptr(unsafe.Pointer(overlapped)))
			var done uint32
			procGetOverlappedResult.Call(
				uintptr(w.handle),
				uintptr(unsafe.Pointer(overlapped)),
				uintptr(unsafe.Pointer(&done)),
				1, // wait for the cancellation
			)
			return
		default:
			return
		}
	}
}

// arm asks Windows to report the next change in the directory.
func (w *Watcher) arm(overlapped *syscall.Overlapped) bool {
	procResetEvent.Call(uintptr(w.change))
	var returned uint32
	ok, _, _ := procReadDirectoryChangesW.Call(
		uintptr(w.handle),
		uintptr(unsafe.Pointer(&w.buffer[0])),
		uintptr(len(w.buffer)),
		0, // this directory only
		notifyFilter,
		uintptr(unsafe.Pointer(&returned)),
		uintptr(unsafe.Pointer(overlapped)),
		0,
	)
	return ok != 0
}

// scan reads and removes every event file in the directory, then hands the
// accepted completions to deliver. The protocol says files are deleted after
// they are read, so none of them is read twice.
func (w *Watcher) scan() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	now := time.Now()
	var found []Event
	for _, entry := range entries {
		name := entry.Name()
		if !IsEventName(name) {
			continue
		}
		path := filepath.Join(w.dir, name)
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		// A file this old is housekeeping, not a cue: leave nothing behind
		// for the next launch to find.
		if info.Size() > MaxFileBytes || !FreshFile(info.ModTime(), now) {
			os.Remove(path)
			continue
		}
		data, err := os.ReadFile(path)
		os.Remove(path)
		if err != nil {
			continue
		}
		event, err := Parse(name, data, info.ModTime())
		if err != nil {
			continue
		}
		found = append(found, event)
	}
	sort.Slice(found, func(i, j int) bool {
		return found[i].CompletedAt.Before(found[j].CompletedAt)
	})
	for _, event := range found {
		w.ledger.Accept(event, now)
	}
	for {
		event, ok := w.ledger.Next(now)
		if !ok {
			return
		}
		w.deliver(event)
	}
}

func createEvent() (syscall.Handle, error) {
	h, _, err := procCreateEventW.Call(0, 1 /* manual reset */, 0, 0)
	if h == 0 {
		return 0, err
	}
	return syscall.Handle(h), nil
}
