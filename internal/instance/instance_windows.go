// Package instance keeps the app to one process, whichever copy of the
// executable is started.
package instance

import (
	"syscall"
	"unsafe"
)

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	createMutexW = kernel32.NewProc("CreateMutexW")
	closeHandle  = kernel32.NewProc("CloseHandle")
)

// errorAlreadyExists is what CreateMutexW reports when the name is taken.
const errorAlreadyExists = 183

// Acquire takes a fixed named mutex and reports whether this process owns it.
// The handle is held until release runs, so the name stays taken even if every
// file the app writes is removed in the meantime.
func Acquire(name string) (release func(), first bool) {
	ptr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return func() {}, true
	}
	handle, _, callErr := createMutexW.Call(0, 0, uintptr(unsafe.Pointer(ptr)))
	if handle == 0 {
		// Locking is not possible: behave as if this were the only instance.
		return func() {}, true
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno == errorAlreadyExists {
		closeHandle.Call(handle)
		return func() {}, false
	}
	return func() { closeHandle.Call(handle) }, true
}
