//go:build !desktop_shell

package main

import (
	"context"
	"errors"
)

func desktopShellAvailable() bool {
	return false
}

func desktopWindowURL() string {
	return ""
}

func runDesktopWindow(string) error {
	return errors.New("desktop shell is not enabled in this build")
}

func runDesktopShell(context.Context, desktopRuntime) error {
	return errors.New("desktop shell is not enabled in this build")
}
