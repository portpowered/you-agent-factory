package configcontractsmoke

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
)

func TestRepositoryConfigurationProjectionsAreDeterministicAndSynchronized(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	diagnostics, err := CheckProjectionParity(repositoryRoot, Families())
	if err != nil {
		t.Fatalf("CheckProjectionParity() error = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("CheckProjectionParity() diagnostics = %#v, want none", diagnostics)
	}
}

func TestProjectionParityReportsExtraFieldWithFamilyAndJSONPath(t *testing.T) {
	family := Families()[0]
	diagnostics := compareProjectionArtifacts([]projectionArtifact{{
		family: family, firstProjection: []byte(`{"type":"object","properties":{}}`),
		secondProjection: []byte(`{"type":"object","properties":{}}`),
		canonical:        []byte(`{"type":"object","properties":{}}`),
		staged:           []byte(`{"type":"object","properties":{},"unexpected":true}`),
	}})
	assertProjectionDiagnostic(t, diagnostics, FamilyGlobal, family.ExportPath, "/unexpected")
}

func TestProjectionParityReportsStaleFactoryComponentAndBothPaths(t *testing.T) {
	family := Families()[2]
	expected := []byte(`{"$defs":{"Worker":{"type":"object"}}}`)
	stale := []byte(`{"$defs":{"Worker":{"type":"string"}}}`)
	diagnostics := compareProjectionArtifacts([]projectionArtifact{{
		family: family, firstProjection: expected, secondProjection: expected,
		canonical: stale, staged: stale,
	}})
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want canonical and staged failures", diagnostics)
	}
	assertProjectionDiagnostic(t, diagnostics[:1], FamilyFactory, family.SchemaProjectionPath, "/$defs/Worker/type")
	assertProjectionDiagnostic(t, diagnostics[1:], FamilyFactory, family.ExportPath, "/$defs/Worker/type")
	if !strings.Contains(diagnostics[1].Message, family.CanonicalOwnerPath) {
		t.Fatalf("staged diagnostic = %#v, want canonical component path", diagnostics[1])
	}
}

func TestProjectionParityReportsStaleGlobalAndMockWorkerExports(t *testing.T) {
	families := Families()[:2]
	for _, family := range families {
		t.Run(string(family.ID), func(t *testing.T) {
			expected := []byte(`{"properties":{"enabled":{"type":"boolean"}}}`)
			stale := []byte(`{"properties":{"enabled":{"type":"string"}}}`)
			diagnostics := compareProjectionArtifacts([]projectionArtifact{{
				family: family, firstProjection: expected, secondProjection: expected,
				canonical: expected, staged: stale,
			}})
			assertProjectionDiagnostic(t, diagnostics, family.ID, family.ExportPath, "/properties/enabled/type")
			if !strings.Contains(diagnostics[0].Message, family.CanonicalOwnerPath) {
				t.Fatalf("diagnostic = %#v, want canonical source path", diagnostics[0])
			}
		})
	}
}

func TestProjectionParityReportsNondeterministicGeneration(t *testing.T) {
	family := Families()[1]
	diagnostics := compareProjectionArtifacts([]projectionArtifact{{
		family: family, firstProjection: []byte(`{"type":"object"}`), secondProjection: []byte(`{"type":"array"}`),
		canonical: []byte(`{"type":"object"}`), staged: []byte(`{"type":"object"}`),
	}})
	assertProjectionDiagnostic(t, diagnostics, FamilyMockWorker, family.ExportPath, "/type")
	if diagnostics[0].Code != "config.projection.nondeterministic" {
		t.Fatalf("diagnostic code = %q, want nondeterministic", diagnostics[0].Code)
	}
}

func TestProjectionParityRejectsForbiddenStagedConfigurationPath(t *testing.T) {
	path := "packages/api/generated/schemas/unauthorized-config.schema.json"
	diagnostic := forbiddenProjectionDiagnostic(path)
	assertProjectionDiagnostic(t, []Diagnostic{diagnostic}, familyConfiguration, path, "allowlist")
}

func assertProjectionDiagnostic(t *testing.T, diagnostics []Diagnostic, family FamilyID, path, messagePart string) {
	t.Helper()
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one", diagnostics)
	}
	diagnostic := diagnostics[0]
	if diagnostic.Family != family || diagnostic.Path != path || !strings.Contains(diagnostic.Message, messagePart) {
		t.Fatalf("diagnostic = %#v, want family %q path %q message containing %q", diagnostic, family, path, messagePart)
	}
}
