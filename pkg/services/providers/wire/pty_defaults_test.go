package wire

// DefaultPTYSessionConfig returns the bounded native-session defaults used by
// the Providers Agy adapter.
func DefaultPTYSessionConfig() PTYSessionConfig {
	return PTYSessionConfig{
		MaxCaptureBytes: DefaultPTYMaxCaptureBytes,
		IdleTimeout:     DefaultPTYIdleTimeout,
		HardTimeout:     DefaultPTYHardTimeout,
	}
}
