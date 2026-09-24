// Package web embeds the production build of the Vue SPA. Run `make web`
// before building the binary; without it only the dist/.gitkeep placeholder
// is embedded and the server reports that the UI is not built.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the embedded build output rooted at web/dist.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // unreachable: "dist" is a valid embedded directory
	}
	return sub
}
