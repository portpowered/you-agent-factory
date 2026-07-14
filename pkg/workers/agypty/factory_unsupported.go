//go:build !windows && !linux && !darwin

package agypty

func newPlatformPTYAllocator() PTYAllocator {
	return NewUnsupportedPTYAllocator()
}
