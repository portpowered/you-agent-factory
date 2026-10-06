//go:build !linux && !windows

package process

// A numeric group cannot fence PID reuse without a retained ownership lifetime.
func (*commandProcessTree) ownedControl(<-chan struct{}, Clock) *ownedCommandControl {
	return nil
}
