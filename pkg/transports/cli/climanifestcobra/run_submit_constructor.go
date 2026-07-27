package climanifestcobra

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

var runServerCommandIDs = []string{"you.run", "you.server"}

// RunServerFamilyComponents holds detached run and server commands ready for
// root fan-in.
type RunServerFamilyComponents struct {
	Run    *cobra.Command
	Server *cobra.Command
}

// RunServerFlagBindings supplies parser-only scalar storage keyed by stable
// manifest input ID. Handlers build typed transport configs per invocation.
type RunServerFlagBindings struct {
	LocalTargets map[string]any
}

// NewRunServerFamilyComponents builds the detached run/server family from
// generated metadata and attaches retained handwritten lifecycles by stable ID.
func NewRunServerFamilyComponents(
	registry *commandregistry.Registry,
	bindings RunServerFlagBindings,
) (RunServerFamilyComponents, error) {
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		return RunServerFamilyComponents{}, fmt.Errorf("build run/server family command: %w", err)
	}
	manifest, err = selectRunServerManifest(manifest)
	if err != nil {
		return RunServerFamilyComponents{}, err
	}
	return NewRunServerFamilyComponentsFromManifest(manifest, registry, bindings)
}

// NewRunServerFamilyComponentsFromManifest builds one isolated family snapshot.
func NewRunServerFamilyComponentsFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings RunServerFlagBindings,
) (RunServerFamilyComponents, error) {
	if registry == nil {
		return RunServerFamilyComponents{}, fmt.Errorf("build run/server family command: registry is required")
	}
	if err := validateRunServerBindings(bindings); err != nil {
		return RunServerFamilyComponents{}, err
	}
	if err := validateRunServerManifest(manifest); err != nil {
		return RunServerFamilyComponents{}, fmt.Errorf("build run/server family command: %w", err)
	}
	if err := registry.VerifyRunServerRunnableCoverage(manifest); err != nil {
		return RunServerFamilyComponents{}, fmt.Errorf("build run/server family command: %w", err)
	}

	runRecord, serverRecord, err := runServerManifestRecords(manifest)
	if err != nil {
		return RunServerFamilyComponents{}, err
	}
	run, err := buildRunnableRunServerCommand(runRecord, registry, bindings)
	if err != nil {
		return RunServerFamilyComponents{}, err
	}
	run.DisableFlagParsing = true
	run.SilenceErrors = true
	server, err := buildRunnableRunServerCommand(serverRecord, registry, bindings)
	if err != nil {
		return RunServerFamilyComponents{}, err
	}
	server.SilenceErrors = true
	return RunServerFamilyComponents{Run: run, Server: server}, nil
}

func selectRunServerManifest(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	selected := climanifest.Manifest{
		FormatVersion: manifest.FormatVersion,
		RootPath:      manifest.RootPath,
		Commands:      make(map[string]climanifest.Command, len(runServerCommandIDs)),
	}
	for _, commandID := range runServerCommandIDs {
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return climanifest.Manifest{}, fmt.Errorf("build run/server family command: %w", err)
		}
		selected.Commands[commandID] = record
	}
	return selected, nil
}

func buildRunnableRunServerCommand(
	record climanifest.Command,
	registry *commandregistry.Registry,
	bindings RunServerFlagBindings,
) (*cobra.Command, error) {
	cmd, err := buildRunServerCommandFromRecord(record)
	if err != nil {
		return nil, fmt.Errorf("build run/server family command: %w", err)
	}
	if !record.Runnable {
		return nil, fmt.Errorf("build run/server family command: %q must remain runnable", record.ID)
	}
	cmd.Args = positionalArgsFromManifest(record)
	if err := registerRunServerLocalFlags(cmd, record, bindings); err != nil {
		return nil, fmt.Errorf("build run/server family command: %w", err)
	}
	relationships, err := planStandaloneCommandRelationships(record)
	if err != nil {
		return nil, fmt.Errorf("build run/server family command: %w", err)
	}
	if err := projectCobraFlagGroupAnnotations(cmd, record.ID, relationships); err != nil {
		return nil, fmt.Errorf("build run/server family command: %w", err)
	}
	if err := registry.AttachHandlers(cmd, record.ID); err != nil {
		return nil, fmt.Errorf("build run/server family command: %w", err)
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

func buildRunServerCommandFromRecord(record climanifest.Command) (*cobra.Command, error) {
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

func registerRunServerLocalFlags(
	cmd *cobra.Command,
	record climanifest.Command,
	bindings RunServerFlagBindings,
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
	}
	return nil
}

func runServerManifestRecords(
	manifest climanifest.Manifest,
) (run, server climanifest.Command, err error) {
	run, err = manifest.CommandByID("you.run")
	if err != nil {
		return run, server, fmt.Errorf("build run/server family command: %w", err)
	}
	server, err = manifest.CommandByID("you.server")
	if err != nil {
		return run, server, fmt.Errorf("build run/server family command: %w", err)
	}
	return run, server, nil
}

func validateRunServerManifest(manifest climanifest.Manifest) error {
	if len(manifest.Commands) != len(runServerCommandIDs) {
		return fmt.Errorf(
			"manifest command count = %d, want %d run/server commands",
			len(manifest.Commands),
			len(runServerCommandIDs),
		)
	}
	for commandID := range manifest.Commands {
		if commandID != "you.run" && commandID != "you.server" {
			return fmt.Errorf("manifest contains non-run/server command %q", commandID)
		}
	}
	for _, commandID := range runServerCommandIDs {
		if _, ok := manifest.Commands[commandID]; !ok {
			return fmt.Errorf("manifest missing run/server command %q", commandID)
		}
	}
	return nil
}

func validateRunServerBindings(bindings RunServerFlagBindings) error {
	if len(bindings.LocalTargets) == 0 {
		return fmt.Errorf("build run/server family command: bindings.LocalTargets is required")
	}
	return nil
}
