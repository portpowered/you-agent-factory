package cli

import (
	"fmt"
	"io"
	"strings"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// CommandHandler owns Cobra-to-Models request transformation for the complete
// `you models` family. The top-level CLI only attaches these methods by ID.
type CommandHandler struct {
	models                  Service
	diagnosticsWriter       func(*cobra.Command) io.Writer
	homeDir                 func() (string, error)
	resolveOperatorDefaults func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error)
	buildLogger             func() (*zap.Logger, error)
}

// NewCommandHandler constructs the Models-owned CLI handler from injected dependencies.
func NewCommandHandler(
	models Service,
	diagnosticsWriter func(*cobra.Command) io.Writer,
	homeDir func() (string, error),
	resolveOperatorDefaults func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error),
	buildLogger func() (*zap.Logger, error),
) *CommandHandler {
	return &CommandHandler{
		models: models, diagnosticsWriter: diagnosticsWriter, homeDir: homeDir,
		resolveOperatorDefaults: resolveOperatorDefaults, buildLogger: buildLogger,
	}
}

const (
	modelsInspectNameInputID = "you.models.inspect.arg.0"
	modelsInvokeNameInputID  = "you.models.invoke.arg.0"
	modelsInvokeOperationID  = "you.models.invoke.flag.operation"
	modelsInvokeTextID       = "you.models.invoke.flag.text"
	modelsInvokeInputID      = "you.models.invoke.flag.input"
	modelsInvokeParameterID  = "you.models.invoke.flag.parameter"
	modelsInvokeOutputID     = "you.models.invoke.flag.output"
	modelsInvokeOutputMapID  = "you.models.invoke.flag.output-map"
	modelsPullNameInputID    = "you.models.pull.arg.0"
	modelsRemoveNameInputID  = "you.models.remove.arg.0"
	serverInputID            = "you.flag.server"
	jsonInputID              = "you.flag.json"
	verboseInputID           = "you.flag.verbose"
	debugInputID             = "you.flag.debug"
)

func (h *CommandHandler) List(
	cmd *cobra.Command,
	_ resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.models == nil {
		return fmt.Errorf("models list service is required")
	}
	cfg := ListConfig{Context: cmd.Context(), Output: cmd.OutOrStdout()}
	if err := h.applyResolvedCommon(cmd, inherited, &cfg.Server, &cfg.JSON, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics); err != nil {
		return fmt.Errorf("resolve models list inputs: %w", err)
	}
	return h.models.List(cfg)
}

func (h *CommandHandler) Inspect(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.models == nil {
		return fmt.Errorf("models inspect service is required")
	}
	modelName, err := inputs.String(modelsInspectNameInputID)
	if err != nil {
		return fmt.Errorf("read models inspect model name: %w", err)
	}
	cfg := InspectConfig{
		Context: cmd.Context(), ModelName: modelName, Output: cmd.OutOrStdout(),
	}
	if err := h.applyResolvedCommon(cmd, inherited, &cfg.Server, &cfg.JSON, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics); err != nil {
		return fmt.Errorf("resolve models inspect inputs: %w", err)
	}
	return h.models.Inspect(cfg)
}

func (h *CommandHandler) applyResolvedCommon(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	server *string,
	json, verbose, debug *bool,
	diagnostics *io.Writer,
) error {
	var err error
	if state, ok := inputs.State(serverInputID); ok && state.Default {
		*server = ""
	} else if *server, err = inputs.String(serverInputID); err != nil {
		return err
	}
	if *json, err = inputs.Bool(jsonInputID); err != nil {
		return err
	}
	if *verbose, err = inputs.Bool(verboseInputID); err != nil {
		return err
	}
	if *debug, err = inputs.Bool(debugInputID); err != nil {
		return err
	}
	if h.diagnosticsWriter != nil {
		*diagnostics = h.diagnosticsWriter(cmd)
	}
	return nil
}

func (h *CommandHandler) Invoke(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
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
	invokeInputs, err := readModelsInvokeInputs(inputs)
	if err != nil {
		return err
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
		Context: cmd.Context(), ModelName: invokeInputs.modelName, Operation: invokeInputs.operation,
		Text: invokeInputs.text, InputMappings: invokeInputs.inputMappings,
		ParameterSpecs: invokeInputs.parameterSpecs, OutputPath: invokeInputs.outputPath,
		OutputMappings: invokeInputs.outputMappings, Output: cmd.OutOrStdout(),
		// Leave FactoryDir empty so the Factory Session invocation boundary owns
		// the documented default-layout discovery. A non-empty value is reserved
		// for an explicit directory supplied by a caller of the Models service.
		WorkingDirectory: startupcli.WorkingDirectory(cmd.Context()), HomeDir: homeDir,
		OperatorDefaults: defaults, Logger: logger,
	}
	if err := h.applyResolvedCommon(cmd, inherited, &cfg.Server, &cfg.JSON, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics); err != nil {
		return fmt.Errorf("resolve models invoke inputs: %w", err)
	}
	return h.models.Invoke(cfg)
}

type modelsInvokeInputs struct {
	modelName      string
	operation      string
	text           string
	inputMappings  []string
	parameterSpecs []string
	outputPath     string
	outputMappings []string
}

func inferGenericCLIModelOperation(modelName string) string {
	switch strings.ToLower(strings.TrimSpace(modelName)) {
	case strings.ToLower(modelinference.BuiltInModelNameLLM):
		return modelinference.OperationOMNI
	case strings.ToLower(modelinference.BuiltInModelNameASR):
		return modelinference.OperationASR
	case strings.ToLower(modelinference.BuiltInModelNameTTS):
		return modelinference.OperationTTS
	case strings.ToLower(modelinference.BuiltInModelNameEmbed):
		return modelinference.OperationEMBED
	default:
		return ""
	}
}

func readModelsInvokeInputs(inputs resolvedinput.Inputs) (modelsInvokeInputs, error) {
	modelName, err := inputs.String(modelsInvokeNameInputID)
	if err != nil {
		return modelsInvokeInputs{}, fmt.Errorf("read models invoke model name: %w", err)
	}
	operation, err := inputs.String(modelsInvokeOperationID)
	if err != nil {
		return modelsInvokeInputs{}, fmt.Errorf("read models invoke operation: %w", err)
	}
	text, err := inputs.String(modelsInvokeTextID)
	if err != nil {
		return modelsInvokeInputs{}, fmt.Errorf("read models invoke text: %w", err)
	}
	inputMappings, err := readModelsInvokeInputMappings(inputs)
	if err != nil {
		return modelsInvokeInputs{}, err
	}
	var parameterSpecs []string
	if _, present := inputs.State(modelsInvokeParameterID); present {
		parameterSpecs, err = inputs.StringArray(modelsInvokeParameterID)
		if err != nil {
			return modelsInvokeInputs{}, fmt.Errorf("read models invoke parameters: %w", err)
		}
	}
	if state, present := inputs.State(modelsInvokeOperationID); present && state.Default && len(inputMappings) > 0 {
		// The manifest keeps the legacy TTS default for text invocations. A
		// generic input binding selects the operation from the built-in model
		// alias unless the caller supplied --operation explicitly.
		operation = ""
	}
	outputPath, outputMappings, err := readModelsInvokeOutputs(inputs)
	if err != nil {
		return modelsInvokeInputs{}, err
	}
	return modelsInvokeInputs{
		modelName: modelName, operation: operation, text: text,
		inputMappings:  inputMappings,
		parameterSpecs: parameterSpecs, outputPath: outputPath,
		outputMappings: outputMappings,
	}, nil
}

func readModelsInvokeInputMappings(inputs resolvedinput.Inputs) ([]string, error) {
	if _, present := inputs.State(modelsInvokeInputID); !present {
		return nil, nil
	}
	inputMappings, err := inputs.StringArray(modelsInvokeInputID)
	if err != nil {
		return nil, fmt.Errorf("read models invoke input mappings: %w", err)
	}
	return inputMappings, nil
}

func readModelsInvokeOutputs(inputs resolvedinput.Inputs) (string, []string, error) {
	var outputValues []string
	var err error
	if _, present := inputs.State(modelsInvokeOutputID); present {
		outputValues, err = inputs.StringArray(modelsInvokeOutputID)
		if err != nil {
			// Keep the adapter tolerant of callers that construct resolved input
			// values directly using the pre-repeatable scalar shape. Production
			// manifest parsing always supplies StringArray here.
			var scalar string
			if scalar, err = inputs.String(modelsInvokeOutputID); err != nil {
				return "", nil, fmt.Errorf("read models invoke output values: %w", err)
			}
			outputValues = []string{scalar}
		}
	} else if _, err = inputs.StringArray(modelsInvokeOutputID); err != nil {
		return "", nil, fmt.Errorf("read models invoke output: %w", err)
	}
	var outputPath string
	var outputMappings []string
	for _, value := range outputValues {
		value = strings.TrimSpace(value)
		if strings.Contains(value, "=") {
			outputMappings = append(outputMappings, value)
			continue
		}
		if outputPath != "" {
			return "", nil, fmt.Errorf("repeatable --output values must use slot=path mappings after the first unqualified path")
		}
		outputPath = value
	}
	if _, present := inputs.State(modelsInvokeOutputMapID); present {
		var legacyMappings []string
		legacyMappings, err = inputs.StringArray(modelsInvokeOutputMapID)
		if err != nil {
			return "", nil, fmt.Errorf("read models invoke output mappings: %w", err)
		}
		outputMappings = append(outputMappings, legacyMappings...)
	}
	if outputPath != "" && len(outputMappings) > 0 {
		return "", nil, fmt.Errorf("--output path cannot be combined with named output mappings")
	}
	return outputPath, outputMappings, nil
}

func (h *CommandHandler) Pull(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.models == nil {
		return fmt.Errorf("models pull service is required")
	}
	modelName, err := inputs.String(modelsPullNameInputID)
	if err != nil {
		return fmt.Errorf("read models pull model name: %w", err)
	}
	cfg := PullConfig{
		Context: cmd.Context(), ModelName: modelName, Output: cmd.OutOrStdout(),
	}
	if err := h.applyResolvedCommon(cmd, inherited, &cfg.Server, &cfg.JSON, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics); err != nil {
		return fmt.Errorf("resolve models pull inputs: %w", err)
	}
	return h.models.Pull(cfg)
}

func (h *CommandHandler) Remove(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.models == nil {
		return fmt.Errorf("models remove service is required")
	}
	modelName, err := inputs.String(modelsRemoveNameInputID)
	if err != nil {
		return fmt.Errorf("read models remove model name: %w", err)
	}
	cfg := RemoveConfig{
		Context: cmd.Context(), ModelName: modelName, Output: cmd.OutOrStdout(),
	}
	if err := h.applyResolvedCommon(cmd, inherited, &cfg.Server, &cfg.JSON, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics); err != nil {
		return fmt.Errorf("resolve models remove inputs: %w", err)
	}
	return h.models.Remove(cfg)
}
