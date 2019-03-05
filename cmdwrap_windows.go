//+build windows

package main

import (
	"errors"
	"os"
	"syscall"
)

func sysProcAttr() *syscall.SysProcAttr {
	return &windows.SysProcAttr{}
}

func (cw *cmdWrapper) Terminate() error {
	if cw.cmd == nil {
		return errors.New("not started")
	}
	p, err := os.FindProcess(cw.cmd.Process.Pid)

	if err != nil {
		return err
	}

	return p.Signal(syscall.SIGTERM)
}
