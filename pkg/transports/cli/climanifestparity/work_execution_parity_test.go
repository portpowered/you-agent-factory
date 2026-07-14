package climanifestparity_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
)

func TestProductionManifestCompletionParity_WorkFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	liveRoot := cli.NewRootCommand()
	inventory, err := cliinputs.Walk(liveRoot)
	if err != nil {
		t.Fatalf("cliinputs.Walk() error = %v", err)
	}

	for _, commandID := range []string{
		"you.work.list",
		"you.work.show",
		"you.work.move",
		"you.work.visualize",
	} {
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			t.Fatalf("CommandByID(%s) error = %v", commandID, err)
		}
		t.Run(commandID, func(t *testing.T) {
			liveArgs, liveFlags := climanifestparity.InputsForCommandPath(inventory, record.Path)
			mismatches := climanifestparity.CompareCompletionParity(record, liveArgs, liveFlags)
			if len(mismatches) == 0 {
				return
			}
			t.Fatalf("contract vs live completion drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
		})
	}
}

func TestProductionManifestHandlerBinding_WorkFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}
	doc := loadBundledOpenAPIContract(t)

	cases := []struct {
		commandID   string
		handlerID   string
		operationID string
		method      string
		path        string
	}{
		{
			commandID:   "you.work.list",
			handlerID:   climanifestparity.WorkListHandlerID,
			operationID: climanifestparity.WorkListOperationID,
			method:      climanifestparity.WorkListHTTPMethod,
			path:        climanifestparity.WorkListHTTPPath,
		},
		{
			commandID:   "you.work.show",
			handlerID:   climanifestparity.WorkShowHandlerID,
			operationID: climanifestparity.WorkShowOperationID,
			method:      climanifestparity.WorkShowHTTPMethod,
			path:        climanifestparity.WorkShowHTTPPath,
		},
		{
			commandID:   "you.work.move",
			handlerID:   climanifestparity.WorkMoveHandlerID,
			operationID: climanifestparity.WorkMoveOperationID,
			method:      climanifestparity.WorkMoveHTTPMethod,
			path:        climanifestparity.WorkMoveHTTPPath,
		},
	}

	for _, tc := range cases {
		t.Run(tc.commandID, func(t *testing.T) {
			record, err := manifest.CommandByID(tc.commandID)
			if err != nil {
				t.Fatalf("CommandByID(%s) error = %v", tc.commandID, err)
			}
			mismatches := climanifestparity.CompareDeclaredHandler(record, tc.handlerID, tc.operationID)
			mismatches = append(mismatches, climanifestparity.CompareHandlerOpenAPIBinding(record, doc, tc.method, tc.path)...)
			if len(mismatches) > 0 {
				t.Fatalf("contract handler/OpenAPI binding drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
			}
		})
	}
}

func TestProductionManifestHandlerBinding_WorkFamilyLiveServiceCalls(t *testing.T) {
	t.Run("you.work.list", func(t *testing.T) {
		var gotMethod, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[]}`))
		}))
		defer srv.Close()

		var out bytes.Buffer
		if err := workcli.List(workcli.ListConfig{
			Server: srv.URL,
			JSON:   true,
			Output: &out,
		}); err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if gotMethod != climanifestparity.WorkListHTTPMethod {
			t.Fatalf("HTTP method = %q, want %s", gotMethod, climanifestparity.WorkListHTTPMethod)
		}
		if gotPath != "/factory-sessions/~default/work" {
			t.Fatalf("HTTP path = %q, want /factory-sessions/~default/work", gotPath)
		}
	})

	t.Run("you.work.show", func(t *testing.T) {
		var gotMethod, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(factoryapi.Work{WorkId: workParityStringPtr("work-review-1")})
		}))
		defer srv.Close()

		var out bytes.Buffer
		if err := workcli.Show(workcli.ShowConfig{
			Server: srv.URL,
			WorkID: "work-review-1",
			JSON:   true,
			Output: &out,
		}); err != nil {
			t.Fatalf("Show() error = %v", err)
		}
		if gotMethod != climanifestparity.WorkShowHTTPMethod {
			t.Fatalf("HTTP method = %q, want %s", gotMethod, climanifestparity.WorkShowHTTPMethod)
		}
		if gotPath != "/factory-sessions/~default/work/work-review-1" {
			t.Fatalf("HTTP path = %q, want /factory-sessions/~default/work/work-review-1", gotPath)
		}
	})

	t.Run("you.work.move", func(t *testing.T) {
		var gotMethod, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(factoryapi.Work{
					WorkId: workParityStringPtr("work-move-1"),
					State:  &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				})
			case http.MethodPost:
				gotMethod = r.Method
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(factoryapi.Work{
					WorkId: workParityStringPtr("work-move-1"),
					State:  &factoryapi.WorkState{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
				})
			}
		}))
		defer srv.Close()

		var out bytes.Buffer
		if err := workcli.Move(workcli.MoveConfig{
			Server:    srv.URL,
			WorkID:    "work-move-1",
			StateName: "complete",
			JSON:      true,
			Output:    &out,
		}); err != nil {
			t.Fatalf("Move() error = %v", err)
		}
		if gotMethod != climanifestparity.WorkMoveHTTPMethod {
			t.Fatalf("HTTP method = %q, want %s", gotMethod, climanifestparity.WorkMoveHTTPMethod)
		}
		if gotPath != "/factory-sessions/~default/work/work-move-1/move" {
			t.Fatalf("HTTP path = %q, want /factory-sessions/~default/work/work-move-1/move", gotPath)
		}
	})
}

func TestProductionManifestOutputModeParity_WorkFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	for _, commandID := range []string{"you.work.list", "you.work.show", "you.work.move"} {
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			t.Fatalf("CommandByID(%s) error = %v", commandID, err)
		}
		if mismatches := climanifestparity.CompareDeclaredOutputs(record); len(mismatches) > 0 {
			t.Fatalf("%s contract output declarations drift detected:\n%s", commandID, climanifestparity.FormatMismatchReport(mismatches))
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/work"):
			_ = json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{Results: []factoryapi.Work{{
				Name:   "Review PRD",
				WorkId: workParityStringPtr("work-1"),
				State:  &factoryapi.WorkState{Name: "review", Type: factoryapi.WorkStateTypePROCESSING},
			}}})
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(factoryapi.Work{
				Name:   "Review PRD",
				WorkId: workParityStringPtr("work-review-1"),
				State:  &factoryapi.WorkState{Name: "review", Type: factoryapi.WorkStateTypePROCESSING},
			})
		case r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(factoryapi.Work{
				WorkId: workParityStringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
			})
		}
	}))
	defer srv.Close()

	for _, tc := range []struct {
		name string
		argv []string
	}{
		{name: "list human", argv: []string{"--server", srv.URL, "work", "list"}},
		{name: "list json", argv: []string{"--server", srv.URL, "--json", "work", "list"}},
		{name: "show human", argv: []string{"--server", srv.URL, "work", "show", "work-review-1"}},
		{name: "show json", argv: []string{"--server", srv.URL, "--json", "work", "show", "work-review-1"}},
		{name: "move human", argv: []string{"--server", srv.URL, "work", "move", "work-move-1", "complete"}},
		{name: "move json", argv: []string{"--server", srv.URL, "--json", "work", "move", "work-move-1", "complete"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacyRoot, generatedRoot, err := cli.NewWorkFamilyParityRootsWithProductionHandlers()
			if err != nil {
				t.Fatalf("NewWorkFamilyParityRootsWithProductionHandlers() error = %v", err)
			}
			legacyOut, legacyErr := executeParityRoot(t, legacyRoot, tc.argv)
			generatedOut, generatedErr := executeParityRoot(t, generatedRoot, tc.argv)
			if (legacyErr == nil) != (generatedErr == nil) {
				t.Fatalf("execute error parity: legacy=%v generated=%v", legacyErr, generatedErr)
			}
			if legacyOut != generatedOut {
				t.Fatalf("stdout mismatch for %v\n--- legacy ---\n%s--- generated ---\n%s", tc.argv, legacyOut, generatedOut)
			}
		})
	}
}

func TestProductionManifestNetworkSideEffectParity_WorkFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	for _, commandID := range []string{"you.work.list", "you.work.show", "you.work.move"} {
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			t.Fatalf("CommandByID(%s) error = %v", commandID, err)
		}
		if mismatches := climanifestparity.CompareBaselineSideEffects(record, []string{"network"}); len(mismatches) > 0 {
			t.Fatalf("%s contract side-effect declarations drift:\n%s", commandID, climanifestparity.FormatMismatchReport(mismatches))
		}
	}

	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := workcli.List(workcli.ListConfig{
		Server: srv.URL,
		JSON:   true,
		Output: &out,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if requestCount == 0 {
		t.Fatal("expected contracted network side effect to perform at least one HTTP request")
	}
}

func TestProductionManifestVisualizeOutputParity_WorkFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}
	record, err := manifest.CommandByID("you.work.visualize")
	if err != nil {
		t.Fatalf("CommandByID(you.work.visualize) error = %v", err)
	}
	if mismatches := climanifestparity.CompareBaselineSideEffects(record, []string{"filesystem"}); len(mismatches) > 0 {
		t.Fatalf("contract side-effect declarations drift:\n%s", climanifestparity.FormatMismatchReport(mismatches))
	}

	path := writeWorkParityBatchFile(t)

	for _, tc := range []struct {
		name string
		argv []string
	}{
		{name: "mermaid default", argv: []string{"work", "visualize", path}},
		{name: "markdown-mermaid", argv: []string{"work", "visualize", "--format", "markdown-mermaid", path}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacyRoot, generatedRoot, err := cli.NewWorkFamilyParityRootsWithProductionHandlers()
			if err != nil {
				t.Fatalf("NewWorkFamilyParityRootsWithProductionHandlers() error = %v", err)
			}
			legacyOut, legacyErr := executeParityRoot(t, legacyRoot, tc.argv)
			generatedOut, generatedErr := executeParityRoot(t, generatedRoot, tc.argv)
			if (legacyErr == nil) != (generatedErr == nil) {
				t.Fatalf("execute error parity: legacy=%v generated=%v", legacyErr, generatedErr)
			}
			if legacyOut != generatedOut {
				t.Fatalf("stdout mismatch for %v\n--- legacy ---\n%s--- generated ---\n%s", tc.argv, legacyOut, generatedOut)
			}
		})
	}

	t.Run("unsupported format rejection", func(t *testing.T) {
		legacyRoot, generatedRoot, err := cli.NewWorkFamilyParityRootsWithProductionHandlers()
		if err != nil {
			t.Fatalf("NewWorkFamilyParityRootsWithProductionHandlers() error = %v", err)
		}
		legacyErr := executeParityRootExpectError(t, legacyRoot, []string{"work", "visualize", "--format", "svg", path})
		generatedErr := executeParityRootExpectError(t, generatedRoot, []string{"work", "visualize", "--format", "svg", path})
		if legacyErr == nil || generatedErr == nil {
			t.Fatalf("want unsupported format rejection: legacy=%v generated=%v", legacyErr, generatedErr)
		}
		if !strings.Contains(legacyErr.Error(), `unsupported format "svg"`) {
			t.Fatalf("legacy error = %q", legacyErr.Error())
		}
		if legacyErr.Error() != generatedErr.Error() {
			t.Fatalf("format rejection mismatch:\nlegacy=%q\ngenerated=%q", legacyErr.Error(), generatedErr.Error())
		}
	})
}

func executeParityRoot(t *testing.T, root *cobra.Command, argv []string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(argv)
	err := root.Execute()
	return out.String(), err
}

func executeParityRootExpectError(t *testing.T, root *cobra.Command, argv []string) error {
	t.Helper()
	_, err := executeParityRoot(t, root, argv)
	return err
}

func writeWorkParityBatchFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "batch.json")
	contents := `{
  "requestId": "work-parity-visualize",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"},
    {"name": "beta", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "beta", "targetWorkName": "alpha"}
  ]
}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write batch file: %v", err)
	}
	return path
}

func workParityStringPtr(value string) *string {
	return &value
}
