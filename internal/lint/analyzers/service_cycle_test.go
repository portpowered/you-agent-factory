package analyzers

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func cycleInput(ceiling string, sources map[string]string) serviceCycleInput {
	input := serviceCycleInput{Head: ceiling, Sources: sources}
	for file := range sources {
		unit := serviceCyclePackage{ImportPath: modulePrefix + pathDirectory(file), GoFiles: []string{filepath.Base(file)}}
		data, _ := json.Marshal(unit)
		input.Metadata += string(data) + "\n"
	}
	return input
}

const cycleOne = "service-cycle-weight|internal/lint/analyzers|1\n"
const cycleZero = "service-cycle-weight|internal/lint/analyzers|0\n"

func cycleSources() map[string]string {
	return map[string]string{
		"pkg/services/a/one/source.go": "package one\nimport _ \"" + modulePrefix + "pkg/services/b/one\"",
		"pkg/services/b/two/source.go": "package two\nimport _ \"" + modulePrefix + "pkg/services/a/two\"",
	}
}

func TestServiceCycleAnalysistest(t *testing.T) {
	useFixtures(t, "")
	analyzer := serviceCycleSnapshot(cycleInput(cycleZero, cycleSources()), nil)
	analysistest.Run(t, analysistest.TestData(), analyzer, "m/internal/lint/analyzers")
}

func TestServiceCycleEqualityAndDrift(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ head, want string }{
		{cycleOne, ""}, {cycleZero, "regression: measured 1, ceiling 0, drift +1"},
		{strings.Replace(cycleOne, "|1", "|2", 1), "uncaptured improvement; lower the ceiling: measured 1, ceiling 2, drift -1"},
		{"", "missing service-cycle-weight"},
	} {
		graph, err := serviceCycleGraph(cycleInput(tc.head, cycleSources()))
		if err != nil {
			t.Fatal(err)
		}
		err = serviceCycleDiagnostic(graph, tc.head)
		if tc.want == "" {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("got %v, want %s", err, tc.want)
		}
		if tc.want != "" && strings.Contains(tc.want, "drift") && !strings.Contains(err.Error(), "carrier packages: pkg/services/b/two") {
			t.Fatal(err)
		}
	}
}

func TestServiceCycleGlobalWeightAndFiltering(t *testing.T) {
	t.Parallel()
	sources := cycleSources()
	sources["pkg/services/b/two/generated.go"] = "// Code generated. DO NOT EDIT.\n" + sources["pkg/services/b/two/source.go"]
	sources["pkg/services/b/three/inactive_windows.go"] = "//go:build never\n\n" + sources["pkg/services/b/two/source.go"]
	sources["pkg/services/a/one/repeated.go"] = sources["pkg/services/a/one/source.go"]
	sources["pkg/services/a/one/ignore_test.go"] = sources["pkg/services/a/one/source.go"]
	sources["pkg/services/a/one/local.go"] = "package one\nimport (_ \"fmt\"; _ \"" + modulePrefix + "pkg/services/a/two\")"
	sources["pkg/services/c/source.go"] = "package c\nimport _ \"" + modulePrefix + "pkg/services/d\""
	sources["pkg/services/d/source.go"] = "package d\nimport _ \"" + modulePrefix + "pkg/services/c\""
	input := cycleInput("", sources)
	input.Metadata += input.Metadata // Duplicate metadata cannot multiply statements.
	graph, err := serviceCycleGraph(input)
	if err != nil {
		t.Fatal(err)
	}
	if graph.weights[serviceEdge{"a", "b"}] != 2 || graph.weights[serviceEdge{"b", "a"}] != 3 {
		t.Fatalf("weights: %v", graph.weights)
	}
	solution, err := minimumFeedbackArcSet(graph.matrix())
	if err != nil || solution.weight != 3 {
		t.Fatalf("disconnected cycles: %+v %v", solution, err)
	}
	for _, name := range []string{"pkg/services/a/testdata/x.go", "pkg/services/a/vendor/x.go", "pkg/services/a/node_modules/x.go", "pkg/services/.git/a.go", "pkg/services/a/x_test.go", "pkg/platform/x.go"} {
		if serviceCycleSourcePath(name) {
			t.Fatalf("excluded path: %s", name)
		}
	}
}

func TestServiceCycleCeilingHistory(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		base, head string
		fail       bool
	}{
		{"", cycleOne, false}, {cycleOne, cycleOne, false}, {cycleOne, cycleZero, false},
		{cycleZero, cycleOne, true}, {cycleOne, "", true},
		{cycleOne, cycleOne + cycleOne, true}, {cycleOne, strings.Replace(cycleOne, "|1", "|-1", 1), true},
		{cycleOne, strings.Replace(cycleOne, "|1", "|bad", 1), true},
		{cycleOne, strings.Replace(cycleOne, "|1", "|01", 1), true},
		{cycleOne, strings.Replace(cycleOne, "internal/lint/analyzers", "elsewhere", 1), true},
		{cycleOne + "old|a|b", cycleZero + "old|a|c", true},
	} {
		if _, err := CompareBaselineGrowth(tc.base, tc.head); (err != nil) != tc.fail {
			t.Fatalf("base %q head %q: %v", tc.base, tc.head, err)
		}
	}
	for _, head := range []string{cycleOne + cycleOne, strings.Replace(cycleOne, "|1", "|-1", 1)} {
		if _, err := compilerOwnedKeys(head); err == nil {
			t.Fatal("compiler owner validation accepted invalid numeric debt")
		}
	}
}

func TestServiceCycleFailClosedAndInactiveMetadata(t *testing.T) {
	t.Parallel()
	for _, metadata := range []string{"not-json", `{"ImportPath":"unknown"}`, `{"Error":{"Err":"no Go files"}}`, `{"DepsErrors":[{"Err":"missing dependency"}]}`, `{"Error":{"Err":"build constraints exclude all Go files"}}`} {
		if _, err := serviceCycleFiles(metadata); err == nil {
			t.Fatalf("invalid metadata accepted: %s", metadata)
		}
	}
	unit := serviceCyclePackage{ImportPath: modulePrefix + "pkg/services/a", CgoFiles: []string{"cgo.go"}, IgnoredGoFiles: []string{"inactive.go"}, Error: &struct{ Err string }{"build constraints exclude all Go files"}}
	data, _ := json.Marshal(unit)
	files, err := serviceCycleFiles(string(data))
	if err != nil || len(files) != 2 {
		t.Fatalf("inactive/cgo sources: %v %v", files, err)
	}
	input := cycleInput(cycleOne, cycleSources())
	delete(input.Sources, "pkg/services/a/one/source.go")
	if _, err := serviceCycleGraph(input); err == nil {
		t.Fatal("missing source accepted")
	}
	input.Sources["pkg/services/a/one/source.go"] = "broken import"
	if _, err := serviceCycleGraph(input); err == nil {
		t.Fatal("parse failure accepted")
	}
	if err := serviceCycleDiagnostic(&serviceGraph{}, cycleZero); err == nil {
		t.Fatal("stale singleton accepted")
	}
	if err := serviceCycleDiagnostic(&serviceGraph{}, ""); err != nil {
		t.Fatal(err)
	}
}

func cycleReports(analyzer *analysis.Analyzer) []string {
	var messages []string
	pass := &analysis.Pass{Analyzer: analyzer, Pkg: types.NewPackage(modulePrefix+"internal/lint/analyzers", "analyzers"), Files: []*ast.File{{Package: token.Pos(1)}}, Report: func(d analysis.Diagnostic) { messages = append(messages, d.Message) }}
	_, _ = analyzer.Run(pass)
	return messages
}

func TestServiceCycleSnapshotIsolationAndInvalidation(t *testing.T) {
	t.Parallel()
	input := cycleInput(cycleOne, cycleSources())
	first := serviceCycleSnapshot(input, nil)
	input.Sources["pkg/services/b/two/source.go"] = "package two"
	second := serviceCycleSnapshot(input, nil)
	if first.Name == second.Name || len(cycleReports(first)) != 0 || len(cycleReports(second)) != 1 {
		t.Fatal("source edit must invalidate an invocation-owned snapshot")
	}
	input.Head = cycleZero
	third := serviceCycleSnapshot(input, nil)
	if third.Name == second.Name || len(cycleReports(third)) != 0 {
		t.Fatal("baseline-only edit must invalidate")
	}
	input.Metadata = ""
	input.Sources = nil
	fourth := serviceCycleSnapshot(input, nil)
	if fourth.Name == third.Name || len(cycleReports(fourth)) != 1 {
		t.Fatal("removed packages must invalidate")
	}
	failed := serviceCycleSnapshot(input, errors.New("compiler unavailable"))
	if failed.Name == fourth.Name || !strings.Contains(strings.Join(cycleReports(failed), ""), "compiler unavailable") {
		t.Fatal("dependency failure must invalidate and report")
	}
	_ = third.Flags.Set("check-stale", "false")
	if len(cycleReports(second)) != 1 {
		t.Fatal("equality cannot be deferred")
	}
}

func TestServiceCycleCollectionFailures(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"git", "baseline", "list", "read", "partial"} {
		read := func(name string) ([]byte, error) {
			if strings.HasSuffix(name, "baseline.txt") {
				if stage == "baseline" {
					return nil, errors.New("baseline denied")
				}
				return []byte(cycleZero), nil
			}
			if stage == "read" {
				return nil, errors.New("source denied")
			}
			return []byte("package a"), nil
		}
		run := func(root, tool string, args ...string) (string, error) {
			if tool == "go" {
				if stage == "list" {
					return "", errors.New("compiler failed")
				}
				if stage == "partial" {
					return "", nil
				}
				data, _ := json.Marshal(serviceCyclePackage{ImportPath: modulePrefix + "pkg/services/a", GoFiles: []string{"source.go"}})
				return string(data), nil
			}
			if stage == "git" {
				return "", errors.New("git failed")
			}
			if args[0] == "ls-files" {
				return "pkg/services/a/source.go\x00", nil
			}
			return ".", nil
		}
		if _, err := collectServiceCycleInput(".", run, read, func(string) (os.FileInfo, error) { return nil, nil }); err == nil {
			t.Fatalf("%s failure accepted", stage)
		}
	}
}
