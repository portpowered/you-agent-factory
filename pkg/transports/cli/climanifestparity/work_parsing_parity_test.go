package climanifestparity_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/spf13/cobra"
)

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
			name:       "work move local request-id flag is parseable",
			commandID:  move.ID,
			argv:       []string{"work", "move", "work-move-1", "complete", "--request-id", "req-move-1"},
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
