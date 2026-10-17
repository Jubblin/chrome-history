// Package web holds the embedded static UI served by the local server.
package web

import "embed"

// FS contains the static frontend files.
//
//go:embed *
var FS embed.FS
