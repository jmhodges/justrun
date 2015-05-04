package main

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/howeyc/fsnotify"
)

func watch(inputPaths, ignoredPaths []string, cmdCh chan<- time.Time) error {
	ignored := make(map[string]bool)
	ignoredDirs := make([]string, 0)
	for _, in := range ignoredPaths {
		in = strings.TrimSpace(in)
		if len(in) == 0 {
			continue
		}
		path, err := filepath.Abs(in)
		if err != nil {
			return errors.New("unable to get current working dir while working with ignored paths")
		}
		ignored[path] = true
		dirPath := path
		if path[len(path)-1] != '/' {
			dirPath += "/"
		}
		ignoredDirs = append(ignoredDirs, dirPath)
	}
	ig := &ignorer{ignored, ignoredDirs}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	go listenForEvents(w, cmdCh, ig)

	for _, path := range inputPaths {
		if ig.IsIgnored(path) {
			continue
		}
		err = w.Watch(path)
		if err != nil {
			return fmt.Errorf("unable to watch '%s': %s", path, err)
		}
	}

	return nil
}

func listenForEvents(w *fsnotify.Watcher, cmdCh chan<- time.Time, ignorer *ignorer) {
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
