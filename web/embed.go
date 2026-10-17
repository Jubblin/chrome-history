// Package web holds the embedded static UI served by the local server.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var assets embed.FS

// FS contains only the static frontend files (the contents of static/),
// so Go sources such as this file are never served.
var FS fs.FS = mustSub()

func mustSub() fs.FS {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
