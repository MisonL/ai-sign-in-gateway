//go:build desktop_shell

package main

type trayMenuItem struct {
	ClickedCh chan struct{}
	impl      any
	id        int
}
