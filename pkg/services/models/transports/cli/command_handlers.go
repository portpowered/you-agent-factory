package cli

import (
	"fmt"
	"io"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// CommandHandler owns Cobra-to-Models request transformation for the complete
// `you models` family. The top-level CLI only attaches these methods by ID.
type CommandHandler struct {
	models                  Service
	server                  *string
	json                    *bool
	verbose                 func() bool
	debug                   *bool
	diagnosticsWriter       func(*cobra.Command) io.Writer
	homeDir                 func() (string, error)
	resolveOperatorDefaults func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error)
	buildLogger             func() (*zap.Logger, error)
}

// NewCommandHandler constructs the Models-owned CLI handler from injected dependencies.
func NewCommandHandler(
	models Service,
	server *string,
	json *bool,
	verbose func() bool,
	debug *bool,
	diagnosticsWriter func(*cobra.Command) io.Writer,
	homeDir func() (string, error),
	resolveOperatorDefaults func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error),
	buildLogger func() (*zap.Logger, error),
) *CommandHandler {
	return &CommandHandler{
		models: models, server: server, json: json, verbose: verbose, debug: debug,
		diagnosticsWriter: diagnosticsWriter, homeDir: homeDir,
		resolveOperatorDefaults: resolveOperatorDefaults, buildLogger: buildLogger,
	}
}

func (h *CommandHandler) List(cmd *cobra.Command, _ []string) error {
	if h == nil || h.models == nil {
		return fmt.Errorf("models list service is required")
	}
	cfg := ListConfig{Context: cmd.Context(), Output: cmd.OutOrStdout()}
	h.applyCommon(cmd, &cfg.Server, &cfg.JSON, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics)
	return h.models.List(cfg)
}

func (h *CommandHandler) Inspect(cmd *cobra.Command, args []string) error {
	if h == nil || h.models == nil {
		return fmt.Errorf("models inspect service is required")
	}
	cfg := InspectConfig{Context: cmd.Context(), Output: cmd.OutOrStdout()}
	if len(args) == 1 {
		cfg.ModelName = args[0]
	}
	h.applyCommon(cmd, &cfg.Server, &cfg.JSON, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics)
	return h.models.Inspect(cfg)
}

func (h *CommandHandler) Invoke(cmd *cobra.Command, args []string) error {
	if h == nil || h.models == nil {
		return fmt.Errorf("models invoke service is required")
	}
	if h.buildLogger == nil {
		return fmt.Errorf("model invocation logger builder is required")
	}
	if h.homeDir == nil {
		return fmt.Errorf("model invocation home directory resolver is required")
	}
	if h.resolveOperatorDefaults == nil {
		return fmt.Errorf("model invocation operator defaults resolver is required")
	}
	logger, err := h.buildLogger()
	if err != nil {
		return err
	}
	homeDir, err := h.homeDir()
	if err != nil {
		return fmt.Errorf("resolve process home directory: %w", err)
	}
	defaults, err := h.resolveOperatorDefaults(cmd, homeDir)
	if err != nil {
		return err
	}
	cfg := InvokeConfig{
		Context: cmd.Context(), Operation: "TTS", Output: cmd.OutOrStdout(),
		HomeDir: homeDir, FactoryDir: startupcli.WorkingDirectory(cmd.Context()),
		OperatorDefaults: defaults, Logger: logger,
	}
	if len(args) == 1 {
		cfg.ModelName = args[0]
	}
	if value, flagErr := cmd.Flags().GetString("operation"); flagErr == nil {
		cfg.Operation = value
	}
	if value, flagErr := cmd.Flags().GetString("text"); flagErr == nil {
		cfg.Text = value
	}
	if value, flagErr := cmd.Flags().GetString("output"); flagErr == nil {
		cfg.OutputPath = value
	}
	h.applyCommon(cmd, &cfg.Server, &cfg.JSON, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics)
	return h.models.Invoke(cfg)
}

func (h *CommandHandler) Pull(cmd *cobra.Command, args []string) error {
	if h == nil || h.models == nil {
		return fmt.Errorf("models pull service is required")
	}
	cfg := PullConfig{Context: cmd.Context(), Output: cmd.OutOrStdout()}
	if len(args) == 1 {
		cfg.ModelName = args[0]
	}
	h.applyCommon(cmd, &cfg.Server, &cfg.JSON, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics)
	return h.models.Pull(cfg)
}

func (h *CommandHandler) applyCommon(
	cmd *cobra.Command,
	server *string,
	json, verbose, debug *bool,
	diagnostics *io.Writer,
) {
	if h.server != nil {
		*server = *h.server
	}
	if h.json != nil {
		*json = *h.json
	}
	if h.verbose != nil {
		*verbose = h.verbose()
	}
	if h.debug != nil {
		*debug = *h.debug
	}
	if h.diagnosticsWriter != nil {
		*diagnostics = h.diagnosticsWriter(cmd)
	}
}
