package artifacts_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	recording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/artifacts"
)

func TestContractFixtures(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"valid-v2.json", "valid-v2-checkpoint.json", "valid-v1.json"} {
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

func TestVersionPinnedSchemaV1FixturePreservesFactorySessionFacts(t *testing.T) {
	t.Parallel()
	assertVersionPinnedFixture(t, "valid-v1.json", versionPinnedFixtureExpectation{
		schemaVersion: "1", sessionID: "session-historical-001", sourceRef: "workflow/historical.js",
		eventIDs: []string{"event-historical-1"},
	})
}

func TestVersionPinnedSchemaV2FixturePreservesFactorySessionFacts(t *testing.T) {
	t.Parallel()
	assertVersionPinnedFixture(t, "valid-v2.json", versionPinnedFixtureExpectation{
		schemaVersion: "2", sessionID: "session-js-001", sourceRef: "workflow/example.js",
		eventIDs: []string{"event-1", "event-2"}, artifactCount: 1, result: true,
	})
}

func TestVersionPinnedSchemaV2CheckpointFixturePreservesFactorySessionFacts(t *testing.T) {
	t.Parallel()
	assertVersionPinnedFixture(t, "valid-v2-checkpoint.json", versionPinnedFixtureExpectation{
		schemaVersion: "2", sessionID: "session-js-checkpoint-001", sourceRef: "workflow/checkpoint.js",
		eventIDs: []string{"event-started", "event-checkpoint", "event-completed"}, artifactCount: 1, result: true, checkpoint: true,
	})
}

type versionPinnedFixtureExpectation struct {
	schemaVersion string
	sessionID     string
	sourceRef     string
	eventIDs      []string
	artifactCount int
	result        bool
	checkpoint    bool
}

func assertVersionPinnedFixture(t *testing.T, name string, want versionPinnedFixtureExpectation) {
	t.Helper()
	value, err := loadFixture(name)
	if err != nil {
		t.Fatalf("DecodeAndValidate() error = %v", err)
	}
	if value.SchemaVersion != want.schemaVersion || value.ReplayCompatibilityVersion != recording.ReplayCompatibilityVersion {
		t.Fatalf("compatibility = %q/%q", value.SchemaVersion, value.ReplayCompatibilityVersion)
	}
	if value.Session.ID != want.sessionID || value.Source.Ref != want.sourceRef {
		t.Fatalf("identity/source = %#v/%#v", value.Session, value.Source)
	}
	if len(value.Events) != len(want.eventIDs) || len(value.Artifacts) != want.artifactCount {
		t.Fatalf("summary counts = events:%d artifacts:%d", len(value.Events), len(value.Artifacts))
	}
	for index, eventID := range want.eventIDs {
		if value.Events[index].ID != eventID || value.Events[index].Sequence != int64(index) {
			t.Fatalf("event[%d] = %#v, want %q at sequence %d", index, value.Events[index], eventID, index)
		}
	}
	if (value.Result != nil) != want.result || (value.Checkpoint != nil) != want.checkpoint {
		t.Fatalf("versioned optional facts = result:%t checkpoint:%t", value.Result != nil, value.Checkpoint != nil)
	}
	if value.Redaction != (recording.RedactionMetadata{
		RuntimeStateOmitted: true, CheckpointBodiesOmitted: true,
		ProviderTranscriptsOmitted: true, ChildDispatchesOmitted: true,
		SecretsRedacted: value.Redaction.SecretsRedacted,
	}) {
		t.Fatalf("redaction metadata = %#v", value.Redaction)
	}
	assertVersionPinnedRoundTrip(t, value)
}

func assertVersionPinnedRoundTrip(t *testing.T, value recording.Recording) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	decoded, err := recording.DecodeAndValidate(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("round-trip DecodeAndValidate() error = %v", err)
	}
	if !reflect.DeepEqual(value, decoded) {
		t.Fatalf("round-trip changed versioned facts:\nwant=%#v\ngot=%#v", value, decoded)
	}
}

func TestSupportedSchemaCorruptionKeepsItsOwnedDiagnostic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		code   recording.DiagnosticCode
		path   string
		mutate func(*recording.Recording)
	}{
		{name: "identity", code: recording.CodeInvalidIdentity, path: "session.id", mutate: func(value *recording.Recording) { value.Session.ID = "" }},
		{name: "digest", code: recording.CodeInvalidDigest, path: "source.hash", mutate: func(value *recording.Recording) { value.Source.Hash = "sha256:not-a-digest" }},
		{name: "order", code: recording.CodeInvalidOrder, path: "events[1].sequence", mutate: func(value *recording.Recording) { value.Events[1].Sequence = value.Events[0].Sequence }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value, err := loadFixture("valid-v2.json")
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&value)
			payload, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			_, err = recording.DecodeAndValidate(bytes.NewReader(payload))
			assertDiagnostic(t, err, test.code, test.path)
		})
	}

	value, err := loadFixture("valid-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	document["unsupportedField"] = json.RawMessage(`true`)
	payload, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	_, err = recording.DecodeAndValidate(bytes.NewReader(payload))
	assertDiagnostic(t, err, recording.CodeMalformedContract, "")
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
	assertDiagnostic(t, recording.Validate(valid), recording.CodeInvalidOrder, "events[1].sequence")
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
