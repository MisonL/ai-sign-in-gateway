//go:build desktop_shell && linux

package main

/*
#cgo linux CFLAGS: -Wno-deprecated-declarations
#cgo linux pkg-config: gtk+-3.0
#include <stdlib.h>
#include "desktop_tray_linux.h"
*/
import "C"

import (
	"os"
	"path/filepath"
	"sync"
	"unsafe"
)

var linuxTrayState = struct {
	sync.Mutex
	items map[int]*trayMenuItem
}{
	items: map[int]*trayMenuItem{},
}

func trayRun(onReady func(), onExit func()) {
	C.desktop_tray_init()
	onReady()
	C.desktop_tray_loop()
	onExit()
}

func trayQuit() {
	C.desktop_tray_quit()
}

func traySetIcon(iconBytes []byte) {
	if len(iconBytes) == 0 {
		return
	}
	path := filepath.Join(os.TempDir(), "ai-sign-in-gateway-tray-icon.png")
	if err := os.WriteFile(path, iconBytes, 0o600); err != nil {
		return
	}
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	C.desktop_tray_set_icon(cPath)
}

func traySetTitle(title string) {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	C.desktop_tray_set_title(cTitle)
}

func traySetTooltip(tooltip string) {
	cTooltip := C.CString(tooltip)
	defer C.free(unsafe.Pointer(cTooltip))
	C.desktop_tray_set_tooltip(cTooltip)
}

func trayAddMenuItem(title string, tooltip string) *trayMenuItem {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	id := int(C.desktop_tray_add_menu_item(cTitle))
	item := &trayMenuItem{ClickedCh: make(chan struct{}, 1), id: id}
	linuxTrayState.Lock()
	linuxTrayState.items[id] = item
	linuxTrayState.Unlock()
	if tooltip != "" {
		item.SetTooltip(tooltip)
	}
	return item
}

func trayAddSeparator() {
	C.desktop_tray_add_separator()
}

func (item *trayMenuItem) Disable() {
	C.desktop_tray_disable_item(C.int(item.id))
}

func (item *trayMenuItem) SetTitle(title string) {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	C.desktop_tray_set_item_title(C.int(item.id), cTitle)
}

func (item *trayMenuItem) SetTooltip(tooltip string) {
	cTooltip := C.CString(tooltip)
	defer C.free(unsafe.Pointer(cTooltip))
	C.desktop_tray_set_item_tooltip(C.int(item.id), cTooltip)
}

//export goDesktopTrayClicked
func goDesktopTrayClicked(id C.int) {
	linuxTrayState.Lock()
	item := linuxTrayState.items[int(id)]
	linuxTrayState.Unlock()
	if item == nil {
		return
	}
	select {
	case item.ClickedCh <- struct{}{}:
	default:
	}
}
