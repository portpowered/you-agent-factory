package workers

import "github.com/portpowered/infinite-you/pkg/interfaces"

// RunnerStatus reports whether a built-in runner can be selected safely in the
// current build before dispatch starts.
type RunnerStatus struct {
	Metadata          interfaces.RunnerMetadata
	Available         bool
	UnavailableReason string
}

var builtInRunnerStatus = map[string]RunnerStatus{
	interfaces.RunnerIDCodex: {
		Metadata:  mustBuiltInRunnerMetadata(interfaces.RunnerIDCodex),
		Available: true,
	},
	interfaces.RunnerIDGemini: {
		Metadata:  mustBuiltInRunnerMetadata(interfaces.RunnerIDGemini),
		Available: true,
	},
	interfaces.RunnerIDKiro: {
		Metadata:  mustBuiltInRunnerMetadata(interfaces.RunnerIDKiro),
		Available: true,
	},
	interfaces.RunnerIDCursorCLI: {
		Metadata:  mustBuiltInRunnerMetadata(interfaces.RunnerIDCursorCLI),
		Available: true,
	},
	interfaces.RunnerIDOpenCode: {
		Metadata:          mustBuiltInRunnerMetadata(interfaces.RunnerIDOpenCode),
		UnavailableReason: "opencode runner is registered but not yet available in this build",
	},
}

// BuiltInRunnerStatus reports the build-local availability of one stable
// runner registration.
func BuiltInRunnerStatus(id string) (RunnerStatus, bool) {
	status, ok := builtInRunnerStatus[interfaces.NormalizeRunnerID(id)]
	if !ok {
		return RunnerStatus{}, false
	}
	return status, true
}

func mustBuiltInRunnerMetadata(id string) interfaces.RunnerMetadata {
	metadata, ok := interfaces.BuiltInRunnerMetadata(id)
	if !ok {
		panic("missing built-in runner metadata: " + id)
	}
	return metadata
}
