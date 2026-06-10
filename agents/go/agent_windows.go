//go:build windows
// +build windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func acquireSingletonLock() (func(), bool) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	createMutex := kernel32.NewProc("CreateMutexW")
	getLastError := kernel32.NewProc("GetLastError")

	mutexName, _ := syscall.UTF16PtrFromString("Global\\MinotaurAgentLock")
	handle, _, _ := createMutex.Call(0, 0, uintptr(unsafe.Pointer(mutexName)))
	if handle == 0 {
		return nil, false
	}
	errCode, _, _ := getLastError.Call()
	if errCode == 183 { // ERROR_ALREADY_EXISTS
		syscall.CloseHandle(syscall.Handle(handle))
		return nil, false
	}
	release := func() {
		syscall.CloseHandle(syscall.Handle(handle))
	}
	return release, true
}

func updateScriptInProgress() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}
	dir := filepath.Dir(exePath)
	if _, err := os.Stat(filepath.Join(dir, "minotaur_update.bat")); err == nil {
		return true
	}
	return false
}