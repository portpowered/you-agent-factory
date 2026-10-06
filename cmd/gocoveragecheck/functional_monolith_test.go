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
		line, err := normalizeFunctionalMonolithEvent(raw, groups, nil)
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
	if _, err := normalizeFunctionalMonolithEvent([]byte(`{"Package":"`+functionalMonolithPackage+`","Test":"TestFunctionalPackages/Unknown/TestCustomer","Action":"pass"}`), groups, nil); err == nil {
		t.Fatal("unknown group accepted")
	}
	native := []byte(`{"Package":"native/package","Test":"TestCustomer","Action":"fail"}`)
	line, err := normalizeFunctionalMonolithEvent(native, groups, nil)
	if err != nil || !bytes.Equal(line, native) {
		t.Fatalf("native failure changed: %s %v", line, err)
	}
	for _, action := range []string{"start", "pass", "output"} {
		line, err := normalizeFunctionalMonolithEvent([]byte(`{"Package":"`+functionalMonolithPackage+`","Action":"`+action+`"}`), groups, nil)
		if err != nil || len(line) != 0 {
			t.Fatalf("coordinator bookkeeping became customer evidence: %s %v", line, err)
		}
	}
	line, err = normalizeFunctionalMonolithEvent([]byte(`{"Package":"`+functionalMonolithPackage+`","Action":"fail"}`), groups, nil)
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
	for _, source := range []string{`os.Getwd()`, `t.Setenv("A", "B")`, `//go:embed fixture`, `filepath.Join("testdata", "input")`, `exec.Command("helper")`, `exec.Command(binary)`, `exec.Command(os.Args[0], "-test.run=TestChild")`, `exec.CommandContext(ctx, executable, os.Executable())`, `exec.Command("git", "status"); exec.Command("helper")`, `func FuzzCustomer(f *testing.F) {}`} {
		if functionalMonolithNativeReason(source) == "" {
			t.Fatalf("process/fixture dependency accepted: %s", source)
		}
	}
	if reason := functionalMonolithNativeReason(`func TestCustomer(t *testing.T) { t.Parallel(); session := newSession(t); session.Execute() }`); reason != "" {
		t.Fatal(reason)
	}
	if reason := functionalMonolithNativeReason(`command := exec.Command("git", args...); command.Dir = workspace; command.CombinedOutput()`); reason != "" {
		t.Fatalf("explicit fixture command depends on native test identity: %s", reason)
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

func TestFunctionalMonolithPackageWallIncludesParallelChildren(t *testing.T) {
	var output bytes.Buffer
	writer := functionalMonolithEventWriter{sink: &output, groups: map[string]string{"Group": "original/package"}}
	for _, event := range []string{
		`{"Time":"2026-10-06T00:00:00Z","Package":"` + functionalMonolithPackage + `","Test":"TestFunctionalPackages/Group","Action":"run"}`,
		`{"Time":"2026-10-06T00:00:01Z","Package":"` + functionalMonolithPackage + `","Test":"TestFunctionalPackages/Group","Action":"pause"}`,
		`{"Time":"2026-10-06T00:00:02Z","Package":"` + functionalMonolithPackage + `","Test":"TestFunctionalPackages/Group","Action":"cont"}`,
		`{"Time":"2026-10-06T00:00:10Z","Package":"` + functionalMonolithPackage + `","Test":"TestFunctionalPackages/Group","Action":"pass","Elapsed":0}`,
	} {
		if _, err := writer.Write([]byte(event + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("normalized events = %s", output.String())
	}
	var terminal struct {
		Package, Action string
		Elapsed         float64
	}
	if err := json.Unmarshal([]byte(lines[1]), &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Package != "original/package" || terminal.Action != "pass" || terminal.Elapsed != 10 {
		t.Fatalf("package wall = %+v, want complete ten-second window despite zero parent Elapsed", terminal)
	}
}

func TestFunctionalMonolithCustomerFailureKeepsRetryFocused(t *testing.T) {
	var output bytes.Buffer
	writer := functionalMonolithEventWriter{sink: &output, groups: map[string]string{"A": "original/a", "B": "original/b"}}
	for _, event := range []string{
		`{"Package":"` + functionalMonolithPackage + `","Test":"TestFunctionalPackages/A/TestCustomer","Action":"fail"}`,
		`{"Package":"` + functionalMonolithPackage + `","Test":"TestFunctionalPackages/A","Action":"fail"}`,
		`{"Package":"` + functionalMonolithPackage + `","Test":"TestFunctionalPackages/B","Action":"pass"}`,
		`{"Package":"` + functionalMonolithPackage + `","Test":"TestFunctionalPackages","Action":"fail"}`,
		`{"Package":"` + functionalMonolithPackage + `","Action":"fail"}`,
	} {
		if _, err := writer.Write([]byte(event + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	decision := decideFlakeRetry(output.String(), "", 2)
	if !decision.retry || len(decision.failures) != 1 || decision.failures[0].Package != "original/a" || decision.failures[0].Test != "TestCustomer" {
		t.Fatalf("customer failure lost or retry broadened: %+v; %s", decision, output.String())
	}
	if strings.Contains(output.String(), functionalMonolithPackage) {
		t.Fatalf("coordinator counted as customer failure: %s", output.String())
	}
	if death := decideFlakeRetry(output.String(), "panic: coordinator died", 2); death.retry {
		t.Fatal("panic after a customer failure became an ordinary retry")
	}
}

func TestFunctionalMonolithUnattributedFailureRemainsVisible(t *testing.T) {
	var output bytes.Buffer
	writer := functionalMonolithEventWriter{sink: &output}
	raw := `{"Package":"` + functionalMonolithPackage + `","Action":"fail"}`
	if _, err := writer.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	if err := writer.flush(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), functionalMonolithPackage) || decideFlakeRetry(output.String(), "", 2).retry {
		t.Fatalf("unattributed process failure hidden or retried: %s", output.String())
	}
}

func TestFunctionalMonolithPanicAfterCustomerFailureRejectsRetry(t *testing.T) {
	var output bytes.Buffer
	writer := functionalMonolithEventWriter{sink: &output, groups: map[string]string{"A": "original/a"}}
	for _, event := range []string{
		`{"Package":"` + functionalMonolithPackage + `","Test":"TestFunctionalPackages/A/TestCustomer","Action":"fail"}`,
		`{"Package":"` + functionalMonolithPackage + `","Test":"TestFunctionalPackages/A","Action":"fail"}`,
		`{"Package":"` + functionalMonolithPackage + `","Action":"output","Output":"panic: cleanup died\n"}`,
		`{"Package":"` + functionalMonolithPackage + `","Action":"fail"}`,
	} {
		if _, err := writer.Write([]byte(event + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	decision := decideFlakeRetry(output.String(), "", 2)
	if decision.retry || !strings.Contains(decision.reason, "panic: cleanup died") {
		t.Fatalf("actual coordinator panic was hidden by prior child failure: %+v; %s", decision, output.String())
	}
}
