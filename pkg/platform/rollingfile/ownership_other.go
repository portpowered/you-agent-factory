//go:build !linux && !darwin

package rollingfile

import "io/fs"

func preserveFileOwnership(string, fs.FileInfo) error { return nil }
