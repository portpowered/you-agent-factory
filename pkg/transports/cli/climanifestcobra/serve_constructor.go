package climanifestcobra

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// NewServeCommand builds the independently injected ACP child of the shared
// `you server` family through the accepted generic manifest constructor.
func NewServeCommand(handler ResolvedCobraHandler) (*cobra.Command, error) {
	manifest, err := generated.ServeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build server ACP command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build server ACP command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build server ACP command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewServeCommandFromManifest(manifest, handler)
}

// NewServeCommandFromManifest projects and detaches the complete serve family.
func NewServeCommandFromManifest(
	manifest climanifest.Manifest,
	handler ResolvedCobraHandler,
) (*cobra.Command, error) {
	if handler == nil {
		return nil, fmt.Errorf("build server ACP command: handler is required")
	}
	records, err := mustServeFamilyRecords(manifest)
	if err != nil {
		return nil, err
	}
	rootRecord, parentRecord, acpRecord := records[0], records[1], records[2]
	parentRecord = protocolServerParentRecord(parentRecord)
	// Rebuild under these exact stable keys (not parentRecord.ID/acpRecord.ID)
	// so that if either lookup above ever returned a mislabeled record from a
	// corrupted upstream manifest (whose own .ID field disagrees with the
	// canonical "you.server"/"you.server.acp" identity it was fetched by),
	// NewCommandTree's own existing map-key/record-id consistency check
	// rejects the tree instead of silently projecting a mislabeled command.
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID:    rootRecord,
		"you.server":     parentRecord,
		"you.server.acp": acpRecord,
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: ResolvedCobraHandlerRegistry{
			acpRecord.Handler.ID: handler,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build server ACP command: %w", err)
	}
	parent, _, err := root.Find([]string{parentRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build server ACP command: find projected command: %w", err)
	}
	root.RemoveCommand(parent)
	return parent, nil
}

// mustServeFamilyRecords looks up the shared root, server parent, and ACP leaf
// records by their stable IDs, funneling every lookup through one shared
// failure path instead of duplicating a "missing record" branch per record.
func mustServeFamilyRecords(manifest climanifest.Manifest) ([3]climanifest.Command, error) {
	var records [3]climanifest.Command
	for i, id := range [3]string{"you", "you.server", "you.server.acp"} {
		record, err := manifest.CommandByID(id)
		if err != nil {
			return [3]climanifest.Command{}, fmt.Errorf("build server ACP command: %w", err)
		}
		records[i] = record
	}
	return records, nil
}

// protocolServerParentRecord projects the shared server parent for an
// independently constructed protocol child. The production HTTP server owns
// the runnable parent and its --listen input; protocol children inherit the
// root globals they historically exposed but must not inherit the HTTP-only
// listener flag or a second server handler.
func protocolServerParentRecord(record climanifest.Command) climanifest.Command {
	record.Runnable = false
	record.Handler = nil
	flags := make(map[string]climanifest.Flag, len(record.Flags))
	for id, flag := range record.Flags {
		if id == "you.server.flag.listen" {
			continue
		}
		flags[id] = flag
	}
	record.Flags = flags
	return record
}
