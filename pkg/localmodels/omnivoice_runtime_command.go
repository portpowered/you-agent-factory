//go:build !omnivoice_cgo || !cgo

package localmodels

import "github.com/portpowered/infinite-you/pkg/workers"

// newOmniVoiceRuntime retains the command-backed adapter for ordinary Go
// builds. Production embedded builds enable the omnivoice_cgo tag.
func newOmniVoiceRuntime(runner workers.CommandRunner) Runtime {
	if runner == nil {
		runner = workers.ExecCommandRunner{}
	}
	return &omniVoiceLocalRuntime{runner: runner}
}
