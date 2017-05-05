package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	help           = flag.Bool("help", false, "print this help text")
	h              = flag.Bool("h", false, "print this help text")
	command        = flag.String("c", "", "command to run when files change in given directories")
	ignoreFlag     pathsFlag
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
	flag.Var(&ignoreFlag, "i", "a file path to ignore events from (may be given multiple times)")
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

	cmd := &cmdReloader{
		cond:           &sync.Cond{L: new(sync.Mutex)},
		command:        *command,
		waitForCommand: *waitForCommand,
	}

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go waitForInterrupt(sigCh, cmd)

	cmdCh := make(chan event, 100)
	_, err := watch(inputPaths, ignoreFlag, cmdCh)
	if err != nil {
		log.Fatal(err)
	}

	wasDelayed := false

	lastStartTime := time.Now()
	cmd.Reload()
	tick := time.NewTicker(*delayDur)
	for {
		select {
		case ev, ok := <-cmdCh:
			if !ok {
				return
			}
			if lastStartTime.After(ev.Time) {
				continue
			}
			// Using delayDur here and in NewTicker is slightly semantically
			// incorrect, but it simplifies our config and prevents the
			// egregious reloading.
			if time.Now().Sub(ev.Time) < *delayDur {
				wasDelayed = true
				continue
			}
			wasDelayed = false
			lastStartTime = time.Now()
			cmd.Reload()
			tick.Stop()
			tick = time.NewTicker(*delayDur)
		case <-tick.C:
			if wasDelayed {
				wasDelayed = false
				lastStartTime = time.Now()
				cmd.Reload()
			}
		}
	}
}

func waitForInterrupt(sigCh chan os.Signal, cmd *cmdReloader) {
	for range sigCh {
		go func() {
			cmd.Terminate()
			os.Exit(0)
		}()
	}
}

type pathsFlag []string

func (pf *pathsFlag) String() string {
	return fmt.Sprint(*pf)
}

func (pf *pathsFlag) Set(value string) error {
	// TODO(jmhodges): remove comma Split in 2.0
	// Only for backwards compatibilty with old -i
	vals := strings.Split(value, ",")
	before := len(*pf)
	for _, p := range vals {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		*pf = append(*pf, p)
	}
	if before == len(*pf) {
		return errors.New("a file path may not be blank")
	}
	return nil
}
