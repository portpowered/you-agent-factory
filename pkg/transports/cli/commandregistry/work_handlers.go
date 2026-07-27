package commandregistry

import (
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

// RunnableWorkCommandIDs returns contracted runnable command IDs for the work
// family in stable sorted order.
func RunnableWorkCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.WorkFamilyCommandIDs))
	for _, commandID := range climanifestgen.WorkFamilyCommandIDs {
		if err := climanifestgen.AssertWorkFamilyCommandID(commandID); err != nil {
			return nil, err
		}
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

// VerifyWorkRunnableCoverage fails when any contracted runnable work-family
// command ID lacks a registered handwritten handler.
func (r *Registry) VerifyWorkRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableWorkCommandIDs(manifest)
	if err != nil {
		return err
	}
	var missing []string
	for _, commandID := range runnableIDs {
		if _, lookupErr := r.Lookup(commandID); lookupErr != nil {
			missing = append(missing, commandID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"work runnable command handlers missing for: %v",
			missing,
		)
	}
	return nil
}

// WorkHandlers carries handwritten RunE handlers for contracted runnable
// work-family command IDs.
type WorkHandlers struct {
	ListRunE      RunE
	ShowRunE      RunE
	MoveRunE      RunE
	VisualizeRunE RunE
}

// NewWorkRegistry registers handwritten handlers for the work family and
// verifies contracted runnable command coverage.
func NewWorkRegistry(handlers WorkHandlers) (*Registry, error) {
	if handlers.ListRunE == nil {
		return nil, fmt.Errorf("build work handler registry: list handler is required")
	}
	if handlers.ShowRunE == nil {
		return nil, fmt.Errorf("build work handler registry: show handler is required")
	}
	if handlers.MoveRunE == nil {
		return nil, fmt.Errorf("build work handler registry: move handler is required")
	}
	if handlers.VisualizeRunE == nil {
		return nil, fmt.Errorf("build work handler registry: visualize handler is required")
	}

	registry := NewRegistry()
	registrations := []struct {
		commandID string
		handler   RunE
	}{
		{commandID: "you.work.list", handler: handlers.ListRunE},
		{commandID: "you.work.show", handler: handlers.ShowRunE},
		{commandID: "you.work.move", handler: handlers.MoveRunE},
		{commandID: "you.work.visualize", handler: handlers.VisualizeRunE},
	}
	for _, registration := range registrations {
		if err := registry.Register(registration.commandID, registration.handler); err != nil {
			return nil, fmt.Errorf("build work handler registry: %w", err)
		}
	}

	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build work handler registry: %w", err)
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build work handler registry: %w", err)
	}
	return registry, nil
}
