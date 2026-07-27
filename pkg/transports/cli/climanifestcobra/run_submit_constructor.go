package climanifestcobra

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// RunSubmitFamilyComponents holds detached run and submit commands. Batch is
// attached only beneath Submit; Run and Submit remain ready for root fan-in.
type RunSubmitFamilyComponents struct {
	Run         *cobra.Command
	Server      *cobra.Command
	Submit      *cobra.Command
	SubmitBatch *cobra.Command
}

// RunSubmitFlagBindings supplies parser-only scalar storage keyed by stable
// manifest input ID. Handlers build typed transport configs per invocation.
type RunSubmitFlagBindings struct {
	LocalTargets map[string]any
}

// NewRunSubmitFamilyComponents builds the detached family from generated
// metadata and attaches retained handwritten lifecycles by stable command ID.
func NewRunSubmitFamilyComponents(
	registry *commandregistry.Registry,
	bindings RunSubmitFlagBindings,
) (RunSubmitFamilyComponents, error) {
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		return RunSubmitFamilyComponents{}, fmt.Errorf("build run/submit family command: %w", err)
	}
	return NewRunSubmitFamilyComponentsFromManifest(manifest, registry, bindings)
}

// NewRunSubmitFamilyComponentsFromManifest builds one isolated family snapshot.
func NewRunSubmitFamilyComponentsFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings RunSubmitFlagBindings,
) (RunSubmitFamilyComponents, error) {
	if registry == nil {
		return RunSubmitFamilyComponents{}, fmt.Errorf("build run/submit family command: registry is required")
	}
	if err := validateRunSubmitBindings(bindings); err != nil {
		return RunSubmitFamilyComponents{}, err
	}
	if err := validateRunSubmitManifest(manifest); err != nil {
		return RunSubmitFamilyComponents{}, fmt.Errorf("build run/submit family command: %w", err)
	}
	if err := registry.VerifyRunSubmitRunnableCoverage(manifest); err != nil {
		return RunSubmitFamilyComponents{}, fmt.Errorf("build run/submit family command: %w", err)
	}

	runRecord, serverRecord, submitRecord, batchRecord, err := runSubmitManifestRecords(manifest)
	if err != nil {
		return RunSubmitFamilyComponents{}, err
	}
	run, err := buildRunnableRunSubmitCommand(runRecord, registry, bindings)
	if err != nil {
		return RunSubmitFamilyComponents{}, err
	}
	run.DisableFlagParsing = true
	run.SilenceErrors = true
	server, err := buildRunnableRunSubmitCommand(serverRecord, registry, bindings)
	if err != nil {
		return RunSubmitFamilyComponents{}, err
	}
	server.SilenceErrors = true
	submit, err := buildRunnableRunSubmitCommand(submitRecord, registry, bindings)
	if err != nil {
		return RunSubmitFamilyComponents{}, err
	}
	batch, err := buildRunnableRunSubmitCommand(batchRecord, registry, bindings)
	if err != nil {
		return RunSubmitFamilyComponents{}, err
	}
	submit.AddCommand(batch)
	return RunSubmitFamilyComponents{Run: run, Server: server, Submit: submit, SubmitBatch: batch}, nil
}

func buildRunnableRunSubmitCommand(
	record climanifest.Command,
	registry *commandregistry.Registry,
	bindings RunSubmitFlagBindings,
) (*cobra.Command, error) {
	cmd, err := buildRunSubmitCommandFromRecord(record)
	if err != nil {
		return nil, fmt.Errorf("build run/submit family command: %w", err)
	}
	if !record.Runnable {
		return nil, fmt.Errorf("build run/submit family command: %q must remain runnable", record.ID)
	}
	// Batch historically validates positional cardinality while resolving its
	// handwritten input channels. Preserve that diagnostic and side-effect
	// ordering instead of adding earlier Cobra rejection on the generated path.
	if record.ID != "you.submit.batch" {
		cmd.Args = positionalArgsFromManifest(record)
	}
	if err := registerRunSubmitLocalFlags(cmd, record, bindings); err != nil {
		return nil, fmt.Errorf("build run/submit family command: %w", err)
	}
	relationships, err := planStandaloneCommandRelationships(record)
	if err != nil {
		return nil, fmt.Errorf("build run/submit family command: %w", err)
	}
	if err := projectCobraFlagGroupAnnotations(cmd, record.ID, relationships); err != nil {
		return nil, fmt.Errorf("build run/submit family command: %w", err)
	}
	if err := registry.AttachHandlers(cmd, record.ID); err != nil {
		return nil, fmt.Errorf("build run/submit family command: %w", err)
	}
	return cmd, nil
}

func planStandaloneCommandRelationships(record climanifest.Command) ([]plannedRelationship, error) {
	arguments := make([]climanifest.Argument, 0, len(record.Arguments))
	for _, argument := range record.Arguments {
		arguments = append(arguments, argument)
	}
	return planRelationships([]plannedCommand{{record: record, arguments: arguments}}, 0)
}

func buildRunSubmitCommandFromRecord(record climanifest.Command) (*cobra.Command, error) {
	if err := climanifestgen.AssertRunSubmitFamilyCommandID(record.ID); err != nil {
		return nil, err
	}
	cmd := &cobra.Command{
		Use:     record.Usage.Line,
		Short:   record.Documentation.Documentation.Title.CanonicalEnglish,
		Long:    record.Documentation.Documentation.Description.CanonicalEnglish,
		Example: record.Usage.Example,
		Aliases: append([]string(nil), record.Aliases...),
	}
	if record.Visibility == "hidden" {
		cmd.Hidden = true
	}
	return cmd, nil
}

func registerRunSubmitLocalFlags(
	cmd *cobra.Command,
	record climanifest.Command,
	bindings RunSubmitFlagBindings,
) error {
	var deprecatedPort int
	for _, flag := range sortedFlags(record.Flags) {
		if flag.Scope != "local" {
			continue
		}
		if flag.Long == "port" {
			registerDeprecatedPortFlag(cmd, &deprecatedPort)
			annotateStableInput(cmd, flag)
			if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
				return err
			}
			continue
		}
		target, err := flagBindingTarget(flag.ID, bindings.LocalTargets)
		if err != nil {
			return err
		}
		if err := registerFlag(cmd.Flags(), flag, target, flag.Usage); err != nil {
			return fmt.Errorf("register local flag %q: %w", flag.Long, err)
		}
		annotateStableInput(cmd, flag)
		if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
			return fmt.Errorf("apply local flag %q contract: %w", flag.Long, err)
		}
		// Required in the command record describes the public input contract.
		// This retained family historically validates named submit inputs inside
		// its handwritten handler, which preserves its stable diagnostics and
		// validation ordering. Do not add Cobra required annotations here.
	}
	return nil
}

func runSubmitManifestRecords(manifest climanifest.Manifest) (run, server, submit, batch climanifest.Command, err error) {
	run, err = manifest.CommandByID("you.run")
	if err != nil {
		return run, server, submit, batch, fmt.Errorf("build run/submit family command: %w", err)
	}
	server, err = manifest.CommandByID("you.server")
	if err != nil {
		return run, server, submit, batch, fmt.Errorf("build run/submit family command: %w", err)
	}
	submit, err = manifest.CommandByID("you.submit")
	if err != nil {
		return run, server, submit, batch, fmt.Errorf("build run/submit family command: %w", err)
	}
	batch, err = manifest.CommandByID("you.submit.batch")
	if err != nil {
		return run, server, submit, batch, fmt.Errorf("build run/submit family command: %w", err)
	}
	return run, server, submit, batch, nil
}

func validateRunSubmitManifest(manifest climanifest.Manifest) error {
	if len(manifest.Commands) != len(climanifestgen.RunSubmitFamilyCommandIDs) {
		return fmt.Errorf(
			"manifest command count = %d, want %d run/submit-family commands",
			len(manifest.Commands),
			len(climanifestgen.RunSubmitFamilyCommandIDs),
		)
	}
	for commandID := range manifest.Commands {
		if err := climanifestgen.AssertRunSubmitFamilyCommandID(commandID); err != nil {
			return err
		}
	}
	for _, commandID := range climanifestgen.RunSubmitFamilyCommandIDs {
		if _, ok := manifest.Commands[commandID]; !ok {
			return fmt.Errorf("manifest missing run/submit-family command %q", commandID)
		}
	}
	return nil
}

func validateRunSubmitBindings(bindings RunSubmitFlagBindings) error {
	if len(bindings.LocalTargets) == 0 {
		return fmt.Errorf("build run/submit family command: bindings.LocalTargets is required")
	}
	return nil
}
