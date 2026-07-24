package sharedsurfaceownership_test

import (
	"encoding/json"
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
		{
			name:       "openapi-http surface mapped away from PSS-I02",
			fixture:    "invalid-openapi-http-wrong-lane.json",
			wantSubstr: "PSS-I02",
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

func TestCanonicalInventoryIncludesPSSI02OpenAPIHTTPSurfaces(t *testing.T) {
	t.Parallel()

	path := testutil.MustRepoPath(t, sharedsurfaceownership.CanonicalInventoryRelPath)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read canonical inventory: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		t.Fatalf("decode canonical inventory: %v", err)
	}
	surfaces, _ := root["surfaces"].(map[string]any)
	for _, surfaceID := range sharedsurfaceownership.RequiredPSSI02SurfaceIDs {
		raw, ok := surfaces[surfaceID]
		if !ok {
			t.Fatalf("canonical inventory missing required PSS-I02 surface %q", surfaceID)
		}
		surface, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("surface %q must be an object", surfaceID)
		}
		if got, _ := surface["protocolFamily"].(string); got != "openapi-http" {
			t.Fatalf("surface %q protocolFamily = %q, want openapi-http", surfaceID, got)
		}
		if got, _ := surface["serialIntegratorLaneId"].(string); got != "PSS-I02" {
			t.Fatalf("surface %q serialIntegratorLaneId = %q, want PSS-I02", surfaceID, got)
		}
		if surface["activeHolder"] != nil {
			t.Fatalf("surface %q activeHolder must be null; this packet performs no cutover", surfaceID)
		}
		queue, ok := surface["ownerRequestQueue"].([]any)
		if !ok {
			t.Fatalf("surface %q must declare an ownerRequestQueue array", surfaceID)
		}
		if len(queue) != 0 {
			t.Fatalf("surface %q ownerRequestQueue must be empty until an HTTP-* adapter cutover is accepted", surfaceID)
		}
		summary, _ := surface["exclusiveChangedPathSummary"].(string)
		if strings.TrimSpace(summary) == "" {
			t.Fatalf("surface %q requires exclusiveChangedPathSummary", surfaceID)
		}
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
		"PSS-I02",
		"openapi-http",
		"service-owned",
		"concurrent",
		"public contract",
		"package motion",
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
