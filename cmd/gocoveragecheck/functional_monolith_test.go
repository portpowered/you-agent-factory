package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFunctionalMonolithOriginalEvents(t *testing.T) {
	groups := map[string]string{"TestPackage0001": "original/package"}
	for _, action := range []string{"run", "pass", "fail", "skip", "output"} {
		raw, _ := json.Marshal(map[string]any{"Package": functionalMonolithPackage, "Test": "TestFunctionalPackages/TestPackage0001/TestCustomer/nested", "Action": action, "Time": "2026-10-05T00:00:00Z", "Output": "TestFunctionalPackages/TestPackage0001/TestCustomer/nested\n"})
		line, err := normalizeFunctionalMonolithEvent(raw, groups)
		if err != nil {
			t.Fatal(err)
		}
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		if event["Package"] != "original/package" || event["Test"] != "TestCustomer/nested" || event["Action"] != action || event["Time"] != "2026-10-05T00:00:00Z" || event["Output"] != "TestCustomer/nested\n" {
			t.Fatalf("identity or evidence changed: %s", line)
		}
	}
}

func TestFunctionalMonolithBoundaryEvents(t *testing.T) {
	groups := map[string]string{"TestPackage0001": "original/package"}
	if _, err := normalizeFunctionalMonolithEvent([]byte(`{"Package":"`+functionalMonolithPackage+`","Test":"TestFunctionalPackages/Unknown/TestCustomer","Action":"pass"}`), groups); err == nil {
		t.Fatal("unknown group accepted")
	}
	native := []byte(`{"Package":"native/package","Test":"TestCustomer","Action":"fail"}`)
	line, err := normalizeFunctionalMonolithEvent(native, groups)
	if err != nil || !bytes.Equal(line, native) {
		t.Fatalf("native failure changed: %s %v", line, err)
	}
	for _, action := range []string{"start", "pass", "output"} {
		line, err := normalizeFunctionalMonolithEvent([]byte(`{"Package":"`+functionalMonolithPackage+`","Action":"`+action+`"}`), groups)
		if err != nil || len(line) != 0 {
			t.Fatalf("coordinator bookkeeping became customer evidence: %s %v", line, err)
		}
	}
	line, err = normalizeFunctionalMonolithEvent([]byte(`{"Package":"`+functionalMonolithPackage+`","Action":"fail"}`), groups)
	if err != nil || len(line) == 0 {
		t.Fatalf("coordinator failure hidden: %s %v", line, err)
	}
}

func TestFunctionalMonolithFragmentedStream(t *testing.T) {
	var output bytes.Buffer
	writer := functionalMonolithEventWriter{sink: &output, groups: map[string]string{"Group": "original/package"}}
	raw := `{"Package":"` + functionalMonolithPackage + `","Test":"TestFunctionalPackages/Group/TestCustomer","Action":"fail"}`
	for _, fragment := range []string{raw[:18], raw[18:]} {
		if _, err := writer.Write([]byte(fragment)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.flush(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"Package":"original/package"`) || !strings.Contains(output.String(), `"Action":"fail"`) {
		t.Fatalf("lost fragmented terminal event: %s", output.String())
	}
}

func TestFunctionalMonolithPlanKeepsNativePackages(t *testing.T) {
	plan := coverageInvocationPlan{invocations: []commandInvocation{{args: []string{"test", "-coverprofile=profile.out", "original/package", "native/package"}}}}
	groups := []functionalMonolithGroup{{Package: "original/package", Group: "Group", Tests: []string{"TestCustomer"}}}
	if err := applyFunctionalMonolithGroups(&plan, groups, "overlay.json"); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(plan.invocations[0].args, " ")
	if !strings.Contains(args, "native/package") || strings.Contains(args, "original/package") || !strings.Contains(args, "-overlay=overlay.json") || !strings.Contains(args, functionalMonolithPackage) {
		t.Fatalf("incorrect plan: %s", args)
	}
	if err := applyFunctionalMonolithGroups(&plan, append(groups, groups...), "overlay.json"); err == nil {
		t.Fatal("duplicate original package accepted")
	}
}

func TestFunctionalMonolithNativeFixtures(t *testing.T) {
	for _, source := range []string{`os.Getwd()`, `t.Setenv("A", "B")`, `//go:embed fixture`, `filepath.Join("testdata", "input")`, `exec.Command("helper")`, `func FuzzCustomer(f *testing.F) {}`} {
		if functionalMonolithNativeReason(source) == "" {
			t.Fatalf("process/fixture dependency accepted: %s", source)
		}
	}
	if reason := functionalMonolithNativeReason(`func TestCustomer(t *testing.T) { t.Parallel(); session := newSession(t); session.Execute() }`); reason != "" {
		t.Fatal(reason)
	}
}

func TestFunctionalMonolithRetryUsesOriginalPackage(t *testing.T) {
	template := commandInvocation{name: "go", args: []string{"test", "-overlay=generated.json", "-covermode=count", "-json", functionalMonolithPackage}, monolithGroups: map[string]string{"Group": "original/package"}}
	invocations, _ := buildFlakeRetryInvocations(template, []flakeFailure{{Package: "original/package", Test: "TestCustomer"}}, t.TempDir())
	if len(invocations) != 1 {
		t.Fatalf("retry invocations = %d", len(invocations))
	}
	args := strings.Join(invocations[0].args, " ")
	if strings.Contains(args, "-overlay=") || strings.Contains(args, functionalMonolithPackage) || !strings.HasSuffix(args, "original/package") || !strings.Contains(args, "-run=^(TestCustomer)$") {
		t.Fatalf("incorrect native retry: %s", args)
	}
}

func TestFunctionalMonolithRawCaptureRequiresEveryOriginalTerminal(t *testing.T) {
	for _, complete := range []bool{true, false} {
		capture := newRawFailureTestCapture(t, t.TempDir(), 1<<20)
		capture.beginInvocation(commandInvocation{name: "go", args: []string{"test", "-json", functionalMonolithPackage}, monolithGroups: map[string]string{"A": "original/a", "B": "original/b"}})
		if err := capture.observeLine(rawFailureTestEvent("pass", "original/a", "", "")); err != nil {
			t.Fatal(err)
		}
		if complete {
			if err := capture.observeLine(rawFailureTestEvent("pass", "original/b", "", "")); err != nil {
				t.Fatal(err)
			}
		}
		capture.finishInvocation(nil)
		err := capture.publish("head", false)
		if (err == nil) != complete {
			t.Fatalf("complete=%v capture error=%v", complete, err)
		}
		if _, found := capture.packages[functionalMonolithPackage]; found {
			t.Fatal("coordinator registered as a customer package")
		}
	}
}
