//go:build embedspa

package main

import (
	"embed"
	"io/fs"
)

//go:embed all:auth-server-webui/dist
var embeddedSPA embed.FS

// spaAssetsFS returns the built SPA files embedded by the embedspa build tag.
func spaAssetsFS() fs.FS {
	dist, err := fs.Sub(embeddedSPA, "auth-server-webui/dist")
	if err != nil {
		panic(err)
	}
	return dist
}
