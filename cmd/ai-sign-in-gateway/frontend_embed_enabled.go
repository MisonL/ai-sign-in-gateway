//go:build embedded_assets

package main

import (
	"embed"
	"io/fs"
)

//go:embed embedded_dist/dist/*
//go:embed embedded_dist/dist/assets/*
var embeddedDist embed.FS

func embeddedFrontend() (fs.FS, bool) {
	dist, err := fs.Sub(embeddedDist, "embedded_dist/dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(dist, "index.html"); err != nil {
		return nil, false
	}
	return dist, true
}
