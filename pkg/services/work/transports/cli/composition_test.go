package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/internal/testutil"
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	transportworkcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
)

func TestBindListRequiresListPreparation(t *testing.T) {
	t.Parallel()

	if operation := workcli.BindList(testHTTPProtocol(t), nil); operation != nil {
		t.Fatalf("BindList(nil prepare) = %T, want nil", operation)
	}
}

func TestBindShowRequiresTransport(t *testing.T) {
	t.Parallel()

	if operation := workcli.BindShow(nil); operation != nil {
		t.Fatalf("BindShow(nil) = %T, want nil", operation)
	}
}

func TestBindMoveRequiresTransport(t *testing.T) {
	t.Parallel()

	if operation := workcli.BindMove(nil); operation != nil {
		t.Fatalf("BindMove(nil) = %T, want nil", operation)
	}
}

func TestBindVisualizeRequiresOperation(t *testing.T) {
	t.Parallel()

	if operation := workcli.BindVisualize(nil); operation != nil {
		t.Fatalf("BindVisualize(nil) = %T, want nil", operation)
	}
}

func TestBindServiceDelegatesThroughAdapterService(t *testing.T) {
	t.Parallel()

	operation := func(request workdomain.VisualizationRequest) (string, error) {
		if request.BatchFile != "batch.json" || request.Format != "mermaid" {
			t.Fatalf("request = %#v", request)
		}
		return "graph TD\n", nil
	}
	service := workcli.BindService(workcli.Config{
		ListPrepare: workdomain.NewListRequestPreparation(),
		Visualize:   operation,
	})
	if service == nil {
		t.Fatal("BindService(cfg) = nil, want Work CLI service")
	}

	var out bytes.Buffer
	err := service.Visualize(workcli.VisualizeConfig{
		BatchFile: "batch.json",
		Format:    "mermaid",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Visualize() error = %v", err)
	}
	if out.String() != "graph TD\n" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestBindListDelegatesThroughAdapterService(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:   "Review PRD",
				WorkId: stringPtr("work-1"),
			}},
		})
	}))
	defer srv.Close()

	operation := workcli.BindList(testHTTPProtocol(t), testListRequestPreparation{})
	if operation == nil {
		t.Fatal("BindList(transport, prepare) = nil, want composition operation")
	}

	var out bytes.Buffer
	err := operation(workcli.ListConfig{
		Context: context.Background(),
		Server:  srv.URL,
		Output:  &out,
	})
	if err != nil {
		t.Fatalf("operation(cfg) error = %v", err)
	}
	if !strings.Contains(out.String(), "work-1\tReview PRD") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestBindListMatchesFreeFunctionFacade(t *testing.T) {
	t.Parallel()

	prepare := workdomain.NewListRequestPreparation()
	transport := testHTTPProtocol(t)
	operation := workcli.BindList(transport, prepare)

	cfg := workcli.ListConfig{
		Context: context.Background(),
		Output:  &bytes.Buffer{},
		HTTP:    transport,
	}
	boundErr := operation(cfg)
	directErr := workcli.List(prepare, cfg)
	if (boundErr == nil) != (directErr == nil) {
		t.Fatalf("bound error = %v, direct error = %v", boundErr, directErr)
	}
	if boundErr != nil && boundErr.Error() != directErr.Error() {
		t.Fatalf("bound error = %q, direct error = %q", boundErr.Error(), directErr.Error())
	}
}

func TestBindListMatchesTransportCompositionFacade(t *testing.T) {
	t.Parallel()

	prepare := workdomain.NewListRequestPreparation()
	transport := testHTTPProtocol(t)
	bound := workcli.BindList(transport, prepare)
	facade := transportworkcli.NewList(transport, prepare)
	if bound == nil || facade == nil {
		t.Fatal("BindList and transport facade must return operations")
	}

	cfg := workcli.ListConfig{
		Context: context.Background(),
		Output:  &bytes.Buffer{},
		HTTP:    transport,
	}
	boundErr := bound(cfg)
	facadeErr := facade(cfg)
	if (boundErr == nil) != (facadeErr == nil) {
		t.Fatalf("bound error = %v, facade error = %v", boundErr, facadeErr)
	}
	if boundErr != nil && boundErr.Error() != facadeErr.Error() {
		t.Fatalf("bound error = %q, facade error = %q", boundErr.Error(), facadeErr.Error())
	}
}

func TestBindListRoutesThroughResolvedCompositionPath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:   "Delegated",
				WorkId: stringPtr("work-bound"),
			}},
		})
	}))
	defer srv.Close()

	list := commandregistry.ResolvedListRunE(commandregistry.ResolvedListBinding{
		ListWork: workcli.BindList(testHTTPProtocol(t), testListRequestPreparation{}),
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return cmd.ErrOrStderr()
		},
	})
	noop := func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
		return nil
	}
	root, err := climanifestcobra.NewResolvedWorkCommandTree(commandregistry.ResolvedWorkHandlers{
		List: list, Show: noop, Move: noop, Visualize: noop,
	})
	if err != nil {
		t.Fatalf("NewResolvedWorkCommandTree() error = %v", err)
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetContext(context.Background())
	root.SetArgs([]string{"--server", srv.URL, "work", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "work-bound\tDelegated") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestBindShowRoutesThroughResolvedCompositionPath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.Work{
			Name:   "Show Item",
			WorkId: stringPtr("work-show"),
		})
	}))
	defer srv.Close()

	show := commandregistry.ResolvedShowRunE(commandregistry.ResolvedShowBinding{
		ShowWork: workcli.BindShow(testHTTPProtocol(t)),
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return cmd.ErrOrStderr()
		},
	})
	noop := func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
		return nil
	}
	root, err := climanifestcobra.NewResolvedWorkCommandTree(commandregistry.ResolvedWorkHandlers{
		List: noop, Show: show, Move: noop, Visualize: noop,
	})
	if err != nil {
		t.Fatalf("NewResolvedWorkCommandTree() error = %v", err)
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetContext(context.Background())
	root.SetArgs([]string{"--server", srv.URL, "work", "show", "work-show"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "work-show") || !strings.Contains(out.String(), "Show Item") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestBindVisualizeRoutesThroughResolvedCompositionPath(t *testing.T) {
	t.Parallel()

	visualize := workcli.BindVisualize(func(request workdomain.VisualizationRequest) (string, error) {
		if request.BatchFile != "batch.json" || request.Format != "mermaid" {
			t.Fatalf("request = %#v", request)
		}
		return "flowchart LR\n", nil
	})
	noop := func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
		return nil
	}
	root, err := climanifestcobra.NewResolvedWorkCommandTree(commandregistry.ResolvedWorkHandlers{
		List: noop, Show: noop, Move: noop,
		Visualize: commandregistry.ResolvedVisualizeRunE(commandregistry.ResolvedVisualizeBinding{
			VisualizeWork: visualize,
		}),
	})
	if err != nil {
		t.Fatalf("NewResolvedWorkCommandTree() error = %v", err)
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetContext(context.Background())
	root.SetArgs([]string{"work", "visualize", "batch.json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.String() != "flowchart LR\n" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestBindMoveRoutesThroughResolvedCompositionPath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.Work{
			Name:   "Moved Item",
			WorkId: stringPtr("work-move"),
			State: &factoryapi.WorkState{
				Name: "done",
				Type: factoryapi.WorkStateTypeTERMINAL,
			},
		})
	}))
	defer srv.Close()

	move := commandregistry.ResolvedMoveRunE(commandregistry.ResolvedMoveBinding{
		MoveWork: workcli.BindMove(testHTTPProtocol(t)),
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return cmd.ErrOrStderr()
		},
	})
	noop := func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
		return nil
	}
	root, err := climanifestcobra.NewResolvedWorkCommandTree(commandregistry.ResolvedWorkHandlers{
		List: noop, Show: noop, Move: move, Visualize: noop,
	})
	if err != nil {
		t.Fatalf("NewResolvedWorkCommandTree() error = %v", err)
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetContext(context.Background())
	root.SetArgs([]string{"--server", srv.URL, "work", "move", "work-move", "done"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "done") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestBindShowMatchesFreeFunctionFacade(t *testing.T) {
	t.Parallel()

	transport := testHTTPProtocol(t)
	operation := workcli.BindShow(transport)
	cfg := workcli.ShowConfig{
		Context: context.Background(),
		Output:  &bytes.Buffer{},
		HTTP:    transport,
		WorkID:  "work-1",
	}
	boundErr := operation(cfg)
	directErr := workcli.Show(cfg)
	if (boundErr == nil) != (directErr == nil) {
		t.Fatalf("bound error = %v, direct error = %v", boundErr, directErr)
	}
	if boundErr != nil && !errors.Is(boundErr, directErr) &&
		boundErr.Error() != directErr.Error() {
		t.Fatalf("bound error = %q, direct error = %q", boundErr.Error(), directErr.Error())
	}
}

const (
	cliAdapterPackagePath = "pkg/services/work/transports/cli"
	cliAdapterImportPath  = "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
	workOwner             = "work"
)

type coverageMinimumManifest struct {
	Lane     string `json:"lane"`
	Packages []struct {
		Package string  `json:"package"`
		Minimum float64 `json:"minimum"`
	} `json:"packages"`
}

func TestManifestRegistration_CLIAdapterPackageIsRegistered(t *testing.T) {
	t.Helper()

	assertPackageTargetManifestRegistration(t)
	assertOwnershipInventoryRegistration(t)
	assertCoverageMinimumRegistration(t, "unit", "docs/internal/baselines/go-unit-coverage-package-minimums.json")
	assertCoverageMinimumRegistration(t, "functional", "docs/internal/baselines/go-functional-coverage-package-minimums.json")
}

func assertPackageTargetManifestRegistration(t *testing.T) {
	t.Helper()

	data, err := os.ReadFile(testutil.MustRepoPath(t, "docs/internal/packaged-service-structure/package-target-manifest.json"))
	if err != nil {
		t.Fatalf("read package-target manifest: %v", err)
	}

	var manifest struct {
		Inventory []string `json:"inventory"`
		Packages  []struct {
			PackagePath string `json:"packagePath"`
			Disposition string `json:"disposition"`
			Destination string `json:"destination"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode package-target manifest: %v", err)
	}

	foundInventory := false
	for _, packagePath := range manifest.Inventory {
		if packagePath == cliAdapterPackagePath {
			foundInventory = true
			break
		}
	}
	if !foundInventory {
		t.Fatalf("package-target manifest inventory missing %q", cliAdapterPackagePath)
	}

	for _, row := range manifest.Packages {
		if row.PackagePath != cliAdapterPackagePath {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionRetain {
			t.Fatalf("package-target manifest disposition = %q, want %q", row.Disposition, ownershipinventory.DispositionRetain)
		}
		if row.Destination != workOwner {
			t.Fatalf("package-target manifest destination = %q, want %q", row.Destination, workOwner)
		}
		return
	}
	t.Fatalf("package-target manifest packages missing %q", cliAdapterPackagePath)
}

func assertOwnershipInventoryRegistration(t *testing.T) {
	t.Helper()

	data, err := os.ReadFile(testutil.MustRepoPath(t, ownershipinventory.InventoryRelativePath))
	if err != nil {
		t.Fatalf("read ownership inventory: %v", err)
	}

	var inventory struct {
		Packages []ownershipinventory.PackageRow `json:"packages"`
	}
	if err := json.Unmarshal(data, &inventory); err != nil {
		t.Fatalf("decode ownership inventory: %v", err)
	}

	for _, row := range inventory.Packages {
		if row.PackagePath != cliAdapterPackagePath {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionRetain {
			t.Fatalf("ownership inventory disposition = %q, want %q", row.Disposition, ownershipinventory.DispositionRetain)
		}
		if row.Destination != workOwner {
			t.Fatalf("ownership inventory destination = %q, want %q", row.Destination, workOwner)
		}
		if row.DestinationKind != ownershipinventory.DestinationKindOwner {
			t.Fatalf(
				"ownership inventory destinationKind = %q, want %q",
				row.DestinationKind,
				ownershipinventory.DestinationKindOwner,
			)
		}
		return
	}
	t.Fatalf("ownership inventory packages missing %q", cliAdapterPackagePath)
}

func assertCoverageMinimumRegistration(t *testing.T, lane string, relativePath string) {
	t.Helper()

	data, err := os.ReadFile(testutil.MustRepoPath(t, relativePath))
	if err != nil {
		t.Fatalf("read %s coverage manifest: %v", lane, err)
	}

	var manifest coverageMinimumManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode %s coverage manifest: %v", lane, err)
	}
	if manifest.Lane != lane {
		t.Fatalf("%s coverage manifest lane = %q, want %q", relativePath, manifest.Lane, lane)
	}

	for _, entry := range manifest.Packages {
		if entry.Package != cliAdapterImportPath {
			continue
		}
		if entry.Minimum < 0 {
			t.Fatalf("%s coverage minimum for %q must be non-negative", lane, cliAdapterImportPath)
		}
		return
	}
	t.Fatalf("%s coverage manifest missing %q", lane, cliAdapterImportPath)
}
