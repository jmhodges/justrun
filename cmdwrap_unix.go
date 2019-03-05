//+build !windows

package main

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

func sysProcAttr() *syscall.SysProcAttr {
	// Necessary so that the SIGTERM's in Terminate will traverse down to the
	// the child processes in the bash command above.
	return &unix.SysProcAttr{Setpgid: true}
}

func (cw *cmdWrapper) Terminate() error {
	if cw.cmd == nil {
		return errors.New("not started")
	}
	// The negation here means to kill, not just the parent pid (which
	// is the bash shell), but also its children. This means that even
	// long-lived servers can be gently killed (e.g "-c 'go
	// build && ./myserver -http=:6000'"). fswatch and other systems
	// can't do this.
	return unix.Kill(-cw.cmd.Process.Pid, unix.SIGTERM)
}
