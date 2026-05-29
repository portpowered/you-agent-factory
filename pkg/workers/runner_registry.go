package workers

import (
	"fmt"
	"os/exec"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
		Metadata:  mustBuiltInRunnerMetadata(interfaces.RunnerIDOpenCode),
		Available: true,
	},
}

var lookPath = exec.LookPath

// BuiltInRunnerStatus reports the build-local availability of one stable
// runner registration.
func BuiltInRunnerStatus(id string) (RunnerStatus, bool) {
	status, ok := builtInRunnerStatus[interfaces.NormalizeRunnerID(id)]
	if !ok {
		return RunnerStatus{}, false
	}
	return status, true
}

// ValidateBuiltInRunnerPrerequisites checks PATH-visible prerequisites for one
// built-in runner before the runtime attempts dispatch.
func ValidateBuiltInRunnerPrerequisites(id string) error {
	status, ok := BuiltInRunnerStatus(id)
	if !ok {
		return fmt.Errorf("unknown runner %q", interfaces.NormalizeRunnerID(id))
	}
	if !status.Available {
		return fmt.Errorf("%s", status.UnavailableReason)
	}

	command := builtInRunnerCommand(id)
	if command == "" {
		return nil
	}
	if _, err := lookPath(command); err != nil {
		return fmt.Errorf("%s runner requires %q on PATH: %w", status.Metadata.DisplayName, command, err)
	}
	return nil
}

func builtInRunnerCommand(id string) string {
	switch interfaces.NormalizeRunnerID(id) {
	case interfaces.RunnerIDCodex:
		return string(interfaces.ModelProviderCodex)
	case interfaces.RunnerIDGemini:
		return string(interfaces.ModelProviderGemini)
	case interfaces.RunnerIDKiro:
		return string(interfaces.ModelProviderKiro)
	case interfaces.RunnerIDCursorCLI:
		return string(interfaces.ModelProviderCursor)
	case interfaces.RunnerIDOpenCode:
		return string(interfaces.ModelProviderOpenCode)
	default:
		return ""
	}
}

func mustBuiltInRunnerMetadata(id string) interfaces.RunnerMetadata {
	metadata, ok := interfaces.BuiltInRunnerMetadata(id)
	if !ok {
		panic("missing built-in runner metadata: " + id)
	}
	return metadata
}
