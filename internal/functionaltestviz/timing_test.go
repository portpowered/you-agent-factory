package functionaltestviz_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

func TestDecodeFunctionalTimingSummaryAcceptsGocoveragecheckShape(t *testing.T) {
	t.Parallel()

	const raw = `{
  "version": 1,
  "complete": true,
  "wallSeconds": 2.25,
  "packageElapsedSecondsSum": 2.25,
  "packageCount": 2,
  "packages": [
    {"package": "github.com/portpowered/infinite-you/tests/functional/alpha", "seconds": 0.75, "outcome": "fail"},
    {"package": "github.com/portpowered/infinite-you/tests/functional/beta", "seconds": 1.5, "outcome": "pass"}
  ]
}
`
	summary, err := functionaltestviz.DecodeFunctionalTimingSummary([]byte(raw))
	if err != nil {
		t.Fatalf("DecodeFunctionalTimingSummary() error = %v", err)
	}
	if !summary.Complete {
		t.Fatal("Complete = false, want true")
	}
	if summary.WallSeconds != 2.25 {
		t.Fatalf("WallSeconds = %v, want 2.25", summary.WallSeconds)
	}
	if summary.PackageElapsedSecondsSum != 2.25 {
		t.Fatalf("PackageElapsedSecondsSum = %v, want 2.25", summary.PackageElapsedSecondsSum)
	}
	if summary.PackageCount != 2 {
		t.Fatalf("PackageCount = %d, want 2", summary.PackageCount)
	}
	if len(summary.Packages) != 2 {
		t.Fatalf("packages len = %d, want 2", len(summary.Packages))
	}
	if summary.Packages[0].Package != "github.com/portpowered/infinite-you/tests/functional/alpha" || summary.Packages[0].Outcome != "fail" {
		t.Fatalf("packages[0] = %+v, want alpha/fail", summary.Packages[0])
	}
}

func TestDecodeFunctionalTimingSummaryAcceptsTopLevelTestOutcomes(t *testing.T) {
	t.Parallel()

	const raw = `{
  "version": 1,
  "complete": true,
  "wallSeconds": 2.25,
  "packageElapsedSecondsSum": 2.25,
  "expectedPackageCount": 1,
  "packageCount": 1,
  "testCount": 3,
  "testPassCount": 1,
  "testFailCount": 1,
  "testSkipCount": 1,
  "packages": [
    {"package": "github.com/portpowered/infinite-you/tests/functional/inventory", "seconds": 2.25, "outcome": "fail", "reason": "package failed"}
  ],
  "tests": [
    {"package": "github.com/portpowered/infinite-you/tests/functional/inventory", "test": "TestBroken", "seconds": 1.0, "outcome": "fail", "reason": "assertion failed"},
    {"package": "github.com/portpowered/infinite-you/tests/functional/inventory", "test": "TestGreen", "seconds": 1.25, "outcome": "pass"},
    {"package": "github.com/portpowered/infinite-you/tests/functional/inventory", "test": "TestSkipped", "seconds": 0.0, "outcome": "skip"}
  ]
}
`
	summary, err := functionaltestviz.DecodeFunctionalTimingSummary([]byte(raw))
	if err != nil {
		t.Fatalf("DecodeFunctionalTimingSummary() error = %v", err)
	}
	if summary.ExpectedPackageCount != 1 || summary.TestCount != 3 {
		t.Fatalf("counts = expected packages %d, observed tests %d, want 1/3", summary.ExpectedPackageCount, summary.TestCount)
	}
	if summary.TestPassCount != 1 || summary.TestFailCount != 1 || summary.TestSkipCount != 1 {
		t.Fatalf("test outcome counts = %d/%d/%d, want 1/1/1", summary.TestPassCount, summary.TestFailCount, summary.TestSkipCount)
	}
	if len(summary.Tests) != 3 || summary.Tests[0].Reason != "assertion failed" {
		t.Fatalf("tests = %+v, want decoded outcomes and reason", summary.Tests)
	}
}

func TestDecodeFunctionalTimingSummaryAcceptsIncompleteWithoutError(t *testing.T) {
	t.Parallel()

	const raw = `{
  "version": 1,
  "complete": false,
  "wallSeconds": 1.0,
  "packageElapsedSecondsSum": 1.0,
  "packageCount": 1,
  "packages": [
    {"package": "github.com/portpowered/infinite-you/tests/functional/alpha", "seconds": 1.0, "outcome": "pass"}
  ]
}
`
	summary, err := functionaltestviz.DecodeFunctionalTimingSummary([]byte(raw))
	if err != nil {
		t.Fatalf("DecodeFunctionalTimingSummary(incomplete) error = %v, want incomplete accepted without error", err)
	}
	if summary.Complete {
		t.Fatal("Complete = true, want false to be preserved as an explicit diagnostic")
	}
}

func TestDecodeFunctionalTimingSummaryRejectsMalformed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name:    "empty",
			raw:     "",
			wantErr: "empty",
		},
		{
			name:    "invalid json",
			raw:     `{"version":1`,
			wantErr: "invalid functional-timing-summary JSON",
		},
		{
			name: "unsupported version",
			raw: `{"version":2,"complete":true,"wallSeconds":1,"packageElapsedSecondsSum":1,"packageCount":1,
			  "packages":[{"package":"github.com/portpowered/infinite-you/tests/functional/alpha","seconds":1,"outcome":"pass"}]}`,
			wantErr: "unsupported functional-timing-summary version",
		},
		{
			name: "negative wall seconds",
			raw: `{"version":1,"complete":true,"wallSeconds":-1,"packageElapsedSecondsSum":1,"packageCount":1,
			  "packages":[{"package":"github.com/portpowered/infinite-you/tests/functional/alpha","seconds":1,"outcome":"pass"}]}`,
			wantErr: "negative",
		},
		{
			name: "negative package seconds",
			raw: `{"version":1,"complete":true,"wallSeconds":1,"packageElapsedSecondsSum":1,"packageCount":1,
			  "packages":[{"package":"github.com/portpowered/infinite-you/tests/functional/alpha","seconds":-1,"outcome":"pass"}]}`,
			wantErr: "negative",
		},
		{
			name: "missing package identity",
			raw: `{"version":1,"complete":true,"wallSeconds":1,"packageElapsedSecondsSum":1,"packageCount":1,
			  "packages":[{"package":"","seconds":1,"outcome":"pass"}]}`,
			wantErr: "missing package",
		},
		{
			name: "invalid outcome",
			raw: `{"version":1,"complete":true,"wallSeconds":1,"packageElapsedSecondsSum":1,"packageCount":1,
			  "packages":[{"package":"github.com/portpowered/infinite-you/tests/functional/alpha","seconds":1,"outcome":"weird"}]}`,
			wantErr: "invalid outcome",
		},
		{
			name: "duplicate package",
			raw: `{"version":1,"complete":true,"wallSeconds":2,"packageElapsedSecondsSum":2,"packageCount":2,
			  "packages":[
			    {"package":"github.com/portpowered/infinite-you/tests/functional/alpha","seconds":1,"outcome":"pass"},
			    {"package":"github.com/portpowered/infinite-you/tests/functional/alpha","seconds":1,"outcome":"pass"}
			  ]}`,
			wantErr: "duplicate",
		},
		{
			name: "package count mismatch",
			raw: `{"version":1,"complete":true,"wallSeconds":1,"packageElapsedSecondsSum":1,"packageCount":5,
			  "packages":[{"package":"github.com/portpowered/infinite-you/tests/functional/alpha","seconds":1,"outcome":"pass"}]}`,
			wantErr: "packageCount",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := functionaltestviz.DecodeFunctionalTimingSummary([]byte(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("DecodeFunctionalTimingSummary(%s) error = %v, want guidance containing %q", tc.name, err, tc.wantErr)
			}
		})
	}
}

func TestLoadFunctionalTimingSummaryFailsClosedForMissingAndEmptyPath(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing-timing-summary.json")
	_, err := functionaltestviz.LoadFunctionalTimingSummary(missing)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("LoadFunctionalTimingSummary(missing) error = %v, want not-found guidance", err)
	}

	_, err = functionaltestviz.LoadFunctionalTimingSummary("")
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("LoadFunctionalTimingSummary(\"\") error = %v, want required-path guidance", err)
	}
}

func TestLoadFunctionalTimingSummaryReadsFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "functional-timing-summary.json")
	const raw = `{
  "version": 1,
  "complete": true,
  "wallSeconds": 1.0,
  "packageElapsedSecondsSum": 1.0,
  "packageCount": 1,
  "packages": [
    {"package": "github.com/portpowered/infinite-you/tests/functional/alpha", "seconds": 1.0, "outcome": "pass"}
  ]
}
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write timing summary: %v", err)
	}
	summary, err := functionaltestviz.LoadFunctionalTimingSummary(path)
	if err != nil {
		t.Fatalf("LoadFunctionalTimingSummary() error = %v", err)
	}
	if summary.PackageCount != 1 {
		t.Fatalf("PackageCount = %d, want 1", summary.PackageCount)
	}
}
