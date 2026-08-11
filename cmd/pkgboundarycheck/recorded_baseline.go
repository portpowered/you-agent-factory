package main

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

	baseRoot, err := extractGitTree(repoRoot, selectedRef)
	if err != nil {
		// Fail open for presentation: if the base cannot be materialized, keep
		// every current finding visible rather than treating it as recorded.
		return recordedBoundaryBaseline{}, nil
	}
	defer os.RemoveAll(baseRoot)

	baseResult, err := scanRepo(config{root: baseRoot, packageRoot: cfg.packageRoot}, policy)
	if err != nil {
		return recordedBoundaryBaseline{}, nil
	}
	return recordedBoundaryBaseline{
		available:           true,
		baseRef:             selectedRef,
		findingFingerprints: boundaryFindingFingerprints(baseResult),
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
		peerServiceBaselineCount:             result.peerServiceBaselineCount,
		testServiceBaselineCount:             result.testServiceBaselineCount,
		supportServiceBaselineCount:          result.supportServiceBaselineCount,
		serviceConstructionBaselineCount:     result.serviceConstructionBaselineCount,
		transportBehaviorBaselineCount:       result.transportBehaviorBaselineCount,
		productionDefaultBaselineCount:       result.productionDefaultBaselineCount,
		initializerBehaviorBaselineCount:     result.initializerBehaviorBaselineCount,
		testBehaviorBaselineCount:            result.testBehaviorBaselineCount,
		petriPublicSurfaceBaselineCount:      result.petriPublicSurfaceBaselineCount,
		recordedPeerServiceImportFindings:    append([]peerServiceImportFinding(nil), result.recordedPeerServiceImportFindings...),
		recordedTestServiceImportFindings:    append([]testServiceImportFinding(nil), result.recordedTestServiceImportFindings...),
		recordedSupportServiceImportFindings: append([]supportServiceImportFinding(nil), result.recordedSupportServiceImportFindings...),
		recordedServiceConstructionFindings:  append([]serviceConstructionFinding(nil), result.recordedServiceConstructionFindings...),
		recordedTransportBehaviorFindings:    append([]transportBehaviorFinding(nil), result.recordedTransportBehaviorFindings...),
		recordedProductionDefaultFindings:    append([]productionDefaultFinding(nil), result.recordedProductionDefaultFindings...),
		recordedInitializerBehaviorFindings:  append([]initializerBehaviorFinding(nil), result.recordedInitializerBehaviorFindings...),
		recordedTestBehaviorFindings:         append([]testBehaviorFinding(nil), result.recordedTestBehaviorFindings...),
		recordedPetriPublicSurfaceFindings:   append([]petriPublicSurfaceFinding(nil), result.recordedPetriPublicSurfaceFindings...),
	}
}

func filterRecordedPackageFindings(visible, recorded *scanResult, baseline recordedBoundaryBaseline) {
	visible.rootPackageFindings, recorded.rootPackageFindings = splitRecordedFindings(visible.rootPackageFindings, func(finding rootPackageFinding) string {
		return boundaryFindingFingerprint("root-package", finding)
	}, baseline)
	visible.retiredPackageRootFindings, recorded.retiredPackageRootFindings = splitRecordedFindings(visible.retiredPackageRootFindings, func(finding retiredPackageRootFinding) string {
		return boundaryFindingFingerprint("retired-package-root", finding)
	}, baseline)
	visible.migrationShimFindings, recorded.migrationShimFindings = splitRecordedFindings(visible.migrationShimFindings, func(finding migrationShimFinding) string {
		return boundaryFindingFingerprint("migration-shim", finding)
	}, baseline)
	visible.retiredPackageImportFindings, recorded.retiredPackageImportFindings = splitRecordedFindings(visible.retiredPackageImportFindings, func(finding retiredPackageImportFinding) string {
		return boundaryFindingFingerprint("retired-package-import", finding)
	}, baseline)
	visible.applicationGraphImportFindings, recorded.applicationGraphImportFindings = splitRecordedFindings(visible.applicationGraphImportFindings, func(finding applicationGraphImportFinding) string {
		return boundaryFindingFingerprint("application-graph-import", finding)
	}, baseline)
	visible.handwrittenGeneratedFindings, recorded.handwrittenGeneratedFindings = splitRecordedFindings(visible.handwrittenGeneratedFindings, func(finding handwrittenGeneratedFinding) string {
		return boundaryFindingFingerprint("handwritten-generated", finding)
	}, baseline)
	visible.domainTransportFindings, recorded.domainTransportFindings = splitRecordedFindings(visible.domainTransportFindings, func(finding domainTransportImportFinding) string {
		return boundaryFindingFingerprint("domain-transport-import", finding)
	}, baseline)
}

func filterRecordedServiceFindings(visible, recorded *scanResult, baseline recordedBoundaryBaseline) {
	visible.peerServiceImportFindings, recorded.peerServiceImportFindings = splitRecordedFindings(visible.peerServiceImportFindings, func(finding peerServiceImportFinding) string {
		return boundaryFindingFingerprint("peer-service-import", finding)
	}, baseline)
	visible.testServiceImportFindings, recorded.testServiceImportFindings = splitRecordedFindings(visible.testServiceImportFindings, func(finding testServiceImportFinding) string {
		return boundaryFindingFingerprint("test-service-import", finding)
	}, baseline)
	visible.supportServiceImportFindings, recorded.supportServiceImportFindings = splitRecordedFindings(visible.supportServiceImportFindings, func(finding supportServiceImportFinding) string {
		return boundaryFindingFingerprint("support-service-import", finding)
	}, baseline)
	visible.serviceConstructionFindings, recorded.serviceConstructionFindings = splitRecordedFindings(visible.serviceConstructionFindings, func(finding serviceConstructionFinding) string {
		return boundaryFindingFingerprint("service-construction", finding)
	}, baseline)
	visible.transportImplementationFindings, recorded.transportImplementationFindings = splitRecordedFindings(visible.transportImplementationFindings, func(finding transportServiceImplementationFinding) string {
		return boundaryFindingFingerprint("transport-implementation", finding)
	}, baseline)
	visible.externalImplementationFindings, recorded.externalImplementationFindings = splitRecordedFindings(visible.externalImplementationFindings, func(finding transportServiceImplementationFinding) string {
		return boundaryFindingFingerprint("external-implementation", finding)
	}, baseline)
}

func filterRecordedRuntimeFindings(visible, recorded *scanResult, baseline recordedBoundaryBaseline) {
	visible.transportBehaviorFindings, recorded.transportBehaviorFindings = splitRecordedFindings(visible.transportBehaviorFindings, func(finding transportBehaviorFinding) string {
		return boundaryFindingFingerprint("transport-behavior", finding)
	}, baseline)
	visible.functionalProcessEdgeFindings, recorded.functionalProcessEdgeFindings = splitRecordedFindings(visible.functionalProcessEdgeFindings, func(finding functionalProcessEdgeFinding) string {
		return boundaryFindingFingerprint("functional-process-edge", finding)
	}, baseline)
	visible.constructedServiceEdgesFindings, recorded.constructedServiceEdgesFindings = splitRecordedFindings(visible.constructedServiceEdgesFindings, func(finding constructedServiceEdgesFinding) string {
		return boundaryFindingFingerprint("constructed-service-edge", finding)
	}, baseline)
	visible.testWorkNormalizationFindings, recorded.testWorkNormalizationFindings = splitRecordedFindings(visible.testWorkNormalizationFindings, func(finding testWorkNormalizationFinding) string {
		return boundaryFindingFingerprint("test-work-normalization", finding)
	}, baseline)
	visible.productionDefaultFindings, recorded.productionDefaultFindings = splitRecordedFindings(visible.productionDefaultFindings, func(finding productionDefaultFinding) string {
		return boundaryFindingFingerprint("production-default", finding)
	}, baseline)
	visible.initializerBehaviorFindings, recorded.initializerBehaviorFindings = splitRecordedFindings(visible.initializerBehaviorFindings, func(finding initializerBehaviorFinding) string {
		return boundaryFindingFingerprint("initializer-behavior", finding)
	}, baseline)
	visible.testBehaviorFindings, recorded.testBehaviorFindings = splitRecordedFindings(visible.testBehaviorFindings, func(finding testBehaviorFinding) string { return boundaryFindingFingerprint("test-behavior", finding) }, baseline)
	visible.petriPublicSurfaceFindings, recorded.petriPublicSurfaceFindings = splitRecordedFindings(visible.petriPublicSurfaceFindings, func(finding petriPublicSurfaceFinding) string {
		return boundaryFindingFingerprint("petri-public-surface", finding)
	}, baseline)
	visible.providerEffectOwnershipFindings, recorded.providerEffectOwnershipFindings = splitRecordedFindings(visible.providerEffectOwnershipFindings, func(finding providerEffectOwnershipFinding) string {
		return boundaryFindingFingerprint("provider-effect-ownership", finding)
	}, baseline)
}

func clearVisibleRecordedFindings(visible *scanResult) {
	visible.recordedPeerServiceImportFindings = nil
	visible.recordedTestServiceImportFindings = nil
	visible.recordedSupportServiceImportFindings = nil
	visible.recordedServiceConstructionFindings = nil
	visible.recordedTransportBehaviorFindings = nil
	visible.recordedProductionDefaultFindings = nil
	visible.recordedInitializerBehaviorFindings = nil
	visible.recordedTestBehaviorFindings = nil
	visible.recordedPetriPublicSurfaceFindings = nil
}

func boundaryFindingFingerprint(category string, finding any) string {
	return fmt.Sprintf("%s:%T:%#v", category, finding, finding)
}

func boundaryFindingFingerprints(result scanResult) map[string]struct{} {
	fingerprints := make(map[string]struct{})
	addBoundaryFindingFingerprints(fingerprints, "root-package", result.rootPackageFindings)
	addBoundaryFindingFingerprints(fingerprints, "retired-package-root", result.retiredPackageRootFindings)
	addBoundaryFindingFingerprints(fingerprints, "retired-package-import", result.retiredPackageImportFindings)
	addBoundaryFindingFingerprints(fingerprints, "migration-shim", result.migrationShimFindings)
	addBoundaryFindingFingerprints(fingerprints, "application-graph-import", result.applicationGraphImportFindings)
	addBoundaryFindingFingerprints(fingerprints, "handwritten-generated", result.handwrittenGeneratedFindings)
	addBoundaryFindingFingerprints(fingerprints, "domain-transport-import", result.domainTransportFindings)
	addBoundaryFindingFingerprints(fingerprints, "peer-service-import", result.peerServiceImportFindings)
	addBoundaryFindingFingerprints(fingerprints, "peer-service-import", result.recordedPeerServiceImportFindings)
	addBoundaryFindingFingerprints(fingerprints, "test-service-import", result.testServiceImportFindings)
	addBoundaryFindingFingerprints(fingerprints, "test-service-import", result.recordedTestServiceImportFindings)
	addBoundaryFindingFingerprints(fingerprints, "support-service-import", result.supportServiceImportFindings)
	addBoundaryFindingFingerprints(fingerprints, "support-service-import", result.recordedSupportServiceImportFindings)
	addBoundaryFindingFingerprints(fingerprints, "service-construction", result.serviceConstructionFindings)
	addBoundaryFindingFingerprints(fingerprints, "service-construction", result.recordedServiceConstructionFindings)
	addBoundaryFindingFingerprints(fingerprints, "transport-implementation", result.transportImplementationFindings)
	addBoundaryFindingFingerprints(fingerprints, "external-implementation", result.externalImplementationFindings)
	addBoundaryFindingFingerprints(fingerprints, "transport-behavior", result.transportBehaviorFindings)
	addBoundaryFindingFingerprints(fingerprints, "transport-behavior", result.recordedTransportBehaviorFindings)
	addBoundaryFindingFingerprints(fingerprints, "functional-process-edge", result.functionalProcessEdgeFindings)
	addBoundaryFindingFingerprints(fingerprints, "constructed-service-edge", result.constructedServiceEdgesFindings)
	addBoundaryFindingFingerprints(fingerprints, "test-work-normalization", result.testWorkNormalizationFindings)
	addBoundaryFindingFingerprints(fingerprints, "production-default", result.productionDefaultFindings)
	addBoundaryFindingFingerprints(fingerprints, "production-default", result.recordedProductionDefaultFindings)
	addBoundaryFindingFingerprints(fingerprints, "initializer-behavior", result.initializerBehaviorFindings)
	addBoundaryFindingFingerprints(fingerprints, "initializer-behavior", result.recordedInitializerBehaviorFindings)
	addBoundaryFindingFingerprints(fingerprints, "test-behavior", result.testBehaviorFindings)
	addBoundaryFindingFingerprints(fingerprints, "test-behavior", result.recordedTestBehaviorFindings)
	addBoundaryFindingFingerprints(fingerprints, "petri-public-surface", result.petriPublicSurfaceFindings)
	addBoundaryFindingFingerprints(fingerprints, "petri-public-surface", result.recordedPetriPublicSurfaceFindings)
	addBoundaryFindingFingerprints(fingerprints, "provider-effect-ownership", result.providerEffectOwnershipFindings)
	return fingerprints
}

func addBoundaryFindingFingerprints[T any](fingerprints map[string]struct{}, category string, findings []T) {
	for _, finding := range findings {
		fingerprints[boundaryFindingFingerprint(category, finding)] = struct{}{}
	}
}
