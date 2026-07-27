package cobracompletion

import (
	"context"
	"fmt"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/spf13/cobra"
)

// PackagedFactoryNamesOperation returns built-in packaged Factory public names
// for shell completion.
type PackagedFactoryNamesOperation func(
	context.Context,
	string,
) ([]cobra.Completion, cobra.ShellCompDirective)

// NewPackagedFactoryNames binds the Definitions-owned embedded catalog to
// stable public-name completion.
func NewPackagedFactoryNames(
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
) PackagedFactoryNamesOperation {
	return func(
		ctx context.Context,
		enteredPrefix string,
	) ([]cobra.Completion, cobra.ShellCompDirective) {
		if completionCancelled(ctx) || catalog.List == nil {
			return packagedFactoryNameFailure()
		}
		listed, err := catalog.ListBuiltInPackagedFactories(
			ctx,
			factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
		)
		if err != nil || completionCancelled(ctx) {
			return packagedFactoryNameFailure()
		}
		completions := make([]cobra.Completion, 0, len(listed.Entries))
		for _, entry := range listed.Entries {
			if !strings.HasPrefix(entry.Name, enteredPrefix) {
				continue
			}
			description := ""
			if len(entry.Formats) > 0 {
				formats := make([]string, 0, len(entry.Formats))
				for _, format := range entry.Formats {
					formats = append(formats, strings.ToLower(string(format)))
				}
				sort.Strings(formats)
				description = fmt.Sprintf("formats: %s", strings.Join(formats, ", "))
			}
			if description == "" {
				completions = append(completions, entry.Name)
				continue
			}
			completions = append(
				completions,
				cobra.CompletionWithDesc(entry.Name, description),
			)
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

// RegisterPackagedFactoryNames attaches packaged Factory name completion to the
// init command's --package flag.
func RegisterPackagedFactoryNames(
	init *cobra.Command,
	complete PackagedFactoryNamesOperation,
) error {
	if init == nil {
		return fmt.Errorf("register packaged factory completion: init command is required")
	}
	if complete == nil {
		return fmt.Errorf("register packaged factory completion: completion operation is required")
	}
	return init.RegisterFlagCompletionFunc("package", func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if cmd == nil {
			return packagedFactoryNameFailure()
		}
		return complete(cmd.Context(), toComplete)
	})
}

func packagedFactoryNameFailure() ([]cobra.Completion, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveError
}
