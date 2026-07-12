package commandidentity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Walk inventories every reachable command in root without mutating the tree.
// Commands are sorted by full path. Duplicate full paths fail the walk.
// Function pointers are not serialized; only handler-presence evidence is recorded.
func Walk(root *cobra.Command) (Inventory, error) {
	before := captureCommandTreeState(root)

	records := collectCommandRecords(root)
	if err := ensureUniqueCommandPaths(records); err != nil {
		return Inventory{}, err
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Path < records[j].Path
	})

	after := captureCommandTreeState(root)
	if err := commandTreeStatesEqual(before, after); err != nil {
		return Inventory{}, fmt.Errorf("command tree mutated during walk: %w", err)
	}

	return Inventory{
		FormatVersion: FormatVersion,
		RootPath:      root.CommandPath(),
		Commands:      records,
	}, nil
}

func collectCommandRecords(cmd *cobra.Command) []CommandRecord {
	records := []CommandRecord{recordCommand(cmd)}

	children := append([]*cobra.Command(nil), cmd.Commands()...)
	sort.Slice(children, func(i, j int) bool {
		return children[i].CommandPath() < children[j].CommandPath()
	})
	for _, child := range children {
		records = append(records, collectCommandRecords(child)...)
	}
	return records
}

func ensureUniqueCommandPaths(records []CommandRecord) error {
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if _, exists := seen[record.Path]; exists {
			return fmt.Errorf("duplicate command path %q", record.Path)
		}
		seen[record.Path] = struct{}{}
	}
	return nil
}

func recordCommand(cmd *cobra.Command) CommandRecord {
	aliases := cmd.Aliases
	if aliases == nil {
		aliases = []string{}
	} else {
		aliases = append([]string(nil), aliases...)
	}

	return CommandRecord{
		IDCandidate:       idCandidate(cmd.CommandPath()),
		Name:              cmd.Name(),
		Path:              cmd.CommandPath(),
		Aliases:           aliases,
		GroupID:           cmd.GroupID,
		Short:             cmd.Short,
		Long:              cmd.Long,
		Example:           cmd.Example,
		Visibility:        commandVisibility(cmd),
		Lifecycle:         commandLifecycle(cmd),
		DeprecatedMessage: cmd.Deprecated,
		Runnable:          cmd.Runnable(),
		DocIDCandidate:    docIDCandidate(cmd),
		HandlerPresent:    cmd.Run != nil || cmd.RunE != nil,
	}
}

func idCandidate(path string) string {
	return strings.ReplaceAll(path, " ", ".")
}

func commandVisibility(cmd *cobra.Command) string {
	if cmd.Hidden {
		return visibilityHidden
	}
	return visibilityVisible
}

func commandLifecycle(cmd *cobra.Command) string {
	if strings.TrimSpace(cmd.Deprecated) != "" {
		return lifecycleDeprecated
	}
	return lifecycleActive
}

func docIDCandidate(cmd *cobra.Command) string {
	if cmd.Annotations != nil {
		if docID, ok := cmd.Annotations["docId"]; ok {
			return docID
		}
	}
	parent := cmd.Parent()
	if parent != nil && parent.Name() == "docs" {
		return cmd.Name()
	}
	return ""
}
