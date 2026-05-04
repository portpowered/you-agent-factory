// Package cli defines Cobra commands for the agent-factory CLI.
// Commands contain only flag parsing and delegate to command-specific packages.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	configcli "github.com/portpowered/infinite-you/pkg/cli/config"
	defaultcmd "github.com/portpowered/infinite-you/pkg/cli/default"
	docscli "github.com/portpowered/infinite-you/pkg/cli/docs"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/spf13/cobra"
)

var runCLI = runcli.Run
var flattenFactoryConfig = configcli.FlattenFactoryConfig
var expandFactoryConfig = configcli.ExpandFactoryConfig
var initFactory = initcmd.Init
var submitWork = submitcli.Submit

const (
	defaultMockWorkersConfigPathSentinel = "__agent_factory_default_mock_workers_config__"
)

const cliBinaryName = "infinite-you"

// NewRootCommand creates the top-level Cobra command for the infinite-you CLI.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   cliBinaryName,
		Short: "Run and manage CPN-based workflow factories",
		Long: "Run and manage CPN-based workflow factories.\n\n" +
			"Running " + cliBinaryName + " with no arguments starts the out-of-the-box flow: " +
			"it prepares ./factory when needed, keeps the runtime alive in continuous mode, " +
			"watches factory/inputs/task/default for Markdown or JSON task files, and reports " +
			"the local dashboard at the first available port, preferring http://localhost:7437/dashboard/ui.\n\n" +
			"Packaged reference topics are also available through " + cliBinaryName + " docs <topic>. " +
			"Supported docs topics: " + supportedDocsTopicsHelpText() + ".",
		Example: "  # Start the default Codex-backed factory in the current project.\n" +
			"  " + cliBinaryName + "\n\n" +
			"  # In another terminal, submit a Markdown task to the default scaffold.\n" +
			"  printf \"Fix the lint issues\\n\" > factory/inputs/task/default/fix-lint.md\n\n" +
			"  # Print the packaged workstation reference page from the installed binary.\n" +
			"  " + cliBinaryName + " docs workstation\n\n" +
			"  # Explicit batch-style runs are still available when you need them.\n" +
			"  " + cliBinaryName + " run --dir factory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFactory(cmd, defaultcmd.OOTBRunConfig(), false, false)
		},
	}

	root.AddCommand(
		newConfigCommand(),
		newDocsCommand(),
		newInitCommand(),
		newRunCommand(),
		newSubmitCommand(),
	)

	return root
}

func newDocsCommand() *cobra.Command {
	docsCmd := &cobra.Command{
		Use:   "docs",
		Short: "Print packaged markdown reference topics",
		Long: "Print packaged markdown reference topics from the installed binary.\n\n" +
			"Use one of the supported topic subcommands to print the authored markdown page with no wrapper formatting.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	for _, topic := range docscli.SupportedTopics() {
		docsCmd.AddCommand(newDocsTopicCommand(topic))
	}

	return docsCmd
}

func supportedDocsTopicsHelpText() string {
	return strings.Join(docscli.SupportedTopics(), ", ")
}

func newDocsTopicCommand(topic string) *cobra.Command {
	return &cobra.Command{
		Use:   topic,
		Short: fmt.Sprintf("Print the packaged %s reference page", topic),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			markdown, err := docscli.Markdown(topic)
			if err != nil {
				return err
			}
			_, err = io.WriteString(cmd.OutOrStdout(), markdown)
			return err
		},
	}
}

func newConfigCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and transform factory configuration",
	}
	configCmd.AddCommand(
		newConfigExpandCommand(),
		newConfigFlattenCommand(),
	)
	return configCmd
}

func newConfigFlattenCommand() *cobra.Command {
	cfg := configcli.FactoryConfigFlattenConfig{}

	cmd := &cobra.Command{
		Use:   "flatten <factory-path>",
		Short: "Write canonical single-file factory config",
		Long: "Write canonical single-file factory config.\n\n" +
			"The path may be a factory directory containing factory.json or a standalone factory.json file. " +
			"The command writes camelCase canonical JSON to stdout.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Path = args[0]
			cfg.Output = cmd.OutOrStdout()
			return flattenFactoryConfig(cfg)
		},
	}

	return cmd
}

func newConfigExpandCommand() *cobra.Command {
	cfg := configcli.FactoryConfigExpandConfig{}

	cmd := &cobra.Command{
		Use:   "expand <factory.json>",
		Short: "Write split factory config layout",
		Long: "Write split factory config layout.\n\n" +
			"The path may be a standalone factory.json file or a factory directory containing factory.json. " +
			"The command writes canonical factory.json plus workers and workstations directories beside the input file.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Path = args[0]
			cfg.Output = cmd.OutOrStdout()
			return expandFactoryConfig(cfg)
		},
	}

	return cmd
}

// Execute runs the root command.
func Execute() {
	if err := NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func newInitCommand() *cobra.Command {
	cfg := initcmd.InitConfig{
		Dir:      defaultcmd.FactoryDir,
		Type:     string(initcmd.DefaultScaffoldType),
		Executor: initcmd.DefaultStarterExecutor,
	}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create factory directory structure",
		Long: "Create factory directory structure.\n\n" +
			"Supported scaffold types:\n" +
			"  default - single-step task-processing scaffold\n" +
			"  ralph   - minimal PRD-to-execution scaffold\n\n" +
			"Omitting --executor preserves the default Codex-backed starter scaffold. " +
			"Supported starter scaffold values are codex and claude. " +
			"Omitting --type keeps the current default scaffold behavior. " +
			"For the default scaffold, --executor chooses which starter worker scaffold is generated.",
		Example: "  # Create the default Codex-backed scaffold in ./factory.\n" +
			"  " + cliBinaryName + " init\n\n" +
			"  # Create a Claude-backed default scaffold in a custom directory.\n" +
			"  " + cliBinaryName + " init --dir my-factory --executor claude\n\n" +
			"  # Create the minimal Ralph PRD-to-execution scaffold.\n" +
			"  " + cliBinaryName + " init --type ralph --dir ralph-factory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return initFactory(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "base directory to create")
	cmd.Flags().StringVar(&cfg.Type, "type", cfg.Type, "scaffold type to generate (supported: default, ralph)")
	cmd.Flags().StringVar(
		&cfg.Executor,
		"executor",
		cfg.Executor,
		fmt.Sprintf(
			"starter scaffold to generate (%s)",
			strings.Join(initcmd.SupportedStarterExecutors(), ", "),
		),
	)
	return cmd
}

func newRunCommand() *cobra.Command {
	cfg := defaultcmd.ExplicitRunConfig()
	var verbose bool
	var debug bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Load workflow and run the factory engine",
		Long: "Load workflow and run the factory engine.\n\n" +
			"For the quickest local setup, run " + cliBinaryName + " with no arguments. " +
			"That default flow bootstraps ./factory, watches factory/inputs/task/default, " +
			"keeps the runtime alive, and reports the first available dashboard URL, preferring http://localhost:7437/dashboard/ui. " +
			"Default execution uses batch mode and exits after idle completion. " +
			"Use --continuously to keep the factory alive while idle until you cancel it. " +
			"Use --with-mock-workers with an optional JSON config path to test workflows with deterministic mock worker outcomes. " +
			"Use --quiet to suppress dashboard output for scripted or CI-oriented runs. " +
			"Runtime logs are structured JSON rolling files; environment details are record-channel diagnostics only, and system logs include command stdout/stderr only on command failures.",
		Example: "  # Start the out-of-the-box continuous factory.\n" +
			"  " + cliBinaryName + "\n\n" +
			"  # Submit a Markdown task to the default scaffold.\n" +
			"  printf \"Fix the lint issues\\n\" > factory/inputs/task/default/fix-lint.md\n\n" +
			"  # Run an existing factory once in explicit batch mode.\n" +
			"  " + cliBinaryName + " run --dir factory",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.MockWorkersEnabled = cmd.Flags().Changed("with-mock-workers")
			if cmd.Flags().Changed("port") {
				cfg.AutoPort = false
			}
			if cfg.MockWorkersConfigPath == defaultMockWorkersConfigPathSentinel {
				if len(args) > 0 {
					cfg.MockWorkersConfigPath = args[0]
				} else {
					cfg.MockWorkersConfigPath = ""
				}
			}
			return runFactory(cmd, cfg, verbose, debug)
		},
	}

	cmd.Flags().StringVar(&cfg.Workflow, "workflow", "", "workflow ID to run (default: all)")
	cmd.Flags().BoolVar(&cfg.Continuously, "continuously", false, "keep the factory alive while idle until cancelled")
	cmd.Flags().StringVar(&cfg.WorkFile, "work", "", "path to initial FACTORY_REQUEST_BATCH JSON file to submit")
	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "factory base directory")
	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port; specifying this flag disables automatic fallback")
	cmd.Flags().StringVar(&cfg.RecordPath, "record", "", "path to write a replay artifact for this run")
	cmd.Flags().StringVar(&cfg.ReplayPath, "replay", "", "path to replay an existing replay artifact")
	cmd.Flags().StringVar(&cfg.RuntimeLogDir, "runtime-log-dir", "", "directory for structured runtime log files (default: ~/.agent-factory/logs)")
	cmd.Flags().IntVar(&cfg.RuntimeLogConfig.MaxSize, "runtime-log-max-size-mb", cfg.RuntimeLogConfig.MaxSize, "rotate each runtime log file after this many megabytes")
	cmd.Flags().IntVar(&cfg.RuntimeLogConfig.MaxBackups, "runtime-log-max-backups", cfg.RuntimeLogConfig.MaxBackups, "maximum rotated runtime log files to retain")
	cmd.Flags().IntVar(&cfg.RuntimeLogConfig.MaxAge, "runtime-log-max-age-days", cfg.RuntimeLogConfig.MaxAge, "maximum days to retain rotated runtime log files")
	cmd.Flags().BoolVar(&cfg.RuntimeLogConfig.Compress, "runtime-log-compress", false, "compress rotated runtime log files")
	cmd.Flags().StringVar(&cfg.MockWorkersConfigPath, "with-mock-workers", "", "enable mock-worker execution with an optional mock-workers JSON config path")
	cmd.Flags().Lookup("with-mock-workers").NoOptDefVal = defaultMockWorkersConfigPathSentinel
	cmd.Flags().BoolVar(&cfg.SuppressDashboardRendering, "quiet", false, "suppress dashboard output for quiet or CI-oriented runs")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose (info-level) logging")
	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "enable debug-level logging (implies verbose)")
	return cmd
}

func runFactory(cmd *cobra.Command, cfg runcli.RunConfig, verbose, debug bool) error {
	logger, err := logging.BuildLogger(verbose, debug)
	if err != nil {
		return err
	}
	cfg.Logger = logger
	cfg.Verbose = verbose || debug
	cfg.StartupOutput = cmd.OutOrStdout()

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			logger.Info("received signal, shutting down")
			cancel()
		case <-ctx.Done():
		}
	}()

	return runCLI(ctx, cfg)
}

func newSubmitCommand() *cobra.Command {
	cfg := submitcli.SubmitConfig{Port: 8080}

	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit work to a running factory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return submitWork(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.WorkTypeName, "work-type-name", "", "work type name to submit to (required)")
	cmd.Flags().StringVar(&cfg.Payload, "payload", "", "path to payload file (.json or .md) (required)")
	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	return cmd
}
