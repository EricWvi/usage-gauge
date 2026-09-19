//go:build production

// Package web serves the frontend in production builds.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// files contains the Vite output. Docker generates dist before compiling Go;
// generated assets are not tracked in Git.
//
//go:embed all:dist
var files embed.FS

// Handler serves the embedded frontend in production builds.
func Handler() (http.Handler, error) {
	assets, err := fs.Sub(files, "dist")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(assets)), nil
}
