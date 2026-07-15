package climanifestcobra

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/spf13/cobra"
)

// RunSubmitFamilyComponents holds detached run and submit commands. Batch is
// attached only beneath Submit; Run and Submit remain ready for root fan-in.
type RunSubmitFamilyComponents struct {
	Run         *cobra.Command
	Submit      *cobra.Command
	SubmitBatch *cobra.Command
}

// RunSubmitFlagBindings supplies explicit storage for generated local flags.
// Inherited flags remain owned by the shared root command.
type RunSubmitFlagBindings struct {
	Run                 *runcli.RunConfig
	RunInvocationOutput *string
	Submit              *submitcli.SubmitConfig
	SubmitBatch         *submitcli.BatchConfig
	FlagUsages          map[string]string
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

	runRecord, submitRecord, batchRecord, err := runSubmitManifestRecords(manifest)
	if err != nil {
		return RunSubmitFamilyComponents{}, err
	}
	run, err := buildRunnableRunSubmitCommand(runRecord, registry, bindings)
	if err != nil {
		return RunSubmitFamilyComponents{}, err
	}
	run.DisableFlagParsing = true
	run.SilenceErrors = true
	submit, err := buildRunnableRunSubmitCommand(submitRecord, registry, bindings)
	if err != nil {
		return RunSubmitFamilyComponents{}, err
	}
	batch, err := buildRunnableRunSubmitCommand(batchRecord, registry, bindings)
	if err != nil {
		return RunSubmitFamilyComponents{}, err
	}
	submit.AddCommand(batch)
	return RunSubmitFamilyComponents{Run: run, Submit: submit, SubmitBatch: batch}, nil
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
	cmd.Args = positionalArgsFromManifest(record)
	if err := registerRunSubmitLocalFlags(cmd, record, bindings); err != nil {
		return nil, fmt.Errorf("build run/submit family command: %w", err)
	}
	if err := registry.AttachHandlers(cmd, record.ID); err != nil {
		return nil, fmt.Errorf("build run/submit family command: %w", err)
	}
	return cmd, nil
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
	var skipPermissions bool
	for _, flag := range sortedFlags(record.Flags) {
		if flag.Scope != "local" {
			continue
		}
		if flag.Long == "port" {
			registerDeprecatedPortFlag(cmd, &deprecatedPort)
			if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
				return err
			}
			continue
		}
		target, err := runSubmitLocalBindingTarget(record.ID, flag.Long, bindings, &skipPermissions)
		if err != nil {
			return err
		}
		if err := registerFlag(cmd.Flags(), flag, target, bindings.FlagUsages[flag.Long]); err != nil {
			return fmt.Errorf("register local flag %q: %w", flag.Long, err)
		}
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

func runSubmitLocalBindingTarget(
	commandID, flagName string,
	bindings RunSubmitFlagBindings,
	skipPermissions *bool,
) (flagTarget, error) {
	switch commandID {
	case "you.run":
		return runLocalBindingTarget(flagName, bindings.Run, bindings.RunInvocationOutput, skipPermissions)
	case "you.submit":
		return submitLocalBindingTarget(flagName, bindings.Submit)
	case "you.submit.batch":
		return submitBatchLocalBindingTarget(flagName, bindings.SubmitBatch)
	default:
		return flagTarget{}, fmt.Errorf("unsupported run/submit command %q", commandID)
	}
}

func runLocalBindingTarget(
	flagName string,
	cfg *runcli.RunConfig,
	invocationOutput *string,
	skipPermissions *bool,
) (flagTarget, error) {
	if target, ok := runExecutionLocalBindingTarget(flagName, cfg); ok {
		return target, nil
	}
	if target, ok := runRuntimeLocalBindingTarget(flagName, cfg); ok {
		return target, nil
	}
	if target, ok := runInvocationLocalBindingTarget(flagName, cfg, invocationOutput, skipPermissions); ok {
		return target, nil
	}
	return flagTarget{}, fmt.Errorf("unsupported run local flag %q", flagName)
}

func runExecutionLocalBindingTarget(flagName string, cfg *runcli.RunConfig) (flagTarget, bool) {
	switch flagName {
	case "continuously":
		return flagTarget{boolValue: &cfg.Continuously}, true
	case "work":
		return flagTarget{stringValue: &cfg.WorkFile}, true
	case "dir":
		return flagTarget{stringValue: &cfg.Dir}, true
	case "named":
		return flagTarget{stringValue: &cfg.NamedFactoryName}, true
	case "factory":
		return flagTarget{stringValue: &cfg.FactoryConfigPath}, true
	case "record":
		return flagTarget{stringValue: &cfg.RecordPath}, true
	case "no-record":
		return flagTarget{boolValue: &cfg.DisableDefaultRecording}, true
	case "replay":
		return flagTarget{stringValue: &cfg.ReplayPath}, true
	default:
		return flagTarget{}, false
	}
}

func runRuntimeLocalBindingTarget(flagName string, cfg *runcli.RunConfig) (flagTarget, bool) {
	switch flagName {
	case "runtime-log-dir":
		return flagTarget{stringValue: &cfg.RuntimeLogDir}, true
	case "runtime-log-max-size-mb":
		return flagTarget{intValue: &cfg.RuntimeLogConfig.MaxSize}, true
	case "runtime-log-max-backups":
		return flagTarget{intValue: &cfg.RuntimeLogConfig.MaxBackups}, true
	case "runtime-log-max-age-days":
		return flagTarget{intValue: &cfg.RuntimeLogConfig.MaxAge}, true
	case "runtime-log-compress":
		return flagTarget{boolValue: &cfg.RuntimeLogConfig.Compress}, true
	case "runtime-metrics-dir":
		return flagTarget{stringValue: &cfg.RuntimeMetricsDir}, true
	case "runtime-metrics-max-size-mb":
		return flagTarget{intValue: &cfg.RuntimeMetricsConfig.MaxSize}, true
	case "runtime-metrics-max-backups":
		return flagTarget{intValue: &cfg.RuntimeMetricsConfig.MaxBackups}, true
	case "runtime-metrics-max-age-days":
		return flagTarget{intValue: &cfg.RuntimeMetricsConfig.MaxAge}, true
	case "runtime-metrics-compress":
		return flagTarget{boolValue: &cfg.RuntimeMetricsConfig.Compress}, true
	default:
		return flagTarget{}, false
	}
}

func runInvocationLocalBindingTarget(
	flagName string,
	cfg *runcli.RunConfig,
	invocationOutput *string,
	skipPermissions *bool,
) (flagTarget, bool) {
	switch flagName {
	case "with-mock-workers":
		return flagTarget{stringValue: &cfg.MockWorkersConfigPath}, true
	case "quiet":
		return flagTarget{boolValue: &cfg.SuppressDashboardRendering}, true
	case "output":
		return flagTarget{stringValue: invocationOutput}, true
	case "skip-permissions":
		return flagTarget{boolValue: skipPermissions}, true
	default:
		return flagTarget{}, false
	}
}

func submitLocalBindingTarget(flagName string, cfg *submitcli.SubmitConfig) (flagTarget, error) {
	switch flagName {
	case "name":
		return flagTarget{stringValue: &cfg.Name}, nil
	case "work-type-name":
		return flagTarget{stringValue: &cfg.WorkTypeName}, nil
	case "payload":
		return flagTarget{stringValue: &cfg.Payload}, nil
	case "session":
		return flagTarget{stringValue: &cfg.SessionID}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported submit local flag %q", flagName)
	}
}

func submitBatchLocalBindingTarget(flagName string, cfg *submitcli.BatchConfig) (flagTarget, error) {
	switch flagName {
	case "file":
		return flagTarget{stringValue: &cfg.FileFlag}, nil
	case "dry-run":
		return flagTarget{boolValue: &cfg.DryRun}, nil
	case "session":
		return flagTarget{stringValue: &cfg.SessionID}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported submit batch local flag %q", flagName)
	}
}

func runSubmitManifestRecords(manifest climanifest.Manifest) (run, submit, batch climanifest.Command, err error) {
	run, err = manifest.CommandByID("you.run")
	if err != nil {
		return run, submit, batch, fmt.Errorf("build run/submit family command: %w", err)
	}
	submit, err = manifest.CommandByID("you.submit")
	if err != nil {
		return run, submit, batch, fmt.Errorf("build run/submit family command: %w", err)
	}
	batch, err = manifest.CommandByID("you.submit.batch")
	if err != nil {
		return run, submit, batch, fmt.Errorf("build run/submit family command: %w", err)
	}
	return run, submit, batch, nil
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
	required := []struct {
		name string
		set  bool
	}{
		{name: "Run", set: bindings.Run != nil},
		{name: "RunInvocationOutput", set: bindings.RunInvocationOutput != nil},
		{name: "Submit", set: bindings.Submit != nil},
		{name: "SubmitBatch", set: bindings.SubmitBatch != nil},
	}
	for _, binding := range required {
		if !binding.set {
			return fmt.Errorf("build run/submit family command: bindings.%s is required", binding.name)
		}
	}
	return nil
}
