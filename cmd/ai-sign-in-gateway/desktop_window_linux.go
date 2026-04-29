//go:build desktop_shell && linux

package main

/*
#cgo linux CFLAGS: -Wno-deprecated-declarations
#cgo linux pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdlib.h>

static void set_ai_sign_in_window_icon(void *window, const char *icon_path) {
  if (window == NULL || icon_path == NULL || icon_path[0] == '\0') {
    return;
  }
  gtk_window_set_icon_from_file(GTK_WINDOW(window), icon_path, NULL);
}
*/
import "C"

import "unsafe"

func setDesktopWindowIcon(window unsafe.Pointer) {
	iconPath := desktopIconFilePath()
	if iconPath == "" {
		return
	}
	cPath := C.CString(iconPath)
	defer C.free(unsafe.Pointer(cPath))
	C.set_ai_sign_in_window_icon(window, cPath)
}
