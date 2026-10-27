//go:build !embedspa

package main

import "io/fs"

// spaAssetsFS is nil for the default build. The SPA is then served by Vite in
// development; production builds use `go build -tags embedspa`.
func spaAssetsFS() fs.FS {
	return nil
}
