package agypty

// DefaultPlatformAllocatorFactory returns the platform PTY allocator for the
// current OS build.
type DefaultPlatformAllocatorFactory struct{}

// NewDefaultPlatformAllocatorFactory constructs the default factory.
func NewDefaultPlatformAllocatorFactory() PlatformAllocatorFactory {
	return DefaultPlatformAllocatorFactory{}
}

// NewAllocator implements PlatformAllocatorFactory.
func (DefaultPlatformAllocatorFactory) NewAllocator() (PTYAllocator, error) {
	return newPlatformPTYAllocator(), nil
}
