package main

import (
	"flag"
	"fmt"
	"github.com/howeyc/fsnotify"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	help    = flag.Bool("help", false, "help")
	command = flag.String("c", "", "command to run when files change in given directories")
	ignore  = flag.String("i", "", "comma separated list of files to ignore")
	delayDur = flag.Duration("delay", 750 * time.Millisecond, "the time to wait between runs of the command if many fs events occur")

	// These globals are for signal handling. The signal handling is required
	// since we do interesting stuff with setpgid on the child process we
	// create.
	sigLock   = &sync.Mutex{}
	globalCmd *exec.Cmd
)

func usage() {
	fmt.Fprintf(os.Stderr, "justrun -c 'SOME BASH COMMAND' [FILEPATH...]\n")
	os.Exit(1)
}

// TODO make container to clean up locking
// TODO fix [FILEPATH]*
// TODO handle ignored directories
func main() {
	flag.Parse()
	if *help {
		usage()
	}
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Interrupt)
	go waitForInterrupt(sigCh)

	ignoreNames := strings.Split(*ignore, ",")
	ignored := make(map[string]bool)
	for _, in := range ignoreNames {
		a, err := filepath.Abs(in)
		if err != nil {
			log.Fatalf("unable to get current working dir")
		}
		ignored[a] = true
	}
	cmdCh := make(chan time.Time, 100)
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
	var cmd *exec.Cmd
	done := make(chan error)
	wasDelayed := false
	tick := time.NewTicker(*delayDur)
	for {
		select {
		case t := <-cmdCh:
			if lastStartTime.After(t) {
				continue
			}
			// Using delayDur here and in NewTicker is slightly semantically
			// incorrect, but it simplifies our config and prevents the
			// egregious reloading.
			if time.Now().Sub(t) < *delayDur {
				wasDelayed = true
				continue
			}
			wasDelayed = false
			cmd = reload(cmd, done, &lastStartTime)
			tick = time.NewTicker(*delayDur)
		case <-tick.C:
			if wasDelayed {
				wasDelayed = false
				cmd = reload(cmd, done, &lastStartTime)
			}
		}
	}
}

func reload(cmd *exec.Cmd, done chan error, lastStartTime *time.Time) *exec.Cmd {
		if cmd != nil {
			shutdownCommand(cmd, done)
		}
		*lastStartTime = time.Now()
		return runCommand(cmd, done)
}

func runCommand(oldCmd *exec.Cmd, done chan error) *exec.Cmd {
	log.Println("running " + *command)
	cmd := exec.Command("bash", "-c", *command)
	// Necessary so that SIGTERM's will traverse down to the the child
	// processes in the bash command below. See shutdownCommand.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	sigLock.Lock()
	err := cmd.Start()
	if err != nil {
		log.Printf("command failed: %s", err)
		return oldCmd
	}
	globalCmd = cmd
	sigLock.Unlock()
	go func() {
		err := cmd.Wait()
		done <- err
	}()
	return cmd
}

func shutdownCommand(cmd *exec.Cmd, done chan error) {
	sigLock.Lock()
	defer sigLock.Unlock()
	// the negation here means to kill not just the parent pid (which is the
	// bash shell), but also its children. This means long-lived servers can
	// be killed as well quickly exiting processes. e.g. "go build &&
	// ./myserver -http=:6000". fswatch and others won't do this.
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	if err != nil {
		return
	}
	for {
		select {
		case <-done:
			goto done
		case <-time.After(300 * time.Millisecond):
			err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			if err != nil {
				goto done
			}
		}
	}
done:
	log.Printf("terminating command %d\n", cmd.Process.Pid)
}

func waitForInterrupt(sigCh chan os.Signal) {
	defer os.Exit(0)
	<-sigCh
	sigLock.Lock()
	defer sigLock.Unlock()
	if globalCmd == nil {
		return
	}
	err := syscall.Kill(-globalCmd.Process.Pid, syscall.SIGTERM)
	if err != nil {
		log.Printf("on interrupt, unable to kill command: %s", err)
	}
}
