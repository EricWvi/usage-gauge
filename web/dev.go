//go:build !production

// Package web leaves frontend serving to Vite during development.
package web

import "net/http"

// Handler returns nil so the development backend only registers API routes.
func Handler() (http.Handler, error) { return nil, nil }
