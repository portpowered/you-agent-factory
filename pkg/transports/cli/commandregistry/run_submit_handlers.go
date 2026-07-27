package commandregistry

import (
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

// RunSubmitHandlers carries handwritten PreRunE/RunE lifecycles for every
// runnable command in the run/submit family.
type RunSubmitHandlers struct {
	Run         CommandHandlers
	Server      CommandHandlers
	Submit      CommandHandlers
	SubmitBatch CommandHandlers
}

// RunnableRunSubmitCommandIDs returns contracted runnable command IDs in stable order.
func RunnableRunSubmitCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.RunSubmitFamilyCommandIDs))
	for _, commandID := range climanifestgen.RunSubmitFamilyCommandIDs {
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, err
		}
		if record.Runnable {
			ids = append(ids, commandID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// VerifyRunSubmitRunnableCoverage fails when a contracted runnable command is
// missing its handwritten lifecycle binding.
func (r *Registry) VerifyRunSubmitRunnableCoverage(manifest climanifest.Manifest) error {
	if r == nil {
		return fmt.Errorf("verify run/submit handlers: registry is nil")
	}
	for commandID := range r.handlers {
		if err := climanifestgen.AssertRunSubmitFamilyCommandID(commandID); err != nil {
			return fmt.Errorf("run/submit handler registry: %w", err)
		}
	}
	runnableIDs, err := RunnableRunSubmitCommandIDs(manifest)
	if err != nil {
		return err
	}
	var missing []string
	for _, commandID := range runnableIDs {
		if _, lookupErr := r.LookupHandlers(commandID); lookupErr != nil {
			missing = append(missing, commandID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("run/submit runnable command handlers missing for: %v", missing)
	}
	return nil
}

// NewRunSubmitRegistry registers the retained handwritten lifecycles by stable
// command ID and verifies complete generated-family coverage.
func NewRunSubmitRegistry(handlers RunSubmitHandlers) (*Registry, error) {
	registrations := []struct {
		commandID string
		handlers  CommandHandlers
	}{
		{commandID: "you.run", handlers: handlers.Run},
		{commandID: "you.server", handlers: handlers.Server},
		{commandID: "you.submit", handlers: handlers.Submit},
		{commandID: "you.submit.batch", handlers: handlers.SubmitBatch},
	}
	registry := NewRegistry()
	for _, registration := range registrations {
		if err := climanifestgen.AssertRunSubmitFamilyCommandID(registration.commandID); err != nil {
			return nil, fmt.Errorf("build run/submit handler registry: %w", err)
		}
		if registration.handlers.PreRunE == nil {
			return nil, fmt.Errorf(
				"build run/submit handler registry: %s pre-run handler is required",
				registration.commandID,
			)
		}
		if err := registry.RegisterHandlers(registration.commandID, registration.handlers); err != nil {
			return nil, fmt.Errorf("build run/submit handler registry: %w", err)
		}
	}
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build run/submit handler registry: %w", err)
	}
	if err := registry.VerifyRunSubmitRunnableCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build run/submit handler registry: %w", err)
	}
	return registry, nil
}
