package main

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
)

const packageBoundaryBaseRefEnvironment = "PACKAGE_BOUNDARY_BASE_REF"

// recordedBoundaryBaseline is the presentation view of the repository's
// known-baseline policy. The package-boundary rule matchers remain the source
// of truth for what is a finding; this layer compares the current classified
// findings with the same checker run against the selected base tree.
type recordedBoundaryBaseline struct {
	available           bool
	baseRef             string
	findingFingerprints map[string]struct{}
}

func loadRecordedBoundaryBaseline(cfg config, policy boundaryPolicy) (recordedBoundaryBaseline, error) {
	requestedBaseRef := strings.TrimSpace(cfg.baseRef)
	if requestedBaseRef == "" {
		requestedBaseRef = strings.TrimSpace(os.Getenv(packageBoundaryBaseRefEnvironment))
	}
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return recordedBoundaryBaseline{}, nil
	}
	if _, ok := runGit(repoRoot, "rev-parse", "--show-toplevel"); !ok {
		if requestedBaseRef != "" {
			return recordedBoundaryBaseline{}, fmt.Errorf("package-boundary base ref %q could not be resolved: root is not a Git repository", requestedBaseRef)
		}
		return recordedBoundaryBaseline{}, nil
	}

	selectedRef, err := selectRecordedBoundaryBaseRef(repoRoot, requestedBaseRef)
	if err != nil {
		return recordedBoundaryBaseline{}, err
	}
	if selectedRef == "" {
		return recordedBoundaryBaseline{}, nil
	}

	cacheFile := baselineCacheFile(cfg, repoRoot, selectedRef)
	if fingerprints, ok := readBaselineCache(cacheFile); ok {
		return recordedBoundaryBaseline{available: true, baseRef: selectedRef, findingFingerprints: fingerprints}, nil
	}

	baseRoot, err := extractGitTree(repoRoot, selectedRef)
	if err != nil {
		// Fail open for presentation: if the base cannot be materialized, keep
		// every current finding visible rather than treating it as recorded.
		return recordedBoundaryBaseline{}, nil
	}
	defer os.RemoveAll(baseRoot)

	baseResult, err := scanBoundaryRepo(config{root: baseRoot, packageRoot: cfg.packageRoot}, policy)
	if err != nil {
		return recordedBoundaryBaseline{}, nil
	}
	fingerprints := boundaryFindingFingerprints(baseResult)
	writeBaselineCache(cacheFile, fingerprints)
	return recordedBoundaryBaseline{
		available:           true,
		baseRef:             selectedRef,
		findingFingerprints: fingerprints,
	}, nil
}

func selectRecordedBoundaryBaseRef(repoRoot, requestedBaseRef string) (string, error) {
	if requestedBaseRef != "" {
		if _, ok := runGit(repoRoot, "rev-parse", "--verify", requestedBaseRef+"^{commit}"); !ok {
			return "", fmt.Errorf("package-boundary base ref %q could not be resolved", requestedBaseRef)
		}
		return requestedBaseRef, nil
	}

	for _, candidate := range []string{"origin/main", "upstream/main", "main"} {
		if _, ok := runGit(repoRoot, "rev-parse", "--verify", candidate+"^{commit}"); ok {
			return candidate, nil
		}
	}
	return "", nil
}

func runGit(repoRoot string, args ...string) ([]byte, bool) {
	commandArgs := append([]string{"-C", repoRoot}, args...)
	output, err := exec.Command("git", commandArgs...).Output()
	if err != nil {
		return nil, false
	}
	return output, true
}

func extractGitTree(repoRoot, ref string) (string, error) {
	treeRoot, err := os.MkdirTemp("", "pkg-boundary-base-")
	if err != nil {
		return "", fmt.Errorf("create package-boundary base tree: %w", err)
	}

	command := exec.Command("git", "-C", repoRoot, "archive", "--format=tar", ref)
	archiveBytes, err := command.Output()
	if err != nil {
		_ = os.RemoveAll(treeRoot)
		return "", fmt.Errorf("read package-boundary base archive: %w", err)
	}

	if err := extractGitArchive(treeRoot, bytes.NewReader(archiveBytes)); err != nil {
		_ = os.RemoveAll(treeRoot)
		return "", fmt.Errorf("extract package-boundary base archive: %w", err)
	}
	return treeRoot, nil
}

func extractGitArchive(root string, reader io.Reader) error {
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		relativePath := filepath.FromSlash(filepath.Clean(header.Name))
		if relativePath == "." || filepath.IsAbs(relativePath) {
			return fmt.Errorf("archive entry %q is not repository-relative", header.Name)
		}
		resolvedPath := filepath.Join(root, relativePath)
		relativeToRoot, err := filepath.Rel(root, resolvedPath)
		if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive entry %q escapes extraction root", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(resolvedPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(resolvedPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode)&0o777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, tarReader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
				return err
			}
			// Git repositories used by this checker may contain symlinks, but
			// Windows checkouts cannot always recreate them without elevation.
			// The package scanners do not follow symlink directories, so omitting
			// the link keeps the base comparison safe and portable.
			continue
		default:
			// Git archive metadata entries carry no source content needed by the
			// scanner. Consume their payload and continue to the next member.
			if _, err := io.Copy(io.Discard, tarReader); err != nil {
				return err
			}
		}
	}
}

func recordedFindingsFromPartition[T any](all, blocking []T, key func(T) string) []T {
	blockingCounts := make(map[string]int, len(blocking))
	for _, finding := range blocking {
		blockingCounts[key(finding)]++
	}
	var recorded []T
	for _, finding := range all {
		findingKey := key(finding)
		if blockingCounts[findingKey] > 0 {
			blockingCounts[findingKey]--
			continue
		}
		recorded = append(recorded, finding)
	}
	return recorded
}

func splitRecordedFindings[T any](findings []T, fingerprint func(T) string, baseline recordedBoundaryBaseline) (visible, recorded []T) {
	if !baseline.available {
		return findings, nil
	}
	for _, finding := range findings {
		if _, found := baseline.findingFingerprints[fingerprint(finding)]; found {
			recorded = append(recorded, finding)
			continue
		}
		visible = append(visible, finding)
	}
	return visible, recorded
}

func filterRecordedScanResult(result scanResult, baseline recordedBoundaryBaseline) (scanResult, scanResult) {
	visible := result
	recorded := newRecordedScanResult(result)
	filterRecordedPackageFindings(&visible, &recorded, baseline)
	filterRecordedServiceFindings(&visible, &recorded, baseline)
	filterRecordedRuntimeFindings(&visible, &recorded, baseline)
	clearVisibleRecordedFindings(&visible)
	return visible, recorded
}

func newRecordedScanResult(result scanResult) scanResult {
	return scanResult{
		serviceConstructionBaselineCount:    result.serviceConstructionBaselineCount,
		productionDefaultBaselineCount:      result.productionDefaultBaselineCount,
		recordedServiceConstructionFindings: append([]serviceConstructionFinding(nil), result.recordedServiceConstructionFindings...),
		recordedProductionDefaultFindings:   append([]productionDefaultFinding(nil), result.recordedProductionDefaultFindings...),
	}
}

func filterRecordedPackageFindings(visible, recorded *scanResult, baseline recordedBoundaryBaseline) {
	visible.rootPackageFindings, recorded.rootPackageFindings = splitRecordedFindings(visible.rootPackageFindings, func(finding rootPackageFinding) string {
		return boundaryFindingFingerprint("root-package", finding)
	}, baseline)
	visible.retiredPackageRootFindings, recorded.retiredPackageRootFindings = splitRecordedFindings(visible.retiredPackageRootFindings, func(finding retiredPackageRootFinding) string {
		return boundaryFindingFingerprint("retired-package-root", finding)
	}, baseline)
}

func filterRecordedServiceFindings(visible, recorded *scanResult, baseline recordedBoundaryBaseline) {
	visible.serviceConstructionFindings, recorded.serviceConstructionFindings = splitRecordedFindings(visible.serviceConstructionFindings, func(finding serviceConstructionFinding) string {
		return boundaryFindingFingerprint("service-construction", finding)
	}, baseline)
}

func filterRecordedRuntimeFindings(visible, recorded *scanResult, baseline recordedBoundaryBaseline) {
	visible.productionDefaultFindings, recorded.productionDefaultFindings = splitRecordedFindings(visible.productionDefaultFindings, func(finding productionDefaultFinding) string {
		return boundaryFindingFingerprint("production-default", finding)
	}, baseline)
}

func clearVisibleRecordedFindings(visible *scanResult) {
	visible.recordedServiceConstructionFindings = nil
	visible.recordedProductionDefaultFindings = nil
}

// volatileBoundaryFindingFields names finding fields that move when unrelated
// code moves. A recorded baseline identifies a known finding across two trees,
// so it must survive code motion: splitting a file, extracting a helper, or
// adding an import above a pre-existing violation all shift its line number
// without changing what the violation is. Including the line made every such
// edit re-surface the whole file's recorded findings as new, which failed
// Backend Lint for lanes that introduced no violation at all.
//
// Occurrence counts stay in the fingerprint on purpose: a second construction
// added to a file raises count, and that is genuine growth the ratchet must
// still catch.
var volatileBoundaryFindingFields = map[string]struct{}{"line": {}}

func boundaryFindingFingerprint(category string, finding any) string {
	value := reflect.ValueOf(finding)
	if value.Kind() != reflect.Struct {
		return fmt.Sprintf("%s:%T:%#v", category, finding, finding)
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "%s:%T:", category, finding)
	fields := value.Type()
	for index := range value.NumField() {
		name := fields.Field(index).Name
		if _, volatile := volatileBoundaryFindingFields[strings.ToLower(name)]; volatile {
			continue
		}
		// Format the reflect.Value itself: these finding structs keep their
		// fields unexported, so Interface() would panic here.
		fmt.Fprintf(&builder, "%s=%v;", name, value.Field(index))
	}
	return builder.String()
}

func boundaryFindingFingerprints(result scanResult) map[string]struct{} {
	fingerprints := make(map[string]struct{})
	addBoundaryFindingFingerprints(fingerprints, "root-package", result.rootPackageFindings)
	addBoundaryFindingFingerprints(fingerprints, "retired-package-root", result.retiredPackageRootFindings)
	addBoundaryFindingFingerprints(fingerprints, "service-construction", result.serviceConstructionFindings)
	addBoundaryFindingFingerprints(fingerprints, "service-construction", result.recordedServiceConstructionFindings)
	addBoundaryFindingFingerprints(fingerprints, "production-default", result.productionDefaultFindings)
	addBoundaryFindingFingerprints(fingerprints, "production-default", result.recordedProductionDefaultFindings)
	return fingerprints
}

func addBoundaryFindingFingerprints[T any](fingerprints map[string]struct{}, category string, findings []T) {
	for _, finding := range findings {
		fingerprints[boundaryFindingFingerprint(category, finding)] = struct{}{}
	}
}
