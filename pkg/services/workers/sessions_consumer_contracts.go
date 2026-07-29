package workers

import platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"

// ProviderRegistry names the validated provider manifest-to-integration catalog
// join Sessions conductor invocation factories receive at runtime. The
// concrete registry is constructed within Workers; Sessions names only this
// root contract instead of importing workers/provider/registry.
type ProviderRegistry interface {
	UsesNativeRunner(identity string) bool
	CanonicalIdentity(identity string) (string, error)
	RunnerIdentities() []string
	RunnerMetadata(identity string) (RunnerMetadata, error)
	ValidateRunnerPrerequisites(platformprocess.ExecutableLocator, string) error
	ResolveRunnerSelection(
		workstationRunner string,
		factoryRunner string,
		workerModelProvider string,
	) (ResolvedRunnerSelection, error)
}
