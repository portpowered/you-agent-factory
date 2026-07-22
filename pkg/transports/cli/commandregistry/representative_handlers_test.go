package commandregistry_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

func noopRunE(*cobra.Command, []string) error { return nil }

func TestRunnableRepresentativeCommandIDsFromGeneratedManifest(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	ids, err := commandregistry.RunnableRepresentativeCommandIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableRepresentativeCommandIDs() error = %v", err)
	}
	if len(ids) != 2 || ids[0] != "you" || ids[1] != "you.session.show" {
		t.Fatalf("runnable IDs = %#v, want [you you.session.show]", ids)
	}
}

func TestVerifyRepresentativeRunnableCoverage(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you.session.show", noopRunE); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.VerifyRepresentativeRunnableCoverage(manifest); err == nil {
		t.Fatal("missing root handler = nil, want error")
	}
	if err := registry.Register("you", noopRunE); err != nil {
		t.Fatalf("Register(you) error = %v", err)
	}
	if err := registry.VerifyRepresentativeRunnableCoverage(manifest); err != nil {
		t.Fatalf("complete coverage error = %v", err)
	}
}

func TestNewRepresentativeRegistryRegistersContractedRunnableIDs(t *testing.T) {
	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE:        noopRunE,
		SessionShowRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}
	for _, commandID := range []string{"you", "you.session.show"} {
		if _, lookupErr := registry.Lookup(commandID); lookupErr != nil {
			t.Fatalf("Lookup(%q) error = %v", commandID, lookupErr)
		}
	}
}

func TestNewSessionRegistryRegistersExactlyCanonicalRunnableIDs(t *testing.T) {
	registry, err := commandregistry.NewSessionRegistry(commandregistry.SessionHandlers{
		CreateRunE: noopRunE, ListRunE: noopRunE, ShowRunE: noopRunE,
		DeleteRunE: noopRunE, PauseRunE: noopRunE, ResumeRunE: noopRunE, DispatchesRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewSessionRegistry() error = %v", err)
	}
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
	}
	if err := registry.VerifySessionRunnableCoverage(manifest); err != nil {
		t.Fatalf("VerifySessionRunnableCoverage() error = %v", err)
	}
	if err := registry.Register("you.work.list", noopRunE); err != nil {
		t.Fatalf("Register(cross-family) error = %v", err)
	}
	if err := registry.VerifySessionRunnableCoverage(manifest); err == nil || !strings.Contains(err.Error(), "you.work.list") {
		t.Fatalf("VerifySessionRunnableCoverage() error = %v, want extra stable ID", err)
	}
}

func TestVerifySessionRunnableCoverageRejectsMissingAndInvalidHandlerIDs(t *testing.T) {
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range []string{
		"you.session.create", "you.session.list", "you.session.show", "you.session.delete",
		"you.session.pause", "you.session.resume",
	} {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifySessionRunnableCoverage(manifest); err == nil || !strings.Contains(err.Error(), "you.session.dispatches") {
		t.Fatalf("missing coverage error = %v", err)
	}

	show := manifest.Commands["you.session.show"]
	show.Handler.ID = "you.session.delete.handler"
	manifest.Commands["you.session.show"] = show
	if _, err := commandregistry.RunnableSessionCommandIDs(manifest); err == nil || !strings.Contains(err.Error(), "you.session.show") {
		t.Fatalf("invalid handler ID error = %v", err)
	}
}

func TestSessionHandlerBindingsMapFlagsArgumentsAndDiagnostics(t *testing.T) {
	var diagnostic bytes.Buffer
	createCfg := sessioncli.CreateConfig{}
	createRunE := commandregistry.SessionCreateRunE(commandregistry.SessionCreateBinding{
		Config: &createCfg,
		SessionDiagnosticsBinding: commandregistry.SessionDiagnosticsBinding{
			Verbose: func() bool { return true }, Debug: boolPtr(true),
			DiagnosticsWriter: func(*cobra.Command) io.Writer { return &diagnostic },
		},
		CreateSession: func(cfg sessioncli.CreateConfig) error {
			if cfg.Dir != "factory" || !cfg.Verbose || !cfg.Debug || cfg.Diagnostics != &diagnostic {
				t.Fatalf("create config = %#v", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "create", RunE: createRunE}
	cmd.Flags().StringVar(&createCfg.Dir, "dir", "", "")
	cmd.SetArgs([]string{"--dir", "factory"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("create Execute() error = %v", err)
	}

	deleteCfg := sessioncli.DeleteConfig{}
	deleteRunE := commandregistry.SessionDeleteRunE(commandregistry.SessionDeleteBinding{
		Config: &deleteCfg,
		DeleteSession: func(cfg sessioncli.DeleteConfig) error {
			if cfg.SessionID != "session-beta" {
				t.Fatalf("delete session ID = %q", cfg.SessionID)
			}
			return nil
		},
	})
	if err := deleteRunE(&cobra.Command{Use: "delete"}, []string{"session-beta"}); err != nil {
		t.Fatalf("delete RunE() error = %v", err)
	}
}

func TestSessionListBindingPreparesAndMapsExecutionConfig(t *testing.T) {
	listCfg := sessioncli.ListConfig{Context: context.Background(), Scope: "persisted"}
	server := "https://factory.example.test"
	prepared := false
	root := &cobra.Command{Use: "you"}
	root.PersistentFlags().String("server", "", "")
	if err := root.PersistentFlags().Set("server", server); err != nil {
		t.Fatalf("set server flag: %v", err)
	}
	listCommand := &cobra.Command{Use: "list"}
	root.AddCommand(listCommand)
	runE := commandregistry.SessionListRunE(commandregistry.SessionListBinding{
		Config: &listCfg,
		Server: &server,
		Prepare: func(_ context.Context, cfg *sessioncli.ListConfig) error {
			prepared = true
			cfg.Scope = "all"
			return nil
		},
		ListSessions: func(cfg sessioncli.ListConfig) error {
			if cfg.Scope != "all" || cfg.Server != server || cfg.Output == nil {
				t.Fatalf("list config = %#v", cfg)
			}
			return nil
		},
	})
	if err := runE(listCommand, nil); err != nil {
		t.Fatalf("list RunE() error = %v", err)
	}
	if !prepared {
		t.Fatal("list Prepare was not invoked")
	}
}

func TestSessionDispatchAndLifecycleBindingsMapExecutionConfig(t *testing.T) {
	server := "https://factory.example.test"
	jsonOutput := true
	dispatchCfg := sessioncli.DispatchesConfig{Context: context.Background(), Phase: "completed"}
	dispatchRunE := commandregistry.SessionDispatchesRunE(commandregistry.SessionDispatchesBinding{
		Config: &dispatchCfg, Server: &server, JSON: &jsonOutput,
		ListDispatches: func(cfg sessioncli.DispatchesConfig) error {
			if cfg.SessionID != "dur-sess-1" || cfg.Server != server || !cfg.JSON || cfg.Output == nil {
				t.Fatalf("dispatch config = %#v", cfg)
			}
			return nil
		},
	})
	if err := dispatchRunE(&cobra.Command{Use: "dispatches"}, []string{"dur-sess-1"}); err != nil {
		t.Fatalf("dispatches RunE() error = %v", err)
	}

	lifecycleCfg := sessioncli.LifecycleControlConfig{Context: context.Background()}
	lifecycleRunE := commandregistry.SessionLifecycleRunE(commandregistry.SessionLifecycleBinding{
		Config: &lifecycleCfg, Server: &server, JSON: &jsonOutput,
		Control: func(cfg sessioncli.LifecycleControlConfig) error {
			if cfg.SessionID != "session-beta" || cfg.Server != server || !cfg.JSON || cfg.Output == nil {
				t.Fatalf("lifecycle config = %#v", cfg)
			}
			return nil
		},
	})
	if err := lifecycleRunE(&cobra.Command{Use: "pause"}, []string{"session-beta"}); err != nil {
		t.Fatalf("lifecycle RunE() error = %v", err)
	}
}

func TestSessionHandlerBindingsRejectMissingRequirements(t *testing.T) {
	if _, err := commandregistry.NewSessionRegistry(commandregistry.SessionHandlers{}); err == nil || !strings.Contains(err.Error(), "you.session.create") {
		t.Fatalf("NewSessionRegistry() error = %v, want missing stable handler ID", err)
	}
	if err := commandregistry.SessionListRunE(commandregistry.SessionListBinding{})(&cobra.Command{Use: "list"}, nil); err == nil {
		t.Fatal("SessionListRunE() missing config = nil, want error")
	}
	if err := commandregistry.SessionDispatchesRunE(commandregistry.SessionDispatchesBinding{})(&cobra.Command{Use: "dispatches"}, nil); err == nil {
		t.Fatal("SessionDispatchesRunE() missing config = nil, want error")
	}
	lifecycle := commandregistry.SessionLifecycleRunE(commandregistry.SessionLifecycleBinding{})
	if err := lifecycle(&cobra.Command{Use: "pause"}, nil); err == nil {
		t.Fatal("SessionLifecycleRunE() missing config = nil, want error")
	}
	lifecycle = commandregistry.SessionLifecycleRunE(commandregistry.SessionLifecycleBinding{Config: &sessioncli.LifecycleControlConfig{Context: context.Background()}})
	if err := lifecycle(&cobra.Command{Use: "pause"}, nil); err == nil {
		t.Fatal("SessionLifecycleRunE() missing control = nil, want error")
	}
}

func TestSessionShowRunEUsesHandwrittenServicePath(t *testing.T) {
	var gotMethod string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"session-beta","runtime":{"orchestratorKind":"JAVASCRIPT"}}`))
	}))
	defer srv.Close()

	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE: noopRunE,
		SessionShowRunE: commandregistry.SessionShowRunE(commandregistry.SessionShowBinding{
			Server:      stringPtr(srv.URL),
			JSON:        boolPtr(true),
			ShowSession: sessioncli.NewShow(testHTTPProtocol(t)),
		}),
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}

	cmd := &cobra.Command{Use: "show"}
	if err := registry.AttachRunE(cmd, "you.session.show"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"session-beta"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("HTTP method = %q, want GET for getFactorySession binding", gotMethod)
	}
	if gotPath != "/factory-sessions/session-beta" {
		t.Fatalf("HTTP path = %q, want /factory-sessions/session-beta", gotPath)
	}
}

func TestSessionShowRunEWritesDiagnosticsToConfiguredWriter(t *testing.T) {
	var diagnostic bytes.Buffer
	runE := commandregistry.SessionShowRunE(commandregistry.SessionShowBinding{
		Server: stringPtr("http://127.0.0.1:1"),
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return &diagnostic
		},
		ShowSession: func(cfg sessioncli.ShowConfig) error {
			if cfg.Diagnostics != &diagnostic {
				t.Fatalf("diagnostics writer = %T, want *bytes.Buffer", cfg.Diagnostics)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "show"}
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestNewRepresentativeRegistryRejectsMissingHandlers(t *testing.T) {
	if _, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		SessionShowRunE: noopRunE,
	}); err == nil {
		t.Fatal("NewRepresentativeRegistry() missing root handler = nil, want error")
	}
	if _, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE: noopRunE,
	}); err == nil {
		t.Fatal("NewRepresentativeRegistry() missing session show handler = nil, want error")
	}
}

func TestSessionShowRunEMapsVerboseAndDebugBindings(t *testing.T) {
	verbose := true
	debug := true
	runE := commandregistry.SessionShowRunE(commandregistry.SessionShowBinding{
		Verbose: func() bool { return verbose },
		Debug:   &debug,
		ShowSession: func(cfg sessioncli.ShowConfig) error {
			if !cfg.Verbose {
				t.Fatal("expected verbose binding")
			}
			if !cfg.Debug {
				t.Fatal("expected debug binding")
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "show"}
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}
