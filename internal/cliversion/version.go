package cliversion

import (
	"runtime/debug"
	"strings"
)

const fallbackVersion = "dev"

// String returns the installed CLI version suitable for machine-readable stdout
// probes. Release builds expose the main module version from build metadata;
// local development builds fall back to a stable dev token.
func String() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return fallbackVersion
	}

	version := strings.TrimSpace(info.Main.Version)
	if version == "" || version == "(devel)" {
		return fallbackVersion
	}
	return version
}
