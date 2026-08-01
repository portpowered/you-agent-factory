package migrationledgercheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

const packagedFactoryInvocationMatrixPrefix = "tests/functional/factory/packaged/"

// CheckPackagedFactoryInvocationMatrix ensures every published packaged Factory
// slug is bound to a declared packaged invocation-matrix destination cell in the
// functional-test checklist.
func CheckPackagedFactoryInvocationMatrix(repoRoot, checklistPath string) error {
	root, err := resolveRepositoryRoot(repoRoot)
	if err != nil {
		return err
	}
	publishedSlugs, err := publishedPackagedFactorySlugs()
	if err != nil {
		return err
	}
	resolvedChecklistPath, err := resolveRepositoryPath(root, checklistPath)
	if err != nil {
		return err
	}
	checklistPaths, err := LoadChecklistPaths(resolvedChecklistPath)
	if err != nil {
		return err
	}
	matrixSlugs := packagedFactoryInvocationMatrixSlugsFromChecklist(checklistPaths)

	wantMatrixSlugs := append([]string(nil), publishedSlugs...)
	slices.Sort(wantMatrixSlugs)

	matrixSlugList := make([]string, 0, len(matrixSlugs))
	for slug := range matrixSlugs {
		matrixSlugList = append(matrixSlugList, slug)
	}
	slices.Sort(matrixSlugList)

	if missing, extra := symmetricStringDiff(wantMatrixSlugs, matrixSlugList); len(missing) > 0 || len(extra) > 0 {
		return fmt.Errorf(
			"packaged factory invocation-matrix drift: missing matrix entries %v, orphan matrix entries %v; published slugs=%v matrix=%v",
			missing,
			extra,
			publishedSlugs,
			matrixSlugList,
		)
	}
	return nil
}

func publishedPackagedFactorySlugs() ([]string, error) {
	homeDir, err := os.MkdirTemp("", "you-migration-catalog-home-")
	if err != nil {
		return nil, fmt.Errorf("create packaged Factory catalog home: %w", err)
	}
	defer os.RemoveAll(homeDir)
	workingDirectory, err := os.MkdirTemp("", "you-migration-catalog-working-")
	if err != nil {
		return nil, fmt.Errorf("create packaged Factory catalog working directory: %w", err)
	}
	defer os.RemoveAll(workingDirectory)

	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		return nil, fmt.Errorf("build process for published packaged Factory catalog: %w", err)
	}
	defer process.Close(context.Background())

	var stdout, stderr bytes.Buffer
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	err = process.Execute(root.Input{
		Args:             []string{"you", "--json", "factory", "list"},
		Env:              env,
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: workingDirectory,
	})
	if err != nil {
		return nil, fmt.Errorf("list published packaged Factory catalog: %w", err)
	}

	var listed []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		return nil, fmt.Errorf("decode published packaged Factory catalog: %w", err)
	}
	slugs := make([]string, 0, len(listed))
	for _, entry := range listed {
		name := strings.TrimSpace(entry.Name)
		if !strings.HasPrefix(name, "@you/") {
			continue
		}
		slug := strings.TrimPrefix(name, "@you/")
		if slug == "" {
			return nil, fmt.Errorf(
				"published packaged Factory catalog entry %q has empty slug",
				name,
			)
		}
		slugs = append(slugs, slug)
	}
	if len(slugs) == 0 {
		return nil, fmt.Errorf("published packaged Factory catalog contained no @you/ entries; diagnostics: %s", strings.TrimSpace(stderr.String()))
	}
	slices.Sort(slugs)
	return slugs, nil
}

func packagedFactoryInvocationMatrixSlugsFromChecklist(checklistPaths map[string]struct{}) map[string]struct{} {
	matrixSlugs := make(map[string]struct{})
	for checklistPath := range checklistPaths {
		slugs, ok := packagedFactorySlugsFromInvocationMatrixDestination(checklistPath)
		if !ok {
			continue
		}
		for _, slug := range slugs {
			matrixSlugs[slug] = struct{}{}
		}
	}
	return matrixSlugs
}

func packagedFactorySlugsFromInvocationMatrixDestination(destination string) ([]string, bool) {
	remainder, ok := strings.CutPrefix(destination, packagedFactoryInvocationMatrixPrefix)
	if !ok {
		return nil, false
	}
	subsection, suffix, ok := strings.Cut(remainder, "/")
	if !ok || suffix != "invocation_test.go" || subsection == "" {
		return nil, false
	}
	if subsection == "javascript_families" {
		return []string{"spawn", "tournament"}, true
	}
	return []string{strings.ReplaceAll(subsection, "_", "-")}, true
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
