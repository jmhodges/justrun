package main

import (
	"flag"
	"fmt"
	"github.com/howeyc/fsnotify"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	help     = flag.Bool("help", false, "help")
	command  = flag.String("c", "", "command to run when files change in given directories")
	ignore   = flag.String("i", "", "comma separated list of files to ignore")
	delayDur = flag.Duration("delay", 750*time.Millisecond, "the time to wait between runs of the command if many fs events occur")
)

func usage() {
	fmt.Fprintf(os.Stderr, "justrun -c 'SOME BASH COMMAND' [FILEPATH]*\n")
	os.Exit(1)
}

// TODO handle ignored directories
func main() {
	flag.Parse()
	if *help {
		usage()
	}
	if len(flag.Args()) == 0 {
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

	cmd := &cmdWrapper{Mutex: new(sync.Mutex), command: *command, cmd: nil}

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Interrupt)
	go waitForInterrupt(sigCh, cmd)

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

	for _, path := range flag.Args() {
		err = w.Watch(path)
		if err != nil {
			log.Fatalf("unable to watch '%s': %s", path, err)
		}
	}
	lastStartTime := time.Unix(0, 0)
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
			reload(cmd, done, &lastStartTime)
			tick = time.NewTicker(*delayDur)
		case <-tick.C:
			if wasDelayed {
				wasDelayed = false
				reload(cmd, done, &lastStartTime)
			}
		}
	}
}

func reload(cmd *cmdWrapper, done chan error, lastStartTime *time.Time) {
	shutdownCommand(cmd, done)
	*lastStartTime = time.Now()
	runCommand(cmd, done)
}

func runCommand(cmd *cmdWrapper, done chan error) {
	log.Println("running " + *command)
	err := cmd.Start()
	if err != nil {
		log.Printf("command failed: %s", err)
		return
	}
	go func() {
		err := cmd.Wait()
		done <- err
	}()
}

func shutdownCommand(cmd *cmdWrapper, done chan error) {
	err := cmd.Terminate()
	if err != nil {
		return
	}

waitOrRetry:
	select {
	case <-done:
		break
	case <-time.After(300 * time.Millisecond):
		err := cmd.Terminate()
		if err == nil {
			// If terminate claims to succeed, we want to make sure the done
			// message came across. If the done message doesn't come, the
			// process didn't die anad we need to try terminating again.
			goto waitOrRetry
		}
		break
	}
	log.Printf("terminating command %d\n", cmd.cmd.Process.Pid)
}

func waitForInterrupt(sigCh chan os.Signal, cmd *cmdWrapper) {
	defer os.Exit(0)
	<-sigCh
	err := cmd.Terminate()
	if err != nil {
		log.Printf("on interrupt, unable to kill command: %s", err)
	}
}
