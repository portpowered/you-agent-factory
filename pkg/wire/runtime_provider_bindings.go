package wire

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
)

// newConfiguredProvidersService always installs the shell-free Antigravity
// print-mode command effect. An injected serviceedges.Edges.AgyPTYHost exists
// only to satisfy the legacy PTY allocator construction port and must never
// suppress the canonical command adapter; the command effect unconditionally
// takes priority over the legacy PTY effect in executionwire's built-in
// dependency selection.
func newConfiguredProvidersService(
	options []providerswire.Option,
	agyRunner platformprocess.CommandRunner,
) (providers.Service, error) {
	options = append(options, providerswire.WithAgyCommandRunner(
		workerswire.NewProviderCommandRunner(agyRunner),
	))
	return providerswire.NewService(options...)
}
