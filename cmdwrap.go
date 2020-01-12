package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

type cmdWrapper struct {
	command string
	shell   string
	cmd     *exec.Cmd
}

// Start creates a new process with the given bash command, starts it, and
// sets it as the wrapped command. If exec.Cmd.Start returns an error, the
// last wrapped cmd will be left in place.
func (cw *cmdWrapper) Start() error {
	cmd := exec.Command(*shell, "-c", *command)
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
	command        string
	shell          string
	cond           *sync.Cond
	waitErr        error
	waitFinished   bool
	reloadGen      int
	waitForCommand bool
	preventReloads bool
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
		// Unlock is here to allow terminate to take care of that itself.
		cs.cond.L.Unlock()
		cs.terminate()
		cs.cond.L.Lock()
		if !cs.waitFinished {
			panic("previous command run did not complete before it was attempted to be run again")
		}
	}

	cs.waitFinished = false
	cs.waitErr = nil

	log.Printf("running '%s'\n", cs.command)
	cs.cmd = &cmdWrapper{
		command: cs.command,
		shell:   cs.shell,
	}

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
			panic(fmt.Sprintf("justrun: internal assertion failure: want command generation %d, got generation %d. Please file a ticket.", cmdGen, cs.reloadGen))
		}
		cs.waitErr = err
		cs.waitFinished = true
		cs.cond.Broadcast()
	}(cs.cmd.cmd, cs.reloadGen)

	if cs.waitForCommand {
		// Unlock is here to allow the code that furnishes the error returned from the
		// channel receive to take the lock itself.

		cs.cond.L.Unlock()
		cs.wait()
		cs.cond.L.Lock()
		if err != nil {
			log.Printf("command finished with error: %s", err)
		}
	}
	return
}

// Terminate shuts down the command process and makes future calls to Reload
// return without actually reloading the command. It will not return until the
// Wait of process created by the cmdReloader has finished. This will never
// return if the process is hung.
func (cs *cmdReloader) Terminate() {
	cs.cond.L.Lock()
	cs.preventReloads = true
	cs.cond.L.Unlock()
	cs.terminate()
}

func isTerminated(err error) bool {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	// taken from exec.ExitError.Error(), which calls os.ProcessState.String()
	status := exitErr.ProcessState.Sys().(syscall.WaitStatus)
	if !status.Signaled() {
		return false
	}
	return status.Signal() == syscall.SIGTERM
}

// terminate must be called without cs.cond.L being held.
func (cs *cmdReloader) terminate() {
	pid := cs.cmd.cmd.Process.Pid
	msg := "terminating current command"
	if *verbose {
		msg += fmt.Sprintf(" pid %d", pid)
	}
	log.Println(msg)

	cs.cond.L.Lock()
	defer cs.cond.L.Unlock()
	err := cs.cmd.Terminate()
	if *verbose && err != nil && err != syscall.ESRCH {
		log.Printf("error when attempting to terminate pid %d: %s", pid, err)
	}
	cs.cond.L.Unlock()
	err = cs.wait()
	cs.cond.L.Lock()
	if *verbose && err != nil && err != syscall.ESRCH && !isTerminated(err) {
		log.Printf("error in process termination of pid %d: %s", pid, err)
	}
}

// wait must be called without the cs.cond.L being held in order to allow the
// cmd.Wait background goroutine a chance to work.
func (cs *cmdReloader) wait() error {
	cs.cond.L.Lock()
	for !cs.waitFinished {
		cs.cond.Wait()
	}
	err := cs.waitErr
	cs.cond.L.Unlock()
	return err
}
