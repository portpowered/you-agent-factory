package sharedsurfaceownership_test

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/sharedsurfaceownership"
	"github.com/portpowered/infinite-you/internal/testutil"
)

type fixtureCase struct {
	name       string
	fixture    string
	complete   bool
	wantOK     bool
	wantSubstr string
	wantRule   string
}

func TestValidateFixtures(t *testing.T) {
	t.Parallel()
	runFixtureCases(t, []fixtureCase{
		{name: "valid empty surfaces model", fixture: "valid-model-empty-surfaces.json", wantOK: true},
		{name: "valid surface with ordered owner-request queue", fixture: "valid-model-with-queued-request.json", wantOK: true},
		{name: "empty integrator lane", fixture: "invalid-empty-integrator-lane.json", wantSubstr: "serial integrator", wantRule: "inventory.empty_serial_integrator"},
		{name: "dual integrators mapped to more than one PSS lane", fixture: "invalid-dual-integrators.json", wantSubstr: "more than one of PSS-I02/I03/I04", wantRule: "inventory.dual_integrators"},
		{name: "unordered owner-request queue", fixture: "invalid-unordered-owner-requests.json", wantSubstr: "owner-request queue"},
		{name: "duplicate queued owner requests", fixture: "invalid-duplicate-owner-requests.json", wantSubstr: "duplicate"},
		{name: "openapi-http surface mapped away from PSS-I02", fixture: "invalid-openapi-http-wrong-lane.json", wantSubstr: "PSS-I02"},
		{name: "cli surface mapped away from PSS-I03", fixture: "invalid-cli-wrong-lane.json", wantSubstr: "PSS-I03"},
		{name: "mcp surface mapped away from PSS-I04", fixture: "invalid-mcp-wrong-lane.json", wantSubstr: "PSS-I04"},
	})
}

func TestValidateCompleteInventoryFailClosedFixtures(t *testing.T) {
	t.Parallel()
	runFixtureCases(t, []fixtureCase{
		{name: "required portfolio hold marked bypassable", fixture: "invalid-hold-bypassable.json", wantSubstr: "bypass"},
		{name: "required portfolio hold attached to wrong serial lane", fixture: "invalid-hold-wrong-lane.json", wantSubstr: "PSS-I03"},
		{name: "complete surface family missing required portfolio hold", fixture: "invalid-missing-required-portfolio-holds.json", wantSubstr: "required portfolio hold", wantRule: "inventory.missing_required_portfolio_hold"},
		{name: "complete inventory missing required surface family", fixture: "invalid-missing-surface-family.json", complete: true, wantSubstr: "required PSS-I04 surface family", wantRule: "inventory.missing_required_surface_family"},
		{name: "surface missing owner-request queue model", fixture: "invalid-missing-owner-request-queue.json", wantSubstr: "owner-request queue", wantRule: "inventory.missing_owner_request_queue"},
	})
}

func runFixtureCases(t *testing.T, tests []fixtureCase) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join("testdata", test.fixture)
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			var diagnostics []sharedsurfaceownership.Diagnostic
			if test.complete {
				diagnostics = sharedsurfaceownership.ValidateCompleteDocument(path, payload)
			} else {
				diagnostics = sharedsurfaceownership.ValidateDocument(path, payload)
			}
			assertFixtureDiagnostics(t, diagnostics, test)
		})
	}
}

func assertFixtureDiagnostics(t *testing.T, diagnostics []sharedsurfaceownership.Diagnostic, test fixtureCase) {
	t.Helper()
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
	if test.wantRule == "" {
		return
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Rule != test.wantRule {
			continue
		}
		if diagnostic.Path == "" {
			t.Fatalf("diagnostic rule %q missing path identifying affected surface or hold", test.wantRule)
		}
		return
	}
	t.Fatalf("diagnostics = %q, want rule %q", joined, test.wantRule)
}

func TestCanonicalInventoryMatchesModelContract(t *testing.T) {
	t.Parallel()

	path := testutil.MustRepoPath(t, sharedsurfaceownership.CanonicalInventoryRelPath)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read canonical inventory: %v", err)
	}
	diagnostics := sharedsurfaceownership.ValidateCompleteDocument(
		sharedsurfaceownership.CanonicalInventoryRelPath,
		payload,
	)
	if len(diagnostics) != 0 {
		t.Fatalf("canonical inventory diagnostics = %#v", diagnostics)
	}
}

func TestValidateCompleteInventoryDoesNotMutateProtectedArtifacts(t *testing.T) {
	t.Parallel()

	before := digestProtectedArtifacts(t)
	path := testutil.MustRepoPath(t, sharedsurfaceownership.CanonicalInventoryRelPath)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read canonical inventory: %v", err)
	}
	diagnostics := sharedsurfaceownership.ValidateCompleteDocument(
		sharedsurfaceownership.CanonicalInventoryRelPath,
		payload,
	)
	if len(diagnostics) != 0 {
		t.Fatalf("canonical inventory diagnostics = %#v", diagnostics)
	}
	after := digestProtectedArtifacts(t)
	for _, rel := range sharedsurfaceownership.ProtectedCompositionArtifactRelPaths {
		if before[rel] != after[rel] {
			t.Fatalf("validation mutated protected composition artifact %s", rel)
		}
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

func TestCanonicalInventoryIncludesPSSI03CLISurfaces(t *testing.T) {
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
	for _, surfaceID := range sharedsurfaceownership.RequiredPSSI03SurfaceIDs {
		raw, ok := surfaces[surfaceID]
		if !ok {
			t.Fatalf("canonical inventory missing required PSS-I03 surface %q", surfaceID)
		}
		surface, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("surface %q must be an object", surfaceID)
		}
		if got, _ := surface["protocolFamily"].(string); got != "cli" {
			t.Fatalf("surface %q protocolFamily = %q, want cli", surfaceID, got)
		}
		if got, _ := surface["serialIntegratorLaneId"].(string); got != "PSS-I03" {
			t.Fatalf("surface %q serialIntegratorLaneId = %q, want PSS-I03", surfaceID, got)
		}
		if surface["activeHolder"] != nil {
			t.Fatalf("surface %q activeHolder must be null; this packet performs no CLI cutover", surfaceID)
		}
		queue, ok := surface["ownerRequestQueue"].([]any)
		if !ok {
			t.Fatalf("surface %q must declare an ownerRequestQueue array", surfaceID)
		}
		if len(queue) != 0 {
			t.Fatalf("surface %q ownerRequestQueue must be empty until a CLI-* adapter cutover is accepted", surfaceID)
		}
		summary, _ := surface["exclusiveChangedPathSummary"].(string)
		if strings.TrimSpace(summary) == "" {
			t.Fatalf("surface %q requires exclusiveChangedPathSummary", surfaceID)
		}
	}
}

func TestCanonicalInventoryIncludesPSSI04MCPSurfaces(t *testing.T) {
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
	for _, surfaceID := range sharedsurfaceownership.RequiredPSSI04SurfaceIDs {
		raw, ok := surfaces[surfaceID]
		if !ok {
			t.Fatalf("canonical inventory missing required PSS-I04 surface %q", surfaceID)
		}
		surface, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("surface %q must be an object", surfaceID)
		}
		if got, _ := surface["protocolFamily"].(string); got != "mcp" {
			t.Fatalf("surface %q protocolFamily = %q, want mcp", surfaceID, got)
		}
		if got, _ := surface["serialIntegratorLaneId"].(string); got != "PSS-I04" {
			t.Fatalf("surface %q serialIntegratorLaneId = %q, want PSS-I04", surfaceID, got)
		}
		if surface["activeHolder"] != nil {
			t.Fatalf("surface %q activeHolder must be null; this packet performs no MCP cutover", surfaceID)
		}
		queue, ok := surface["ownerRequestQueue"].([]any)
		if !ok {
			t.Fatalf("surface %q must declare an ownerRequestQueue array", surfaceID)
		}
		if len(queue) != 0 {
			t.Fatalf("surface %q ownerRequestQueue must be empty until an MCP-* adapter cutover is accepted", surfaceID)
		}
		summary, _ := surface["exclusiveChangedPathSummary"].(string)
		if strings.TrimSpace(summary) == "" {
			t.Fatalf("surface %q requires exclusiveChangedPathSummary", surfaceID)
		}
	}
}

func TestCanonicalInventoryIncludesRequiredPortfolioHolds(t *testing.T) {
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
	holds, _ := root["holds"].(map[string]any)
	surfaces, _ := root["surfaces"].(map[string]any)

	for _, spec := range sharedsurfaceownership.RequiredPortfolioHoldSpecs {
		raw, ok := holds[spec.HoldID]
		if !ok {
			t.Fatalf("canonical inventory missing required portfolio hold %q", spec.HoldID)
		}
		hold, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("hold %q must be an object", spec.HoldID)
		}
		if got, _ := hold["holdId"].(string); got != spec.HoldID {
			t.Fatalf("hold %q holdId = %q, want matching key", spec.HoldID, got)
		}
		externalOwner, _ := hold["externalOwner"].(string)
		if !strings.Contains(externalOwner, spec.ExternalOwnerSubstr) {
			t.Fatalf("hold %q externalOwner = %q, want substring %q", spec.HoldID, externalOwner, spec.ExternalOwnerSubstr)
		}
		blocked, _ := hold["blockedLaneOrSurfaceClass"].(string)
		if !strings.Contains(blocked, spec.BlockedLaneSubstr) {
			t.Fatalf("hold %q blockedLaneOrSurfaceClass = %q, want substring %q", spec.HoldID, blocked, spec.BlockedLaneSubstr)
		}
		release, _ := hold["releaseCondition"].(string)
		if !strings.Contains(strings.ToLower(release), strings.ToLower(spec.ReleaseSubstr)) {
			t.Fatalf("hold %q releaseCondition = %q, want substring %q", spec.HoldID, release, spec.ReleaseSubstr)
		}
		if hold["bypassable"] != false {
			t.Fatalf("hold %q bypassable must be false; holds never authorize bypass", spec.HoldID)
		}
		if hold["ownerLocalNonOverlappingAllowed"] != true {
			t.Fatalf("hold %q ownerLocalNonOverlappingAllowed must be true", spec.HoldID)
		}
	}

	schemaHold := sharedsurfaceownership.HoldSchemaCLIPR1262CLIManifestGeneration
	providerHold := sharedsurfaceownership.HoldStandardizedProvidersConductor
	for _, surfaceID := range sharedsurfaceownership.RequiredPSSI03SurfaceIDs {
		assertSurfaceHoldRef(t, surfaces, surfaceID, schemaHold)
	}
	for _, surfaceID := range sharedsurfaceownership.RequiredPSSI02SurfaceIDs {
		assertSurfaceHoldRef(t, surfaces, surfaceID, providerHold)
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
		"PSS-I03",
		"CLI root",
		"manifest",
		"help",
		"completion",
		"Schema CLI",
		"does not transfer",
		"disjoint",
		"PSS-I04",
		"mcp",
		"server and registration",
		"injected root contracts",
		"MCP-*",
		"registry",
		"discovery",
		"portfolio hold",
		"PR #1262",
		"Standardized Providers",
		"non-bypassable",
		"does not seize",
		"owner-local",
		"release condition",
		"ValidateCompleteDocument",
		"read-only",
		"does not mutate",
	}
	for _, needle := range required {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(needle)) {
			t.Fatalf("model doc missing required phrase %q", needle)
		}
	}
}

func assertSurfaceHoldRef(t *testing.T, surfaces map[string]any, surfaceID, holdID string) {
	t.Helper()
	raw, ok := surfaces[surfaceID]
	if !ok {
		t.Fatalf("canonical inventory missing surface %q", surfaceID)
	}
	surface, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("surface %q must be an object", surfaceID)
	}
	refs, ok := surface["holdConditionRefs"].([]any)
	if !ok {
		t.Fatalf("surface %q must declare holdConditionRefs", surfaceID)
	}
	for _, ref := range refs {
		if got, _ := ref.(string); got == holdID {
			return
		}
	}
	t.Fatalf("surface %q holdConditionRefs = %#v, want %q", surfaceID, refs, holdID)
}

func joinDiagnostics(diagnostics []sharedsurfaceownership.Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, diagnostic.Rule+" "+diagnostic.Path+" "+diagnostic.Message)
	}
	return strings.Join(parts, " | ")
}

func digestProtectedArtifacts(t *testing.T) map[string][sha256.Size]byte {
	t.Helper()
	digests := make(map[string][sha256.Size]byte, len(sharedsurfaceownership.ProtectedCompositionArtifactRelPaths))
	for _, rel := range sharedsurfaceownership.ProtectedCompositionArtifactRelPaths {
		path := testutil.MustRepoPath(t, rel)
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read protected artifact %s: %v", rel, err)
		}
		digests[rel] = sha256.Sum256(payload)
	}
	return digests
}
