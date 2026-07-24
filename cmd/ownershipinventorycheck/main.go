// Command ownershipinventorycheck runs the PSS-F01 ownership-inventory and
// path-lease freeze verification gate.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func main() {
	rootFlag := flag.String("root", "", "repository root (defaults to discovering from cwd)")
	flag.Parse()
	os.Exit(run(*rootFlag, os.Stdout, os.Stderr))
}

func run(root string, stdout, stderr io.Writer) int {
	if root == "" {
		discovered, err := ownershipinventory.FindRepositoryRoot()
		if err != nil {
			fmt.Fprintf(stderr, "[ownership-inventory-check] find repository root: %v\n", err)
			return 1
		}
		root = discovered
	}
	report, err := ownershipinventory.VerifyFreeze(root)
	if err != nil {
		fmt.Fprintf(stderr, "[ownership-inventory-check] verify failed: %v\n", err)
		return 1
	}
	if !report.OK() {
		fmt.Fprintf(stderr, "[ownership-inventory-check] freeze gate failed: inventoryOK=%v pathLeaseOK=%v completeness=%v stableSort=%v rationale=%v edges=%v namedOwners=%v processEdges=%v nonOverlappingLeases=%v\n",
			report.InventoryOK,
			report.PathLeaseOK,
			report.Completeness,
			report.StableSortOrder,
			report.RequiredRationaleFields,
			report.EdgeClassifications,
			report.NamedOwnerCoverage,
			report.ProcessEdgesException,
			report.NonOverlappingActiveLeases,
		)
		fmt.Fprintf(stderr, "[ownership-inventory-check] inventory report: %#v\n", report.Inventory)
		fmt.Fprintf(stderr, "[ownership-inventory-check] path-lease report: %#v\n", report.PathLease)
		return 1
	}
	fmt.Fprintln(stdout, "[ownership-inventory-check] ownership inventory and path-lease freeze verified")
	return 0
}
