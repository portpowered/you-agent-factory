//go:build !windows

package platform_conformance

import "os"

func syncAtomicDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
