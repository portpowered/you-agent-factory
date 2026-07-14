package agypty

import "time"

// Default capture and timeout limits for the approved Agy PTY boundary.
// Story 17 implementation must honor these defaults unless factory config
// documents an explicit override within MaxMaxCaptureBytes.
const (
	DefaultMaxCaptureBytes = 4 * 1024 * 1024  // 4 MiB
	MaxMaxCaptureBytes     = 16 * 1024 * 1024 // 16 MiB hard ceiling
	DefaultIdleTimeout     = 30 * time.Second
	DefaultHardTimeout     = 10 * time.Minute
)
