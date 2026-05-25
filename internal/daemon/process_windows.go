//go:build windows

package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobObject creation flag: kill all processes in the job when the last job
// handle is closed or when TerminateJobObject is called.
const jobObjectLimitKillOnJobClose = 0x00002000

// createNoWindow prevents PowerShell (and its children) from creating a visible
// console window. Without this, spawning a console app from a DETACHED_PROCESS
// daemon causes Windows to allocate a new console window for each child —
// visible as flashing terminal popups during worktree activation.
const createNoWindow = 0x08000000

// jobObjectExtendedLimitInformation is the info class for SetInformationJobObject.
const jobObjectExtendedLimitInformationClass = 9

// These structs mirror the Win32 JOBOBJECT_BASIC_LIMIT_INFORMATION,
// IO_COUNTERS, and JOBOBJECT_EXTENDED_LIMIT_INFORMATION layouts.
// Field order and types match the Windows ABI on 64-bit; Go's automatic
// alignment produces the same padding as the C structs.

type jobobjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobobjectExtendedLimitInformation struct {
	BasicLimitInformation jobobjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

type Process struct {
	cmd *exec.Cmd
	pid int
	job windows.Handle
}

// StartProcess spawns command via PowerShell in a Job Object so the entire
// process tree (shell + grandchildren) can be terminated atomically on Stop.
//
// The process is assigned to the job immediately after Start(). There is a
// narrow race before PowerShell can spawn any children (~hundreds of ms),
// which is acceptable in practice. If a future startup command is a fast-
// spawning native binary, the correct fix is to use CreateProcess with
// CREATE_SUSPENDED, assign to the job, then ResumeThread — but that requires
// bypassing exec.Cmd and losing its stdio plumbing. Deliberate trade-off.
func StartProcess(command, dir string) (*Process, error) {
	name, args := shellCommand(command)
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateJobObject: %w", err)
	}

	info := jobobjectExtendedLimitInformation{
		BasicLimitInformation: jobobjectBasicLimitInformation{
			LimitFlags: jobObjectLimitKillOnJobClose,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		jobObjectExtendedLimitInformationClass,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("SetInformationJobObject: %w", err)
	}

	if err := cmd.Start(); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}

	procHandle, err := windows.OpenProcess(windows.PROCESS_ALL_ACCESS, false, uint32(cmd.Process.Pid))
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait() //nolint:errcheck
		windows.CloseHandle(job)
		return nil, fmt.Errorf("OpenProcess: %w", err)
	}
	err = windows.AssignProcessToJobObject(job, procHandle)
	windows.CloseHandle(procHandle)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait() //nolint:errcheck
		windows.CloseHandle(job)
		return nil, fmt.Errorf("AssignProcessToJobObject: %w", err)
	}

	return &Process{cmd: cmd, pid: cmd.Process.Pid, job: job}, nil
}

func (p *Process) Pid() int { return p.pid }

func (p *Process) Wait() error { return p.cmd.Wait() }

// Stop terminates the entire job object (all processes in the tree) atomically.
// It does NOT call cmd.Wait() — the crash-monitor goroutine started in
// daemon.activate() owns the Wait() call. Calling Wait() here too would race
// with that goroutine on the same exec.Cmd. Instead, Stop polls the process
// handle directly to confirm the root process has exited before returning,
// preserving the invariant that no child holds a port when Stop returns.
func (p *Process) Stop() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	if p.job == 0 {
		return
	}
	if err := windows.TerminateJobObject(p.job, 1); err != nil {
		slog.Warn("TerminateJobObject failed", "pid", p.pid, "err", err)
	}
	// Poll the process handle until the root process exits or the deadline
	// passes, then escalate to a direct Kill. We do not call cmd.Wait() here
	// to avoid racing with the crash-monitor goroutine in daemon.activate().
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(p.pid))
	if err == nil {
		windows.WaitForSingleObject(h, 7000) //nolint:errcheck
		windows.CloseHandle(h)
	} else {
		// Process already gone — nothing to wait on.
	}
	// Belt-and-braces: if the root process somehow survived, force-kill it.
	if isAlive(p.pid) {
		p.cmd.Process.Kill()
	}
	windows.CloseHandle(p.job)
	p.job = 0
}
