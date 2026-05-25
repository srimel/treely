//go:build windows

package daemon

import (
	"errors"
	"syscall"
)

// isAFUnixError reports whether err looks like an AF_UNIX unavailability
// error — WSAEPFNOSUPPORT (10046) or WSAEAFNOSUPPORT (10047) — as opposed to
// a permission or path error that would be misleadingly attributed to OS version.
func isAFUnixError(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	const (
		wsaepfnosupport = syscall.Errno(10046)
		wsaeafnosupport = syscall.Errno(10047)
	)
	return errno == wsaepfnosupport || errno == wsaeafnosupport
}
