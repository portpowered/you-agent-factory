package psslease_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/psslease"
)

func TestValidateManifestRejectsContractViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
		want    string
	}{
		{
			name:    "unknown state",
			fixture: "invalid-unknown-state.json",
			want:    `unknown packet state "running"`,
		},
		{
			name:    "missing packet id",
			fixture: "invalid-missing-packet-id.json",
			want:    "missing packetId",
		},
		{
			name:    "empty exclusive paths",
			fixture: "invalid-empty-exclusive-paths.json",
			want:    "empty exclusivePaths",
		},
		{
			name:    "duplicate packet ids",
			fixture: "invalid-duplicate-packet-ids.json",
			want:    `duplicate packetId "FND-10"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manifest := loadFixture(t, test.fixture)
			err := psslease.ValidateManifest(manifest)
			if err == nil {
				t.Fatal("ValidateManifest() error = nil, want failure")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateManifest() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateManifestAcceptsContractExample(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-contract-example.json")
	if err := psslease.ValidateManifest(manifest); err != nil {
		t.Fatalf("ValidateManifest() error = %v, want nil", err)
	}

	if got := manifest.Packets[0].State; got != psslease.StateReady {
		t.Fatalf("packet state = %q, want %q", got, psslease.StateReady)
	}
	if !psslease.IsLeaseHoldingState(psslease.StateActive) {
		t.Fatal("expected active to be lease-holding")
	}
	if !psslease.IsLeaseHoldingState(psslease.StateReview) {
		t.Fatal("expected review to be lease-holding")
	}
	if !psslease.IsLeaseHoldingState(psslease.StateIntegration) {
		t.Fatal("expected integration to be lease-holding")
	}
	for _, state := range []string{psslease.StateBlocked, psslease.StateReady, psslease.StateDone} {
		if psslease.IsLeaseHoldingState(state) {
			t.Fatalf("expected %q not to be lease-holding", state)
		}
	}
}

func TestCommittedProgramMetadataManifestPassesContractValidation(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "docs", "internal", "projects", "packaged-service-structure", "path-lease-packet-manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed manifest: %v", err)
	}
	manifest, err := psslease.DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if err := psslease.ValidateManifest(manifest); err != nil {
		t.Fatalf("ValidateManifest() error = %v, want nil", err)
	}
}

func TestValidateCatalogRejectsMissingRequiredPacket(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-missing-cataloged-packet.json")
	err := psslease.ValidateCatalog(manifest)
	if err == nil {
		t.Fatal("ValidateCatalog() error = nil, want missing required packet failure")
	}
	if !strings.Contains(err.Error(), `missing cataloged packet "FND-01"`) {
		t.Fatalf("ValidateCatalog() error = %v, want missing FND-01", err)
	}
}

func TestValidateCatalogRejectsMissingState(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-missing-packet-state.json")
	err := psslease.ValidateCatalog(manifest)
	if err == nil {
		t.Fatal("ValidateCatalog() error = nil, want missing state failure")
	}
	if !strings.Contains(err.Error(), "missing packet state") && !strings.Contains(err.Error(), "unknown packet state") {
		t.Fatalf("ValidateCatalog() error = %v, want missing/unknown state", err)
	}
}

func TestValidateCatalogRejectsEmptyExclusivePaths(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-catalog-empty-exclusive-paths.json")
	err := psslease.ValidateCatalog(manifest)
	if err == nil {
		t.Fatal("ValidateCatalog() error = nil, want empty exclusivePaths failure")
	}
	if !strings.Contains(err.Error(), "empty exclusivePaths") {
		t.Fatalf("ValidateCatalog() error = %v, want empty exclusivePaths", err)
	}
}

func TestValidateCatalogRejectsDuplicatePacketIDs(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-duplicate-packet-ids.json")
	err := psslease.ValidateCatalog(manifest)
	if err == nil {
		t.Fatal("ValidateCatalog() error = nil, want duplicate packetId failure")
	}
	if !strings.Contains(err.Error(), `duplicate packetId "FND-10"`) {
		t.Fatalf("ValidateCatalog() error = %v, want duplicate FND-10", err)
	}
}

func TestCommittedProgramMetadataManifestPassesCatalogValidation(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "docs", "internal", "projects", "packaged-service-structure", "path-lease-packet-manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed manifest: %v", err)
	}
	manifest, err := psslease.DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if err := psslease.ValidateCatalog(manifest); err != nil {
		t.Fatalf("ValidateCatalog() error = %v, want nil", err)
	}

	seen := make(map[string]psslease.Packet, len(manifest.Packets))
	for _, packet := range manifest.Packets {
		seen[packet.PacketID] = packet
		if psslease.IsLeaseHoldingState(packet.State) {
			t.Fatalf("catalog packet %q state = %q; undispatched catalog defaults must not silently use lease-holding states", packet.PacketID, packet.State)
		}
	}
	for _, packetID := range psslease.RequiredCatalogPacketIDs {
		packet, ok := seen[packetID]
		if !ok {
			t.Fatalf("committed manifest missing required packet %q", packetID)
		}
		if len(packet.ExclusivePaths) == 0 {
			t.Fatalf("packet %q has empty exclusivePaths", packetID)
		}
		if strings.TrimSpace(packet.State) == "" {
			t.Fatalf("packet %q missing state", packetID)
		}
	}
}

func TestValidateLeaseHoldersRejectsOverlappingActiveLeases(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-overlapping-active-leases.json")
	err := psslease.ValidateLeaseHolders(manifest)
	if err == nil {
		t.Fatal("ValidateLeaseHolders() error = nil, want overlapping lease conflict")
	}
	message := err.Error()
	for _, want := range []string{"PKT-A", "PKT-B", "pkg/services/factory_runtime/"} {
		if !strings.Contains(message, want) {
			t.Fatalf("ValidateLeaseHolders() error = %v, want substring %q", err, want)
		}
	}
}

func TestValidateLeaseHoldersAcceptsDisjointActiveLeases(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-disjoint-active-leases.json")
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("ValidateLeaseHolders() error = %v, want nil for disjoint holders", err)
	}
}

func TestValidateLeaseHoldersAllowsNonHoldingPathOverlap(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-blocked-ready-path-overlap.json")
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("ValidateLeaseHolders() error = %v, want nil while overlap is only among non-holders", err)
	}
}

func TestValidateDispatchCandidateRejectsOverlapWithExistingHolder(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-blocked-ready-path-overlap.json")
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("precondition ValidateLeaseHolders() error = %v", err)
	}

	// Promote PKT-A to active first so it holds the overlapping prefix.
	for i := range manifest.Packets {
		if manifest.Packets[i].PacketID == "PKT-A" {
			manifest.Packets[i].State = psslease.StateActive
		}
	}
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("ValidateLeaseHolders() after promoting PKT-A error = %v, want nil", err)
	}

	err := psslease.ValidateDispatchCandidate(manifest, "PKT-B", psslease.StateActive)
	if err == nil {
		t.Fatal("ValidateDispatchCandidate() error = nil, want rejection before PKT-B becomes active")
	}
	message := err.Error()
	for _, want := range []string{"PKT-A", "PKT-B", "docs/internal/projects/packaged-service-structure/"} {
		if !strings.Contains(message, want) {
			t.Fatalf("ValidateDispatchCandidate() error = %v, want substring %q", err, want)
		}
	}
}

func TestValidateDispatchCandidateAllowsDisjointActivation(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-disjoint-active-leases.json")
	// Reset PKT-B to ready so we can ask whether it may become active.
	for i := range manifest.Packets {
		if manifest.Packets[i].PacketID == "PKT-B" {
			manifest.Packets[i].State = psslease.StateReady
		}
	}

	if err := psslease.ValidateDispatchCandidate(manifest, "PKT-B", psslease.StateActive); err != nil {
		t.Fatalf("ValidateDispatchCandidate() error = %v, want nil for disjoint activation", err)
	}
}

// TestValidateLeaseHoldersAcceptsReconciledRuntimeDispatchSingleOwner proves
// the docs/internal/projects/packaged-service-structure/README.md "Runtime
// dispatch ownership reconciliation" record: PSS IMP-RUN-03 superseded (done,
// lease released) and L2 IMP-RUN-DISPATCH as the single prospective owner
// validate cleanly even though their exclusive paths would otherwise overlap.
func TestValidateLeaseHoldersAcceptsReconciledRuntimeDispatchSingleOwner(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-runtime-dispatch-single-owner.json")
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("ValidateLeaseHolders() error = %v, want nil for reconciled single owner", err)
	}

	superseded := packetByID(t, manifest, "IMP-RUN-03")
	if superseded.State != psslease.StateDone {
		t.Fatalf("IMP-RUN-03 state = %q, want %q (superseded/lease-released)", superseded.State, psslease.StateDone)
	}
}

// TestValidateLeaseHoldersRejectsAmbiguousRuntimeDispatchOwnership is the
// ambiguity/overlap regression: if PSS IMP-RUN-03 were reactivated instead of
// superseded while L2 IMP-RUN-DISPATCH also holds the overlapping Factory
// Runtime path, validation must deterministically reject the ambiguous
// dual-owner state.
func TestValidateLeaseHoldersRejectsAmbiguousRuntimeDispatchOwnership(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-runtime-dispatch-ambiguous-owner.json")
	err := psslease.ValidateLeaseHolders(manifest)
	if err == nil {
		t.Fatal("ValidateLeaseHolders() error = nil, want ambiguous Runtime dispatch owner rejection")
	}
	message := err.Error()
	for _, want := range []string{"IMP-RUN-03", "IMP-RUN-DISPATCH", "pkg/services/factory_runtime/"} {
		if !strings.Contains(message, want) {
			t.Fatalf("ValidateLeaseHolders() error = %v, want substring %q", err, want)
		}
	}
}

// TestSetPacketStateAdmitsRuntimeDispatchThenRejectsReintroducedAmbiguity
// proves both PSS-002 requirements together: the reconciled ledger permits
// L2 IMP-RUN-DISPATCH admission (blocked -> active) once no lease holder
// overlaps it, and reintroducing PSS IMP-RUN-03 as a second active owner is
// rejected before either competing packet is treated as lease-holding.
func TestSetPacketStateAdmitsRuntimeDispatchThenRejectsReintroducedAmbiguity(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-runtime-dispatch-single-owner.json")

	if err := psslease.SetPacketState(manifest, "IMP-RUN-DISPATCH", psslease.StateActive); err != nil {
		t.Fatalf("SetPacketState(IMP-RUN-DISPATCH, active) error = %v, want nil admission for the reconciled single owner", err)
	}
	if got := packetByID(t, manifest, "IMP-RUN-DISPATCH").State; got != psslease.StateActive {
		t.Fatalf("IMP-RUN-DISPATCH state = %q, want %q after admission", got, psslease.StateActive)
	}

	err := psslease.SetPacketState(manifest, "IMP-RUN-03", psslease.StateActive)
	if err == nil {
		t.Fatal("SetPacketState(IMP-RUN-03, active) error = nil, want rejection of reintroduced ambiguous ownership")
	}
	for _, want := range []string{"IMP-RUN-03", "IMP-RUN-DISPATCH"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("SetPacketState() error = %v, want substring %q", err, want)
		}
	}
	if got := packetByID(t, manifest, "IMP-RUN-03").State; got != psslease.StateDone {
		t.Fatalf("IMP-RUN-03 state = %q after rejected promotion, want unchanged %q", got, psslease.StateDone)
	}
}

func TestCommittedProgramMetadataManifestPassesLeaseHolderValidation(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "docs", "internal", "projects", "packaged-service-structure", "path-lease-packet-manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed manifest: %v", err)
	}
	manifest, err := psslease.DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("ValidateLeaseHolders() error = %v, want nil", err)
	}
}

func TestPlannerStateUpdateWalksReadyActiveReviewIntegrationDoneOnProgramMetadataOnly(t *testing.T) {
	t.Parallel()

	manifest := loadCommittedCatalog(t)
	packetID := "FND-10"
	packet := packetByID(t, manifest, packetID)
	if packet.State != psslease.StateReady {
		t.Fatalf("FND-10 starting state = %q, want %q for representative planner walk", packet.State, psslease.StateReady)
	}
	for _, path := range packet.ExclusivePaths {
		if strings.Contains(path, "api/openapi") ||
			strings.Contains(path, "pkg/transports/http/") ||
			strings.Contains(path, "cmd/factory/") ||
			strings.Contains(path, "pkg/transports/mcp/") ||
			strings.Contains(path, "pkg/wire/") {
			t.Fatalf("FND-10 exclusive path %q crosses shared OpenAPI/CLI/provider/Wire surfaces; planner updates must stay in program metadata", path)
		}
	}

	transitions := []string{
		psslease.StateActive,
		psslease.StateReview,
		psslease.StateIntegration,
		psslease.StateDone,
	}
	for _, target := range transitions {
		if err := psslease.SetPacketState(manifest, packetID, target); err != nil {
			t.Fatalf("SetPacketState(%q, %q) error = %v, want nil for planner-only lifecycle update", packetID, target, err)
		}
		updated := packetByID(t, manifest, packetID)
		if updated.State != target {
			t.Fatalf("after SetPacketState packet state = %q, want %q", updated.State, target)
		}
		if err := psslease.ValidateCatalog(manifest); err != nil {
			t.Fatalf("ValidateCatalog() after %q error = %v, want nil", target, err)
		}
	}
}

func TestSetPacketStateRejectsOverlappingLeaseHoldingPromotion(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-blocked-ready-path-overlap.json")
	if err := psslease.SetPacketState(manifest, "PKT-A", psslease.StateActive); err != nil {
		t.Fatalf("SetPacketState(PKT-A, active) error = %v, want nil", err)
	}
	err := psslease.SetPacketState(manifest, "PKT-B", psslease.StateActive)
	if err == nil {
		t.Fatal("SetPacketState(PKT-B, active) error = nil, want overlapping lease rejection")
	}
	for _, want := range []string{"PKT-A", "PKT-B"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("SetPacketState() error = %v, want substring %q", err, want)
		}
	}
	if got := packetByID(t, manifest, "PKT-B").State; got != psslease.StateBlocked {
		t.Fatalf("PKT-B state = %q after rejected promotion, want unchanged %q", got, psslease.StateBlocked)
	}
}

// TestValidateDispatchCandidateAdmitsNarrowedPSSI01WithoutMutatingState proves
// the committed ledger's narrowed PSS-I01 (pkg/root/process.go,
// pkg/wire/profiles.go, pkg/wire/wire.go instead of the blanket pkg/wire/,
// pkg/root/, and pkg/initializer/ prefixes) is admissible into a
// lease-holding state before dispatch, and that ValidateDispatchCandidate is
// a pure pre-dispatch check: it never mutates the candidate's committed state.
func TestValidateDispatchCandidateAdmitsNarrowedPSSI01WithoutMutatingState(t *testing.T) {
	t.Parallel()

	manifest := loadCommittedCatalog(t)
	before := packetByID(t, manifest, "PSS-I01")

	if err := psslease.ValidateDispatchCandidate(manifest, "PSS-I01", psslease.StateActive); err != nil {
		t.Fatalf("ValidateDispatchCandidate(PSS-I01, active) error = %v, want nil for narrowed candidate", err)
	}

	after := packetByID(t, manifest, "PSS-I01")
	if after.State != before.State {
		t.Fatalf("PSS-I01 state = %q after ValidateDispatchCandidate, want unchanged %q (candidate check must not commit a state change)", after.State, before.State)
	}
	for i, path := range []string{"pkg/root/process.go", "pkg/wire/profiles.go", "pkg/wire/wire.go"} {
		if after.ExclusivePaths[i] != path {
			t.Fatalf("PSS-I01 exclusivePaths[%d] = %q, want %q", i, after.ExclusivePaths[i], path)
		}
	}
}

// TestValidateLeaseHoldersAllowsAdditiveCompositionOutsideNarrowedPSSI01Paths
// proves an active additive-composition claim outside the retained
// structural files (pkg/wire/service_registration.go) does not conflict with
// the narrowed PSS-I01 lease.
func TestValidateLeaseHoldersAllowsAdditiveCompositionOutsideNarrowedPSSI01Paths(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-pss-i01-narrowed-additive-composition.json")
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("ValidateLeaseHolders() error = %v, want nil for additive composition outside PSS-I01's retained structural files", err)
	}

	if err := psslease.SetPacketState(manifest, "PKT-WIRE-ADDITIVE", psslease.StateActive); err != nil {
		t.Fatalf("SetPacketState(PKT-WIRE-ADDITIVE, active) error = %v, want nil admission alongside active PSS-I01", err)
	}
}

// TestValidateDispatchCandidateRejectsClaimOnRetainedPSSI01StructuralPath
// proves an active claim equal to a retained PSS-I01 structural path
// (pkg/root/process.go) is rejected before activation, with both packet
// identities and the overlapping path in the diagnostic, and that the
// rejected packet remains non-holding.
func TestValidateDispatchCandidateRejectsClaimOnRetainedPSSI01StructuralPath(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-pss-i01-narrowed-structural-conflict.json")

	err := psslease.SetPacketState(manifest, "PKT-STRUCTURAL-CONFLICT", psslease.StateActive)
	if err == nil {
		t.Fatal("SetPacketState(PKT-STRUCTURAL-CONFLICT, active) error = nil, want rejection for overlap with PSS-I01's retained structural path")
	}
	message := err.Error()
	for _, want := range []string{"PSS-I01", "PKT-STRUCTURAL-CONFLICT", "pkg/root/process.go"} {
		if !strings.Contains(message, want) {
			t.Fatalf("SetPacketState() error = %v, want substring %q", err, want)
		}
	}

	if got := packetByID(t, manifest, "PKT-STRUCTURAL-CONFLICT").State; got != psslease.StateBlocked {
		t.Fatalf("PKT-STRUCTURAL-CONFLICT state = %q after rejected promotion, want unchanged %q (non-holding)", got, psslease.StateBlocked)
	}
}

// TestValidateDispatchCandidateAdmitsNarrowedPSSI05WithoutMutatingState proves
// the committed ledger's re-scoped PSS-I05 (single dedicated
// event-boundary-d2-rescope.md metadata path instead of the prior
// pkg/factory/contracts/ and event-backbone-convergence.md event-backbone
// lease) is admissible into a lease-holding state before dispatch, and that
// ValidateDispatchCandidate never mutates the candidate's committed state.
func TestValidateDispatchCandidateAdmitsNarrowedPSSI05WithoutMutatingState(t *testing.T) {
	t.Parallel()

	manifest := loadCommittedCatalog(t)
	before := packetByID(t, manifest, "PSS-I05")
	if len(before.ExclusivePaths) != 1 {
		t.Fatalf("PSS-I05 exclusivePaths = %v, want exactly one residual metadata path", before.ExclusivePaths)
	}
	wantPath := "docs/internal/projects/packaged-service-structure/event-boundary-d2-rescope.md"
	if before.ExclusivePaths[0] != wantPath {
		t.Fatalf("PSS-I05 exclusivePaths[0] = %q, want %q", before.ExclusivePaths[0], wantPath)
	}
	for _, forbidden := range []string{"pkg/factory/contracts/", "event-backbone-convergence.md"} {
		for _, path := range before.ExclusivePaths {
			if strings.Contains(path, forbidden) {
				t.Fatalf("PSS-I05 exclusivePaths %v still references forbidden path %q", before.ExclusivePaths, forbidden)
			}
		}
	}

	if err := psslease.ValidateDispatchCandidate(manifest, "PSS-I05", psslease.StateActive); err != nil {
		t.Fatalf("ValidateDispatchCandidate(PSS-I05, active) error = %v, want nil for re-scoped candidate", err)
	}

	after := packetByID(t, manifest, "PSS-I05")
	if after.State != before.State {
		t.Fatalf("PSS-I05 state = %q after ValidateDispatchCandidate, want unchanged %q (candidate check must not commit a state change)", after.State, before.State)
	}
	if after.ExclusivePaths[0] != wantPath {
		t.Fatalf("PSS-I05 exclusivePaths[0] = %q after ValidateDispatchCandidate, want unchanged %q", after.ExclusivePaths[0], wantPath)
	}
}

// TestValidateDispatchCandidateRejectsClaimOnRetainedPSSI05BoundaryPath proves
// a genuine equal/path-prefix claim on the retained PSS-I05 residual metadata
// path is rejected before activation, with both packet identities and the
// conflicting path in the diagnostic, and that the rejected packet remains
// non-holding.
func TestValidateDispatchCandidateRejectsClaimOnRetainedPSSI05BoundaryPath(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-pss-i05-narrowed-boundary-conflict.json")

	err := psslease.SetPacketState(manifest, "PKT-BOUNDARY-CONFLICT", psslease.StateActive)
	if err == nil {
		t.Fatal("SetPacketState(PKT-BOUNDARY-CONFLICT, active) error = nil, want rejection for overlap with PSS-I05's retained metadata path")
	}
	message := err.Error()
	for _, want := range []string{"PSS-I05", "PKT-BOUNDARY-CONFLICT", "docs/internal/projects/packaged-service-structure/event-boundary-d2-rescope.md"} {
		if !strings.Contains(message, want) {
			t.Fatalf("SetPacketState() error = %v, want substring %q", err, want)
		}
	}

	if got := packetByID(t, manifest, "PKT-BOUNDARY-CONFLICT").State; got != psslease.StateBlocked {
		t.Fatalf("PKT-BOUNDARY-CONFLICT state = %q after rejected promotion, want unchanged %q (non-holding)", got, psslease.StateBlocked)
	}
}

func loadCommittedCatalog(t *testing.T) *psslease.Manifest {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "internal", "projects", "packaged-service-structure", "path-lease-packet-manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed manifest: %v", err)
	}
	manifest, err := psslease.DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	return manifest
}

func packetByID(t *testing.T, manifest *psslease.Manifest, packetID string) psslease.Packet {
	t.Helper()
	for _, packet := range manifest.Packets {
		if packet.PacketID == packetID {
			return packet
		}
	}
	t.Fatalf("packet %q not found", packetID)
	return psslease.Packet{}
}

func loadFixture(t *testing.T, name string) *psslease.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	manifest, err := psslease.DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest(%s) error = %v", name, err)
	}
	return manifest
}
