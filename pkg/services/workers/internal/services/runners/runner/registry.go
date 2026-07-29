package runner

import (
	"fmt"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// Status reports whether a built-in runner can be selected safely in the
// current build before dispatch starts.
type Status struct {
	Metadata          workerexecution.RunnerMetadata
	Available         bool
	UnavailableReason string
}

var builtInRunnerStatus = map[string]Status{
	workerexecution.RunnerIDCodex: {
		Metadata:  mustBuiltInRunnerMetadata(workerexecution.RunnerIDCodex),
		Available: true,
	},
	workerexecution.RunnerIDClaude: {
		Metadata:  mustBuiltInRunnerMetadata(workerexecution.RunnerIDClaude),
		Available: true,
	},
	workerexecution.RunnerIDCursorCLI: {
		Metadata:  mustBuiltInRunnerMetadata(workerexecution.RunnerIDCursorCLI),
		Available: true,
	},
	workerexecution.RunnerIDAntigravity: {
		Metadata:  mustBuiltInRunnerMetadata(workerexecution.RunnerIDAntigravity),
		Available: true,
	},
}

// BuiltInRunnerStatus reports the build-local availability of one stable
// runner registration.
func BuiltInRunnerStatus(id string) (Status, bool) {
	status, ok := builtInRunnerStatus[NormalizeRunnerID(id)]
	if !ok {
		return Status{}, false
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
	case workerexecution.RunnerIDCodex:
		return string(modelprovider.ProviderCodex)
	case workerexecution.RunnerIDClaude:
		return string(modelprovider.ProviderClaude)
	case workerexecution.RunnerIDCursorCLI:
		return string(modelprovider.ProviderCursor)
	case workerexecution.RunnerIDAntigravity:
		return "agy"
	default:
		return ""
	}
}

func mustBuiltInRunnerMetadata(id string) workerexecution.RunnerMetadata {
	metadata, ok := BuiltInRunnerMetadata(id)
	if !ok {
		panic("missing built-in runner metadata: " + id)
	}
	return metadata
}
