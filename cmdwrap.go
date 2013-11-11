package main

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

type cmdWrapper struct {
	*sync.Mutex
	command string
	cmd     *exec.Cmd
}

// Start creates a new process with the given bash command, starts it, and
// sets it as the wrapped command. If exec.Cmd.Start returns an error, the
// last wrapped cmd will be left in place.
func (cw *cmdWrapper) Start() error {
	cw.Lock()
	defer cw.Unlock()
	cmd := exec.Command("bash", "-c", *command)
	// Necessary so that the SIGTERM's in Terminate will traverse down to the
	// the child processes in the bash command below.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	if err != nil {
		return err
	}
	cw.cmd = cmd
	return nil
}

func (cw *cmdWrapper) Terminate() error {
	cw.Lock()
	defer cw.Unlock()
	if cw.cmd == nil {
		return errors.New("not started")
	}
	// The negation here means to kill not just the parent pid (which is the
	// bash shell), but also its children. This means long-lived servers can
	// be killed as well quickly exiting processes. e.g. "go build &&
	// ./myserver -http=:6000". fswatch and others won't do this.
	return syscall.Kill(-cw.cmd.Process.Pid, syscall.SIGTERM)
}

func (cw *cmdWrapper) Wait() error {
	cw.Lock()
	cmd := cw.cmd
	cw.Unlock()
	return cmd.Wait()
}
