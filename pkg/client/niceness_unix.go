//go:build unix

package client

import "syscall"

// setProcessNiceness sets the process niceness to the given value (0-19).
// Higher values mean lower priority.
func setProcessNiceness(nice int) error {
	return syscall.Setpriority(syscall.PRIO_PROCESS, 0, nice)
}
