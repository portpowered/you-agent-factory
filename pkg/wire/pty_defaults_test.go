package wire

import providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"

func testDefaultPTYSessionConfig() providerswire.PTYSessionConfig {
	return providerswire.PTYSessionConfig{MaxCaptureBytes: providerswire.DefaultPTYMaxCaptureBytes, IdleTimeout: providerswire.DefaultPTYIdleTimeout, HardTimeout: providerswire.DefaultPTYHardTimeout}
}
