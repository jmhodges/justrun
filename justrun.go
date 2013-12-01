package main

import (
	"bufio"
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
	help           = flag.Bool("help", false, "print this help text")
	h              = flag.Bool("h", false, "print this help text")
	command        = flag.String("c", "", "command to run when files change in given directories")
	ignore         = flag.String("i", "", "comma-separated list of files to ignore")
	stdin          = flag.Bool("stdin", false, "read list of files to track from stdin, not the command-line")
	waitForCommand = flag.Bool("w", false, "wait for the command to finish and do not attempt to kill it")
	delayDur       = flag.Duration("delay", 750*time.Millisecond, "the time to wait between runs of the command if many fs events occur")
	verbose        = flag.Bool("v", false, "verbose output")
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: justrun -c 'SOME BASH COMMAND' [FILEPATH]*\n")
	flag.PrintDefaults()
	os.Exit(1)
}

func argError(format string, obj ...interface{}) {
	fmt.Fprintf(os.Stderr, "justrun: "+format+"\n", obj...)
	usage()
}

func main() {
	flag.Usage = usage
	flag.Parse()
	if *help || *h {
		argError("help requested")
	}
	if len(*command) == 0 {
		argError("no command given with -c")
	}
	if *stdin && len(flag.Args()) != 0 {
		argError("expected files to come in over stdin, but got paths '%s' in the commandline", strings.Join(flag.Args(), ", "))
	}
	var inputPaths []string
	if *stdin {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			inputPaths = append(inputPaths, sc.Text())
		}
		if sc.Err() != nil {
			argError("error reading from stdin: %s", sc.Err())
		}
	} else {
		inputPaths = flag.Args()
	}

	if len(inputPaths) == 0 {
		argError("no file paths provided to watch")
	}
	ignoreNames := strings.Split(*ignore, ",")
	ignored := make(map[string]bool)
	ignoredDirs := make([]string, 0)
	for _, in := range ignoreNames {
		in = strings.TrimSpace(in)
		if len(in) == 0 {
			continue
		}
		path, err := filepath.Abs(in)
		if err != nil {
			log.Fatalf("unable to get current working dir")
		}
		ignored[path] = true
		dirPath := path
		if path[len(path)-1] != '/' {
			dirPath += "/"
		}
		ignoredDirs = append(ignoredDirs, dirPath)
	}
	ig := &ignorer{ignored, ignoredDirs}
	cmd := &cmdWrapper{Mutex: new(sync.Mutex), command: *command, cmd: nil}

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Interrupt)
	go waitForInterrupt(sigCh, cmd)

	cmdCh := make(chan time.Time, 100)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	go listenForEvents(w, cmdCh, ig)

	for _, path := range inputPaths {
		if ig.IsIgnored(path) {
			continue
		}
		err = w.Watch(path)
		if err != nil {
			log.Fatalf("unable to watch '%s': %s", path, err)
		}
	}
	lastStartTime := time.Unix(0, 0)
	done := make(chan error)
	wasDelayed := false
	reload(cmd, done, &lastStartTime)
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
	log.Printf("running '%s'\n",  *command)
	err := cmd.Start()
	if err != nil {
		log.Printf("command failed: %s", err)
		return
	}
	if *waitForCommand {
		err := cmd.Wait()
		go func() { done <- err }()
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
	msg := "terminating current command"
	if *verbose {
		msg += fmt.Sprintf(" %d", cmd.cmd.Process.Pid)
	}
	log.Println(msg)
}

func waitForInterrupt(sigCh chan os.Signal, cmd *cmdWrapper) {
	defer os.Exit(0)
	<-sigCh
	err := cmd.Terminate()
	if err != nil {
		log.Printf("on interrupt, unable to kill command: %s", err)
	}
}

type ignorer struct {
	ignored     map[string]bool
	ignoredDirs []string
}

func (ig *ignorer) IsIgnored(path string) bool {
	en, err := filepath.Abs(path)
	if err != nil {
		log.Fatalf("unable to get current working dir")
	}
	if ig.ignored[en] {
		return true
	}
	for _, dir := range ig.ignoredDirs {
		if strings.HasPrefix(en, dir) {
			return true
		}
	}
	return false
}

func listenForEvents(w *fsnotify.Watcher, cmdCh chan time.Time, ignorer *ignorer) {
	for {
		select {
		case ev := <-w.Event:
			if ignorer.IsIgnored(ev.Name) {
				continue
			}
			if *verbose {
				log.Printf("file changed: %s", ev)
			}
			cmdCh <- time.Now()
		case err := <-w.Error:
			log.Println("error:", err)
		}
	}
}
