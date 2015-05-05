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

func watch(inputPaths, ignoredPaths []string, cmdCh chan<- time.Time) (*fsnotify.Watcher, error) {
	ignored := make(map[string]bool)
	ignoredDirs := make([]string, 0)
	for _, in := range ignoredPaths {
		in = strings.TrimSpace(in)
		if len(in) == 0 {
			continue
		}
		path, err := filepath.Abs(in)
		if err != nil {
			return nil, errors.New("unable to get current working dir while working with ignored paths")
		}
		ignored[path] = true
		dirPath := path
		if path[len(path)-1] != '/' {
			dirPath += "/"
		}
		ignoredDirs = append(ignoredDirs, dirPath)
	}

	ui := &userIgnorer{ignored, ignoredDirs}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	includedHiddenFiles := make(map[string]bool)
	for _, path := range inputPaths {
		fullPath, err := filepath.Abs(path)
		if err != nil {
			w.Close()
			return nil, errors.New("unable to get current working directory while working with user-watched paths")
		}
		if ui.IsIgnored(path) {
			continue
		}
		err = w.Watch(path)
		if err != nil {
			w.Close()
			return nil, fmt.Errorf("unable to watch '%s': %s", path, err)
		}
		baseName := filepath.Base(fullPath)
		if strings.HasPrefix(baseName, ".") {
			includedHiddenFiles[fullPath] = true
		}
	}
	ig := &smartIgnorer{includedHiddenFiles: includedHiddenFiles, ui: ui}

	go listenForEvents(w, cmdCh, ig)
	return w, nil
}

func listenForEvents(w *fsnotify.Watcher, cmdCh chan<- time.Time, ignorer Ignorer) {
	for {
		select {
		case ev := <-w.Event:
			// w.Close causes this.
			if ev == nil {
				return
			}
			if ignorer.IsIgnored(ev.Name) {
				continue
			}
			if *verbose {
				log.Printf("file changed: %s", ev)
			}
			cmdCh <- time.Now()
		case err := <-w.Error:
			if err == nil {
				return
			}
			log.Println("watch error:", err)
		}
	}
}
