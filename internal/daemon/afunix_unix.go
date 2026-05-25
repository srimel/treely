//go:build !windows

package daemon

func isAFUnixError(err error) bool { return false }
