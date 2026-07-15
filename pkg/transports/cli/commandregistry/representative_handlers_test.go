package commandregistry_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
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
			ShowSession: sessioncli.Show,
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
