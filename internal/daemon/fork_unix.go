//go:build !windows

package daemon

import "syscall"

func forkSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
