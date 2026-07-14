package agypty

// ptyAllocation is the platform-specific PTY resource bundle returned by Allocate.
type ptyAllocation interface {
	Close() error
}
