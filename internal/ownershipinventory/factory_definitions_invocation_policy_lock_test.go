package ownershipinventory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const (
	factoryDefinitionsInvocationPolicyServiceID  = "factory_definitions/invocation_policy"
	factoryDefinitionsInvocationPolicyTargetPath = "pkg/services/factory_definitions/internal/services/invocation_policy"
)

var factoryDefinitionsInvocationPolicyResidualRests = []string{
	"internal/services/invocation_policy/decisionenvelope",
	"internal/services/invocation_policy/invocationinterpolation",
	"internal/services/invocation_policy/invocationoutput",
	"internal/services/invocation_policy/invocationworktype",
	"internal/services/invocation_policy/quorumpolicy",
	"internal/services/invocation_policy/workpropagation",
	"internal/services/invocation_policy/workstationexecution",
	"internal/services/invocation_policy/ttsobservability",
	"internal/services/distribution/goal",
}

// Invocation Policy is a nested subservice of Factory Definitions. It used to be
// proved by a rationale card in the registry, which meant every nested service
// had to be registered twice. The durable claim is about the tree: the
// subservice sits at its committed path and derives to the Factory Definitions
// owner from that path alone. Its design rationale is prose and lives in
// docs/architecture/service-ownership-rationale.md.
func TestInvocationPolicyNestedServiceDerivesToFactoryDefinitions(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(factoryDefinitionsInvocationPolicyTargetPath))); err != nil {
		t.Fatalf("nested subservice %q must exist on disk: %v", factoryDefinitionsInvocationPolicyServiceID, err)
	}

	owner, ok := ownershipinventory.OwnerForPackage(factoryDefinitionsInvocationPolicyTargetPath)
	if !ok || owner != "factory_definitions" {
		t.Fatalf("OwnerForPackage(%q) = %q, %v; want factory_definitions, true", factoryDefinitionsInvocationPolicyTargetPath, owner, ok)
	}

	// The subservice is settled where it lives, so it carries no open-move row.
	if moveRow, ok := committedMoveLedger(t, root)[factoryDefinitionsInvocationPolicyTargetPath]; ok {
		t.Fatalf(
			"move ledger carries a row for settled nested subservice %q (successor %q)",
			factoryDefinitionsInvocationPolicyTargetPath,
			moveRow.Successor,
		)
	}
}

func TestFactoryDefinitionsResidualInvocationPolicyPackagesLocked(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	moveByPath := committedMoveLedger(t, root)

	for _, rest := range factoryDefinitionsInvocationPolicyResidualRests {
		packagePath := "pkg/services/factory_definitions/" + rest

		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition != ownershipinventory.DispositionRetain {
			t.Fatalf("MapPackage(%q) disposition = %q, want retain", packagePath, got.Disposition)
		}

		// Retain is the derived default, so a package staying put carries no row.
		// Reconstruct the implicit row and hold it to the same contract.
		derived, ok := ownershipinventory.DerivedPackageRow(packagePath)
		if !ok {
			t.Fatalf("no derivable owner for locked package %q", packagePath)
		}
		if derived.Disposition != ownershipinventory.DispositionRetain {
			t.Fatalf("derived row for %q disposition = %q, want retain", packagePath, derived.Disposition)
		}

		// The ledger tracks only unfinished migration intent. These packages are
		// locked where they are, so the lock is proven by the absence of a row:
		// adding a move row for one of them fails here.
		if moveRow, ok := moveByPath[packagePath]; ok {
			t.Fatalf(
				"move ledger carries a row for locked package %q (successor %q); locked packages carry no row",
				packagePath,
				moveRow.Successor,
			)
		}
	}
}

func TestFactoryDefinitionsResidualInvocationPolicyPackagesRejectRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	for _, rest := range factoryDefinitionsInvocationPolicyResidualRests {
		packagePath := "pkg/services/factory_definitions/" + rest
		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition != ownershipinventory.DispositionRetain {
			t.Fatalf("remapped residual policy package %q disposition = %q, want retain", packagePath, got.Disposition)
		}
	}
}

func TestFactoryDefinitionsSnapshotsPortabilityAndDefinitionDestinationsLocked(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	moveByPath := committedMoveLedger(t, root)

	definitionPath := "pkg/services/factory_definitions/definition"
	got, err := ownershipinventory.MapPackage(definitionPath)
	if err != nil {
		t.Fatalf("MapPackage(%q) error = %v", definitionPath, err)
	}
	if got.Disposition != ownershipinventory.DispositionMove || got.Successor != "pkg/services/factory_definitions/internal" {
		t.Fatalf("MapPackage(%q) = %#v, want move→pkg/services/factory_definitions/internal", definitionPath, got)
	}

	definitionMove, ok := moveByPath[definitionPath]
	if !ok {
		t.Fatalf("move ledger missing row for %q", definitionPath)
	}
	if definitionMove.Destination != "factory_definitions/internal" {
		t.Fatalf("move ledger %q = %#v, want destination factory_definitions/internal", definitionPath, definitionMove)
	}
	if definitionMove.Successor != got.Successor {
		t.Fatalf("move ledger %q successor = %q, want %q from MapPackage", definitionPath, definitionMove.Successor, got.Successor)
	}

	// snapshots_portability already sits at its destination, so it carries no
	// migration row. A row appearing here would mean a move was reopened.
	const snapshotsPrefix = "pkg/services/factory_definitions/internal/services/snapshots_portability"
	for packagePath := range moveByPath {
		if strings.HasPrefix(packagePath, snapshotsPrefix) {
			t.Fatalf("move ledger carries a row for settled package %q; settled packages carry no row", packagePath)
		}
	}
}

func TestFactoryDefinitionsInventoryPacketHasNoPackageDeletes(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	const ownerPrefix = "pkg/services/factory_definitions"
	for _, row := range inventory.Packages {
		if row.PackagePath != ownerPrefix && !strings.HasPrefix(row.PackagePath, ownerPrefix+"/") {
			continue
		}
		if row.Disposition == ownershipinventory.DispositionDelete {
			t.Fatalf("factory_definitions inventory packet must not delete %q", row.PackagePath)
		}
	}

	deletedTransitionalRests := []string{
		"decisionenvelope",
		"invocationinterpolation",
		"invocationoutput",
		"invocationworktype",
		"quorumpolicy",
		"workpropagation",
		"workstationexecution",
		"ttsobservability",
		"namedpaths",
		"persistence",
		"validation",
		"workers",
	}
	for _, rest := range deletedTransitionalRests {
		packagePath := ownerPrefix + "/" + rest
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(packagePath))); !os.IsNotExist(err) {
			t.Fatalf("deleted transitional package %q must not exist on disk; stat err = %v", packagePath, err)
		}
	}
}
