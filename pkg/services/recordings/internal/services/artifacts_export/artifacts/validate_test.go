package artifacts_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	recording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/artifacts"
)

func TestContractFixtures(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"valid-v2.json", "valid-v1.json"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := loadFixture(name)
			if err != nil {
				t.Fatalf("DecodeAndValidate() error = %v", err)
			}
			if got.Session.ID == "" {
				t.Fatal("validated fixture has empty session identity")
			}
		})
	}
}

func TestMalformedFixtureReturnsTypedAreaDiagnostic(t *testing.T) {
	_, err := loadFixture("malformed.json")
	var diagnostic *recording.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %T %v, want *Diagnostic", err, err)
	}
	if diagnostic.Code != recording.CodeInvalidDigest || diagnostic.Area != "digest" || diagnostic.Path != "source.hash" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestUnsupportedCompatibilityIsRejectedBeforeMalformedBody(t *testing.T) {
	_, err := loadFixture("unsupported.json")
	var diagnostic *recording.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %T %v, want *Diagnostic", err, err)
	}
	if diagnostic.Code != recording.CodeUnsupportedVersion || diagnostic.Area != "compatibility" || diagnostic.Path != "replayCompatibilityVersion" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if len(diagnostic.SupportedVersions) != 1 || diagnostic.SupportedVersions[0] != recording.ReplayCompatibilityVersion {
		t.Fatalf("supported versions = %#v", diagnostic.SupportedVersions)
	}
}

func TestValidationRejectsEventOrderingAndUnknownArtifactReferences(t *testing.T) {
	valid, err := loadFixture("valid-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	valid.Events[1].Sequence = valid.Events[0].Sequence
	assertDiagnostic(t, recording.Validate(valid), recording.CodeInvalidSummary, "events[1].sequence")
	valid, _ = loadFixture("valid-v2.json")
	valid.Events[1].ArtifactIDs[0] = "missing"
	assertDiagnostic(t, recording.Validate(valid), recording.CodeInvalidSummary, "events[1].artifactIds[0]")
}

func TestStrictDecodeRejectsProhibitedOrUnknownDetail(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "valid-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-2], []byte(",\n  \"runtimeState\": {\"secret\": true}\n}\n")...)
	_, err = recording.DecodeAndValidate(bytes.NewReader(data))
	assertDiagnostic(t, err, recording.CodeMalformedContract, "")
}

func TestValidationRejectsMalformedPublicSummaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		code   recording.DiagnosticCode
		path   string
		mutate func(*recording.Recording)
	}{
		{"missing session id", recording.CodeInvalidIdentity, "session.id", func(r *recording.Recording) { r.Session.ID = "" }},
		{"unknown session status", recording.CodeInvalidSummary, "session.status", func(r *recording.Recording) { r.Session.Status = "UNKNOWN" }},
		{"wrong orchestrator", recording.CodeInvalidSummary, "session.orchestratorKind", func(r *recording.Recording) { r.Session.OrchestratorKind = "GO" }},
		{"missing source ref", recording.CodeInvalidSummary, "source.ref", func(r *recording.Recording) { r.Source.Ref = "" }},
		{"incomplete artifact", recording.CodeInvalidSummary, "artifacts[0]", func(r *recording.Recording) { r.Artifacts[0].Kind = "" }},
		{"duplicate artifact", recording.CodeInvalidIdentity, "artifacts[1].id", func(r *recording.Recording) { r.Artifacts = append(r.Artifacts, r.Artifacts[0]) }},
		{"artifact digest", recording.CodeInvalidDigest, "artifacts[0].contentHash", func(r *recording.Recording) { r.Artifacts[0].ContentHash = "invalid" }},
		{"negative artifact size", recording.CodeInvalidSummary, "artifacts[0].sizeBytes", func(r *recording.Recording) { r.Artifacts[0].SizeBytes = -1 }},
		{"incomplete event", recording.CodeInvalidSummary, "events[0]", func(r *recording.Recording) { r.Events[0].Type = "" }},
		{"duplicate event", recording.CodeInvalidIdentity, "events[1].id", func(r *recording.Recording) { r.Events[1].ID = r.Events[0].ID }},
		{"unsupported result status", recording.CodeInvalidSummary, "result.status", func(r *recording.Recording) { r.Result.Status = "UNKNOWN" }},
		{"unsupported result mode", recording.CodeInvalidSummary, "result.mode", func(r *recording.Recording) { r.Result.Mode = "unknown" }},
		{"unknown result artifact", recording.CodeInvalidSummary, "result.artifactIds[0]", func(r *recording.Recording) { r.Result.ArtifactIDs[0] = "missing" }},
		{"hash without result", recording.CodeInvalidDigest, "result.contentHash", func(r *recording.Recording) { r.Result.ContentHash = digestForTest('5') }},
		{"invalid inline result", recording.CodeInvalidSummary, "result.primaryResult", func(r *recording.Recording) { r.Result.PrimaryResult = []byte(`{`) }},
		{"failed final result", recording.CodeInvalidSummary, "result.status", func(r *recording.Recording) { r.Session.Status = "FAILED" }},
		{"omission flag false", recording.CodeInvalidSummary, "redaction", func(r *recording.Recording) { r.Redaction.RuntimeStateOmitted = false }},
		{"negative redaction count", recording.CodeInvalidSummary, "redaction.secretsRedacted", func(r *recording.Recording) { r.Redaction.SecretsRedacted = -1 }},
		{"redaction count above maximum", recording.CodeInvalidSummary, "redaction.secretsRedacted", func(r *recording.Recording) { r.Redaction.SecretsRedacted = recording.MaxSecretsRedacted + 1 }},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			value, err := loadFixture("valid-v2.json")
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&value)
			assertDiagnostic(t, recording.Validate(value), tc.code, tc.path)
		})
	}
}

func TestValidationRejectsMalformedCheckpointSummaries(t *testing.T) {
	valid, err := loadFixture("valid-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	valid.Checkpoint = &recording.CheckpointSummary{ID: "checkpoint-1", Timestamp: valid.Events[0].Timestamp}
	assertDiagnostic(t, recording.Validate(valid), recording.CodeInvalidSummary, "checkpoint.id")

	valid.Events[0].CheckpointID = "checkpoint-1"
	valid.Checkpoint.ArtifactID = "missing"
	assertDiagnostic(t, recording.Validate(valid), recording.CodeInvalidSummary, "checkpoint.artifactId")

	valid.Checkpoint.ArtifactID = "artifact-1"
	valid.Checkpoint.ID = ""
	assertDiagnostic(t, recording.Validate(valid), recording.CodeInvalidSummary, "checkpoint")
}

func TestDiagnosticErrorIncludesPathWhenPresent(t *testing.T) {
	if got := (*recording.Diagnostic)(nil).Error(); got != "" {
		t.Fatalf("nil diagnostic error = %q", got)
	}
	withoutPath := &recording.Diagnostic{Message: "message"}
	if got := withoutPath.Error(); got != "message" {
		t.Fatalf("diagnostic error = %q", got)
	}
	withPath := &recording.Diagnostic{Path: "session.id", Message: "is required"}
	if got := withPath.Error(); got != "session.id: is required" {
		t.Fatalf("diagnostic error = %q", got)
	}
}

func digestForTest(character byte) string {
	return "sha256:" + string(bytes.Repeat([]byte{character}, 64))
}

func loadFixture(name string) (recording.Recording, error) {
	file, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		return recording.Recording{}, err
	}
	defer file.Close()
	return recording.DecodeAndValidate(file)
}

func assertDiagnostic(t *testing.T, err error, code recording.DiagnosticCode, path string) {
	t.Helper()
	var diagnostic *recording.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %T %v, want *Diagnostic", err, err)
	}
	if diagnostic.Code != code || diagnostic.Path != path {
		t.Fatalf("diagnostic = %#v, want code %s path %q", diagnostic, code, path)
	}
}
