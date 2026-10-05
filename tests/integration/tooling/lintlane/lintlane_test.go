//go:build integration

package lintlane_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This is the public report-v1 representation consumed by CI, independent of
// the command's private implementation types.
type report struct {
	Version             int            `json:"version"`
	Jobs                int            `json:"jobs"`
	TotalDurationMillis int64          `json:"totalDurationMillis"`
	Targets             []reportTarget `json:"targets"`
}

type reportTarget struct {
	Name                 string `json:"name"`
	Status               string `json:"status"`
	DurationMillis       int64  `json:"durationMillis"`
	Output               string `json:"output"`
	Error                string `json:"error"`
	ViolationCount       *int   `json:"violationCount"`
	ViolationCountSource string `json:"violationCountSource"`
}

func TestDeliveredRunnerCompletesAndReportsEveryMakeTarget(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprintf("mixed=%t", mixed), func(t *testing.T) {
			t.Parallel()
			artifact := os.Getenv("YOU_LINTLANE_ARTIFACT")
			if !filepath.IsAbs(artifact) {
				t.Fatal("YOU_LINTLANE_ARTIFACT must name the invoking build's absolute prebuilt artifact")
			}
			makeTool, err := exec.LookPath("make")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			failure := ""
			if mixed {
				failure = "\t@echo 'LINT_VIOLATION_COUNT: 1';\n\t@exit 7\n"
			}
			// Semicolons force GNU Make to use its shell for builtins on Windows.
			fixture := ".PHONY: first second third\n"
			for _, target := range []string{"first", "second", "third"} {
				fixture += target + ":\n\t@echo 'stdout:" + target + "';\n\t@echo 'stderr:" + target + "' >&2\n\t@echo \"env:$$LINTLANE_SCENARIO\"\n"
				if target == "first" {
					fixture += failure
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(fixture), 0o600); err != nil {
				t.Fatal(err)
			}
			reportPath := filepath.Join(dir, "report.json")
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			command := exec.CommandContext(ctx, artifact, "-make", makeTool, "-jobs", "1", "-report-file", reportPath, "--", "first", "second", "third")
			command.Dir = dir
			command.Env = append(os.Environ(), "LINTLANE_SCENARIO="+t.Name(), "MAKEFLAGS=", "MFLAGS=", "MAKEOVERRIDES=")
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			err = command.Run()
			code := 0
			if err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatalf("run artifact: %v", err)
				}
				code = exit.ExitCode()
			}
			wantCode := 0
			if mixed {
				wantCode = 1
			}
			if code != wantCode {
				t.Fatalf("exit = %d, want %d; stdout=%s stderr=%s", code, wantCode, stdout.String(), stderr.String())
			}
			t.Logf("artifact=%s exit=%d fixture=%s", artifact, code, fixture)
			assertReport(t, reportPath, stdout.String(), stderr.String(), mixed)
		})
	}
}

func assertReport(t *testing.T, path, stdout, stderr string, mixed bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Jobs != 1 || got.TotalDurationMillis < 0 || len(got.Targets) != 3 {
		t.Fatalf("report metadata = %+v", got)
	}
	previous := -1
	for i, name := range []string{"first", "second", "third"} {
		assertTarget(t, got.Targets[i], name, stdout, mixed && i == 0)
		position := strings.Index(stdout, "===== lint target: "+name+" =====")
		if position <= previous || !strings.Contains(stdout, "===== lint target: "+name+": "+strings.ToUpper(got.Targets[i].Status)+" =====") {
			t.Fatalf("target order/status missing: %s", stdout)
		}
		previous = position
	}
	assertSummary(t, stdout, stderr, mixed)
}

func assertSummary(t *testing.T, stdout, stderr string, mixed bool) {
	t.Helper()
	if mixed {
		if !strings.Contains(stdout, "LINT FAILED: 1 target(s)") || !strings.Contains(stdout, "first (rerun: make first)") || !strings.Contains(stderr, "lint failed for target(s): first") {
			t.Fatalf("failure summary missing: stdout=%s stderr=%s", stdout, stderr)
		}
	} else if !strings.Contains(stdout, "LINT PASSED: 3 target(s) completed successfully") || stderr != "" {
		t.Fatalf("success summary: stdout=%s stderr=%s", stdout, stderr)
	}
}

func assertTarget(t *testing.T, target reportTarget, name, stdout string, failed bool) {
	t.Helper()
	status, count, source := "pass", 0, "successful-check"
	if failed {
		status, count, source = "fail", 1, "checker-marker"
	}
	if target.Name != name || target.Status != status || target.DurationMillis < 0 {
		t.Fatalf("target = %+v, want %s/%s with duration", target, name, status)
	}
	if target.ViolationCount == nil || *target.ViolationCount != count || target.ViolationCountSource != source {
		t.Fatalf("target count = %+v, want %d/%s", target, count, source)
	}
	if (target.Error != "") != failed {
		t.Fatalf("target error = %q, failed = %t", target.Error, failed)
	}
	for _, diagnostic := range []string{"stdout:" + name, "stderr:" + name, "env:" + t.Name()} {
		if !strings.Contains(target.Output, diagnostic) || !strings.Contains(stdout, diagnostic) {
			t.Fatalf("missing %q in report/output: %+v / %s", diagnostic, target, stdout)
		}
	}
}
