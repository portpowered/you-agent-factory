package commandregistry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workservice "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	cligenerated "github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
)

func TestNewWorkRegistryRegistersContractedRunnableIDs(t *testing.T) {
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      noopRunE,
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	for _, commandID := range []string{
		"you.work.list",
		"you.work.show",
		"you.work.move",
		"you.work.visualize",
	} {
		if _, lookupErr := registry.Lookup(commandID); lookupErr != nil {
			t.Fatalf("Lookup(%q) error = %v", commandID, lookupErr)
		}
	}
}

func TestNewWorkRegistryRejectsMissingHandlers(t *testing.T) {
	cases := []struct {
		name     string
		handlers commandregistry.WorkHandlers
	}{
		{
			name: "missing list",
			handlers: commandregistry.WorkHandlers{
				ShowRunE:      noopRunE,
				MoveRunE:      noopRunE,
				VisualizeRunE: noopRunE,
			},
		},
		{
			name: "missing show",
			handlers: commandregistry.WorkHandlers{
				ListRunE:      noopRunE,
				MoveRunE:      noopRunE,
				VisualizeRunE: noopRunE,
			},
		},
		{
			name: "missing move",
			handlers: commandregistry.WorkHandlers{
				ListRunE:      noopRunE,
				ShowRunE:      noopRunE,
				VisualizeRunE: noopRunE,
			},
		},
		{
			name: "missing visualize",
			handlers: commandregistry.WorkHandlers{
				ListRunE: noopRunE,
				ShowRunE: noopRunE,
				MoveRunE: noopRunE,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := commandregistry.NewWorkRegistry(tc.handlers); err == nil {
				t.Fatal("NewWorkRegistry() missing handler = nil, want error")
			}
		})
	}
}

func TestListRunEUsesHandwrittenServicePath(t *testing.T) {
	var gotMethod string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	listCfg := workcli.ListConfig{Context: context.Background()}
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: commandregistry.ListRunE(commandregistry.ListBinding{
			Config:   &listCfg,
			Server:   stringPtr(srv.URL),
			ListWork: workcli.NewList(testHTTPProtocol(t), commandRegistryListPreparation{}),
		}),
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}

	cmd := &cobra.Command{Use: "list"}
	if err := registry.AttachRunE(cmd, "you.work.list"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("HTTP method = %q, want GET for listWorkBySessionId binding", gotMethod)
	}
	if gotPath != "/factory-sessions/~default/work" {
		t.Fatalf("HTTP path = %q, want /factory-sessions/~default/work", gotPath)
	}
}

type commandRegistryListPreparation struct{}

func (commandRegistryListPreparation) PrepareListRequest(
	_ context.Context,
	options workservice.ListOptions,
) (workservice.PreparedListRequest, error) {
	return workservice.PreparedListRequest{Options: options, FilterSummary: "test"}, nil
}

func TestShowRunEUsesHandwrittenServicePath(t *testing.T) {
	var gotMethod string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.Work{
			Name:   "Review PRD",
			WorkId: stringPtr("work-review-1"),
			State: &factoryapi.WorkState{
				Name: "review",
				Type: factoryapi.WorkStateTypePROCESSING,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	showCfg := workcli.ShowConfig{Context: context.Background()}
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: noopRunE,
		ShowRunE: commandregistry.ShowRunE(commandregistry.ShowBinding{
			Config:   &showCfg,
			Server:   stringPtr(srv.URL),
			ShowWork: workcli.NewShow(testHTTPProtocol(t)),
		}),
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}

	cmd := &cobra.Command{Use: "show"}
	if err := registry.AttachRunE(cmd, "you.work.show"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"work-review-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("HTTP method = %q, want GET for getWorkBySessionId binding", gotMethod)
	}
	if gotPath != "/factory-sessions/~default/work/work-review-1" {
		t.Fatalf("HTTP path = %q, want /factory-sessions/~default/work/work-review-1", gotPath)
	}
}

func TestMoveRunEUsesHandwrittenServicePath(t *testing.T) {
	var gotMoveMethod string
	var gotMovePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			writeWorkJSON(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
			})
		case r.Method == http.MethodPost:
			gotMoveMethod = r.Method
			gotMovePath = r.URL.Path
			writeWorkJSON(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	moveCfg := workcli.MoveConfig{Context: context.Background()}
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: noopRunE,
		ShowRunE: noopRunE,
		MoveRunE: commandregistry.MoveRunE(commandregistry.MoveBinding{
			Config:   &moveCfg,
			Server:   stringPtr(srv.URL),
			MoveWork: workcli.NewMove(testHTTPProtocol(t)),
		}),
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}

	cmd := &cobra.Command{Use: "move"}
	if err := registry.AttachRunE(cmd, "you.work.move"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"work-move-1", "complete"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotMoveMethod != http.MethodPost {
		t.Fatalf("HTTP method = %q, want POST for moveWorkBySessionId binding", gotMoveMethod)
	}
	if gotMovePath != "/factory-sessions/~default/work/work-move-1/move" {
		t.Fatalf("HTTP path = %q, want /factory-sessions/~default/work/work-move-1/move", gotMovePath)
	}
}

func TestVisualizeRunERemainsLocalReadOnly(t *testing.T) {
	path := writeWorkHandlerBatchFile(t, `{
  "requestId": "visualize-registry-test",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"},
    {"name": "beta", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "beta", "targetWorkName": "alpha"}
  ]
}`)
	format := "mermaid"

	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: noopRunE,
		ShowRunE: noopRunE,
		MoveRunE: noopRunE,
		VisualizeRunE: commandregistry.VisualizeRunE(commandregistry.VisualizeBinding{
			Format: &format,
			Visualize: func(cfg workcli.VisualizeConfig) error {
				_, err := io.WriteString(cfg.Output, "flowchart TD\n")
				return err
			},
		}),
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}

	cmd := &cobra.Command{Use: "visualize"}
	if err := registry.AttachRunE(cmd, "you.work.visualize"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasPrefix(out.String(), "flowchart TD\n") {
		t.Fatalf("output missing flowchart header:\n%s", out.String())
	}
}

func TestListRunEWritesDiagnosticsToConfiguredWriter(t *testing.T) {
	var diagnostic bytes.Buffer
	listCfg := workcli.ListConfig{Context: context.Background()}
	runE := commandregistry.ListRunE(commandregistry.ListBinding{
		Config: &listCfg,
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return &diagnostic
		},
		ListWork: func(cfg workcli.ListConfig) error {
			if cfg.Diagnostics != &diagnostic {
				t.Fatalf("diagnostics writer = %T, want *bytes.Buffer", cfg.Diagnostics)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "list"}
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func writeWorkJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func writeWorkHandlerBatchFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "batch.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write batch file: %v", err)
	}
	return path
}

func TestRunnableWorkCommandIDsFromGeneratedManifest(t *testing.T) {
	manifest, err := cligenerated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	ids, err := commandregistry.RunnableWorkCommandIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableWorkCommandIDs() error = %v", err)
	}
	want := []string{
		"you.work.list",
		"you.work.move",
		"you.work.show",
		"you.work.visualize",
	}
	if len(ids) != len(want) {
		t.Fatalf("runnable IDs = %#v, want %#v", ids, want)
	}
	for i, commandID := range want {
		if ids[i] != commandID {
			t.Fatalf("runnable IDs[%d] = %q, want %q", i, ids[i], commandID)
		}
	}
}

func TestVerifyWorkRunnableCoverageRejectsMissingHandler(t *testing.T) {
	manifest, err := cligenerated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range []string{
		"you.work.show",
		"you.work.move",
		"you.work.visualize",
	} {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err == nil {
		t.Fatal("VerifyWorkRunnableCoverage() missing list handler = nil, want error")
	}
}

func TestVerifyWorkRunnableCoverageAcceptsCompleteRegistry(t *testing.T) {
	manifest, err := cligenerated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range []string{
		"you.work.list",
		"you.work.show",
		"you.work.move",
		"you.work.visualize",
	} {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err != nil {
		t.Fatalf("VerifyWorkRunnableCoverage() error = %v", err)
	}
}
