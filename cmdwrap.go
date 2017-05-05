package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type cmdWrapper struct {
	command string
	cmd     *exec.Cmd
}

// Start creates a new process with the given bash command, starts it, and
// sets it as the wrapped command. If exec.Cmd.Start returns an error, the
// last wrapped cmd will be left in place.
func (cw *cmdWrapper) Start() error {
	cmd := exec.Command("bash", "-c", *command)
	// Necessary so that the SIGTERM's in Terminate will traverse down to the
	// the child processes in the bash command above.
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
	if cw.cmd == nil {
		return errors.New("not started")
	}
	// The negation here means to kill, not just the parent pid (which
	// is the bash shell), but also its children. This means that even
	// long-lived servers can be gently killed (e.g "-c 'go
	// build && ./myserver -http=:6000'"). fswatch and other systems
	// can't do this.
	return syscall.Kill(-cw.cmd.Process.Pid, syscall.SIGTERM)
}

func (cw *cmdWrapper) Wait() error {
	return cw.cmd.Wait()
}

type cmdReloader struct {
	cond           *sync.Cond
	waitErr        error
	waitFinished   bool
	reloadGen      int
	waitForCommand bool
	preventReloads bool
	command        string
	cmd            *cmdWrapper
}

// Reload stops the currently running process started by a previous Reload (if
// called) and starts a new one. If Terminate has been previously called, it
// will do nothing.
func (cs *cmdReloader) Reload() {
	cs.cond.L.Lock()
	defer cs.cond.L.Unlock()

	if cs.preventReloads {
		// unable to reload the command because we are stopping but we don't
		// want to have the main goroutine error out.
		return
	}

	if cs.cmd != nil {
		cs.terminate()
	}

	log.Printf("running '%s'\n", cs.command)
	cs.cmd = &cmdWrapper{command: cs.command}

	err := cs.cmd.Start()
	if err != nil {
		log.Printf("command failed: %s", err)
		return
	}
	cs.reloadGen++

	go func(cmd *exec.Cmd, cmdGen int) {
		err := cmd.Wait()
		cs.cond.L.Lock()
		defer cs.cond.L.Unlock()
		if cs.reloadGen != cmdGen {
			panic(fmt.Sprintf("justrun: interal assertion failure: want command generation %d, got generation %d. Please file a ticket.", cmdGen, cs.reloadGen))
		}
		cs.waitErr = err
		cs.waitFinished = true
		cs.cond.Broadcast()
	}(cs.cmd.cmd, cs.reloadGen)

	if cs.waitForCommand {
		err := <-cs.waitChan()
		if err != nil {
			log.Printf("command finished with error: %s", err)
		}
	}
	return
}

// Terminate shuts down the command process and silently prevents Reload from
// actually reloading. It will not return until the Wait of process created by
// the cmdReloader has finished. This will never return if the process is hung.
func (cs *cmdReloader) Terminate() {
	cs.cond.L.Lock()
	cs.preventReloads = true
	cs.terminate()
	cs.cond.L.Unlock()
	// The read of this channel is deliberately left outside of the lock.
	<-cs.waitChan()
}

func (cs *cmdReloader) terminate() {
	err := cs.cmd.Terminate()
	if err == syscall.ESRCH {
		return
	}

	done := cs.waitChan()
	// Done is sent to after the command's Wait call returns. But it's really a
	// latency optimization (or spin-loop prevention) over polling the process
	// with Terminate until the process stops existing (when syscall.ESRCH
	// will be returned).
	for err != syscall.ESRCH {
		select {
		case <-done:
			break
		case <-time.After(50 * time.Millisecond):
			err = cs.cmd.Terminate()
		}
	}
	msg := "terminating current command"
	if *verbose {
		msg += fmt.Sprintf(" %d", cs.cmd.cmd.Process.Pid)
	}
	log.Println(msg)
}

func (cs *cmdReloader) waitChan() <-chan error {
	done := make(chan error, 1)
	go func(done chan<- error) {
		cs.cond.L.Lock()
		for !cs.waitFinished {
			cs.cond.Wait()
		}
		err := cs.waitErr
		cs.cond.L.Unlock()
		done <- err
	}(done)
	return done
}
