package commandregistry

import (
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

var runServerCommandIDs = []string{"you.run", "you.server"}

// RunServerHandlers carries handwritten PreRunE/RunE lifecycles for the
// retained run and server commands.
type RunServerHandlers struct {
	Run    CommandHandlers
	Server CommandHandlers
}

// RunnableRunServerCommandIDs returns contracted runnable command IDs in stable order.
func RunnableRunServerCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(runServerCommandIDs))
	for _, commandID := range runServerCommandIDs {
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

// VerifyRunServerRunnableCoverage fails when a retained runnable command is
// missing its handwritten lifecycle binding.
func (r *Registry) VerifyRunServerRunnableCoverage(manifest climanifest.Manifest) error {
	if r == nil {
		return fmt.Errorf("verify run/server handlers: registry is nil")
	}
	expected := make(map[string]bool, len(runServerCommandIDs))
	for _, commandID := range runServerCommandIDs {
		expected[commandID] = true
	}
	for commandID := range r.handlers {
		if !expected[commandID] {
			return fmt.Errorf("run/server handler registry: command %q is not retained", commandID)
		}
	}
	runnableIDs, err := RunnableRunServerCommandIDs(manifest)
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
		return fmt.Errorf("run/server runnable command handlers missing for: %v", missing)
	}
	return nil
}

// NewRunServerRegistry registers the retained handwritten lifecycles by stable
// command ID and verifies complete run/server coverage.
func NewRunServerRegistry(handlers RunServerHandlers) (*Registry, error) {
	registrations := []struct {
		commandID string
		handlers  CommandHandlers
	}{
		{commandID: "you.run", handlers: handlers.Run},
		{commandID: "you.server", handlers: handlers.Server},
	}
	registry := NewRegistry()
	for _, registration := range registrations {
		if registration.handlers.PreRunE == nil {
			return nil, fmt.Errorf(
				"build run/server handler registry: %s pre-run handler is required",
				registration.commandID,
			)
		}
		if err := registry.RegisterHandlers(registration.commandID, registration.handlers); err != nil {
			return nil, fmt.Errorf("build run/server handler registry: %w", err)
		}
	}
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build run/server handler registry: %w", err)
	}
	if err := registry.VerifyRunServerRunnableCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build run/server handler registry: %w", err)
	}
	return registry, nil
}
