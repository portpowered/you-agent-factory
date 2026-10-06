//go:build !linux && !windows

package process

import "io/fs"

// A numeric group cannot fence PID reuse without a retained ownership lifetime.
func (*commandProcessTree) ownedControl(<-chan struct{}, Clock, fs.FS) *ownedCommandControl {
	return nil
}
