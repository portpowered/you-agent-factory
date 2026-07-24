package support

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProviderSessionCaseManifest_ValidCase(t *testing.T) {
	caseDir := writeProviderSessionManifestCase(t, validProviderSessionManifest())

	manifest, paths, err := LoadProviderSessionCaseManifest(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCaseManifest: %v", err)
	}

	if manifest.ID != "codex-message-tool-success" {
		t.Fatalf("manifest.ID = %q, want codex-message-tool-success", manifest.ID)
	}
	if manifest.FidelityClass != ProviderSessionFidelityPartialStream {
		t.Fatalf("manifest.FidelityClass = %q, want %q", manifest.FidelityClass, ProviderSessionFidelityPartialStream)
	}
	if got := filepath.Base(paths.Request); got != "request.json" {
		t.Fatalf("paths.Request base = %q, want request.json", got)
	}
	if got := filepath.Base(paths.Stdout); got != "stdout.jsonl" {
		t.Fatalf("paths.Stdout base = %q, want stdout.jsonl", got)
	}
	if got := filepath.Base(paths.ExpectedResponseEvents); got != "expected-response-events.ndjson" {
		t.Fatalf("paths.ExpectedResponseEvents base = %q, want expected-response-events.ndjson", got)
	}
	for _, abs := range []string{
		paths.Request,
		paths.Process,
		paths.Stdout,
		paths.Stderr,
		paths.ExpectedProviderSession,
		paths.ExpectedResponseEvents,
		paths.ExpectedInvocationResult,
	} {
		if !strings.HasPrefix(abs, caseDir+string(filepath.Separator)) {
			t.Fatalf("resolved path %q is outside case dir %s", abs, caseDir)
		}
		if _, err := os.Stat(abs); err != nil {
			t.Fatalf("resolved fixture %s: %v", abs, err)
		}
	}
}

func TestValidateProviderSessionGoldenManifest_RejectsInvalidFidelityClass(t *testing.T) {
	manifest := validProviderSessionManifest()
	manifest.FidelityClass = "streaming"

	err := ValidateProviderSessionGoldenManifest(manifest)
	assertProviderSessionManifestError(t, err, "codex-message-tool-success", "fidelityClass", "allowed-values")
}

func TestValidateProviderSessionGoldenManifest_RejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		field string
		rule  string
		mut   func(*ProviderSessionGoldenManifest)
	}{
		{
			name:  "schemaVersion",
			field: "schemaVersion",
			rule:  "schema-version",
			mut:   func(m *ProviderSessionGoldenManifest) { m.SchemaVersion = 2 },
		},
		{
			name:  "id",
			field: "id",
			rule:  "required",
			mut:   func(m *ProviderSessionGoldenManifest) { m.ID = "" },
		},
		{
			name:  "provider",
			field: "provider",
			rule:  "required",
			mut:   func(m *ProviderSessionGoldenManifest) { m.Provider = "" },
		},
		{
			name:  "providerVersion",
			field: "providerVersion",
			rule:  "required",
			mut:   func(m *ProviderSessionGoldenManifest) { m.ProviderVersion = " " },
		},
		{
			name:  "case",
			field: "case",
			rule:  "required",
			mut:   func(m *ProviderSessionGoldenManifest) { m.Case = "" },
		},
		{
			name:  "sanitizerVersion",
			field: "sanitizerVersion",
			rule:  "sanitizer-version",
			mut:   func(m *ProviderSessionGoldenManifest) { m.SanitizerVersion = 0 },
		},
		{
			name:  "source",
			field: "source",
			rule:  "required",
			mut:   func(m *ProviderSessionGoldenManifest) { m.Source = "" },
		},
		{
			name:  "normalizedFields",
			field: "normalizedFields",
			rule:  "required",
			mut:   func(m *ProviderSessionGoldenManifest) { m.NormalizedFields = nil },
		},
		{
			name:  "requestFile",
			field: "requestFile",
			rule:  "required",
			mut:   func(m *ProviderSessionGoldenManifest) { m.RequestFile = "" },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest := validProviderSessionManifest()
			tc.mut(&manifest)
			err := ValidateProviderSessionGoldenManifest(manifest)
			wantCaseID := "codex-message-tool-success"
			if tc.field == "id" {
				wantCaseID = "(unknown)"
			}
			assertProviderSessionManifestError(t, err, wantCaseID, tc.field, tc.rule)
		})
	}
}

func TestLoadProviderSessionCaseManifest_RejectsAbsoluteAndEscapingPointers(t *testing.T) {
	t.Run("absolute", func(t *testing.T) {
		manifest := validProviderSessionManifest()
		manifest.StdoutFile = "/tmp/stdout.jsonl"
		caseDir := writeProviderSessionManifestCase(t, manifest)
		_, _, err := LoadProviderSessionCaseManifest(caseDir)
		assertProviderSessionManifestError(t, err, "codex-message-tool-success", "stdoutFile", "relative-path")
	})

	t.Run("escape", func(t *testing.T) {
		manifest := validProviderSessionManifest()
		manifest.ProcessFile = "../secrets.json"
		caseDir := writeProviderSessionManifestCase(t, manifest)
		_, _, err := LoadProviderSessionCaseManifest(caseDir)
		assertProviderSessionManifestError(t, err, "codex-message-tool-success", "processFile", "relative-path")
	})
}

func TestLoadProviderSessionCaseManifest_RejectsMissingResolvedFixture(t *testing.T) {
	caseDir := writeProviderSessionManifestCase(t, validProviderSessionManifest())
	missingPath := filepath.Join(caseDir, "expected-invocation-result.json")
	if err := os.Remove(missingPath); err != nil {
		t.Fatalf("remove expected-invocation-result.json: %v", err)
	}

	_, _, err := LoadProviderSessionCaseManifest(caseDir)
	assertProviderSessionMissingFixtureError(
		t,
		err,
		"codex-message-tool-success",
		"expected-invocation-result",
		missingPath,
	)
}

func TestLoadProviderSessionCaseManifest_RejectsMalformedJSON(t *testing.T) {
	caseDir := t.TempDir()
	path := filepath.Join(caseDir, ProviderSessionGoldenManifestFile)
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,`), 0o644); err != nil {
		t.Fatalf("write malformed manifest: %v", err)
	}

	_, _, err := LoadProviderSessionCaseManifest(caseDir)
	assertProviderSessionManifestError(t, err, "(unknown)", "", "manifest.json")
}

func validProviderSessionManifest() ProviderSessionGoldenManifest {
	return ProviderSessionGoldenManifest{
		SchemaVersion:                ProviderSessionGoldenManifestSchemaVersion,
		ID:                           "codex-message-tool-success",
		Provider:                     "codex",
		ProviderVersion:              "fixture-1",
		FidelityClass:                ProviderSessionFidelityPartialStream,
		Case:                         "success",
		StdoutFile:                   "stdout.jsonl",
		StderrFile:                   "stderr.txt",
		RequestFile:                  "request.json",
		ProcessFile:                  "process.json",
		ExpectedProviderSessionFile:  "expected-provider-session.json",
		ExpectedResponseEventsFile:   "expected-response-events.ndjson",
		ExpectedInvocationResultFile: "expected-invocation-result.json",
		NormalizedFields:             []string{"eventId", "recordedAt", "factorySessionId", "runId"},
		SanitizerVersion:             ProviderSessionGoldenSanitizerVersion,
		Source:                       "sanitized-provider-exec",
	}
}

func writeProviderSessionManifestCase(t *testing.T, manifest ProviderSessionGoldenManifest) string {
	t.Helper()

	caseDir := t.TempDir()
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, ProviderSessionGoldenManifestFile), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	files := []string{
		manifest.RequestFile,
		manifest.ProcessFile,
		manifest.StdoutFile,
		manifest.StderrFile,
		manifest.ExpectedProviderSessionFile,
		manifest.ExpectedResponseEventsFile,
		manifest.ExpectedInvocationResultFile,
	}
	for _, name := range files {
		if strings.TrimSpace(name) == "" || filepath.IsAbs(name) || strings.Contains(name, "..") {
			continue
		}
		path := filepath.Join(caseDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	return caseDir
}

func assertProviderSessionManifestError(t *testing.T, err error, wantCaseID, wantField, wantRule string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected manifest error, got nil")
	}
	var manifestErr *ProviderSessionManifestError
	if !errors.As(err, &manifestErr) {
		t.Fatalf("error type = %T (%v), want *ProviderSessionManifestError", err, err)
	}
	message := manifestErr.Error()
	if wantCaseID != "" && !strings.Contains(message, wantCaseID) {
		t.Fatalf("error %q does not name case id %q", message, wantCaseID)
	}
	if wantField != "" && manifestErr.Field != wantField {
		t.Fatalf("error field = %q, want %q (message=%q)", manifestErr.Field, wantField, message)
	}
	if wantRule != "" && manifestErr.Rule != wantRule {
		t.Fatalf("error rule = %q, want %q (message=%q)", manifestErr.Rule, wantRule, message)
	}
	if wantField != "" && !strings.Contains(message, wantField) {
		t.Fatalf("error %q does not name field %q", message, wantField)
	}
	if wantRule != "" && !strings.Contains(message, wantRule) {
		t.Fatalf("error %q does not name rule %q", message, wantRule)
	}
}
