package ownershipinventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// NonServiceFamilies are the approved top-level pkg/ families that are not
// product services.
//
// This list stays closed on purpose. It is not a service roster: it encodes the
// "six approved top-level package families" architectural rule from CLAUDE.md,
// which cmd/pkgstructurecheck enforces independently. Adding a product service
// never adds a family, so this list is not the registration tax that
// DiscoverProductOwners retires.
var NonServiceFamilies = []string{
	"initializer",
	"platform",
	"root",
	"transports",
	"wire",
}

// architectureExceptionServices are pkg/services children that are not product
// services. Process Edges is the sole broad external-effect exception; it is
// construction input for root.BuildProcess, not a service, so it is classified
// as an architecture exception rather than an owner.
var architectureExceptionServices = []string{DestinationEdges}

// ignoredServiceDirectoryNames are directory names under pkg/services that
// never name a service. They mirror the ignored-directory rules used by the
// other package-tree checkers.
var ignoredServiceDirectoryNames = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"testdata":     {},
	"vendor":       {},
}

// DiscoverProductOwners derives the product-owner roster for root by listing
// the immediate subdirectories of pkg/services, excluding the architecture
// exception and the shared ignored directory names. The result is sorted.
//
// The directory tree is the roster. Deriving it at check time is what lets a
// new service be a directory creation instead of an edit to a closed Go list
// inside a checker tool: previously both cmd/ownershipinventorycheck (through
// this package) and cmd/packagetargetmanifestcheck carried the service names as
// hand-maintained literals that had to be extended in lockstep with the tree.
func DiscoverProductOwners(root string) ([]string, error) {
	servicesRoot := filepath.Join(root, filepath.FromSlash(servicesRootRelative))
	entries, err := os.ReadDir(servicesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			// A root with no pkg/services tree hosts no services. Answering with an
			// empty roster keeps artifact-only fixture roots loadable; a wrong root
			// still fails loudly upstream, because ListProductionPackages requires
			// pkg/ to exist and every move-row destination would fall outside an
			// empty vocabulary.
			return nil, nil
		}
		return nil, fmt.Errorf("read services root %s: %w", servicesRootRelative, err)
	}
	owners := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, ignored := ignoredServiceDirectoryNames[name]; ignored {
			continue
		}
		if slices.Contains(architectureExceptionServices, name) {
			continue
		}
		owners = append(owners, name)
	}
	slices.Sort(owners)
	return owners, nil
}

// DiscoverDestinationVocabulary derives the destination vocabulary for root.
// Owners come from the live pkg/services tree; families and the architecture
// exception are the closed architectural rules documented above.
func DiscoverDestinationVocabulary(root string) (DestinationVocabulary, error) {
	owners, err := DiscoverProductOwners(root)
	if err != nil {
		return DestinationVocabulary{}, err
	}
	return DestinationVocabulary{
		Owners:    owners,
		Families:  slices.Clone(NonServiceFamilies),
		Exception: slices.Clone(architectureExceptionServices),
	}, nil
}

// KindOf reports the destination kind for a bare destination name.
func (v DestinationVocabulary) KindOf(destination string) (string, bool) {
	switch {
	case destination == DestinationDeletionQueue:
		return DestinationKindDeletionQueue, true
	case slices.Contains(v.Owners, destination):
		return DestinationKindOwner, true
	case slices.Contains(v.Families, destination):
		return DestinationKindFamily, true
	case slices.Contains(v.Exception, destination):
		return DestinationKindArchitectureException, true
	default:
		return "", false
	}
}

// IsKnownDestination reports whether destination is inside the vocabulary.
func (v DestinationVocabulary) IsKnownDestination(destination string) bool {
	_, ok := v.KindOf(destination)
	return ok
}

// IsOwner reports whether name is a derived product owner.
func (v DestinationVocabulary) IsOwner(name string) bool {
	return slices.Contains(v.Owners, name)
}
