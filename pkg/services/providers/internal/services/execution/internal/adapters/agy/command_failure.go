package agy

import (
	"fmt"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
)

func commandFailure(result providerservice.CommandResult) providers.ExecuteFailure {
	output := strings.ToLower(commandOutput(result))
	if strings.Contains(output, "does not support a separate reasoning effort") {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: "Agy does not support a separate reasoning effort.",
		}
	}
	return providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindUnknown,
		Message: fmt.Sprintf("Agy execution exited with code %d.", result.ExitCode),
	}
}

func commandOutput(result providerservice.CommandResult) string {
	stderr := strings.TrimSpace(string(result.Stderr))
	if stderr != "" {
		return stderr
	}
	return strings.TrimSpace(string(result.Stdout))
}
