package ui

import (
	"embed"
	"io/fs"
)

const (
	// BasePath is the HTTP route prefix used when the embedded dashboard shell
	// is served by the agent-factory API server.
	BasePath = "/dashboard/ui"
)

var (
	distFSProvider = fallbackDistFS

	//go:embed fallback_dist fallback_dist/* fallback_dist/assets fallback_dist/assets/*
	fallbackDist embed.FS
)

// DistFS returns the embedded production dashboard assets rooted at dist/.
//
// Fresh checkouts fall back to a tiny stable shell so backend-only work can
// still compile and exercise the dashboard route without requiring a Bun build.
// `make ui-build` generates a registration file that swaps this provider to the
// real normalized dist output for CI and release-style verification.
func DistFS() (fs.FS, error) {
	return distFSProvider()
}

func fallbackDistFS() (fs.FS, error) {
	return fs.Sub(fallbackDist, "fallback_dist")
}
