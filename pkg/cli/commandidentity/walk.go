package commandidentity

import (
	"strings"

	"github.com/spf13/cobra"
)

// Walk inventories every reachable command in root without mutating the tree.
// Function pointers are not serialized; only handler-presence evidence is recorded.
func Walk(root *cobra.Command) Inventory {
	return Inventory{
		FormatVersion: FormatVersion,
		RootPath:      root.CommandPath(),
		Commands:      collectCommandRecords(root),
	}
}

func collectCommandRecords(cmd *cobra.Command) []CommandRecord {
	records := []CommandRecord{recordCommand(cmd)}
	for _, child := range cmd.Commands() {
		records = append(records, collectCommandRecords(child)...)
	}
	return records
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
