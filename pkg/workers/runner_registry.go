package workers

import (
	"fmt"
	"os/exec"

	workerrunner "github.com/portpowered/infinite-you/pkg/workers/runner"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// RunnerStatus reports whether a built-in runner can be selected safely in the
// current build before dispatch starts.
type RunnerStatus struct {
	Metadata          workerexecution.RunnerMetadata
	Available         bool
	UnavailableReason string
}

var builtInRunnerStatus = map[string]RunnerStatus{
	workerexecution.RunnerIDCodex: {
		Metadata:  mustBuiltInRunnerMetadata(workerexecution.RunnerIDCodex),
		Available: true,
	},
	workerexecution.RunnerIDGemini: {
		Metadata:  mustBuiltInRunnerMetadata(workerexecution.RunnerIDGemini),
		Available: true,
	},
	workerexecution.RunnerIDKiro: {
		Metadata:  mustBuiltInRunnerMetadata(workerexecution.RunnerIDKiro),
		Available: true,
	},
	workerexecution.RunnerIDCursorCLI: {
		Metadata:  mustBuiltInRunnerMetadata(workerexecution.RunnerIDCursorCLI),
		Available: true,
	},
	workerexecution.RunnerIDOpenCode: {
		Metadata:  mustBuiltInRunnerMetadata(workerexecution.RunnerIDOpenCode),
		Available: true,
	},
	workerexecution.RunnerIDPi: {
		Metadata:  mustBuiltInRunnerMetadata(workerexecution.RunnerIDPi),
		Available: true,
	},
}

var lookPath = exec.LookPath

// BuiltInRunnerStatus reports the build-local availability of one stable
// runner registration.
func BuiltInRunnerStatus(id string) (RunnerStatus, bool) {
	status, ok := builtInRunnerStatus[workerrunner.NormalizeRunnerID(id)]
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
		return fmt.Errorf("unknown runner %q", workerrunner.NormalizeRunnerID(id))
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
	switch workerrunner.NormalizeRunnerID(id) {
	case workerexecution.RunnerIDCodex:
		return string(modelprovider.Codex)
	case workerexecution.RunnerIDGemini:
		return string(modelprovider.Gemini)
	case workerexecution.RunnerIDKiro:
		return string(modelprovider.Kiro)
	case workerexecution.RunnerIDCursorCLI:
		return string(modelprovider.Cursor)
	case workerexecution.RunnerIDOpenCode:
		return string(modelprovider.OpenCode)
	case workerexecution.RunnerIDPi:
		return string(modelprovider.Pi)
	default:
		return ""
	}
}

func mustBuiltInRunnerMetadata(id string) workerexecution.RunnerMetadata {
	metadata, ok := workerrunner.BuiltInRunnerMetadata(id)
	if !ok {
		panic("missing built-in runner metadata: " + id)
	}
	return metadata
}
