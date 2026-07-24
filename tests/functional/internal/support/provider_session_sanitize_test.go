package support

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateProviderSessionFixtureContent_AcceptsSanitizedStructuralValues(t *testing.T) {
	content := []byte(`{
  "session_id": "session_fixture_001",
  "tool_call_id": "call_fixture_abc123",
  "item_id": "item_fixture_1",
  "usage": {"input_tokens": 12, "output_tokens": 34},
  "finish_reason": "stop",
  "error": {"code": "rate_limit_error", "message": "rate limit exceeded"},
  "email": "provider@example.com",
  "workingDirectoryRole": "factory-root"
}
`)
	if err := ValidateProviderSessionFixtureContent("codex-message-tool-success", "stdout.jsonl", content); err != nil {
		t.Fatalf("expected sanitized structural values to pass, got %v", err)
	}
}

func TestValidateProviderSessionFixtureContent_RejectsForbiddenCategories(t *testing.T) {
	tests := []struct {
		name     string
		category string
		path     string
		content  string
		field    string
	}{
		{
			name:     "bearer credential",
			category: ProviderSessionForbiddenCredential,
			path:     "request.json",
			content:  `{"headers":{"Authorization":"Bearer sk-live-super-secret-token-value"}}`,
			field:    "headers.Authorization",
		},
		{
			name:     "api key assignment",
			category: ProviderSessionForbiddenCredential,
			path:     "process.json",
			content:  `{"note":"api_key=sk-abcdefghijklmnopqrstuvwxyz012345"}`,
		},
		{
			name:     "cookie header",
			category: ProviderSessionForbiddenCredential,
			path:     "stderr.txt",
			content:  "cookie: session=raw-secret-cookie-value\n",
		},
		{
			name:     "user home path",
			category: ProviderSessionForbiddenHostPath,
			path:     "process.json",
			content:  `{"cwd":"/Users/abdifamily/infinite-you/factory"}`,
			field:    "cwd",
		},
		{
			name:     "windows home path",
			category: ProviderSessionForbiddenHostPath,
			path:     "process.json",
			content:  `{"cwd":"C:\\Users\\abdifamily\\project"}`,
			field:    "cwd",
		},
		{
			name:     "private git url",
			category: ProviderSessionForbiddenPrivateRepoURL,
			path:     "request.json",
			content:  `{"repo":"git@github.com:portpowered/infinite-you.git"}`,
			field:    "repo",
		},
		{
			name:     "credentialed https url",
			category: ProviderSessionForbiddenPrivateRepoURL,
			path:     "request.json",
			content:  `{"repo":"https://x-access-token:ghs_secret@github.com/org/repo.git"}`,
			field:    "repo",
		},
		{
			name:     "raw env dump object",
			category: ProviderSessionForbiddenEnvDump,
			path:     "process.json",
			content:  `{"env":{"PATH":"/usr/bin","HOME":"/tmp/fixture","TERM":"xterm"}}`,
			field:    "env",
		},
		{
			name:     "raw env dump list",
			category: ProviderSessionForbiddenEnvDump,
			path:     "process.json",
			content:  `{"environ":["PATH=/usr/bin","HOME=/tmp/fixture","TERM=xterm"]}`,
			field:    "environ",
		},
		{
			name:     "unbounded prompt",
			category: ProviderSessionForbiddenUnboundedContent,
			path:     "request.json",
			content:  `{"prompt":"` + strings.Repeat("a", providerSessionMaxPromptRunes+1) + `"}`,
			field:    "prompt",
		},
		{
			name:     "account identifier",
			category: ProviderSessionForbiddenAccountIdentifier,
			path:     "expected-provider-session.json",
			content:  `{"accountId":"acct_live_987654321"}`,
			field:    "accountId",
		},
		{
			name:     "non-fixture email",
			category: ProviderSessionForbiddenAccountIdentifier,
			path:     "stdout.json",
			content:  `{"note":"contact real.user@company.io for access"}`,
			field:    "note",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateProviderSessionFixtureContent("codex-message-tool-success", tc.path, []byte(tc.content))
			assertProviderSessionSanitizeError(t, err, "codex-message-tool-success", tc.category, tc.path, tc.field)
		})
	}
}

func TestValidateProviderSessionCaseSanitization_ScansResolvedFixtures(t *testing.T) {
	caseDir := writeProviderSessionManifestCase(t, validProviderSessionManifest())
	manifest, paths, err := LoadProviderSessionCaseManifest(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCaseManifest: %v", err)
	}

	if err := ValidateProviderSessionManifestSanitization(manifest); err != nil {
		t.Fatalf("manifest sanitization: %v", err)
	}
	if err := ValidateProviderSessionCaseSanitization(manifest.ID, paths); err != nil {
		t.Fatalf("case sanitization on empty fixtures: %v", err)
	}

	unsafe := []byte(`{"Authorization":"Bearer sk-live-case-level-secret-token"}`)
	if err := os.WriteFile(paths.Request, unsafe, 0o644); err != nil {
		t.Fatalf("write unsafe request: %v", err)
	}
	err = ValidateProviderSessionCaseSanitization(manifest.ID, paths)
	assertProviderSessionSanitizeError(t, err, manifest.ID, ProviderSessionForbiddenCredential, paths.Request, "Authorization")
}

func TestValidateProviderSessionManifestSanitization_RejectsCredentialInSource(t *testing.T) {
	manifest := validProviderSessionManifest()
	manifest.Source = "captured with Authorization: Bearer sk-live-manifest-secret-token"
	err := ValidateProviderSessionManifestSanitization(manifest)
	assertProviderSessionSanitizeError(t, err, manifest.ID, ProviderSessionForbiddenCredential, ProviderSessionGoldenManifestFile, "source")
}

func TestValidateProviderSessionFixtureContent_AcceptsNDJSONStructuralEvents(t *testing.T) {
	content := []byte(`{"type":"item.started","item_id":"item_fixture_1","tool_call_id":"call_fixture_1"}
{"type":"item.completed","item_id":"item_fixture_1","finish_reason":"stop","usage":{"input_tokens":3}}
`)
	if err := ValidateProviderSessionFixtureContent("codex-stream", "stdout.jsonl", content); err != nil {
		t.Fatalf("expected NDJSON structural events to pass, got %v", err)
	}
}

func TestValidateProviderSessionCaseSanitization_NamesRelativeFriendlyPath(t *testing.T) {
	caseDir := writeProviderSessionManifestCase(t, validProviderSessionManifest())
	_, paths, err := LoadProviderSessionCaseManifest(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCaseManifest: %v", err)
	}
	rel := ProviderSessionFixtureRelativePath(caseDir, paths.Process)
	if rel != "process.json" {
		t.Fatalf("ProviderSessionFixtureRelativePath = %q, want process.json", rel)
	}
	if !filepath.IsAbs(paths.Process) {
		t.Fatalf("resolved process path should be absolute, got %q", paths.Process)
	}
}

func assertProviderSessionSanitizeError(t *testing.T, err error, wantCaseID, wantCategory, wantPath, wantField string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected sanitize error, got nil")
	}
	var sanitizeErr *ProviderSessionSanitizeError
	if !errors.As(err, &sanitizeErr) {
		t.Fatalf("error type = %T (%v), want *ProviderSessionSanitizeError", err, err)
	}
	message := sanitizeErr.Error()
	if wantCaseID != "" && !strings.Contains(message, wantCaseID) {
		t.Fatalf("error %q does not name case id %q", message, wantCaseID)
	}
	if wantCategory != "" && sanitizeErr.Category != wantCategory {
		t.Fatalf("category = %q, want %q (message=%q)", sanitizeErr.Category, wantCategory, message)
	}
	if wantCategory != "" && !strings.Contains(message, wantCategory) {
		t.Fatalf("error %q does not name category %q", message, wantCategory)
	}
	if wantPath != "" && sanitizeErr.Path != wantPath {
		t.Fatalf("path = %q, want %q (message=%q)", sanitizeErr.Path, wantPath, message)
	}
	if wantPath != "" && !strings.Contains(message, wantPath) {
		t.Fatalf("error %q does not name path %q", message, wantPath)
	}
	if wantField != "" && sanitizeErr.Field != wantField {
		t.Fatalf("field = %q, want %q (message=%q)", sanitizeErr.Field, wantField, message)
	}
	if wantField != "" && !strings.Contains(message, wantField) {
		t.Fatalf("error %q does not name field %q", message, wantField)
	}
}
