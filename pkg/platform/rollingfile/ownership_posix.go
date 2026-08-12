//go:build linux || darwin

package rollingfile

import (
	"io/fs"
	"os"
	"syscall"
)

func preserveFileOwnership(path string, sourceInfo fs.FileInfo) error {
	stat, ok := sourceInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return os.Chown(path, int(stat.Uid), int(stat.Gid))
}
