// Package web embeds the built interface (web/dist, produced by
// `npm run build`).
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the interface files, and whether the interface was built.
func FS() (fs.FS, bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	_, err = fs.Stat(sub, "index.html")
	return sub, err == nil
}
