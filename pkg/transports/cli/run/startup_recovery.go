package run

import (
	"fmt"
	"io"

	"github.com/portpowered/infinite-you/pkg/initializer"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func startupRecoveryForRunner(runner initializer.LocalRuntimeRunner) *factorysessions.StartupRecovery {
	provider, ok := runner.(interface {
		StartupRecovery() *factorysessions.StartupRecovery
	})
	if !ok {
		return nil
	}
	return provider.StartupRecovery()
}

func emitStartupRecovery(output io.Writer, runner initializer.LocalRuntimeRunner) {
	recovery := startupRecoveryForRunner(runner)
	if output == nil || recovery == nil {
		return
	}
	// Quoted paths keep filenames containing newlines on one diagnostic line.
	_, _ = fmt.Fprintf(output, "Durable state %q quarantined as %q: %s. Started an empty board.\n",
		recovery.File, recovery.QuarantinedFile, recovery.Cause)
}

func (runner hostedInvocationRunner) StartupRecovery() *factorysessions.StartupRecovery {
	return startupRecoveryForRunner(runner.runner)
}

func (runner cleanInvocationSnapshotRunner) StartupRecovery() *factorysessions.StartupRecovery {
	return startupRecoveryForRunner(runner.runner)
}
