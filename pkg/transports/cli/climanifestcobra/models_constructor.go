package climanifestcobra

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// NewDocsCommand builds the independently injected `you docs` command through
// the accepted generic manifest constructor.
func NewDocsCommand(handler ResolvedCobraHandler) (*cobra.Command, error) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewDocsCommandFromManifest(manifest, handler)
}

// NewDocsCommandFromManifest projects and detaches `you docs` from the generic
// root/docs tree. Positional validation, help, and completion remain entirely
// manifest-owned.
func NewDocsCommandFromManifest(
	manifest climanifest.Manifest,
	handler ResolvedCobraHandler,
) (*cobra.Command, error) {
	if handler == nil {
		return nil, fmt.Errorf("build docs command: handler is required")
	}
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	docsRecord, err := manifest.CommandByID("you.docs")
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID: rootRecord,
		docsRecord.ID: docsRecord,
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: ResolvedCobraHandlerRegistry{
			docsRecord.Handler.ID: handler,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	cmd, _, err := root.Find([]string{docsRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build docs command: find projected command: %w", err)
	}
	root.RemoveCommand(cmd)
	cmd.SilenceUsage = true
	return cmd, nil
}

// NewModelsCommand builds the independently injected `you models` family.
func NewModelsCommand(handler commandregistry.ModelsHandler) (*cobra.Command, error) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewModelsCommandFromManifest(manifest, handler)
}

// NewModelsCommandFromManifest projects the complete Models family through the
// generic manifest constructor.
func NewModelsCommandFromManifest(
	manifest climanifest.Manifest,
	handler commandregistry.ModelsHandler,
) (*cobra.Command, error) {
	if handler == nil {
		return nil, fmt.Errorf("build models command: handler is required")
	}
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	parentRecord, err := manifest.CommandByID("you.models")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	if parentRecord.Runnable {
		return nil, fmt.Errorf("build models command: %q must remain non-runnable", parentRecord.ID)
	}
	listRecord, err := manifest.CommandByID("you.models.list")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	inspectRecord, err := manifest.CommandByID("you.models.inspect")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	pullRecord, err := manifest.CommandByID("you.models.pull")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	invokeRecord, err := manifest.CommandByID("you.models.invoke")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	parent, err := buildResolvedModelsParent(
		manifest,
		rootRecord,
		parentRecord,
		listRecord,
		inspectRecord,
		pullRecord,
		invokeRecord,
		handler,
	)
	if err != nil {
		return nil, err
	}
	return parent, nil
}

func buildResolvedModelsParent(
	manifest climanifest.Manifest,
	rootRecord climanifest.Command,
	parentRecord climanifest.Command,
	listRecord climanifest.Command,
	inspectRecord climanifest.Command,
	pullRecord climanifest.Command,
	invokeRecord climanifest.Command,
	handler commandregistry.ModelsHandler,
) (*cobra.Command, error) {
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID:    rootRecord,
		parentRecord.ID:  parentRecord,
		listRecord.ID:    listRecord,
		inspectRecord.ID: inspectRecord,
		pullRecord.ID:    pullRecord,
		invokeRecord.ID:  invokeRecord,
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: ResolvedCobraHandlerRegistry{
			listRecord.Handler.ID:    handler.List,
			inspectRecord.Handler.ID: handler.Inspect,
			pullRecord.Handler.ID:    handler.Pull,
			invokeRecord.Handler.ID:  handler.Invoke,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	parent, _, err := root.Find([]string{parentRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build models command: find projected command: %w", err)
	}
	root.RemoveCommand(parent)
	for _, id := range []string{listRecord.ID, inspectRecord.ID, pullRecord.ID, invokeRecord.ID} {
		command, _, findErr := parent.Find([]string{strings.TrimPrefix(id, parentRecord.ID+".")})
		if findErr != nil {
			return nil, fmt.Errorf("build models command: find %q: %w", id, findErr)
		}
		command.PreRunE = rejectDeprecatedPortFlag
		if id != listRecord.ID {
			preserveExactArgumentDiagnostic(command, manifest.Commands[id])
		}
	}
	return parent, nil
}

func preserveExactArgumentDiagnostic(
	command *cobra.Command,
	record climanifest.Command,
) {
	arguments := make([]climanifest.Argument, 0, len(record.Arguments))
	for _, argument := range record.Arguments {
		arguments = append(arguments, argument)
	}
	minimum, maximum := argumentCardinality(arguments)
	if minimum != maximum {
		return
	}
	validate := command.Args
	command.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) != minimum {
			return cobra.ExactArgs(minimum)(cmd, args)
		}
		return validate(cmd, args)
	}
}

const (
	submitCommandID      = "you.submit"
	submitBatchCommandID = "you.submit.batch"
	submitRootPath       = "you"
	submitCommandPath    = "you submit"
	submitBatchPath      = "you submit batch"
)

// NewSubmitFamilyCommand constructs and detaches the canonical submit family
// without requiring run/server construction or handlers.
func NewSubmitFamilyCommand(registry *commandregistry.SubmitRegistry) (*cobra.Command, error) {
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build submit family command: %w", err)
	}
	family, err := selectSubmitRecords(manifest)
	if err != nil {
		return nil, err
	}
	return NewSubmitFamilyCommandFromManifest(family, registry)
}

// NewSubmitFamilyCommandFromManifest constructs exactly one submit parent and
// its batch child from the supplied canonical records.
func NewSubmitFamilyCommandFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.SubmitRegistry,
) (*cobra.Command, error) {
	if err := validateSubmitFamily(manifest, registry); err != nil {
		return nil, err
	}
	rootRecord, err := submitCanonicalRootRecord()
	if err != nil {
		return nil, err
	}
	submitRecord := manifest.Commands[submitCommandID]
	batchRecord := manifest.Commands[submitBatchCommandID]
	projection := climanifest.Manifest{
		FormatVersion: manifest.FormatVersion,
		RootPath:      rootRecord.Path,
		Commands: map[string]climanifest.Command{
			rootRecord.ID:   rootRecord,
			submitRecord.ID: submitRecord,
			batchRecord.ID:  batchRecord,
		},
	}
	noop := func(context.Context, map[string]any) error { return nil }
	root, err := NewCommandTree(projection, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID:   noop,
			submitRecord.Handler.ID: noop,
			batchRecord.Handler.ID:  noop,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build submit family command: %w", err)
	}
	submit, _, err := root.Find([]string{submitRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build submit family command: find submit: %w", err)
	}
	root.RemoveCommand(submit)
	submit.Use = submitRecord.Usage.Line
	if err := attachSubmitHandler(submit, submit, submitRecord, registry); err != nil {
		return nil, err
	}
	batch, _, err := submit.Find([]string{batchRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build submit family command: find batch: %w", err)
	}
	batch.Use = batchRecord.Usage.Line
	if err := attachSubmitHandler(submit, batch, batchRecord, registry); err != nil {
		return nil, err
	}
	return submit, nil
}

func selectSubmitRecords(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	submit, err := manifest.CommandByID(submitCommandID)
	if err != nil {
		return climanifest.Manifest{}, fmt.Errorf("build submit family command: %w", err)
	}
	batch, err := manifest.CommandByID(submitBatchCommandID)
	if err != nil {
		return climanifest.Manifest{}, fmt.Errorf("build submit family command: %w", err)
	}
	return climanifest.Manifest{
		FormatVersion: manifest.FormatVersion,
		RootPath:      manifest.RootPath,
		Commands: map[string]climanifest.Command{
			submit.ID: submit,
			batch.ID:  batch,
		},
	}, nil
}

func validateSubmitFamily(
	manifest climanifest.Manifest,
	registry *commandregistry.SubmitRegistry,
) error {
	if registry == nil {
		return fmt.Errorf("build submit family command: registry is required")
	}
	if manifest.RootPath != submitRootPath {
		return fmt.Errorf(
			"build submit family command: manifest rootPath = %q, want %q",
			manifest.RootPath,
			submitRootPath,
		)
	}
	if len(manifest.Commands) != 2 {
		return fmt.Errorf(
			"build submit family command: manifest command count = %d, want 2",
			len(manifest.Commands),
		)
	}
	submit, ok := manifest.Commands[submitCommandID]
	if !ok {
		return fmt.Errorf("build submit family command: missing command %q", submitCommandID)
	}
	batch, ok := manifest.Commands[submitBatchCommandID]
	if !ok {
		return fmt.Errorf("build submit family command: missing command %q", submitBatchCommandID)
	}
	if submit.ID != submitCommandID || submit.Path != submitCommandPath || !submit.Runnable {
		return fmt.Errorf("build submit family command: command %q has contradictory identity, path, or runnable metadata", submitCommandID)
	}
	if batch.ID != submitBatchCommandID || batch.Path != submitBatchPath || !batch.Runnable {
		return fmt.Errorf("build submit family command: command %q has contradictory identity, path, or runnable metadata", submitBatchCommandID)
	}
	if err := registry.Verify(manifest); err != nil {
		return fmt.Errorf("build submit family command: %w", err)
	}
	return nil
}

func submitCanonicalRootRecord() (climanifest.Command, error) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return climanifest.Command{}, fmt.Errorf("build submit family command: %w", err)
	}
	record, err := manifest.CommandByID("you")
	if err != nil {
		return climanifest.Command{}, fmt.Errorf("build submit family command: %w", err)
	}
	return record, nil
}

func attachSubmitHandler(
	familyRoot *cobra.Command,
	command *cobra.Command,
	record climanifest.Command,
	registry *commandregistry.SubmitRegistry,
) error {
	handler, err := registry.Lookup(record.Handler.ID)
	if err != nil {
		return fmt.Errorf("build submit family command: %w", err)
	}
	validate := command.PreRunE
	command.PreRunE = func(cmd *cobra.Command, args []string) error {
		if err := rejectDeprecatedPortFlag(cmd, args); err != nil {
			return err
		}
		if validate != nil {
			return validate(cmd, args)
		}
		return nil
	}
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		inputs, resolveErr := resolveSubmitLocalInputs(cmd, record)
		if resolveErr != nil {
			return fmt.Errorf("dispatch submit handler %q: %w", record.Handler.ID, resolveErr)
		}
		inherited, resolveErr := submitInheritedInputs(cmd, familyRoot)
		if resolveErr != nil {
			return fmt.Errorf("dispatch submit handler %q: %w", record.Handler.ID, resolveErr)
		}
		return handler(cmd, inputs, inherited)
	}
	return nil
}

func submitInheritedInputs(cmd, familyRoot *cobra.Command) (resolvedinput.Inputs, error) {
	if cmd.Root() == familyRoot {
		return resolvedinput.Inputs{}, nil
	}
	return ResolvedPersistentInputs(cmd)
}

func resolveSubmitLocalInputs(
	cmd *cobra.Command,
	record climanifest.Command,
) (resolvedinput.Inputs, error) {
	values, err := LocalInputValues(cmd, record)
	if err != nil {
		return resolvedinput.Inputs{}, err
	}
	definitions, candidates, err := resolvedSubmitArguments(record, values)
	if err != nil {
		return resolvedinput.Inputs{}, err
	}
	flagDefinitions, flagCandidates, err := resolvedSubmitFlags(cmd, record, values)
	if err != nil {
		return resolvedinput.Inputs{}, err
	}
	definitions = append(definitions, flagDefinitions...)
	candidates = append(candidates, flagCandidates...)
	inputs, err := resolvedinput.Resolve(definitions, candidates)
	if err != nil {
		return resolvedinput.Inputs{}, fmt.Errorf("resolve submit inputs: %w", err)
	}
	return inputs, nil
}

func resolvedSubmitArguments(
	record climanifest.Command,
	values map[string]any,
) ([]resolvedinput.Definition, []resolvedinput.Candidate, error) {
	definitions := make([]resolvedinput.Definition, 0, len(record.Arguments))
	candidates := make([]resolvedinput.Candidate, 0, len(record.Arguments))
	for _, argument := range record.Arguments {
		kind, err := resolvedValueKind(argument.ValueType)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve input %q: %w", argument.ID, err)
		}
		definitions = append(definitions, resolvedinput.Definition{
			ID: argument.ID, Kind: kind,
			Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument},
		})
		if values[argument.ID] == nil {
			continue
		}
		value, err := resolvedValue(values[argument.ID])
		if err != nil {
			return nil, nil, fmt.Errorf("resolve input %q: %w", argument.ID, err)
		}
		candidates = append(candidates, resolvedinput.Candidate{
			InputID: argument.ID, Source: resolvedinput.SourcePositionalArgument, Value: value,
		})
	}
	return definitions, candidates, nil
}

func resolvedSubmitFlags(
	cmd *cobra.Command,
	record climanifest.Command,
	values map[string]any,
) ([]resolvedinput.Definition, []resolvedinput.Candidate, error) {
	definitions := make([]resolvedinput.Definition, 0, len(record.Flags))
	candidates := make([]resolvedinput.Candidate, 0, 2*len(record.Flags))
	for _, flag := range record.Flags {
		if flag.Scope != "local" {
			continue
		}
		kind, err := resolvedValueKind(flag.ValueType)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve input %q: %w", flag.ID, err)
		}
		definitions = append(definitions, resolvedinput.Definition{
			ID: flag.ID, Kind: kind,
			Precedence: []resolvedinput.Source{
				resolvedinput.SourceCLIFlag,
				resolvedinput.SourceManifestDefault,
			},
			Sensitive: flag.Sensitivity != "" && flag.Sensitivity != "public",
		})
		value, err := resolvedValue(values[flag.ID])
		if err != nil {
			return nil, nil, fmt.Errorf("resolve input %q: %w", flag.ID, err)
		}
		candidates = append(candidates, resolvedinput.Candidate{
			InputID: flag.ID, Source: resolvedinput.SourceManifestDefault, Value: value,
		})
		if cmd.Flags().Changed(flag.Long) {
			candidates = append(candidates, resolvedinput.Candidate{
				InputID: flag.ID, Source: resolvedinput.SourceCLIFlag, Value: value,
			})
		}
	}
	return definitions, candidates, nil
}

// NewMCPCommand builds the independently injected `you mcp` family through
// the accepted generic manifest constructor.
func NewMCPCommand(handler ResolvedCobraHandler) (*cobra.Command, error) {
	manifest, err := generated.MCPFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewMCPCommandFromManifest(manifest, handler)
}

// NewMCPCommandFromManifest projects and detaches the complete MCP family.
func NewMCPCommandFromManifest(
	manifest climanifest.Manifest,
	handler ResolvedCobraHandler,
) (*cobra.Command, error) {
	if handler == nil {
		return nil, fmt.Errorf("build MCP command: handler is required")
	}
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	parentRecord, err := manifest.CommandByID("you.mcp")
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	if parentRecord.Runnable {
		return nil, fmt.Errorf("build MCP command: %q must remain non-runnable", parentRecord.ID)
	}
	serveRecord, err := manifest.CommandByID("you.mcp.serve")
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID:   rootRecord,
		parentRecord.ID: parentRecord,
		serveRecord.ID:  serveRecord,
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: ResolvedCobraHandlerRegistry{
			serveRecord.Handler.ID: handler,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	parent, _, err := root.Find([]string{parentRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build MCP command: find projected command: %w", err)
	}
	root.RemoveCommand(parent)
	serve, _, err := parent.Find([]string{serveRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build MCP command: find projected serve command: %w", err)
	}
	preserveMCPSourceRelationshipDiagnostic(serve, serveRecord)
	return parent, nil
}

func preserveMCPSourceRelationshipDiagnostic(
	command *cobra.Command,
	record climanifest.Command,
) {
	const relationshipID = "you.mcp.serve.relationship.runtime-source"
	if _, declared := record.Relationships[relationshipID]; !declared {
		return
	}
	validate := command.PreRunE
	command.PreRunE = func(cmd *cobra.Command, args []string) error {
		err := validate(cmd, args)
		if err != nil && strings.Contains(err.Error(), `input relationship "`+relationshipID+`"`) {
			return fmt.Errorf("cannot combine --runtime with --fixture-catalog")
		}
		return err
	}
}
