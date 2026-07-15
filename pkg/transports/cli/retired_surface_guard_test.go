package cli

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/retiredsurfaceguard"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
)

func TestRetiredSurfaceGuards_ProductionTreePasses(t *testing.T) {
	root := NewRootCommand()
	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("walk command tree: %v", err)
	}

	cliInventory := retiredsurfaceguard.CLIInventory{
		Commands: make([]retiredsurfaceguard.CLICommandRecord, 0, len(inventory.Commands)),
	}
	for _, record := range inventory.Commands {
		cliInventory.Commands = append(cliInventory.Commands, retiredsurfaceguard.CLICommandRecord{
			Path:              record.Path,
			Aliases:           append([]string(nil), record.Aliases...),
			Visibility:        record.Visibility,
			Lifecycle:         record.Lifecycle,
			DeprecatedMessage: record.DeprecatedMessage,
		})
	}

	indexEntries := make([]retiredsurfaceguard.DocsTopicEntry, 0, len(docscli.TopicIndexEntries()))
	for _, entry := range docscli.TopicIndexEntries() {
		indexEntries = append(indexEntries, retiredsurfaceguard.DocsTopicEntry{
			Name:    entry.Name,
			Aliases: append([]string(nil), entry.Aliases...),
		})
	}
	docsRegistry := retiredsurfaceguard.DocsRegistry{
		SupportedTopics:   docscli.SupportedTopics(),
		SupportedCommands: docscli.SupportedTopicCommands(),
		IndexEntries:      indexEntries,
	}

	if violations := retiredsurfaceguard.ScanCLIReintroductionViolations(cliInventory); len(violations) != 0 {
		t.Fatalf("CLI guard violations = %#v", violations)
	}
	if violations := retiredsurfaceguard.ScanDocsReintroductionViolations(docsRegistry); len(violations) != 0 {
		t.Fatalf("docs guard violations = %#v", violations)
	}
}
