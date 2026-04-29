//go:build desktop_shell && !linux

package main

import "unsafe"

func setDesktopWindowIcon(unsafe.Pointer) {}
