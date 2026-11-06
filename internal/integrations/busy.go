package integrations

import (
	"errors"
	"syscall"
)

// Windows answers a file another program holds open with one of these. The
// panel words them as "in use" rather than passing the number on: the user can
// do something about a program holding the file, not about an error code.
const (
	errorSharingViolation = syscall.Errno(32)
	errorLockViolation    = syscall.Errno(33)
)

// Busy reports whether err means a file could not be replaced because another
// program has it open.
func Busy(err error) bool {
	return errors.Is(err, errorSharingViolation) || errors.Is(err, errorLockViolation)
}
