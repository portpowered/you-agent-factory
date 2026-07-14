//go:build linux || darwin

package agypty

func newPlatformPTYAllocator() PTYAllocator {
	return NewPOSIXPTYAllocator()
}
