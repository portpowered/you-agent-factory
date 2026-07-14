//go:build windows

package agypty

func newPlatformPTYAllocator() PTYAllocator {
	return NewWindowsConPTYAllocator()
}
