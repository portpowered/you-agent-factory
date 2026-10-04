package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		writeFile(t, root, name, body)
	}
	return root
}

func writeFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCheck(root string, regenerate bool) (string, error) {
	var out, errOut bytes.Buffer
	err := run(config{root: root, baseline: "base.json", regenerate: regenerate}, &out, &errOut)
	return errOut.String(), err
}

const sleepy = `package x

import (
	"context"
	"testing"
	"time"
)

func TestA(t *testing.T) {
	time.Sleep(10 * time.Millisecond)
	<-time.After(2 * time.Second)
	ctx, c := context.WithTimeout(context.Background(), time.Second)
	_ = ctx
	c()
	start := time.Now()
	if time.Since(start) < 300*time.Millisecond {
		t.Fatal("fast")
	}
}
`

func TestScanClassifiesSitesAndIgnoresGenerousCeilings(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pkg/a/a_test.go": sleepy + `
func TestCeiling(t *testing.T) {
	<-time.After(30 * time.Second)
	d := 2 * time.Second
	<-time.After(d)
}
`,
		"pkg/a/helper.go": "package a\nimport \"time\"\nfunc H() { time.Sleep(1) }\n",
	})
	result, err := scanRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[siteKey]int{
		{"pkg/a/a_test.go", "TestA", kindSleep}:    1,
		{"pkg/a/a_test.go", "TestA", kindDeadline}: 2,
		{"pkg/a/a_test.go", "TestA", kindElapsed}:  1,
	}
	if len(result.counts) != len(want) {
		t.Fatalf("counts = %v", result.counts)
	}
	for k, v := range want {
		if result.counts[k] != v {
			t.Errorf("%v = %d, want %d", k, result.counts[k], v)
		}
	}
}

func TestRatchetFailsOnNewSiteAndToleratesMotionAndRemoval(t *testing.T) {
	root := writeTree(t, map[string]string{"pkg/a/a_test.go": sleepy})
	if _, err := runCheck(root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := runCheck(root, false); err != nil {
		t.Fatalf("unchanged tree must pass: %v", err)
	}
	moved := "// moved\n\n\n" + strings.Replace(sleepy, "func TestA", "func Other() {}\n\nfunc TestA", 1)
	writeFile(t, root, "pkg/a/a_test.go", moved)
	if _, err := runCheck(root, false); err != nil {
		t.Fatalf("code motion must pass: %v", err)
	}
	writeFile(t, root, "pkg/a/a_test.go", "package x\n")
	if _, err := runCheck(root, false); err != nil {
		t.Fatalf("removal must pass: %v", err)
	}
	writeFile(t, root, "pkg/a/b_test.go", "package x\nimport \"time\"\nfunc TestB() { time.Sleep(time.Second) }\n")
	stderr, err := runCheck(root, false)
	if err == nil || !strings.Contains(stderr, "b_test.go: TestB (sleep)") {
		t.Fatalf("new site must fail, got err=%v stderr=%q", err, stderr)
	}
}

func TestExtraSiteInBaselinedFunctionFails(t *testing.T) {
	root := writeTree(t, map[string]string{"pkg/a/a_test.go": sleepy})
	if _, err := runCheck(root, true); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "pkg/a/a_test.go", strings.Replace(sleepy, "c()\n", "c()\n\ttime.Sleep(1)\n", 1))
	if _, err := runCheck(root, false); err == nil {
		t.Fatal("an additional sleep in a baselined function must fail")
	}
}

func TestExemptionRequiresReason(t *testing.T) {
	root := writeTree(t, map[string]string{"pkg/a/a_test.go": `package x

import "time"

func TestOk() {
	time.Sleep(1) //nolint:testsleep // exercising real timer wiring
	//nolint:testsleep // reason on previous line
	time.Sleep(1)
}

func TestBad() {
	time.Sleep(1) //nolint:testsleep
}
`})
	if _, err := runCheck(root, true); err != nil {
		t.Fatal(err)
	}
	stderr, err := runCheck(root, false)
	if err == nil || !strings.Contains(stderr, "requires a reason") {
		t.Fatalf("reasonless exemption must fail, got %v %q", err, stderr)
	}
	result, err := scanRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.counts[siteKey{"pkg/a/a_test.go", "TestOk", kindSleep}]; got != 0 {
		t.Fatalf("exempt sites must not count, got %d", got)
	}
	if got := result.counts[siteKey{"pkg/a/a_test.go", "TestBad", kindSleep}]; got != 1 {
		t.Fatalf("reasonless exemption must not exempt, got %d", got)
	}
}

func TestBaselineIsSortedAndDeterministic(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pkg/z/z_test.go": "package z\nimport \"time\"\nfunc TestZ() { time.Sleep(1) }\n",
		"pkg/a/a_test.go": sleepy,
	})
	var first []byte
	for i := 0; i < 2; i++ {
		if _, err := runCheck(root, true); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(root, "base.json"))
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = data
		} else if !bytes.Equal(first, data) {
			t.Fatal("baseline not deterministic")
		}
	}
	if strings.Index(string(first), "pkg/a/a_test.go") > strings.Index(string(first), "pkg/z/z_test.go") {
		t.Fatal("baseline not sorted by file")
	}
}

func TestAliasedTimeImport(t *testing.T) {
	root := writeTree(t, map[string]string{"pkg/a/a_test.go": "package x\nimport tm \"time\"\nfunc TestA() { tm.Sleep(1) }\n"})
	result, err := scanRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.counts[siteKey{"pkg/a/a_test.go", "TestA", kindSleep}] != 1 {
		t.Fatalf("alias not resolved: %v", result.counts)
	}
}
