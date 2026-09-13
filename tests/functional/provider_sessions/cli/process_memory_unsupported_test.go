//go:build !windows

package cli_test

import "errors"

func processWorkingSetHighWater() (uint64, error) {
	return 0, errors.New("working-set high-water is only available on Windows")
}
