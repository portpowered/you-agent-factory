package agypty

func DefaultSessionConfig() SessionConfig {
	return SessionConfig{MaxCaptureBytes: DefaultMaxCaptureBytes, IdleTimeout: DefaultIdleTimeout, HardTimeout: DefaultHardTimeout}
}
