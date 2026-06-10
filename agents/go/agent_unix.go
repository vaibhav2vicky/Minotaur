//go:build !windows
// +build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

func acquireSingletonLock() (func(), bool) {
	lockPath := filepath.Join(os.TempDir(), "minotaur_agent.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, false
	}
	release := func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
		os.Remove(lockPath)
	}
	return release, true
}

func updateScriptInProgress() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}
	dir := filepath.Dir(exePath)
	if _, err := os.Stat(filepath.Join(dir, "minotaur_update.sh")); err == nil {
		return true
	}
	return false
}