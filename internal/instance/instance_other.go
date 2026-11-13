//go:build !windows

package instance

// Acquire reports true everywhere but Windows: the app ships for Windows, and
// the other platforms' single-instance handling is MyGo's.
func Acquire(name string) (release func(), first bool) {
	return func() {}, true
}
