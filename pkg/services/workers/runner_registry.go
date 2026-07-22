package workers

import (
	"fmt"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
)

// RunnerStatus reports whether a built-in runner can be selected safely in the
// current build before dispatch starts.
type RunnerStatus struct {
	Metadata          RunnerMetadata
	Available         bool
	UnavailableReason string
}

var builtInRunnerStatus = map[string]RunnerStatus{
	RunnerIDCodex: {
		Metadata:  mustBuiltInRunnerMetadata(RunnerIDCodex),
		Available: true,
	},
	RunnerIDGemini: {
		Metadata:  mustBuiltInRunnerMetadata(RunnerIDGemini),
		Available: true,
	},
	RunnerIDKiro: {
		Metadata:  mustBuiltInRunnerMetadata(RunnerIDKiro),
		Available: true,
	},
	RunnerIDCursorCLI: {
		Metadata:  mustBuiltInRunnerMetadata(RunnerIDCursorCLI),
		Available: true,
	},
	RunnerIDOpenCode: {
		Metadata:  mustBuiltInRunnerMetadata(RunnerIDOpenCode),
		Available: true,
	},
	RunnerIDPi: {
		Metadata:  mustBuiltInRunnerMetadata(RunnerIDPi),
		Available: true,
	},
}

// BuiltInRunnerStatus reports the build-local availability of one stable
// runner registration.
func BuiltInRunnerStatus(id string) (RunnerStatus, bool) {
	status, ok := builtInRunnerStatus[NormalizeRunnerID(id)]
	if !ok {
		return RunnerStatus{}, false
	}
	return status, true
}

// ValidateBuiltInRunnerPrerequisites checks PATH-visible prerequisites for one
// built-in runner before the runtime attempts dispatch.
func ValidateBuiltInRunnerPrerequisites(locator platformprocess.ExecutableLocator, id string) error {
	status, ok := BuiltInRunnerStatus(id)
	if !ok {
		return fmt.Errorf("unknown runner %q", NormalizeRunnerID(id))
	}
	if !status.Available {
		return fmt.Errorf("%s", status.UnavailableReason)
	}

	command := builtInRunnerCommand(id)
	if command == "" {
		return nil
	}
	if locator == nil {
		return fmt.Errorf("%s runner executable locator is required", status.Metadata.DisplayName)
	}
	if _, err := locator.LookPath(command); err != nil {
		return fmt.Errorf("%s runner requires %q on PATH: %w", status.Metadata.DisplayName, command, err)
	}
	return nil
}

func builtInRunnerCommand(id string) string {
	switch NormalizeRunnerID(id) {
	case RunnerIDCodex:
		return string(modelprovider.ProviderCodex)
	case RunnerIDGemini:
		return string(modelprovider.ProviderGemini)
	case RunnerIDKiro:
		return string(modelprovider.ProviderKiro)
	case RunnerIDCursorCLI:
		return string(modelprovider.ProviderCursor)
	case RunnerIDOpenCode:
		return string(modelprovider.ProviderOpenCode)
	case RunnerIDPi:
		return string(modelprovider.ProviderPi)
	default:
		return ""
	}
}

func mustBuiltInRunnerMetadata(id string) RunnerMetadata {
	metadata, ok := BuiltInRunnerMetadata(id)
	if !ok {
		panic("missing built-in runner metadata: " + id)
	}
	return metadata
}
