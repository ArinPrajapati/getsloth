//go:build !windows

package main

import "errors"

func startWindowsHostControlLauncher(string, []string) error {
	return errors.New("windows terminal launcher is only available on Windows")
}

func startWindowsHostControlFallback(string, []string) error {
	return errors.New("windows new-console launcher is only available on Windows")
}
