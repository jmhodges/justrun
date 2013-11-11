package main

import (
	"flag"
	"fmt"
	"github.com/howeyc/fsnotify"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"syscall"
)

var (
	help    = flag.Bool("help", false, "help")
	command = flag.String("c", "", "command to run when files change in given directories")
	ignore  = flag.String("i", "", "comma separated list of files to ignore")
)

func usage() {
	fmt.Fprintf(os.Stderr, "justrun -c 'SOME BASH COMMAND' [FILEPATH...]\n")
	os.Exit(1)
}

func main() {
	flag.Parse()
	if *help {
		usage()
	}
	ignoreNames := strings.Split(*ignore, ",")
	ignored := make(map[string]bool)
	for _, in := range ignoreNames {
		a, err := filepath.Abs(in)
		if err != nil {
			log.Fatalf("unable to get current working dir")
		}
		ignored[a] = true
	}
	cmdCh := make(chan time.Time)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		for {
			select {
			case ev := <-w.Event:
				en, err := filepath.Abs(ev.Name)
				if err != nil {
					log.Fatalf("unable to get current working dir")
				}
				if ignored[en] {
					continue
				}
				//				log.Printf("event %s: file %s", ev, ev.Name)
				cmdCh <- time.Now()
			case err := <-w.Error:
				log.Println("error:", err)
			}
		}
	}()

	if len(flag.Args()) == 0 {
		usage()
	}
	for _, path := range flag.Args() {
		err = w.Watch(path)
		if err != nil {
			log.Fatalf("unable to watch: %s", err)
		}
	}
	lastStartTime := time.Unix(0, 0)
	var proc *os.Process
	done := make(chan error)
	for t := range cmdCh {
		log.Printf("event at %d", t)
		if t.After(lastStartTime.Add(30 * time.Millisecond)) {
			if proc != nil {
				shutdownCommand(proc, done)
			}
			lastStartTime = time.Now()
			proc = runCommand(proc, done)
		}
	}
}

func runCommand(oldProc *os.Process, done chan error) *os.Process {
	log.Println("running")
	cmd := exec.Command("bash", "-c", *command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		log.Printf("error process %d", cmd.Process)
		log.Printf("command failed: %s", err)
		return oldProc
	}
	log.Printf("got process %d", cmd.Process.Pid)
	
	go func() {
		err := cmd.Wait()
		log.Printf("exiting process %d", cmd.Process.Pid)
		done <- err
	}()
	return cmd.Process
}

func shutdownCommand(proc *os.Process, done chan error) {
	err := syscall.Kill(proc.Pid, syscall.SIGKILL)
	if err != nil {
		log.Printf("returning after first kill")
		return
	}
	select {
	case <-done:
		break
	case <-time.After(300 * time.Millisecond):
		err := syscall.Kill(proc.Pid, syscall.SIGKILL)
		log.Printf("kill error?: %#v", err)
		if err != nil {
			log.Printf("returning after kill")
			return
		}
	}
	log.Printf("Shutdown cleanly, i guess? %d", proc.Pid)
}
