package migrationledgercheck

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
)

const packagedFactoryInvocationMatrixPrefix = "tests/functional/factory/packaged/"

// CheckPackagedFactoryInvocationMatrix ensures every embedded packaged Factory
// slug is bound to a declared packaged invocation-matrix destination cell in the
// functional-test checklist.
func CheckPackagedFactoryInvocationMatrix(repoRoot, checklistPath string) error {
	embeddedSlugs, err := embeddedPackagedFactorySlugs()
	if err != nil {
		return err
	}
	checklistPaths, err := LoadChecklistPaths(filepath.Join(repoRoot, checklistPath))
	if err != nil {
		return err
	}
	matrixSlugs := packagedFactoryInvocationMatrixSlugsFromChecklist(checklistPaths)

	wantMatrixSlugs := append([]string(nil), embeddedSlugs...)
	slices.Sort(wantMatrixSlugs)

	matrixSlugList := make([]string, 0, len(matrixSlugs))
	for slug := range matrixSlugs {
		matrixSlugList = append(matrixSlugList, slug)
	}
	slices.Sort(matrixSlugList)

	if missing, extra := symmetricStringDiff(wantMatrixSlugs, matrixSlugList); len(missing) > 0 || len(extra) > 0 {
		return fmt.Errorf(
			"packaged factory invocation-matrix drift: missing matrix entries %v, orphan matrix entries %v; embedded slugs=%v matrix=%v",
			missing,
			extra,
			embeddedSlugs,
			matrixSlugList,
		)
	}
	return nil
}

func embeddedPackagedFactorySlugs() ([]string, error) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		return nil, fmt.Errorf("published packaged Factory catalog: %w", err)
	}
	names := catalog.Names()
	slugs := make([]string, len(names))
	for index, name := range names {
		slug, ok := strings.CutPrefix(name, "@you/")
		if !ok || strings.TrimSpace(slug) == "" || strings.Contains(slug, "/") {
			return nil, fmt.Errorf("published packaged Factory %q has no usable @you/ slug", name)
		}
		slugs[index] = slug
	}
	slices.Sort(slugs)
	return slugs, nil
}

func packagedFactoryInvocationMatrixSlugsFromChecklist(checklistPaths map[string]struct{}) map[string]struct{} {
	matrixSlugs := make(map[string]struct{})
	for checklistPath := range checklistPaths {
		slug, ok := packagedFactorySlugFromInvocationMatrixDestination(checklistPath)
		if !ok {
			continue
		}
		matrixSlugs[slug] = struct{}{}
	}
	return matrixSlugs
}

func packagedFactorySlugFromInvocationMatrixDestination(destination string) (string, bool) {
	remainder, ok := strings.CutPrefix(destination, packagedFactoryInvocationMatrixPrefix)
	if !ok {
		return "", false
	}
	subsection, suffix, ok := strings.Cut(remainder, "/")
	if !ok || suffix != "invocation_test.go" || subsection == "" {
		return "", false
	}
	return strings.ReplaceAll(subsection, "_", "-"), true
}

func symmetricStringDiff(want, got []string) (missing, extra []string) {
	wantSet := make(map[string]struct{}, len(want))
	for _, value := range want {
		wantSet[value] = struct{}{}
	}
	gotSet := make(map[string]struct{}, len(got))
	for _, value := range got {
		gotSet[value] = struct{}{}
		if _, ok := wantSet[value]; !ok {
			extra = append(extra, value)
		}
	}
	for _, value := range want {
		if _, ok := gotSet[value]; !ok {
			missing = append(missing, value)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	return missing, extra
}
