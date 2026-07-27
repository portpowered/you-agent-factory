package commandregistry_test

import (
	"bytes"
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
	want := []string{"you", "you.session.show"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("runnable IDs = %v, want %v", ids, want)
	}
}

func TestVerifyRepresentativeRunnableCoverage(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you", noopRunE); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.VerifyRepresentativeRunnableCoverage(manifest); err == nil {
		t.Fatal("VerifyRepresentativeRunnableCoverage() missing session show = nil, want error")
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
		if _, err := registry.Lookup(commandID); err != nil {
			t.Fatalf("Lookup(%q) error = %v", commandID, err)
		}
	}
}

func TestSessionShowRunEUsesHandwrittenServicePath(t *testing.T) {
	var gotMethod, gotPath string
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
	if gotMethod != http.MethodGet || gotPath != "/factory-sessions/session-beta" {
		t.Fatalf("request = %s %s, want GET /factory-sessions/session-beta", gotMethod, gotPath)
	}
}

func TestSessionShowRunEMapsDiagnosticsVerboseAndDebug(t *testing.T) {
	var diagnostic bytes.Buffer
	debug := true
	runE := commandregistry.SessionShowRunE(commandregistry.SessionShowBinding{
		Verbose: func() bool { return true },
		Debug:   &debug,
		DiagnosticsWriter: func(*cobra.Command) io.Writer {
			return &diagnostic
		},
		ShowSession: func(cfg sessioncli.ShowConfig) error {
			if cfg.Diagnostics != &diagnostic || !cfg.Verbose || !cfg.Debug {
				t.Fatalf("show config = %#v, want diagnostic, verbose, and debug bindings", cfg)
			}
			return nil
		},
	})
	if err := runE(&cobra.Command{Use: "show"}, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestRepresentativeRegistriesRejectMissingHandlers(t *testing.T) {
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

func stringPtr(value string) *string { return &value }

func boolPtr(value bool) *bool { return &value }
