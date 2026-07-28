package cli

import (
	"context"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// BindService returns the composition-facing Factory Runtime CLI adapter Service
// constructed from accepted Runtime-root collaborators. Wire and other
// composition roots inject the returned Service without constructing adapter
// behavior at the composition boundary.
func BindService(cfg Config) Service {
	return New(cfg)
}

// NormalizeInvocationOutputMode delegates invocation output normalization to the
// Runtime-owned CLI adapter Service.
func NormalizeInvocationOutputMode(raw string) (string, error) {
	return New(Config{}).NormalizeInvocationOutputMode(raw)
}

// ValidateInvocationOutputSelection delegates invocation output selection
// validation to the Runtime-owned CLI adapter Service.
func ValidateInvocationOutputSelection(quiet, jsonOutput, explicitOutput bool) error {
	return New(Config{}).ValidateInvocationOutputSelection(quiet, jsonOutput, explicitOutput)
}

// ValidateInvocationOutputMode delegates invocation output mode validation to
// the Runtime-owned CLI adapter Service.
func ValidateInvocationOutputMode(req ValidateInvocationOutputModeRequest) error {
	return New(Config{}).ValidateInvocationOutputMode(req)
}

// MapCurrentFactoryFailure delegates Current Factory failure mapping to the
// Runtime-owned CLI adapter Service.
func MapCurrentFactoryFailure(err error) error {
	return New(Config{}).MapCurrentFactoryFailure(err)
}

// MapServerFailure delegates server bind failure mapping to the Runtime-owned
// CLI adapter Service.
func MapServerFailure(err error) error {
	return New(Config{}).MapServerFailure(err)
}

// MapInvocationFailure delegates invocation failure mapping to the Runtime-owned
// CLI adapter Service.
func MapInvocationFailure(err error) error {
	return New(Config{}).MapInvocationFailure(err)
}

// MapRuntimeRootFailure delegates Runtime root failure mapping to the
// Runtime-owned CLI adapter Service.
func MapRuntimeRootFailure(runtime factoryruntime.Service, err error) error {
	return New(Config{Runtime: runtime}).MapRuntimeRootFailure(err)
}

// ObserveRuntime delegates one Runtime observation request through the
// Runtime-owned CLI adapter Service.
func ObserveRuntime(
	ctx context.Context,
	runtime factoryruntime.Service,
	req factoryruntime.ObserveRequest,
) error {
	return New(Config{Runtime: runtime}).ObserveRuntime(ctx, req)
}

// CountTokenStates delegates token-state presentation to the Runtime-owned CLI
// adapter Service.
func CountTokenStates(snap *factoryruntime.PetriMarkingSnapshot) (wip, completed, failed int) {
	return New(Config{}).CountTokenStates(snap)
}

// FormatDuration delegates duration presentation to the Runtime-owned CLI
// adapter Service.
func FormatDuration(d time.Duration) string {
	return New(Config{}).FormatDuration(d)
}
