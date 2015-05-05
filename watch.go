package main

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmhodges/justrun/Godeps/_workspace/src/github.com/howeyc/fsnotify"
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
		return nil, fmt.Errorf("unable to create watcher: %s", err)
	}

	userPaths := make(map[string]bool)
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
		err = w.Watch(fullPath)
		if err != nil {
			w.Close()
			return nil, fmt.Errorf("unable to watch '%s': %s", path, err)
		}
		// only to be used for explicit file paths the user gave us, not any future magic globbing.
		userPaths[fullPath] = true
		baseName := filepath.Base(fullPath)
		if strings.HasPrefix(baseName, ".") {
			includedHiddenFiles[fullPath] = true
		}
	}

	// Folks might want to watch only "foobar" but their tooling moves
	// foobar away and then back (like vim). This will cause the watch
	// to fire on the first move and never again. So, we have to track
	// the parent directory of foobar in order to capture when foobar
	// shows up in its parent directory again but we don't want to
	// send all events in that parent directory.

	renameDirs := make(map[string]bool)
	renameChildren := make(map[string]bool)
	for fpath, _ := range userPaths {
		dirPath := filepath.Dir(fpath)
		if !userPaths[dirPath] && dirPath != "" {
			if !renameDirs[dirPath] {
				err = w.Watch(dirPath)
				if err != nil {
					w.Close()
					return nil, fmt.Errorf("unable to watch rename-watched-only dir '%s': %s", fpath, err)
				}
			}
			renameDirs[dirPath] = true
			renameChildren[fpath] = true
		}
	}
	ig := &smartIgnorer{
		includedHiddenFiles: includedHiddenFiles,
		ui:                  ui,
		renameDirs:          renameDirs,
		renameChildren:      renameChildren,
	}

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
				log.Printf("filtered file change: %s", ev)
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
