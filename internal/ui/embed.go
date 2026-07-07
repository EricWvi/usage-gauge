// Package ui embeds the HTML templates and static assets so the binary is
// self-contained; only endpoints.yaml, user parsers and gauge.db are external.
package ui

import "embed"

// Files holds templates/ and static/ at their original paths.
//
//go:embed templates/* static/*
var Files embed.FS
