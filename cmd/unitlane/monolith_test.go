package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreparedMonolithRejectsMissingAndMismatchedArtifacts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binary := filepath.Join(dir, "unit.test")
	if err := os.WriteFile(binary, []byte("prepared artifact"), 0700); err != nil {
		t.Fatal(err)
	}
	suite := preparedMonolithSuite{Root: "./pkg/...", PreparedAt: time.Now(), Binary: binary,
		Packages: []string{"owner", "isolated"}, Groups: []monolithGroup{{Package: "owner"}},
		Native: map[string]string{"isolated": binary}}
	cfg := config{root: suite.Root}
	if err := validatePreparedMonolith(cfg, suite); err != nil {
		t.Fatal(err)
	}
	cfg.root = "./pkg/other"
	if err := validatePreparedMonolith(cfg, suite); err == nil {
		t.Fatal("accepted an inventory prepared for a different root")
	}
	cfg.root = suite.Root
	suite.Native["isolated"] = filepath.Join(dir, "missing.test")
	if err := validatePreparedMonolith(cfg, suite); err == nil {
		t.Fatal("silently dropped a missing isolated binary")
	}
	suite.Native = map[string]string{}
	if err := validatePreparedMonolith(cfg, suite); err == nil {
		t.Fatal("silently omitted an inventoried package")
	}
}

func TestMonolithDiscoveryUsesBuildSelectedDeclarationsAndRefreshesSources(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	internal := filepath.Join(dir, "internal_test.go")
	external := filepath.Join(dir, "external_test.go")
	write := func(path, source string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(internal, `package fixture
func TestMain(m *testing.M) {}
func TestFirst(t *testing.T) {}
func Testlower(t *testing.T) {}
func (r Receiver) TestMethod(t *testing.T) {}
func BenchmarkFirst(b *testing.B) {}
func FuzzFirst(f *testing.F) {}
`)
	write(external, `package fixture_test
func TestExternal(t *testing.T) {}
func Example() {
 // Output: example
}
`)
	pkg := monolithPackageMetadata{Dir: dir, TestGoFiles: []string{"internal_test.go"}, XTestGoFiles: []string{"external_test.go"}}
	first, err := monolithRegistrations(pkg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"_test.TestFirst", "_xtest.TestExternal", "_test.BenchmarkFirst", "_test.FuzzFirst", "_xtest.Example", "_test.TestMain(m)"} {
		if !strings.Contains(first, want) {
			t.Fatalf("missing %s in %s", want, first)
		}
	}
	if strings.Contains(first, "Testlower") || strings.Contains(first, "TestMethod") {
		t.Fatalf("registered non-test declaration: %s", first)
	}
	write(internal, `package fixture
func TestChanged(t *testing.T) {}
`)
	second, err := monolithRegistrations(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(second, "TestFirst") || !strings.Contains(second, "TestChanged") {
		t.Fatalf("stale source registration: %s", second)
	}
}

func TestMonolithInventoryRejectsMissingDuplicateAndUnselectedPackages(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		groups    []monolithGroup
		native    map[string]string
		wantError bool
	}{
		{"complete", []monolithGroup{{Package: "a"}}, map[string]string{"b": "custom TestMain"}, false},
		{"missing", []monolithGroup{{Package: "a"}}, nil, true},
		{"duplicated", []monolithGroup{{Package: "a"}, {Package: "a"}}, map[string]string{"b": "native"}, true},
		{"both lanes", []monolithGroup{{Package: "a"}, {Package: "b"}}, map[string]string{"b": "native"}, true},
		{"unexpected", []monolithGroup{{Package: "a"}, {Package: "b"}, {Package: "c"}}, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateMonolithInventory([]string{"a", "b"}, test.groups, test.native); (err != nil) != test.wantError {
				t.Fatalf("inventory error = %v, want error %t", err, test.wantError)
			}
		})
	}
}

func TestMonolithEventsKeepFailuresAndOriginalTestIdentities(t *testing.T) {
	t.Parallel()
	groups := []monolithGroup{{Package: "a", Group: "TestPackage0001", Tests: []string{"TestDuplicate", "TestDuplicate", "TestUnique"}}}
	input := strings.NewReader(`{"Action":"pass","Test":"TestUnitPackages/TestPackage0001/TestDuplicate#01/case#01","Package":"overlay"}
{"Action":"fail","Test":"TestUnitPackages/TestPackage0001","Package":"overlay","Elapsed":1.25}
{"Action":"fail","Test":"TestUnitPackages","Package":"overlay","Output":"FAIL\n"}
`)
	var output bytes.Buffer
	if err := remapMonolithEvents(input, &output, groups); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var child, terminal, overall goTestUnitTimingEvent
	for _, event := range []*goTestUnitTimingEvent{&child, &terminal, &overall} {
		if err := decoder.Decode(event); err != nil {
			t.Fatal(err)
		}
	}
	if child.Package != "a" || child.Test != "TestDuplicate/case#01" {
		t.Fatalf("child=%+v", child)
	}
	if terminal.Package != "a" || terminal.Test != "" || terminal.Action != "fail" || terminal.Elapsed != 1.25 {
		t.Fatalf("terminal=%+v", terminal)
	}
	if overall.Action != "output" || overall.Package != "" || overall.Test != "" || overall.Output != "FAIL\n" {
		t.Fatalf("overall=%+v", overall)
	}
}

func TestMonolithEventsRejectUnknownGroupsAndKeepUniqueNames(t *testing.T) {
	t.Parallel()
	groups := []monolithGroup{{Package: "a", Group: "TestPackage0001", Tests: []string{"TestDuplicate", "TestDuplicate", "TestUnique"}}}
	if got := originalMonolithTestName("TestUnique#01/case", groups[0].Tests); got != "TestUnique#01/case" {
		t.Fatalf("normalized a unique test: %s", got)
	}
	if err := remapMonolithEvents(strings.NewReader(`{"Test":"TestUnitPackages/unknown/TestCase"}`), &bytes.Buffer{}, groups); err == nil {
		t.Fatal("unknown group accepted")
	}
}

func TestCompactMonolithEventsKeepFailureDiagnosticsAndRejectMalformedRecords(t *testing.T) {
	t.Parallel()
	input := strings.NewReader("failure detail\nUNIT_EVENT {\"Action\":\"fail\",\"Package\":\"a\",\"Test\":\"TestCase\",\"Elapsed\":0.5}\nUNIT_EVENT {\"Action\":\"fail\",\"Package\":\"a\",\"Test\":\"\",\"Elapsed\":1}\n")
	var output bytes.Buffer
	if err := remapCompactMonolithEvents(input, &output); err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	capture, err := collectUnitTimingCapture(&output, []string{"a"}, &diagnostics)
	if err != nil || !capture.Complete || len(capture.Packages) != 1 || capture.Packages[0].Outcome != "fail" || len(capture.Packages[0].Tests) != 1 || capture.Packages[0].Tests[0] != "TestCase" {
		t.Fatalf("capture=%+v error=%v", capture, err)
	}
	if err := remapCompactMonolithEvents(strings.NewReader("UNIT_EVENT malformed\n"), &bytes.Buffer{}); err == nil {
		t.Fatal("malformed compact event accepted")
	}
	if got := diagnostics.String(); got != "failure detail\n" {
		t.Fatalf("compact records leaked into diagnostics: %q", got)
	}
}
