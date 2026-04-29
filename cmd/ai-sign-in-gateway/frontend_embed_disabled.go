//go:build !embedded_assets

package main

import "io/fs"

func embeddedFrontend() (fs.FS, bool) {
	return nil, false
}
