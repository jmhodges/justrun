package main

import (
	"path/filepath"
	"strings"
)

type Ignorer interface {
	IsIgnored(path string) bool
}

type userIgnorer struct {
	ignored     map[string]bool
	ignoredDirs []string
}

func (ug *userIgnorer) IsIgnored(path string) bool {
	if ug.ignored[path] {
		return true
	}
	for _, dir := range ug.ignoredDirs {
		if strings.HasPrefix(path, dir) {
			return true
		}
	}
	return false
}

type smartIgnorer struct {
	includedHiddenFiles map[string]bool
	ui                  *userIgnorer
}

func (si *smartIgnorer) IsIgnored(path string) bool {
	if si.ui.IsIgnored(path) {
		return true
	}
	baseName := filepath.Base(path)
	if strings.HasPrefix(baseName, ".") && !si.includedHiddenFiles[path] {
		return true
	}
	return false
}
