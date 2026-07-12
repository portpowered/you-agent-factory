package recording_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/recording"
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
