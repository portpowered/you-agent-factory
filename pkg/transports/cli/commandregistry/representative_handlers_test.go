package commandregistry_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	"github.com/spf13/cobra"
)

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

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}
