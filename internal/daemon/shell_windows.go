//go:build windows

package daemon

// shellCommand returns the shell and arguments for running command on Windows.
// powershell.exe (Windows PowerShell 5.1) is always present on Windows 10+.
// -NoProfile skips loading the user profile (avoids adding seconds of cold-start).
// -NonInteractive prevents the shell from blocking on prompts.
// Using ; as the statement separator matches POSIX shells, so the same
// config.yaml works unchanged across platforms.
func shellCommand(command string) (string, []string) {
	return "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", command}
}
