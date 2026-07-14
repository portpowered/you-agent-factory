package commandregistry

import (
	"fmt"
	"io"

	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	modelscli "github.com/portpowered/infinite-you/pkg/transports/cli/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// ModelsDocsHandlers carries handwritten RunE handlers for contracted runnable
// models/docs-family command IDs.
type ModelsDocsHandlers struct {
	DocsRunE          RunE
	ModelsListRunE    RunE
	ModelsInspectRunE RunE
	ModelsInvokeRunE  RunE
	ModelsPullRunE    RunE
}

// NewModelsDocsRegistry registers handwritten handlers for the models/docs family
// and verifies contracted runnable command coverage.
func NewModelsDocsRegistry(handlers ModelsDocsHandlers) (*Registry, error) {
	required := []struct {
		commandID string
		handler   RunE
	}{
		{"you.docs", handlers.DocsRunE},
		{"you.models.list", handlers.ModelsListRunE},
		{"you.models.inspect", handlers.ModelsInspectRunE},
		{"you.models.invoke", handlers.ModelsInvokeRunE},
		{"you.models.pull", handlers.ModelsPullRunE},
	}
	for _, entry := range required {
		if entry.handler == nil {
			return nil, fmt.Errorf("build models/docs handler registry: %s handler is required", entry.commandID)
		}
	}

	registry := NewRegistry()
	for _, entry := range required {
		if err := registry.Register(entry.commandID, entry.handler); err != nil {
			return nil, fmt.Errorf("build models/docs handler registry: %w", err)
		}
	}

	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build models/docs handler registry: %w", err)
	}
	if err := registry.VerifyModelsDocsRunnableCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build models/docs handler registry: %w", err)
	}
	return registry, nil
}

// DocsBinding supplies handwritten docs execution dependencies.
type DocsBinding struct {
	BinaryName        string
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	Verbose           func() bool
}

// DocsRunE returns the handwritten docs RunE used by production wiring.
func DocsRunE(binding DocsBinding) RunE {
	binaryName := binding.BinaryName
	if binaryName == "" {
		binaryName = "you"
	}
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			_, err := io.WriteString(cmd.OutOrStdout(), docscli.IndexMarkdown(binaryName))
			return err
		}

		topic := args[0]
		diagnosticsOutput := binding.DiagnosticsWriter(cmd)
		clidiag.Printf(diagnosticsOutput, binding.Verbose(), "docs request topic=%s", topic)
		markdown, err := docscli.Markdown(topic)
		if err != nil {
			clidiag.Printf(diagnosticsOutput, binding.Verbose(), "docs failed topic=%s phase=resolve-topic", topic)
			return err
		}
		clidiag.Printf(diagnosticsOutput, binding.Verbose(), "docs resolved topic=%s contentBytes=%d", topic, len(markdown))
		_, err = io.WriteString(cmd.OutOrStdout(), markdown)
		if err != nil {
			clidiag.Printf(diagnosticsOutput, binding.Verbose(), "docs failed topic=%s phase=write-output", topic)
		}
		return err
	}
}

// ModelsListBinding supplies handwritten models list execution dependencies.
type ModelsListBinding struct {
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	ListModels        func(modelscli.ListConfig) error
}

// ModelsListRunE returns the handwritten models list RunE used by production wiring.
func ModelsListRunE(binding ModelsListBinding) RunE {
	listModels := binding.ListModels
	if listModels == nil {
		listModels = modelscli.List
	}
	return func(cmd *cobra.Command, args []string) error {
		cfg := modelscli.ListConfig{}
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		if binding.Verbose != nil {
			cfg.Verbose = binding.Verbose()
		}
		if binding.Debug != nil {
			cfg.Debug = *binding.Debug
		}
		return listModels(cfg)
	}
}

// ModelsInspectBinding supplies handwritten models inspect execution dependencies.
type ModelsInspectBinding struct {
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	InspectModel      func(modelscli.InspectConfig) error
}

// ModelsInspectRunE returns the handwritten models inspect RunE used by production wiring.
func ModelsInspectRunE(binding ModelsInspectBinding) RunE {
	inspectModel := binding.InspectModel
	if inspectModel == nil {
		inspectModel = modelscli.Inspect
	}
	return func(cmd *cobra.Command, args []string) error {
		cfg := modelscli.InspectConfig{}
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if len(args) == 1 {
			cfg.ModelName = args[0]
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		if binding.Verbose != nil {
			cfg.Verbose = binding.Verbose()
		}
		if binding.Debug != nil {
			cfg.Debug = *binding.Debug
		}
		return inspectModel(cfg)
	}
}

// ModelsInvokeBinding supplies handwritten models invoke execution dependencies.
type ModelsInvokeBinding struct {
	Server               *string
	JSON                 *bool
	Operation            *string
	Text                 *string
	OutputPath           *string
	Verbose              func() bool
	Debug                *bool
	DiagnosticsWriter    func(cmd *cobra.Command) io.Writer
	HomeDir              func() (string, error)
	ResolveOperatorDefaults func(cmd *cobra.Command, homeDir string) (operatorconfig.ResolvedDefaults, error)
	BuildLogger          func() (*zap.Logger, error)
	BuildModelInvocation modelscli.InvocationBuilder
	InvokeModel          func(modelscli.InvokeConfig) error
}

// ModelsInvokeRunE returns the handwritten models invoke RunE used by production wiring.
func ModelsInvokeRunE(binding ModelsInvokeBinding) RunE {
	invokeModel := binding.InvokeModel
	if invokeModel == nil {
		invokeModel = modelscli.Invoke
	}
	return func(cmd *cobra.Command, args []string) error {
		logger, err := binding.BuildLogger()
		if err != nil {
			return err
		}
		homeDir, err := binding.HomeDir()
		if err != nil {
			return fmt.Errorf("resolve process home directory: %w", err)
		}
		resolvedOperatorDefaults, err := binding.ResolveOperatorDefaults(cmd, homeDir)
		if err != nil {
			return err
		}
		cfg := modelscli.InvokeConfig{Operation: "TTS"}
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if len(args) == 1 {
			cfg.ModelName = args[0]
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		if binding.Operation != nil {
			cfg.Operation = *binding.Operation
		}
		if binding.Text != nil {
			cfg.Text = *binding.Text
		}
		if binding.OutputPath != nil {
			cfg.OutputPath = *binding.OutputPath
		}
		cfg.HomeDir = homeDir
		cfg.OperatorDefaults = resolvedOperatorDefaults
		cfg.Logger = logger
		cfg.Output = cmd.OutOrStdout()
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		if binding.Verbose != nil {
			cfg.Verbose = binding.Verbose()
		}
		if binding.Debug != nil {
			cfg.Debug = *binding.Debug
		}
		cfg.BuildInvocation = binding.BuildModelInvocation
		return invokeModel(cfg)
	}
}

// ModelsPullBinding supplies handwritten models pull execution dependencies.
type ModelsPullBinding struct {
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	PullModel         func(modelscli.PullConfig) error
}

// ModelsPullRunE returns the handwritten models pull RunE used by production wiring.
func ModelsPullRunE(binding ModelsPullBinding) RunE {
	pullModel := binding.PullModel
	if pullModel == nil {
		pullModel = modelscli.Pull
	}
	return func(cmd *cobra.Command, args []string) error {
		cfg := modelscli.PullConfig{}
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if len(args) == 1 {
			cfg.ModelName = args[0]
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		if binding.Verbose != nil {
			cfg.Verbose = binding.Verbose()
		}
		if binding.Debug != nil {
			cfg.Debug = *binding.Debug
		}
		return pullModel(cfg)
	}
}
