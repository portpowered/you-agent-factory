package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSelectChangedTestsOnlyReturnsNewOrBodyModifiedTopLevelTests(t *testing.T) {
	base := map[string]string{
		"pkg/example/example_test.go": `package example

func TestUnchanged(t *testing.T) { t.Log("same") }
func TestBodyChanged(t *testing.T) { t.Log("old") }
func ExampleExample() {}
func BenchmarkExample(b *testing.B) {}
func helperTest(t *testing.T) {}
`,
	}
	head := map[string]string{
		"pkg/example/example_test.go": `package example

func TestUnchanged(t *testing.T) { t.Log("same") }
func TestBodyChanged(t *testing.T) { t.Log("new") }
func TestNew(t *testing.T) {}
func ExampleExample() {}
func BenchmarkExample(b *testing.B) {}
func helperTest(t *testing.T) {}
`,
	}
	diff := "diff --git a/pkg/example/example_test.go b/pkg/example/example_test.go\n--- a/pkg/example/example_test.go\n+++ b/pkg/example/example_test.go\n@@ -1,7 +1,8 @@\n"
	selected, err := selectChangedTests(diff, "head", "base", fixtureSourceReader(base, head))
	if err != nil {
		t.Fatal(err)
	}
	assertSelectedTests(t, selected, []selectedTest{{File: "pkg/example/example_test.go", Name: "TestBodyChanged"}, {File: "pkg/example/example_test.go", Name: "TestNew"}})
}

func TestSelectChangedTestsHandlesRenameAndMovedUnchangedTest(t *testing.T) {
	base := map[string]string{
		"old/old_test.go":     "package old\nfunc TestMoved(t *testing.T) { t.Log(\"same\") }\n",
		"pkg/changed_test.go": "package changed\nfunc TestRenamed(t *testing.T) { t.Log(\"old\") }\n",
	}
	head := map[string]string{
		"new/new_test.go":     "package old\nfunc TestMoved(t *testing.T) { t.Log(\"same\") }\n",
		"pkg/changed_test.go": "package changed\nfunc TestRenamed(t *testing.T) { t.Log(\"new\") }\n",
	}
	diff := strings.Join([]string{
		"diff --git a/old/old_test.go b/new/new_test.go",
		"similarity index 100%",
		"rename from old/old_test.go",
		"rename to new/new_test.go",
		"diff --git a/pkg/changed_test.go b/pkg/changed_test.go",
		"--- a/pkg/changed_test.go",
		"+++ b/pkg/changed_test.go",
		"@@ -1 +1 @@",
	}, "\n")
	selected, err := selectChangedTests(diff, "head", "base", fixtureSourceReader(base, head))
	if err != nil {
		t.Fatal(err)
	}
	assertSelectedTests(t, selected, []selectedTest{{File: "pkg/changed_test.go", Name: "TestRenamed"}})
}

func TestSelectChangedTestsIgnoresDeletedTestsAndUnchangedFileEdits(t *testing.T) {
	base := map[string]string{
		"pkg/example_test.go": "package example\nfunc TestDeleted(t *testing.T) {}\nfunc TestKept(t *testing.T) {}\n",
		"pkg/deleted_test.go": "package example\nfunc TestDeletedFile(t *testing.T) {}\n",
	}
	head := map[string]string{
		"pkg/example_test.go": "package example\nfunc TestKept(t *testing.T) {}\n// unrelated change\n",
	}
	diff := strings.Join([]string{
		"diff --git a/pkg/example_test.go b/pkg/example_test.go",
		"--- a/pkg/example_test.go",
		"+++ b/pkg/example_test.go",
		"@@ -1,3 +1,3 @@",
		"diff --git a/pkg/deleted_test.go b/pkg/deleted_test.go",
		"--- a/pkg/deleted_test.go",
		"+++ /dev/null",
	}, "\n")
	selected, err := selectChangedTests(diff, "head", "base", fixtureSourceReader(base, head))
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 0 {
		t.Fatalf("selected = %#v, want no unchanged or deleted tests", selected)
	}
}

func TestGroupSelectedTestsSortsPackagesAndTestNames(t *testing.T) {
	packages := map[string]string{"z_test.go": "example/z", "a_test.go": "example/a", "m_test.go": "example/z"}
	groups, err := groupSelectedTests([]selectedTest{
		{File: "z_test.go", Name: "TestZ"},
		{File: "a_test.go", Name: "TestB"},
		{File: "m_test.go", Name: "TestA"},
		{File: "z_test.go", Name: "TestZ"},
	}, func(path string) (string, error) { return packages[path], nil })
	if err != nil {
		t.Fatal(err)
	}
	want := []testGroup{{Package: "example/a", Tests: []string{"TestB"}}, {Package: "example/z", Tests: []string{"TestA", "TestZ"}}}
	if fmt.Sprint(groups) != fmt.Sprint(want) {
		t.Fatalf("groups = %#v, want %#v", groups, want)
	}
}

func TestStabilityRunnerNoOpAndFullAttemptAccounting(t *testing.T) {
	var output strings.Builder
	starts := 0
	runner := stabilityRunner{
		attempts: 3,
		budget:   time.Hour,
		now:      func() time.Time { return time.Unix(100, 0) },
		run: func(context.Context, testGroup, string) (string, error) {
			starts++
			return "ok", nil
		},
	}
	if err := runner.runAll(nil, &output); err != nil {
		t.Fatal(err)
	}
	if starts != 0 || !strings.Contains(output.String(), "no qualifying tests") {
		t.Fatalf("no-op output=%q starts=%d", output.String(), starts)
	}

	output.Reset()
	if err := runner.runAll([]testGroup{{Package: "example/pkg", Tests: []string{"TestOne", "TestTwo"}}}, &output); err != nil {
		t.Fatal(err)
	}
	if starts != 6 || !strings.Contains(output.String(), "measured=6") {
		t.Fatalf("success output=%q starts=%d", output.String(), starts)
	}
}

func TestStabilityRunnerStopsAtFirstFailureWithDiagnostics(t *testing.T) {
	var output strings.Builder
	starts := 0
	runner := stabilityRunner{
		attempts: 20,
		budget:   time.Hour,
		now:      func() time.Time { return time.Unix(100, 0) },
		run: func(context.Context, testGroup, string) (string, error) {
			starts++
			if starts == 2 {
				return "--- FAIL: TestFlaky (0.00s)\nreceived diagnostic", errors.New("exit status 1")
			}
			return "ok", nil
		},
	}
	err := runner.runAll([]testGroup{{Package: "example/pkg", Tests: []string{"TestFlaky"}}}, &output)
	if err == nil || !strings.Contains(err.Error(), "attempt 2/20") {
		t.Fatalf("error=%v, want first failed attempt", err)
	}
	if starts != 2 {
		t.Fatalf("starts=%d, want 2", starts)
	}
	for _, want := range []string{"package=example/pkg", "test=TestFlaky", "attempt=2/20", "received diagnostic", "go test -count=1 -run=^TestFlaky$ example/pkg"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output=%q, want %q", output.String(), want)
		}
	}
}

func TestStabilityRunnerFailsClosedWhenBudgetExpires(t *testing.T) {
	var output strings.Builder
	current := time.Unix(100, 0)
	runner := stabilityRunner{
		attempts: 3,
		budget:   10 * time.Second,
		now:      func() time.Time { return current },
		run: func(context.Context, testGroup, string) (string, error) {
			current = current.Add(10 * time.Second)
			return "ok", nil
		},
	}
	err := runner.runAll([]testGroup{{Package: "example/pkg", Tests: []string{"TestSlow"}}}, &output)
	if err == nil || !strings.Contains(err.Error(), "budget expired") {
		t.Fatalf("error=%v, want budget failure", err)
	}
	if !strings.Contains(output.String(), "completed-attempts=1") || !strings.Contains(output.String(), "TestSlow (1/3)") {
		t.Fatalf("output=%q, want completed and unmeasured attempt counts", output.String())
	}
}

func fixtureSourceReader(base, head map[string]string) sourceReader {
	return func(revision, path string) ([]byte, error) {
		var source string
		var ok bool
		if revision == "base" {
			source, ok = base[path]
		} else {
			source, ok = head[path]
		}
		if !ok {
			return nil, fmt.Errorf("missing fixture %s:%s", revision, path)
		}
		return []byte(source), nil
	}
}

func assertSelectedTests(t *testing.T, got, want []selectedTest) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("selected = %#v, want %#v", got, want)
	}
}
