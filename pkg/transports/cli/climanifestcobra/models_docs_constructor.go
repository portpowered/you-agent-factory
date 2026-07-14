package climanifestcobra

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// ModelsDocsFamilyComponents holds detached models/docs commands before they are
// attached to the production root.
type ModelsDocsFamilyComponents struct {
	Docs    *cobra.Command
	Models  *cobra.Command
	List    *cobra.Command
	Inspect *cobra.Command
	Invoke  *cobra.Command
	Pull    *cobra.Command
}

// ModelsInvokeFlagBindings supplies local invoke flag storage for generated wiring.
type ModelsInvokeFlagBindings struct {
	Operation  *string
	Text       *string
	OutputPath *string
	FlagUsages map[string]string
}

// NewModelsDocsFamilyComponents builds detached docs and models commands from
// generated metadata and attaches handwritten handlers by stable command ID.
func NewModelsDocsFamilyComponents(
	registry *commandregistry.Registry,
	invokeFlags ModelsInvokeFlagBindings,
) (ModelsDocsFamilyComponents, error) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		return ModelsDocsFamilyComponents{}, fmt.Errorf("build models/docs family command: %w", err)
	}
	return NewModelsDocsFamilyComponentsFromManifest(manifest, registry, invokeFlags)
}

// NewModelsDocsFamilyComponentsFromManifest builds detached models/docs commands
// from one generated manifest snapshot.
func NewModelsDocsFamilyComponentsFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	invokeFlags ModelsInvokeFlagBindings,
) (ModelsDocsFamilyComponents, error) {
	if registry == nil {
		return ModelsDocsFamilyComponents{}, fmt.Errorf("build models/docs family command: registry is required")
	}
	if err := validateModelsDocsManifest(manifest); err != nil {
		return ModelsDocsFamilyComponents{}, fmt.Errorf("build models/docs family command: %w", err)
	}
	if err := registry.VerifyModelsDocsRunnableCoverage(manifest); err != nil {
		return ModelsDocsFamilyComponents{}, fmt.Errorf("build models/docs family command: %w", err)
	}

	docsRecord, modelsRecord, listRecord, inspectRecord, invokeRecord, pullRecord, err := modelsDocsManifestRecords(manifest)
	if err != nil {
		return ModelsDocsFamilyComponents{}, err
	}

	docs, err := buildModelsDocsCommandFromRecord(docsRecord)
	if err != nil {
		return ModelsDocsFamilyComponents{}, fmt.Errorf("build models/docs family command: %w", err)
	}
	docs.SilenceUsage = true
	docs.Args = positionalArgsFromManifest(docsRecord)
	docs.ValidArgs = docscli.SupportedTopicCommands()
	if err := registry.AttachRunE(docs, docsRecord.ID); err != nil {
		return ModelsDocsFamilyComponents{}, fmt.Errorf("build models/docs family command: %w", err)
	}

	models, err := buildModelsDocsCommandFromRecord(modelsRecord)
	if err != nil {
		return ModelsDocsFamilyComponents{}, fmt.Errorf("build models/docs family command: %w", err)
	}
	if modelsRecord.Runnable {
		return ModelsDocsFamilyComponents{}, fmt.Errorf("build models/docs family command: %q must remain non-runnable", modelsRecord.ID)
	}

	list, err := buildRunnableModelsLeaf(listRecord, registry, invokeFlags, false)
	if err != nil {
		return ModelsDocsFamilyComponents{}, err
	}
	inspect, err := buildRunnableModelsLeaf(inspectRecord, registry, invokeFlags, false)
	if err != nil {
		return ModelsDocsFamilyComponents{}, err
	}
	invoke, err := buildRunnableModelsLeaf(invokeRecord, registry, invokeFlags, true)
	if err != nil {
		return ModelsDocsFamilyComponents{}, err
	}
	pull, err := buildRunnableModelsLeaf(pullRecord, registry, invokeFlags, false)
	if err != nil {
		return ModelsDocsFamilyComponents{}, err
	}

	models.AddCommand(list, inspect, invoke, pull)

	return ModelsDocsFamilyComponents{
		Docs:    docs,
		Models:  models,
		List:    list,
		Inspect: inspect,
		Invoke:  invoke,
		Pull:    pull,
	}, nil
}

func buildRunnableModelsLeaf(
	record climanifest.Command,
	registry *commandregistry.Registry,
	invokeFlags ModelsInvokeFlagBindings,
	includeInvokeLocalFlags bool,
) (*cobra.Command, error) {
	cmd, err := buildModelsDocsCommandFromRecord(record)
	if err != nil {
		return nil, fmt.Errorf("build models/docs family command: %w", err)
	}
	cmd.Args = positionalArgsFromManifest(record)
	cmd.PreRunE = rejectDeprecatedPortFlag
	if err := registerModelsLocalFlags(cmd, record, invokeFlags, includeInvokeLocalFlags); err != nil {
		return nil, fmt.Errorf("build models/docs family command: %w", err)
	}
	if err := registry.AttachRunE(cmd, record.ID); err != nil {
		return nil, fmt.Errorf("build models/docs family command: %w", err)
	}
	return cmd, nil
}

func modelsDocsManifestRecords(manifest climanifest.Manifest) (
	docs, models, list, inspect, invoke, pull climanifest.Command,
	err error,
) {
	docs, err = manifest.CommandByID("you.docs")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build models/docs family command: %w", err)
	}
	models, err = manifest.CommandByID("you.models")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build models/docs family command: %w", err)
	}
	list, err = manifest.CommandByID("you.models.list")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build models/docs family command: %w", err)
	}
	inspect, err = manifest.CommandByID("you.models.inspect")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build models/docs family command: %w", err)
	}
	invoke, err = manifest.CommandByID("you.models.invoke")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build models/docs family command: %w", err)
	}
	pull, err = manifest.CommandByID("you.models.pull")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build models/docs family command: %w", err)
	}
	return docs, models, list, inspect, invoke, pull, nil
}

func validateModelsDocsManifest(manifest climanifest.Manifest) error {
	if len(manifest.Commands) != len(climanifestgen.ModelsDocsFamilyCommandIDs) {
		return fmt.Errorf(
			"manifest command count = %d, want %d models/docs-family commands",
			len(manifest.Commands),
			len(climanifestgen.ModelsDocsFamilyCommandIDs),
		)
	}
	for commandID := range manifest.Commands {
		if err := climanifestgen.AssertModelsDocsFamilyCommandID(commandID); err != nil {
			return err
		}
	}
	for _, commandID := range climanifestgen.ModelsDocsFamilyCommandIDs {
		if _, ok := manifest.Commands[commandID]; !ok {
			return fmt.Errorf("manifest missing models/docs-family command %q", commandID)
		}
	}
	return nil
}

func buildModelsDocsCommandFromRecord(record climanifest.Command) (*cobra.Command, error) {
	if err := climanifestgen.AssertModelsDocsFamilyCommandID(record.ID); err != nil {
		return nil, err
	}
	short := record.Documentation.Documentation.Title.CanonicalEnglish
	long := ""
	if record.ID == "you.models" || record.ID == "you.docs" {
		long = record.Documentation.Documentation.Description.CanonicalEnglish
	}
	cmd := &cobra.Command{
		Use:     record.Usage.Line,
		Short:   short,
		Long:    long,
		Aliases: append([]string(nil), record.Aliases...),
	}
	if record.Visibility == "hidden" {
		cmd.Hidden = true
	}
	return cmd, nil
}

func registerModelsLocalFlags(
	cmd *cobra.Command,
	record climanifest.Command,
	invokeFlags ModelsInvokeFlagBindings,
	includeInvokeLocalFlags bool,
) error {
	var deprecatedPort int
	flags := sortedFlags(record.Flags)
	for _, flag := range flags {
		if flag.Scope != "local" {
			continue
		}
		switch flag.Long {
		case "port":
			registerDeprecatedPortFlag(cmd, &deprecatedPort)
			if err := applyFlagContract(cmd.Flags().Lookup("port"), flag); err != nil {
				return fmt.Errorf("apply port flag contract: %w", err)
			}
		case "operation":
			if !includeInvokeLocalFlags || invokeFlags.Operation == nil {
				return fmt.Errorf("missing operation binding for invoke flag %q", flag.Long)
			}
			if err := registerStringLocalFlag(cmd, flag, invokeFlags.Operation, invokeFlags.FlagUsages[flag.Long]); err != nil {
				return err
			}
		case "text":
			if !includeInvokeLocalFlags || invokeFlags.Text == nil {
				return fmt.Errorf("missing text binding for invoke flag %q", flag.Long)
			}
			if err := registerStringLocalFlag(cmd, flag, invokeFlags.Text, invokeFlags.FlagUsages[flag.Long]); err != nil {
				return err
			}
		case "output":
			if !includeInvokeLocalFlags || invokeFlags.OutputPath == nil {
				return fmt.Errorf("missing output binding for invoke flag %q", flag.Long)
			}
			if err := registerStringLocalFlag(cmd, flag, invokeFlags.OutputPath, invokeFlags.FlagUsages[flag.Long]); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported local flag %q for models/docs constructor", flag.Long)
		}
	}
	return nil
}

func registerStringLocalFlag(cmd *cobra.Command, contract climanifest.Flag, target *string, usage string) error {
	if target == nil {
		return fmt.Errorf("missing string binding for flag %q", contract.Long)
	}
	if contract.Shorthand != "" {
		return fmt.Errorf("string flag %q does not support shorthand in generated constructor", contract.Long)
	}
	cmd.Flags().StringVar(target, contract.Long, contract.Default, usage)
	flag := cmd.Flags().Lookup(contract.Long)
	if flag == nil {
		return fmt.Errorf("flag %q was not registered", contract.Long)
	}
	if contract.Visibility == "hidden" {
		flag.Hidden = true
	}
	return nil
}
