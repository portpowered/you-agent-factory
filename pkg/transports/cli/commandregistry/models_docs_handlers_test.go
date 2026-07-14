package commandregistry_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	modelscli "github.com/portpowered/infinite-you/pkg/transports/cli/models"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func TestNewModelsDocsRegistryRejectsMissingHandlers(t *testing.T) {
	handlers := commandregistry.ModelsDocsHandlers{
		DocsRunE:          noopRunE,
		ModelsListRunE:    noopRunE,
		ModelsInspectRunE: noopRunE,
		ModelsInvokeRunE:  noopRunE,
		ModelsPullRunE:    noopRunE,
	}
	for _, tc := range []struct {
		name    string
		mutate  func(*commandregistry.ModelsDocsHandlers)
	}{
		{"docs", func(h *commandregistry.ModelsDocsHandlers) { h.DocsRunE = nil }},
		{"list", func(h *commandregistry.ModelsDocsHandlers) { h.ModelsListRunE = nil }},
		{"inspect", func(h *commandregistry.ModelsDocsHandlers) { h.ModelsInspectRunE = nil }},
		{"invoke", func(h *commandregistry.ModelsDocsHandlers) { h.ModelsInvokeRunE = nil }},
		{"pull", func(h *commandregistry.ModelsDocsHandlers) { h.ModelsPullRunE = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutated := handlers
			tc.mutate(&mutated)
			if _, err := commandregistry.NewModelsDocsRegistry(mutated); err == nil {
				t.Fatalf("NewModelsDocsRegistry() missing %s handler = nil, want error", tc.name)
			}
		})
	}
}

func TestDocsRunEPrintsPackagedIndexWithoutTopic(t *testing.T) {
	runE := commandregistry.DocsRunE(commandregistry.DocsBinding{BinaryName: "you"})
	cmd := &cobra.Command{Use: "docs"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if !strings.Contains(out.String(), "you docs") {
		t.Fatalf("stdout = %q, want packaged docs index guidance", out.String())
	}
}

func TestDocsRunEDefaultsBinaryNameWhenUnset(t *testing.T) {
	runE := commandregistry.DocsRunE(commandregistry.DocsBinding{})
	cmd := &cobra.Command{Use: "docs"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if !strings.Contains(out.String(), "you docs") {
		t.Fatalf("stdout = %q, want default binary name in index", out.String())
	}
}

func TestDocsRunEPrintsTopicMarkdownAndDiagnostics(t *testing.T) {
	var diagnostic bytes.Buffer
	runE := commandregistry.DocsRunE(commandregistry.DocsBinding{
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return &diagnostic
		},
		Verbose: func() bool { return true },
	})
	cmd := &cobra.Command{Use: "docs"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runE(cmd, []string{"models"}); err != nil {
		t.Fatalf("RunE(models) error = %v", err)
	}
	if !strings.Contains(out.String(), "you models") {
		t.Fatalf("stdout = %q, want packaged models topic markdown", out.String())
	}
	if !strings.Contains(diagnostic.String(), "docs request topic=models") {
		t.Fatalf("diagnostics = %q, want topic request log", diagnostic.String())
	}
}

func TestDocsRunEPropagatesUnsupportedTopicError(t *testing.T) {
	runE := commandregistry.DocsRunE(commandregistry.DocsBinding{
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return io.Discard
		},
		Verbose: func() bool { return false },
	})
	cmd := &cobra.Command{Use: "docs"}
	cmd.SetOut(io.Discard)
	if err := runE(cmd, []string{"not-a-real-topic"}); err == nil {
		t.Fatal("RunE(unsupported topic) error = nil, want failure")
	}
}

func TestModelsListRunEMapsBindingsToListConfig(t *testing.T) {
	server := "http://127.0.0.1:7437"
	json := true
	debug := true
	var diagnostic bytes.Buffer
	runE := commandregistry.ModelsListRunE(commandregistry.ModelsListBinding{
		Server: &server,
		JSON:   &json,
		Verbose: func() bool { return true },
		Debug:  &debug,
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return &diagnostic
		},
		ListModels: func(cfg modelscli.ListConfig) error {
			if cfg.Server != server || !cfg.JSON || !cfg.Verbose || !cfg.Debug {
				t.Fatalf("ListConfig = %#v, want bound server/json/verbose/debug", cfg)
			}
			if cfg.Diagnostics != &diagnostic {
				t.Fatalf("diagnostics writer = %T, want *bytes.Buffer", cfg.Diagnostics)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "list"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want list output routed through ListModels", out.String())
	}
}

func TestModelsInspectRunEMapsModelNameAndBindings(t *testing.T) {
	server := "http://127.0.0.1:7437"
	runE := commandregistry.ModelsInspectRunE(commandregistry.ModelsInspectBinding{
		Server: &server,
		InspectModel: func(cfg modelscli.InspectConfig) error {
			if cfg.ModelName != "OMNIVOICE_Q4_K_M" || cfg.Server != server {
				t.Fatalf("InspectConfig = %#v, want model name and server binding", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "inspect"}
	cmd.SetOut(io.Discard)
	if err := runE(cmd, []string{"OMNIVOICE_Q4_K_M"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestModelsPullRunEMapsModelNameAndBindings(t *testing.T) {
	runE := commandregistry.ModelsPullRunE(commandregistry.ModelsPullBinding{
		PullModel: func(cfg modelscli.PullConfig) error {
			if cfg.ModelName != "OMNIVOICE_Q4_K_M" {
				t.Fatalf("PullConfig model = %q, want OMNIVOICE_Q4_K_M", cfg.ModelName)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "pull"}
	cmd.SetOut(io.Discard)
	if err := runE(cmd, []string{"OMNIVOICE_Q4_K_M"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestModelsInvokeRunEMapsBindingsAndDependencies(t *testing.T) {
	server := "http://127.0.0.1:7437"
	json := true
	operation := "TTS"
	text := "hello"
	output := "./speech.wav"
	debug := true
	logger := zap.NewNop()
	runE := commandregistry.ModelsInvokeRunE(commandregistry.ModelsInvokeBinding{
		Server:     &server,
		JSON:       &json,
		Operation:  &operation,
		Text:       &text,
		OutputPath: &output,
		Verbose:    func() bool { return true },
		Debug:      &debug,
		HomeDir: func() (string, error) {
			return "/home/tester", nil
		},
		ResolveOperatorDefaults: func(cmd *cobra.Command, homeDir string) (operatorconfig.ResolvedDefaults, error) {
			if homeDir != "/home/tester" {
				t.Fatalf("homeDir = %q, want /home/tester", homeDir)
			}
			return operatorconfig.ResolvedDefaults{}, nil
		},
		BuildLogger: func() (*zap.Logger, error) {
			return logger, nil
		},
		InvokeModel: func(cfg modelscli.InvokeConfig) error {
			if cfg.ModelName != "OMNIVOICE_Q4_K_M" || cfg.Operation != operation || cfg.Text != text || cfg.OutputPath != output {
				t.Fatalf("InvokeConfig = %#v, want bound model/operation/text/output", cfg)
			}
			if cfg.Server != server || !cfg.JSON || !cfg.Verbose || !cfg.Debug || cfg.HomeDir != "/home/tester" || cfg.Logger != logger {
				t.Fatalf("InvokeConfig bindings = %#v, want server/json/verbose/debug/home/logger", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "invoke"}
	cmd.SetOut(io.Discard)
	if err := runE(cmd, []string{"OMNIVOICE_Q4_K_M"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestModelsInvokeRunEPropagatesHomeDirFailure(t *testing.T) {
	wantErr := errors.New("home dir unavailable")
	runE := commandregistry.ModelsInvokeRunE(commandregistry.ModelsInvokeBinding{
		HomeDir: func() (string, error) {
			return "", wantErr
		},
		BuildLogger: func() (*zap.Logger, error) {
			return zap.NewNop(), nil
		},
		ResolveOperatorDefaults: func(cmd *cobra.Command, homeDir string) (operatorconfig.ResolvedDefaults, error) {
			return operatorconfig.ResolvedDefaults{}, nil
		},
	})
	cmd := &cobra.Command{Use: "invoke"}
	if err := runE(cmd, []string{"OMNIVOICE_Q4_K_M"}); !errors.Is(err, wantErr) {
		t.Fatalf("RunE() error = %v, want %v", err, wantErr)
	}
}
