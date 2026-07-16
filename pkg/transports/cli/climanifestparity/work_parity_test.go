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

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
)

func TestLegacyWorkFamilyParityExportsBuildContractedTree(t *testing.T) {
	legacyWork := cli.NewLegacyWorkFamilyCommand()
	if legacyWork.Name() != "work" {
		t.Fatalf("legacy work name = %q, want work", legacyWork.Name())
	}
	for _, path := range []string{"work list", "work show", "work move", "work visualize"} {
		if _, err := climanifestparity.FindCommandByPath(legacyWork, path); err != nil {
			t.Fatalf("FindCommandByPath(%q) error = %v", path, err)
		}
	}

	legacyRoot := cli.NewLegacyWorkFamilyRootForParity()
	if legacyRoot.Name() != "you" {
		t.Fatalf("legacy root name = %q, want you", legacyRoot.Name())
	}
	if _, err := climanifestparity.FindCommandByPath(legacyRoot, "you work list"); err != nil {
		t.Fatalf("legacy root missing work list: %v", err)
	}

	if _, _, err := cli.NewWorkFamilyParityRoots(nil, climanifestcobra.WorkFamilyBindings{}); err == nil {
		t.Fatal("NewWorkFamilyParityRoots(nil registry) = nil, want error")
	}
}

func TestProductionManifestParsingParity_WorkFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		t.Fatalf("CommandByID(you) error = %v", err)
	}
	workRecords := mustWorkFamilyRecords(t, manifest)

	cases := productionManifestWorkParsingParityCases(rootRecord, workRecords)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			record, err := manifest.CommandByID(tc.commandID)
			if err != nil {
				t.Fatalf("CommandByID(%q) error = %v", tc.commandID, err)
			}

			leaf, positionals, parseErr := cli.ParseArgvForCLIInputsInventory(tc.argv)
			if tc.wantParseErr {
				if parseErr == nil {
					t.Fatalf("ParseArgvForCLIInputsInventory(%v) error = nil, want parse failure", tc.argv)
				}
				if tc.errContains != "" && !strings.Contains(parseErr.Error(), tc.errContains) {
					t.Fatalf("parse error = %q, want substring %q", parseErr.Error(), tc.errContains)
				}
				if tc.verify != nil {
					tc.verify(t, manifest, record, leaf, positionals)
				}
				return
			}
			if parseErr != nil {
				t.Fatalf("ParseArgvForCLIInputsInventory(%v) error = %v", tc.argv, parseErr)
			}
			if mismatch := climanifestparity.AssertLeafCommandPath(record.ID, record.Path, leaf); mismatch != nil {
				t.Fatal(mismatch.Error())
			}
			if tc.verify != nil {
				tc.verify(t, manifest, record, leaf, positionals)
			}
		})
	}
}

type workFamilyRecords struct {
	list      climanifest.Command
	show      climanifest.Command
	move      climanifest.Command
	visualize climanifest.Command
}

func mustWorkFamilyRecords(t *testing.T, manifest climanifest.Manifest) workFamilyRecords {
	t.Helper()
	list, err := manifest.CommandByID("you.work.list")
	if err != nil {
		t.Fatalf("CommandByID(you.work.list) error = %v", err)
	}
	show, err := manifest.CommandByID("you.work.show")
	if err != nil {
		t.Fatalf("CommandByID(you.work.show) error = %v", err)
	}
	move, err := manifest.CommandByID("you.work.move")
	if err != nil {
		t.Fatalf("CommandByID(you.work.move) error = %v", err)
	}
	visualize, err := manifest.CommandByID("you.work.visualize")
	if err != nil {
		t.Fatalf("CommandByID(you.work.visualize) error = %v", err)
	}
	return workFamilyRecords{list: list, show: show, move: move, visualize: visualize}
}

func productionManifestWorkParsingParityCases(rootRecord climanifest.Command, work workFamilyRecords) []parsingParityCase {
	cases := make([]parsingParityCase, 0)
	cases = append(cases, workListParsingCases(work.list)...)
	cases = append(cases, workShowParsingCases(work.show)...)
	cases = append(cases, workMoveParsingCases(work.move)...)
	cases = append(cases, workVisualizeParsingCases(work.visualize)...)
	cases = append(cases, workListInheritedDefaultsCase(rootRecord, work.list)...)
	return cases
}

func workListParsingCases(list climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "work list accepts filter and pagination flags",
			commandID: list.ID,
			argv:      []string{"work", "list", "--session", "session-beta", "--max-results", "10", "--name", "prd"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				for _, flagName := range []string{"session", "max-results", "name"} {
					contract, err := record.RequireFlagByLong(flagName)
					if err != nil {
						t.Fatal(err)
					}
					if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, flagName), true, contract.Default); mismatch != nil {
						if flagName == "session" && mismatch != nil {
							if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, flagName), true, "session-beta"); mismatch != nil {
								t.Fatal(mismatch.Error())
							}
							continue
						}
						if flagName == "name" {
							if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, flagName), true, "prd"); mismatch != nil {
								t.Fatal(mismatch.Error())
							}
							continue
						}
						if flagName == "max-results" {
							if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, flagName), true, "10"); mismatch != nil {
								t.Fatal(mismatch.Error())
							}
							continue
						}
						t.Fatal(mismatch.Error())
					}
				}
			},
		},
		{
			name:      "work list inherited json flag is parseable",
			commandID: list.ID,
			argv:      []string{"--json", "work", "list"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("json")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "json"), true, "true"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
	}
}

func workShowParsingCases(show climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "work show required positional accepts work id",
			commandID: show.ID,
			argv:      []string{"work", "show", "work-review-1"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg, err := record.RequireArgumentAt(0)
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareArgumentCardinality(record.ID, arg, positionals); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
				if len(positionals) != 1 || positionals[0] != "work-review-1" {
					t.Fatalf("positionals = %v, want [work-review-1]", positionals)
				}
			},
		},
		{
			name:         "work show rejects missing required positional",
			commandID:    show.ID,
			argv:         []string{"work", "show"},
			wantParseErr: true,
			errContains:  "accepts 1 arg",
		},
	}
}

func workMoveParsingCases(move climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "work move required positionals accept work id and state",
			commandID: move.ID,
			argv:      []string{"work", "move", "work-move-1", "complete"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, _ *cobra.Command, positionals []string) {
				t.Helper()
				workID, err := record.RequireArgumentAt(0)
				if err != nil {
					t.Fatal(err)
				}
				stateName, err := record.RequireArgumentAt(1)
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareArgumentCardinality(record.ID, workID, positionals[:1]); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
				if mismatch := climanifestparity.CompareArgumentCardinality(record.ID, stateName, positionals[1:]); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:      "work move local request-id flag is parseable",
			commandID: move.ID,
			argv:      []string{"work", "move", "work-move-1", "complete", "--request-id", "req-move-1"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("request-id")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "request-id"), true, "req-move-1"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
	}
}

func workVisualizeParsingCases(visualize climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "work visualize required batch path accepts one value",
			commandID: visualize.ID,
			argv:      []string{"work", "visualize", "batch.json"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg, err := record.RequireArgumentAt(0)
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareArgumentCardinality(record.ID, arg, positionals); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:         "work visualize rejects missing batch path",
			commandID:    visualize.ID,
			argv:         []string{"work", "visualize"},
			wantParseErr: true,
			errContains:  "accepts 1 arg",
		},
		{
			name:      "work visualize format keeps contract default",
			commandID: visualize.ID,
			argv:      []string{"work", "visualize", "batch.json"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("format")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagDefault(record.ID, contract, climanifestparity.LiveFlag(leaf, "format")); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
	}
}

func workListInheritedDefaultsCase(rootRecord, list climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "work list inherited flags match root persistent defaults",
			commandID: list.ID,
			argv:      []string{"work", "list"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				mismatches := climanifestparity.CompareInheritedFlagDefaultsAgainstRoot(rootRecord, record, leaf)
				if len(mismatches) == 0 {
					return
				}
				t.Fatalf("contract vs live inherited flag default drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
			},
		},
	}
}

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

func TestGeneratedVsLegacyParityMatrix_WorkFamily(t *testing.T) {
	workFamilyCases := []struct {
		commandID string
		path      string
	}{
		{commandID: "you.work", path: "you work"},
		{commandID: "you.work.list", path: "you work list"},
		{commandID: "you.work.show", path: "you work show"},
		{commandID: "you.work.move", path: "you work move"},
		{commandID: "you.work.visualize", path: "you work visualize"},
	}

	t.Run("identity", func(t *testing.T) {
		legacyRoot, generatedRoot := mustWorkConstructorRoots(t)
		for _, tc := range workFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyCmd, err := climanifestparity.FindCommandByPath(legacyRoot, tc.path)
				if err != nil {
					t.Fatalf("legacy FindCommandByPath(%q) error = %v", tc.path, err)
				}
				generatedCmd, err := climanifestparity.FindCommandByPath(generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("generated FindCommandByPath(%q) error = %v", tc.path, err)
				}
				mismatches := climanifestparity.CompareConstructorIdentityParity(tc.commandID, legacyCmd, generatedCmd)
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("help", func(t *testing.T) {
		for _, tc := range workFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustWorkConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorHelpParity(tc.commandID, legacyRoot, generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("CompareConstructorHelpParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("completion", func(t *testing.T) {
		for _, tc := range workFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustWorkConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorCompletionInventoryParity(tc.commandID, tc.path, legacyRoot, generatedRoot)
				if err != nil {
					t.Fatalf("CompareConstructorCompletionInventoryParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("parsing", func(t *testing.T) {
		for _, tc := range workGeneratedVsLegacyParsingCases() {
			t.Run(tc.name, func(t *testing.T) {
				legacyRoot, generatedRoot := mustWorkConstructorRoots(t)
				mismatches := climanifestparity.CompareConstructorParseParity(
					tc.commandID,
					legacyRoot,
					generatedRoot,
					tc.argv,
					tc.wantParseErr,
					tc.errContains,
				)
				assertNoConstructorMismatches(t, mismatches)

				if tc.wantParseErr {
					return
				}

				legacyLeaf, _, legacyErr := climanifestparity.ParseArgvOnRoot(legacyRoot, tc.argv)
				if legacyErr != nil {
					t.Fatalf("legacy ParseArgvOnRoot(%v) error = %v", tc.argv, legacyErr)
				}
				generatedLeaf, _, generatedErr := climanifestparity.ParseArgvOnRoot(generatedRoot, tc.argv)
				if generatedErr != nil {
					t.Fatalf("generated ParseArgvOnRoot(%v) error = %v", tc.argv, generatedErr)
				}
				for _, flagLong := range tc.flagChecks {
					mismatches := climanifestparity.CompareConstructorFlagParity(tc.commandID, flagLong, legacyLeaf, generatedLeaf)
					assertNoConstructorMismatches(t, mismatches)
				}
				if tc.checkPreRun {
					mismatches := climanifestparity.CompareConstructorPreRunParity(tc.commandID, legacyLeaf, generatedLeaf, tc.errContains)
					assertNoConstructorMismatches(t, mismatches)
				}
			})
		}
	})
}

type workGeneratedVsLegacyParsingCase struct {
	name         string
	commandID    string
	argv         []string
	wantParseErr bool
	errContains  string
	flagChecks   []string
	checkPreRun  bool
}

func workGeneratedVsLegacyParsingCases() []workGeneratedVsLegacyParsingCase {
	return []workGeneratedVsLegacyParsingCase{
		{
			name:      "work list accepts filter and pagination flags",
			commandID: "you.work.list",
			argv:      []string{"work", "list", "--session", "session-beta", "--max-results", "10", "--name", "prd"},
			flagChecks: []string{
				"session", "max-results", "name",
			},
		},
		{
			name:       "work list inherited json flag is parseable",
			commandID:  "you.work.list",
			argv:       []string{"--json", "work", "list"},
			flagChecks: []string{"json"},
		},
		{
			name:       "work list inherited server flag accepts explicit value",
			commandID:  "you.work.list",
			argv:       []string{"--server", "http://127.0.0.1:9090", "work", "list"},
			flagChecks: []string{"server"},
		},
		{
			name:        "work list rejects deprecated port flag",
			commandID:   "you.work.list",
			argv:        []string{"--port", "9090", "work", "list"},
			errContains: "--port is no longer supported",
			checkPreRun: true,
		},
		{
			name:      "work show required positional accepts work id",
			commandID: "you.work.show",
			argv:      []string{"work", "show", "work-review-1"},
		},
		{
			name:         "work show rejects missing required positional",
			commandID:    "you.work.show",
			argv:         []string{"work", "show"},
			wantParseErr: true,
			errContains:  "accepts 1 arg",
		},
		{
			name:       "work show inherited json flag is parseable",
			commandID:  "you.work.show",
			argv:       []string{"--json", "work", "show", "work-review-1"},
			flagChecks: []string{"json"},
		},
		{
			name:      "work move required positionals accept work id and state",
			commandID: "you.work.move",
			argv:      []string{"work", "move", "work-move-1", "complete"},
		},
		{
			name:         "work move rejects missing state positional",
			commandID:    "you.work.move",
			argv:         []string{"work", "move", "work-move-1"},
			wantParseErr: true,
			errContains:  "accepts 2 arg",
		},
		{
			name:       "work move local request-id flag is parseable",
			commandID:  "you.work.move",
			argv:       []string{"work", "move", "work-move-1", "complete", "--request-id", "req-move-1"},
			flagChecks: []string{"request-id"},
		},
		{
			name:      "work visualize required batch path accepts one value",
			commandID: "you.work.visualize",
			argv:      []string{"work", "visualize", "batch.json"},
		},
		{
			name:         "work visualize rejects missing batch path",
			commandID:    "you.work.visualize",
			argv:         []string{"work", "visualize"},
			wantParseErr: true,
			errContains:  "accepts 1 arg",
		},
		{
			name:       "work visualize format keeps contract default",
			commandID:  "you.work.visualize",
			argv:       []string{"work", "visualize", "batch.json"},
			flagChecks: []string{"format"},
		},
		{
			name:       "work visualize format accepts markdown-mermaid",
			commandID:  "you.work.visualize",
			argv:       []string{"work", "visualize", "--format", "markdown-mermaid", "batch.json"},
			flagChecks: []string{"format"},
		},
	}
}

func mustWorkConstructorRoots(t *testing.T) (*cobra.Command, *cobra.Command) {
	t.Helper()
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      parityNoopRunE,
		ShowRunE:      parityNoopRunE,
		MoveRunE:      parityNoopRunE,
		VisualizeRunE: parityNoopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	legacyRoot, generatedRoot, err := cli.NewWorkFamilyParityRoots(registry, workParityBindings())
	if err != nil {
		t.Fatalf("NewWorkFamilyParityRoots() error = %v", err)
	}
	return legacyRoot, generatedRoot
}

func workParityBindings() climanifestcobra.WorkFamilyBindings {
	listCfg := workcli.ListConfig{}
	showCfg := workcli.ShowConfig{}
	moveCfg := workcli.MoveConfig{}
	format := "mermaid"
	return climanifestcobra.WorkFamilyBindings{
		ListConfig:      &listCfg,
		ShowConfig:      &showCfg,
		MoveConfig:      &moveCfg,
		VisualizeFormat: &format,
		FlagUsages:      workFamilyParityFlagUsages(),
	}
}

func workFamilyParityFlagUsages() map[string]string {
	// Mirror handwritten work flag help from root_work.go for generated-vs-legacy help parity.
	return map[string]string{
		"state-name":     "filter by current state name",
		"state-type":     "filter by current state type (INITIAL, PROCESSING, TERMINAL, FAILED)",
		"name":           "filter by case-insensitive substring of work name (applied before pagination)",
		"work-type-name": "filter by exact workTypeName (applied before pagination)",
		"trace-id":       "filter by exact traceId or currentChainingTraceId (applied before pagination)",
		"sort-by":        "sort returned work by field (state.type)",
		"max-results":    "maximum work items to return per page after server-side filters",
		"next-token":     "pagination cursor returned by a previous work list response",
		"session":        "target one live factory session; omit to use the default compatibility session",
		"request-id":     "optional client idempotency key for operator moves",
		"format":         "output format: mermaid or markdown-mermaid",
	}
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
