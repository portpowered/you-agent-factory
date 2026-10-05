package analyzers

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func ownerMetadata(unit string, files ...string) string {
	data, _ := json.Marshal(compilerOwnerPackage{ImportPath: modulePrefix + unit, GoFiles: files})
	return string(data)
}

func TestCompilerMetadataExactOwners(t *testing.T) {
	t.Parallel()
	for _, key := range []string{
		"testsleep-sleep-test|pkg/a_test|pkg/a/a_test.go::Run::sleep::1",
		"test-cross-owner-policy|pkg/a_test|pkg/a/a_test.go#pkg/services/work.Normalize::count=2",
		"test-transport-owner-policy|pkg/a_test|pkg/a/a_test.go#pkg/services/work.Normalize::count=2",
		"petri-reference|pkg/a_test|pkg/a/a_test.go#pkg/services/factory_runtime.Net::count=2",
		"deprecated-runtime-api-test|pkg/a_test|pkg/a/a_test.go#TestRun",
		"functional-test-missing-subsection|pkg/a_test|pkg/a/a_test.go",
		"transport-recorded-site|pkg/a_test|pkg/a/a_test.go#transport-lifecycle#context.WithCancel::count=1",
		"service-root-exported-function|pkg/a_test|pkg/a/a_test.go#New",
		"service-container-go-file|pkg/a_test|pkg/a/a_test.go",
	} {
		t.Run(strings.Split(key, "|")[0], func(t *testing.T) {
			t.Parallel()
			for _, tc := range []struct {
				metadata string
				missing  bool
			}{
				{ownerMetadata("pkg/a_test [m/pkg/a.test]", "a_test.go"), false},
				{ownerMetadata("pkg/a", "a_test.go"), true},
				{ownerMetadata("pkg/a_test", "other_test.go"), true},
			} {
				keys, err := orphanCompilerKeys(key, tc.metadata, modulePrefix)
				if err != nil || (len(keys) == 1) != tc.missing {
					t.Fatalf("keys=%v err=%v", keys, err)
				}
			}
		})
	}
}

func TestCompilerMetadataParsingFailures(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"bad", "a||c", "a| b|c", "a|b|c|d", "a|b|c\na|b|c", "petri-public|a|b", "testsleep-unknown|a|a.go::Run::unknown::1", "testsleep-sleep-test|a|a_test.go::Run::sleep::0"} {
		if _, err := compilerOwnedKeys(key); err == nil {
			t.Fatalf("invalid key accepted: %s", key)
		}
	}
	const key = "service-root-interface-count|pkg/a|<none>"
	for _, metadata := range []string{"", "not-json", `{"ImportPath":"x","DepsErrors":[{"Err":"missing dependency"}]}`, `{"Error":{"Err":"broken compiler"}}`} {
		if _, err := orphanCompilerKeys(key, metadata, modulePrefix); err == nil {
			t.Fatalf("missing/invalid metadata accepted: %s", metadata)
		}
	}
	for _, key := range []string{"petri-public|a|Exported|pkg/b.Type", ""} {
		if keys, err := orphanCompilerKeys(key, "", modulePrefix); err != nil || len(keys) != 0 {
			t.Fatalf("non-owned debt requires no metadata: %v %v", keys, err)
		}
	}
}

func TestCompilerMetadataInternalTestsAndIgnoredSources(t *testing.T) {
	t.Parallel()
	const key = "testsleep-sleep-test|pkg/a|pkg/a/a_test.go::Run::sleep::1"
	for _, tc := range []struct {
		unit compilerOwnerPackage
		want int
	}{
		{compilerOwnerPackage{ImportPath: modulePrefix + "pkg/a", GoFiles: []string{"source.go"}}, 1},
		{compilerOwnerPackage{ImportPath: modulePrefix + "pkg/a", TestGoFiles: []string{"a_test.go"}}, 0},
		{compilerOwnerPackage{ImportPath: modulePrefix + "pkg/a", IgnoredGoFiles: []string{"a_test.go"}}, 0},
		{compilerOwnerPackage{ImportPath: modulePrefix + "pkg/a", TestGoFiles: []string{"other_test.go"}}, 1},
	} {
		metadata, _ := json.Marshal(tc.unit)
		keys, err := orphanCompilerKeys(key, string(metadata), modulePrefix)
		if err != nil || len(keys) != tc.want {
			t.Fatalf("keys=%v err=%v", keys, err)
		}
	}
	for _, key := range []string{"service-root-interface-count|pkg/a|<none>", "service-root-unexpected-directory|pkg/a|pkg/a"} {
		keys, err := orphanCompilerKeys(key, ownerMetadata("pkg/a", "contract.go"), modulePrefix)
		if err != nil || len(keys) != 0 {
			t.Fatalf("owner-only debt: keys=%v err=%v", keys, err)
		}
	}
}

func TestCompilerMetadataPlatformResolution(t *testing.T) {
	t.Parallel()
	const windows = "testsleep-sleep-test|tests/real_test_test|tests/real_test/windows_test.go::Run::sleep::1"
	const gone = "service-root-interface-count|pkg/gone|<none>"
	var calls []string
	metadata, err := collectCompilerOwners(windows+"\n"+gone, "linux", func(goos string, dirs []string) (string, error) {
		calls = append(calls, goos+":"+strings.Join(dirs, ","))
		if goos == "windows" {
			return ownerMetadata("tests/real_test_test [m/tests/real_test.test]", "windows_test.go"), nil
		}
		return ownerMetadata("tests/real_test"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := orphanCompilerKeys(windows+"\n"+gone, metadata, modulePrefix)
	if err != nil || !reflect.DeepEqual(keys, []string{gone}) {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
	if !reflect.DeepEqual(calls, []string{"linux:./pkg/gone,./tests/real_test", "windows:./pkg/gone,./tests/real_test", "darwin:./pkg/gone"}) {
		t.Fatalf("wrong targeted platform queries: %v", calls)
	}
}

func TestCompilerMetadataInactiveHostAndCompilerFailure(t *testing.T) {
	t.Parallel()
	const key = "testsleep-sleep-test|pkg/a_test|pkg/a/a_test.go::Run::sleep::1"
	queries := 0
	metadata, err := collectCompilerOwners(key, "linux", func(goos string, _ []string) (string, error) {
		queries++
		if goos == "linux" {
			return `{"Error":{"Err":"build constraints exclude all Go files"}}`, nil
		}
		return ownerMetadata("pkg/a_test", "a_test.go"), nil
	})
	if err != nil || queries != 2 || !strings.Contains(metadata, "a_test.go") {
		t.Fatalf("inactive host: queries=%d err=%v metadata=%s", queries, err, metadata)
	}
	if _, err := collectCompilerOwners(key, "linux", func(string, []string) (string, error) {
		return "", errors.New("compiler unavailable")
	}); err == nil {
		t.Fatal("compiler failure accepted")
	}
}

func TestCompilerMetadataSnapshotDiagnosticsAndCache(t *testing.T) {
	t.Parallel()
	const key = "service-root-exported-function|pkg/a|pkg/a/source.go#New"
	clean := compilerOwnersSnapshot(key, ownerMetadata("pkg/a", "source.go"), nil)
	for _, tc := range []struct {
		head, metadata string
		failure        error
		want           string
	}{
		{key, ownerMetadata("pkg/a", "source.go"), nil, ""},
		{key, ownerMetadata("pkg/a", "other.go"), nil, key},
		{key, ownerMetadata("pkg/other", "source.go"), nil, key},
		{key, "", nil, "no repository units"},
		{key, "", errors.New("missing compiler"), "missing compiler"},
	} {
		rule := compilerOwnersSnapshot(tc.head, tc.metadata, tc.failure)
		var messages []string
		pass := &analysis.Pass{Analyzer: rule, Pkg: types.NewPackage(modulePrefix+"internal/lint/analyzers", "analyzers"), Files: []*ast.File{{Package: token.Pos(1)}}, Report: func(d analysis.Diagnostic) { messages = append(messages, d.Message) }}
		if _, err := rule.Run(pass); err != nil {
			t.Fatal(err)
		}
		if tc.want == "" {
			if len(messages) != 0 || rule.Name != clean.Name {
				t.Fatalf("clean snapshot changed: %v", messages)
			}
		} else if len(messages) != 1 || !strings.Contains(messages[0], tc.want) || rule.Name == clean.Name {
			t.Fatal(fmt.Sprintf("failure/cache: %v %s", messages, rule.Name))
		}
	}
}
