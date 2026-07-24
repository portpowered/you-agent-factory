package sharedsurfaceownership_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/sharedsurfaceownership"
	"github.com/portpowered/infinite-you/internal/testutil"
)

func TestValidateFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fixture    string
		wantOK     bool
		wantSubstr string
	}{
		{
			name:    "valid empty surfaces model",
			fixture: "valid-model-empty-surfaces.json",
			wantOK:  true,
		},
		{
			name:    "valid surface with ordered owner-request queue",
			fixture: "valid-model-with-queued-request.json",
			wantOK:  true,
		},
		{
			name:       "empty integrator lane",
			fixture:    "invalid-empty-integrator-lane.json",
			wantSubstr: "serial integrator",
		},
		{
			name:       "dual integrators",
			fixture:    "invalid-dual-integrators.json",
			wantSubstr: "exactly one serial integrator",
		},
		{
			name:       "unordered owner-request queue",
			fixture:    "invalid-unordered-owner-requests.json",
			wantSubstr: "owner-request queue",
		},
		{
			name:       "duplicate queued owner requests",
			fixture:    "invalid-duplicate-owner-requests.json",
			wantSubstr: "duplicate",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join("testdata", test.fixture)
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			diagnostics := sharedsurfaceownership.ValidateDocument(path, payload)
			if test.wantOK {
				if len(diagnostics) != 0 {
					t.Fatalf("diagnostics = %#v, want none", diagnostics)
				}
				return
			}
			if len(diagnostics) == 0 {
				t.Fatal("expected validation to fail")
			}
			joined := joinDiagnostics(diagnostics)
			if !strings.Contains(strings.ToLower(joined), strings.ToLower(test.wantSubstr)) {
				t.Fatalf("diagnostics = %q, want substring %q", joined, test.wantSubstr)
			}
		})
	}
}

func TestCanonicalInventoryMatchesModelContract(t *testing.T) {
	t.Parallel()

	path := testutil.MustRepoPath(t, sharedsurfaceownership.CanonicalInventoryRelPath)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read canonical inventory: %v", err)
	}
	diagnostics := sharedsurfaceownership.ValidateDocument(
		sharedsurfaceownership.CanonicalInventoryRelPath,
		payload,
	)
	if len(diagnostics) != 0 {
		t.Fatalf("canonical inventory diagnostics = %#v", diagnostics)
	}
}

func TestModelDocsDeclareMetadataOnlySchedulingContract(t *testing.T) {
	t.Parallel()

	path := testutil.MustRepoPath(t, sharedsurfaceownership.CanonicalModelDocRelPath)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read model doc: %v", err)
	}
	text := string(payload)
	required := []string{
		"serial integrator",
		"owner-request queue",
		"integration metadata only",
		"does not authorize",
		"head",
		"owner-local",
	}
	for _, needle := range required {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(needle)) {
			t.Fatalf("model doc missing required phrase %q", needle)
		}
	}
}

func joinDiagnostics(diagnostics []sharedsurfaceownership.Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, diagnostic.Rule+" "+diagnostic.Path+" "+diagnostic.Message)
	}
	return strings.Join(parts, " | ")
}
