package climanifestcobra

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// WorkFamilyComponents holds detached work-family commands before production
// wiring attaches the generated work parent to the root.
type WorkFamilyComponents struct {
	Work      *cobra.Command
	List      *cobra.Command
	Show      *cobra.Command
	Move      *cobra.Command
	Visualize *cobra.Command
}

// WorkFamilyBindings supplies parser-only scalar storage keyed by stable
// manifest input ID. Handlers build typed Work requests per invocation.
type WorkFamilyBindings struct {
	LocalTargets map[string]any
}

// NewWorkFamilyCommand builds the work you.work → list/show/move/visualize tree
// from generated metadata and attaches handwritten handlers by stable command ID.
// Only contracted work-family commands are constructed.
func NewWorkFamilyCommand(registry *commandregistry.Registry, bindings WorkFamilyBindings) (*cobra.Command, error) {
	components, err := NewWorkFamilyComponents(registry, bindings)
	if err != nil {
		return nil, err
	}
	components.Work.AddCommand(components.List, components.Show, components.Move, components.Visualize)
	return components.Work, nil
}

// NewWorkFamilyComponents builds detached work-family commands so production
// wiring can attach the generated work parent without rewriting unrelated roots.
func NewWorkFamilyComponents(registry *commandregistry.Registry, bindings WorkFamilyBindings) (WorkFamilyComponents, error) {
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	return NewWorkFamilyComponentsFromManifest(manifest, registry, bindings)
}

// NewWorkFamilyCommandFromManifest builds the work tree from one generated
// manifest snapshot. Manifest command IDs must stay within the work family.
func NewWorkFamilyCommandFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings WorkFamilyBindings,
) (*cobra.Command, error) {
	components, err := NewWorkFamilyComponentsFromManifest(manifest, registry, bindings)
	if err != nil {
		return nil, err
	}
	components.Work.AddCommand(components.List, components.Show, components.Move, components.Visualize)
	return components.Work, nil
}

// NewWorkFamilyComponentsFromManifest builds detached work-family commands from
// one generated manifest snapshot.
func NewWorkFamilyComponentsFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings WorkFamilyBindings,
) (WorkFamilyComponents, error) {
	if registry == nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: registry is required")
	}
	if err := validateWorkBindings(bindings); err != nil {
		return WorkFamilyComponents{}, err
	}
	if err := validateWorkManifest(manifest); err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}

	workRecord, listRecord, showRecord, moveRecord, visualizeRecord, err := workManifestRecords(manifest)
	if err != nil {
		return WorkFamilyComponents{}, err
	}

	work, err := buildWorkCommandFromRecord(workRecord)
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	if workRecord.Runnable {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %q must remain non-runnable", workRecord.ID)
	}

	list, err := buildRunnableWorkLeaf(listRecord, registry, bindings)
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	show, err := buildRunnableWorkLeaf(showRecord, registry, bindings)
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	move, err := buildRunnableWorkLeaf(moveRecord, registry, bindings)
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	visualize, err := buildRunnableWorkLeaf(visualizeRecord, registry, bindings)
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}

	return WorkFamilyComponents{
		Work:      work,
		List:      list,
		Show:      show,
		Move:      move,
		Visualize: visualize,
	}, nil
}

func buildRunnableWorkLeaf(
	record climanifest.Command,
	registry *commandregistry.Registry,
	bindings WorkFamilyBindings,
) (*cobra.Command, error) {
	cmd, err := buildWorkCommandFromRecord(record)
	if err != nil {
		return nil, err
	}
	cmd.Args = positionalArgsFromManifest(record)
	cmd.PreRunE = rejectDeprecatedPortFlag
	if err := registerWorkLocalFlags(cmd, record, bindings); err != nil {
		return nil, err
	}
	if err := registry.AttachRunE(cmd, record.ID); err != nil {
		return nil, err
	}
	return cmd, nil
}

func workManifestRecords(manifest climanifest.Manifest) (
	work, list, show, move, visualize climanifest.Command,
	err error,
) {
	work, err = manifest.CommandByID("you.work")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build work family command: %w", err)
	}
	list, err = manifest.CommandByID("you.work.list")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build work family command: %w", err)
	}
	show, err = manifest.CommandByID("you.work.show")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build work family command: %w", err)
	}
	move, err = manifest.CommandByID("you.work.move")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build work family command: %w", err)
	}
	visualize, err = manifest.CommandByID("you.work.visualize")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build work family command: %w", err)
	}
	return work, list, show, move, visualize, nil
}

func validateWorkManifest(manifest climanifest.Manifest) error {
	if len(manifest.Commands) != len(climanifestgen.WorkFamilyCommandIDs) {
		return fmt.Errorf(
			"manifest command count = %d, want %d work-family commands",
			len(manifest.Commands),
			len(climanifestgen.WorkFamilyCommandIDs),
		)
	}
	for commandID := range manifest.Commands {
		if err := climanifestgen.AssertWorkFamilyCommandID(commandID); err != nil {
			return err
		}
	}
	for _, commandID := range climanifestgen.WorkFamilyCommandIDs {
		if _, ok := manifest.Commands[commandID]; !ok {
			return fmt.Errorf("manifest missing work-family command %q", commandID)
		}
	}
	return nil
}

func validateWorkBindings(bindings WorkFamilyBindings) error {
	if len(bindings.LocalTargets) == 0 {
		return fmt.Errorf("build work family command: bindings.LocalTargets is required")
	}
	return nil
}

func buildWorkCommandFromRecord(record climanifest.Command) (*cobra.Command, error) {
	if err := climanifestgen.AssertWorkFamilyCommandID(record.ID); err != nil {
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

func registerWorkLocalFlags(cmd *cobra.Command, record climanifest.Command, bindings WorkFamilyBindings) error {
	var deprecatedPort int
	flags := sortedFlags(record.Flags)
	for _, flag := range flags {
		if flag.Scope != "local" {
			continue
		}
		if flag.Long == "port" {
			registerDeprecatedPortFlag(cmd, &deprecatedPort)
			annotateStableInput(cmd, flag)
			if err := applyFlagContract(cmd.Flags().Lookup("port"), flag); err != nil {
				return fmt.Errorf("apply port flag contract: %w", err)
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
