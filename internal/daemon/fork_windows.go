//go:build windows

package daemon

import "syscall"

const (
	// DETACHED_PROCESS prevents the daemon child from inheriting the parent's
	// console, so closing the launching terminal does not kill the daemon.
	detachedProcess = 0x00000008
	// CREATE_BREAKAWAY_FROM_JOB lets the daemon escape any enclosing Job Object
	// (e.g. CI runners, Windows Terminal) so it can assign its own child
	// processes to a new job without hitting ERROR_ACCESS_STATE_CHANGE.
	createBreakawayFromJob = 0x01000000
)

func forkSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess | createBreakawayFromJob,
	}
}
