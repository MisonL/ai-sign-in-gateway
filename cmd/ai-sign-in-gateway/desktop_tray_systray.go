//go:build desktop_shell && !linux

package main

import "github.com/getlantern/systray"

func trayRun(onReady func(), onExit func()) {
	systray.Run(onReady, onExit)
}

func trayQuit() {
	systray.Quit()
}

func traySetIcon(iconBytes []byte) {
	systray.SetIcon(iconBytes)
}

func traySetTitle(title string) {
	systray.SetTitle(title)
}

func traySetTooltip(tooltip string) {
	systray.SetTooltip(tooltip)
}

func trayAddMenuItem(title string, tooltip string) *trayMenuItem {
	item := systray.AddMenuItem(title, tooltip)
	return &trayMenuItem{ClickedCh: item.ClickedCh, impl: item}
}

func trayAddSeparator() {
	systray.AddSeparator()
}

func (item *trayMenuItem) native() *systray.MenuItem {
	return item.impl.(*systray.MenuItem)
}

func (item *trayMenuItem) Disable() {
	item.native().Disable()
}

func (item *trayMenuItem) SetTitle(title string) {
	item.native().SetTitle(title)
}

func (item *trayMenuItem) SetTooltip(tooltip string) {
	item.native().SetTooltip(tooltip)
}
