package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
)

func parseGenericSource(raw string) (genericSource, error) {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(lower, "hf://"):
		return parseGenericHFSource(raw)
	case strings.HasPrefix(lower, "file://"):
		return parseGenericFileSource(raw)
	case strings.HasPrefix(lower, "https://"):
		return parseGenericReleaseSource(raw)
	case strings.Contains(raw, "://"):
		return genericSource{}, models.ErrAssetSourceUnsupported
	case looksLikeLocalPath(raw):
		return genericSource{kind: genericSourceLocal, safe: "local://path", localPath: raw}, nil
	default:
		return genericSource{}, models.ErrAssetSourceUnsupported
	}
}

func parseGenericReleaseSource(raw string) (genericSource, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		!strings.Contains(path.Clean(parsed.Path), "/releases/download/") {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\\\x00") {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	checksum := sha256.Sum256([]byte(raw))
	return genericSource{
		kind: genericSourceRelease, safe: "release://" + hex.EncodeToString(checksum[:]),
		artifactURL: raw, revision: hex.EncodeToString(checksum[:]),
	}, nil
}

func parseGenericHFSource(raw string) (genericSource, error) {
	rest := strings.TrimSpace(raw[len("hf://"):])
	if rest == "" || strings.ContainsAny(rest, "\x00?#\\") {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	at := strings.LastIndex(rest, "@")
	base, revision := rest, ""
	if at >= 0 {
		base, revision = rest[:at], rest[at+1:]
	}
	parts := strings.Split(base, "/")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || part == "." || part == ".." || strings.ContainsAny(part, " @\t\r\n") {
			return genericSource{}, models.ErrModelReferenceInvalid
		}
	}
	if strings.ContainsAny(revision, "\x00/@\\?# \t\r\n") {
		return genericSource{}, models.ErrModelRevisionUnresolved
	}
	file := strings.Join(parts[2:], "/")
	safe := "hf://" + parts[0] + "/" + parts[1]
	if file != "" {
		safe += "/" + file
	}
	if revision != "" {
		safe += "@" + revision
	}
	return genericSource{
		kind: genericSourceHF, safe: safe, owner: parts[0], repository: parts[1],
		file: file, revision: revision,
	}, nil
}

func genericHFSafeReference(source genericSource) string {
	safe := "hf://" + source.owner + "/" + source.repository
	if source.file != "" {
		safe += "/" + source.file
	}
	return safe + "@" + source.revision
}

func genericRevisionFailure() error {
	return &models.InvocationFailure{
		Class:   models.InvocationFailureClassRevisionResolution,
		Message: "model source revision could not be resolved to an immutable commit",
		Cause:   models.ErrModelRevisionUnresolved,
	}
}

func parseGenericFileSource(raw string) (genericSource, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !validGenericFileURL(parsed) {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	localPath, err := genericFileURLPath(parsed)
	if err != nil {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	if localPath == "" {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	return genericSource{
		kind: genericSourceFile, safe: "file://local", localPath: filepath.FromSlash(localPath),
	}, nil
}

func validGenericFileURL(parsed *url.URL) bool {
	return parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func genericFileURLPath(parsed *url.URL) (string, error) {
	localPath, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", err
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		if len(parsed.Host) != 2 || parsed.Host[1] != ':' || !isASCIIAlphaByte(parsed.Host[0]) {
			return "", models.ErrModelReferenceInvalid
		}
		localPath = parsed.Host + localPath
	}
	if len(localPath) >= 3 && localPath[0] == '/' && localPath[2] == ':' && isASCIIAlphaByte(localPath[1]) {
		localPath = localPath[1:]
	}
	return localPath, nil
}

func isImmutableGenericRevision(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (s *service) genericAssetURL(source genericSource, name string) string {
	if source.kind == genericSourceRelease {
		return source.artifactURL
	}
	return strings.TrimRight(s.endpoints.BaseURL, "/") + "/" + source.owner + "/" +
		source.repository + "/resolve/" + source.revision + "/" + url.PathEscape(name) + "?download=true"
}

func (s *service) addGenericURLs(source genericSource, artifacts []genericArtifact) {
	if source.kind != genericSourceHF {
		return
	}
	for index := range artifacts {
		if artifacts[index].url == "" {
			artifacts[index].url = s.genericAssetURL(source, artifacts[index].requirement.Name)
		}
	}
}

func sourceDisplayName(source genericSource) string {
	if source.kind == genericSourceHF {
		return source.owner + "/" + source.repository
	}
	return "local-model"
}

func sourceMetadata(source genericSource) models.SourceMetadata {
	if source.kind == genericSourceHF {
		return models.SourceMetadata{Provider: "HUGGINGFACE", Reference: source.owner + "/" + source.repository, Revision: source.revision}
	}
	if source.kind == genericSourceRelease {
		return models.SourceMetadata{Provider: "PINNED_BACKEND", Reference: "pinned-backend", Revision: source.revision}
	}
	return models.SourceMetadata{Provider: "LOCAL", Reference: source.safe}
}

func genericCacheKey(kind string, source genericSource, artifacts []genericArtifact) string {
	names := genericArtifactIdentityNames(artifacts)
	sort.Strings(names)
	return kind + "|" + genericSourceIdentity(source) + "|" + strings.Join(names, ",")
}

func genericSourceIdentity(source genericSource) string {
	if source.kind != genericSourceLocal && source.kind != genericSourceFile {
		return source.safe
	}
	checksum := sha256.Sum256([]byte(source.localPath))
	return source.safe + "|" + hex.EncodeToString(checksum[:])
}

func genericArtifactIdentityNames(artifacts []genericArtifact) []string {
	names := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		requirement := artifact.requirement
		names = append(names, fmt.Sprintf(
			"%s:%d:%s",
			requirement.Name,
			requirement.Bytes,
			strings.ToLower(strings.TrimSpace(requirement.SHA256)),
		))
	}
	return names
}

func genericArtifactIdentityHash(kind string, source genericSource, artifacts []genericArtifact) string {
	return genericIdentityHash(kind, source, genericArtifactIdentityNames(artifacts))
}

func genericIdentityHash(kind string, source genericSource, names []string) string {
	identity := kind + "|" + genericSourceIdentity(source)
	if len(names) > 0 {
		cloned := append([]string(nil), names...)
		sort.Strings(cloned)
		identity += "|" + strings.Join(cloned, ",")
	}
	hash := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(hash[:])
}

func missingArtifactNames(artifacts []genericArtifact) []string {
	names := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		names = append(names, artifact.requirement.Name)
	}
	sort.Strings(names)
	return uniqueStrings(names)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func isASCIIAlphaByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func looksLikeLocalPath(value string) bool {
	return filepath.IsAbs(value) || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") ||
		strings.HasPrefix(value, ".\\") || strings.HasPrefix(value, "..\\") ||
		strings.ContainsAny(value, `/\\`) || filepath.Ext(value) != ""
}

func (s *service) inspectLegacyGenericCache(
	ctx context.Context,
	kind string,
	source genericSource,
	artifacts []genericArtifact,
	roots []string,
) (map[string]genericCachePath, bool, error) {
	if len(roots) == 0 || !genericArtifactsHaveTrustedFacts(artifacts) {
		return nil, false, nil
	}
	ownedRoot := roots[len(roots)-1]
	records := s.discoverContentAddressedRecords(ownedRoot, kind, source)
	var candidate *genericCacheRecord
	for index := range records {
		record := records[index]
		if !isRepairableGenericCacheRecord(record, kind, source, artifacts) {
			continue
		}
		if candidate != nil {
			// Multiple stale records for the same source and artifact set do
			// not identify a unique owner. Leave them untouched.
			return nil, false, nil
		}
		candidate = &records[index]
	}
	if candidate == nil {
		return nil, false, nil
	}

	cached := make(map[string]genericCachePath, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.Join(candidate.snapshotPath, filepath.FromSlash(artifact.requirement.Name))
		found, ok, err := s.verifyGenericCachedArtifact(ctx, path, artifact.requirement)
		if err != nil {
			return cached, false, err
		}
		if !ok {
			return nil, false, nil
		}
		found.legacySnapshotPath = candidate.snapshotPath
		found.legacyMetadataPath = candidate.metadataPath
		found.legacyRoot = candidate.root
		found.legacyMetadata = candidate.metadata
		cached[artifact.requirement.Name] = found
	}
	return cached, true, nil
}

func (s *service) verifyGenericCachedArtifact(
	ctx context.Context,
	path string,
	requirement models.AssetRequirement,
) (genericCachePath, bool, error) {
	info, err := s.inspectPath(path)
	if err != nil || info == nil || !info.Mode().IsRegular() {
		return genericCachePath{}, false, nil
	}
	actual, err := s.fileSHA256(ctx, path)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return genericCachePath{}, false, contextErr
		}
		return genericCachePath{}, false, nil
	}
	if requirement.Bytes <= 0 || info.Size() != requirement.Bytes {
		return genericCachePath{}, false, nil
	}
	expected := strings.ToLower(strings.TrimSpace(requirement.SHA256))
	if expected == "" || actual != expected {
		return genericCachePath{}, false, nil
	}
	return genericCachePath{
		artifact: models.AssetArtifact{
			Name: requirement.Name, Bytes: info.Size(), SHA256: actual,
		},
		path: path,
	}, true, nil
}

func genericArtifactsHaveTrustedFacts(artifacts []genericArtifact) bool {
	if len(artifacts) == 0 {
		return false
	}
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.requirement.Name) == "" || artifact.requirement.Bytes <= 0 ||
			strings.TrimSpace(artifact.requirement.SHA256) == "" {
			return false
		}
	}
	return true
}

func isRepairableGenericCacheRecord(
	record genericCacheRecord,
	kind string,
	source genericSource,
	artifacts []genericArtifact,
) bool {
	metadata := record.metadata
	if !genericCacheMetadataNeedsRepair(metadata) || metadata.Kind != kind ||
		metadata.Source != source.safe || metadata.SourceKey != genericSourceIdentity(source) ||
		len(metadata.Artifacts) != len(artifacts) || len(metadata.Artifacts) == 0 {
		return false
	}
	metadataArtifacts := genericArtifactsFromRequirements(metadata.Artifacts)
	requested := genericArtifactRequirements(artifacts)
	if genericCacheKey(kind, source, metadataArtifacts) != metadata.Identity ||
		genericArtifactIdentityHash(kind, source, metadataArtifacts) != filepath.Base(record.snapshotPath) ||
		!sameGenericArtifactNames(metadata.Artifacts, requested) ||
		!legacyMetadataFactsAgree(metadata.Artifacts, requested) {
		return false
	}
	return true
}

func genericCacheMetadataNeedsRepair(metadata genericCacheMetadata) bool {
	if len(metadata.Artifacts) == 0 {
		return false
	}
	for _, artifact := range metadata.Artifacts {
		if artifact.Bytes < 0 {
			return false
		}
		if artifact.Bytes == 0 || strings.TrimSpace(artifact.SHA256) == "" {
			return true
		}
	}
	return false
}

func genericArtifactsFromRequirements(requirements []models.AssetRequirement) []genericArtifact {
	artifacts := make([]genericArtifact, 0, len(requirements))
	for _, requirement := range requirements {
		artifacts = append(artifacts, genericArtifact{requirement: requirement})
	}
	return artifacts
}

func genericArtifactRequirements(artifacts []genericArtifact) []models.AssetRequirement {
	requirements := make([]models.AssetRequirement, 0, len(artifacts))
	for _, artifact := range artifacts {
		requirements = append(requirements, artifact.requirement)
	}
	return requirements
}

func sameGenericArtifactNames(first, second []models.AssetRequirement) bool {
	if len(first) != len(second) {
		return false
	}
	seen := make(map[string]struct{}, len(first))
	for _, artifact := range first {
		name := filepath.ToSlash(strings.TrimSpace(artifact.Name))
		if !validGenericArtifactName(name) {
			return false
		}
		if _, ok := seen[name]; ok {
			return false
		}
		seen[name] = struct{}{}
	}
	for _, artifact := range second {
		name := filepath.ToSlash(strings.TrimSpace(artifact.Name))
		if !validGenericArtifactName(name) {
			return false
		}
		if _, ok := seen[name]; !ok {
			return false
		}
		delete(seen, name)
	}
	return len(seen) == 0
}

func legacyMetadataFactsAgree(
	metadata []models.AssetRequirement,
	requested []models.AssetRequirement,
) bool {
	requestedByName := make(map[string]models.AssetRequirement, len(requested))
	for _, artifact := range requested {
		requestedByName[filepath.ToSlash(strings.TrimSpace(artifact.Name))] = artifact
	}
	for _, artifact := range metadata {
		name := filepath.ToSlash(strings.TrimSpace(artifact.Name))
		requirement, ok := requestedByName[name]
		if !ok {
			return false
		}
		// A non-zero legacy fact is retained only when it agrees with the
		// caller's complete requirement. Zero and blank values are the sole
		// facts this compatibility path is allowed to fill.
		if artifact.Bytes < 0 || (artifact.Bytes > 0 && artifact.Bytes != requirement.Bytes) {
			return false
		}
		if digest := strings.TrimSpace(artifact.SHA256); digest != "" &&
			!strings.EqualFold(digest, strings.TrimSpace(requirement.SHA256)) {
			return false
		}
	}
	return true
}

func validGenericArtifactName(name string) bool {
	raw := strings.TrimSpace(name)
	if raw == "" || strings.ContainsAny(raw, "\\\x00") {
		return false
	}
	pathName := filepath.FromSlash(raw)
	if filepath.IsAbs(pathName) || filepath.VolumeName(pathName) != "" {
		return false
	}
	name = filepath.ToSlash(raw)
	return name != "." && name != ".." && !strings.HasPrefix(name, "/") &&
		filepath.ToSlash(filepath.Clean(filepath.FromSlash(name))) == name
}

func (s *service) repairLegacyGenericCache(
	ctx context.Context,
	kind string,
	source genericSource,
	artifacts []genericArtifact,
	cached map[string]genericCachePath,
	roots []string,
) (map[string]genericCachePath, error) {
	record, ok := legacyGenericCacheRecord(cached, artifacts)
	if !legacyGenericCacheCanBeRepaired(record, ok, kind, source, artifacts, roots) {
		return cached, nil
	}
	observed, err := s.reverifyLegacyGenericArtifacts(ctx, record, artifacts)
	if err != nil {
		return cached, err
	}
	return s.commitLegacyGenericCacheRepair(ctx, kind, source, record, observed, cached)
}

func legacyGenericCacheCanBeRepaired(
	record genericCacheRecord,
	found bool,
	kind string,
	source genericSource,
	artifacts []genericArtifact,
	roots []string,
) bool {
	return found && genericArtifactsHaveTrustedFacts(artifacts) && len(roots) > 0 &&
		filepath.Clean(record.root) == filepath.Clean(roots[len(roots)-1]) &&
		isRepairableGenericCacheRecord(record, kind, source, artifacts)
}

func (s *service) commitLegacyGenericCacheRepair(
	ctx context.Context,
	kind string,
	source genericSource,
	record genericCacheRecord,
	observed []genericArtifact,
	cached map[string]genericCachePath,
) (map[string]genericCachePath, error) {
	if err := assetContextError(ctx); err != nil {
		return cached, err
	}
	finalPath, err := s.prepareLegacyRepairDestination(record, kind, source, observed)
	if err != nil {
		return cached, err
	}
	if err := assetContextError(ctx); err != nil {
		return cached, err
	}
	backupPath, hadMetadata, err := s.replaceLegacyGenericMetadata(record, kind, source, observed)
	if err != nil {
		return cached, err
	}
	if err := assetContextError(ctx); err != nil {
		restoreErr := s.restoreLegacyMetadata(record.metadataPath, backupPath, hadMetadata)
		return cached, joinLegacyRepairErrors(err, restoreErr)
	}
	if err := s.renamePath(record.snapshotPath, finalPath); err != nil {
		restoreErr := s.restoreLegacyMetadata(record.metadataPath, backupPath, hadMetadata)
		return cached, joinLegacyRepairErrors(
			legacyRepairInterruption("move legacy cache snapshot", err), restoreErr,
		)
	}
	if err := assetContextError(ctx); err != nil {
		return s.rollbackLegacyRepairResult(
			cached, err, finalPath, record.snapshotPath, backupPath, hadMetadata,
		)
	}

	repaired, err := s.reverifyRepairedGenericArtifacts(ctx, finalPath, observed)
	if err != nil {
		return s.rollbackLegacyRepairResult(
			cached, err, finalPath, record.snapshotPath, backupPath, hadMetadata,
		)
	}
	if err := assetContextError(ctx); err != nil {
		return s.rollbackLegacyRepairResult(
			cached, err, finalPath, record.snapshotPath, backupPath, hadMetadata,
		)
	}
	if err := s.removeLegacyMetadataBackup(finalPath, backupPath, hadMetadata); err != nil {
		return repaired, err
	}
	return repaired, nil
}

func (s *service) prepareLegacyRepairDestination(
	record genericCacheRecord,
	kind string,
	source genericSource,
	observed []genericArtifact,
) (string, error) {
	identity := genericArtifactIdentityHash(kind, source, observed)
	finalPath := filepath.Join(filepath.Dir(record.snapshotPath), identity)
	if filepath.Clean(finalPath) == filepath.Clean(record.snapshotPath) {
		return "", fmt.Errorf(
			"%w: legacy cache identity is not incomplete", models.ErrAssetIntegrityFailed,
		)
	}
	if err := s.ensureLegacyRepairDestinationAbsent(finalPath); err != nil {
		return "", err
	}
	return finalPath, nil
}

func (s *service) rollbackLegacyRepairResult(
	cached map[string]genericCachePath,
	primary error,
	currentPath string,
	legacyPath string,
	backupPath string,
	hadMetadata bool,
) (map[string]genericCachePath, error) {
	return cached, joinLegacyRepairErrors(
		primary, s.rollbackLegacyRepair(currentPath, legacyPath, backupPath, hadMetadata),
	)
}

func (s *service) replaceLegacyGenericMetadata(
	record genericCacheRecord,
	kind string,
	source genericSource,
	observed []genericArtifact,
) (string, bool, error) {
	metadataStagePath := record.metadataPath + ".partial"
	if err := s.removeTree(metadataStagePath); err != nil {
		return "", false, legacyRepairInterruption("clear legacy metadata staging", err)
	}
	if err := s.writeGenericMetadata(
		metadataStagePath, kind, genericCacheKey(kind, source, observed), source.safe,
		genericSourceIdentity(source), observed,
	); err != nil {
		cleanupErr := s.removeTree(metadataStagePath)
		return "", false, joinLegacyRepairErrors(
			legacyRepairInterruption("stage legacy cache metadata", err), cleanupErr,
		)
	}
	backupPath, hadMetadata, err := s.moveExistingGenericMetadata(record.metadataPath)
	if err != nil {
		cleanupErr := s.removeTree(metadataStagePath)
		return "", false, joinLegacyRepairErrors(
			legacyRepairInterruption("backup legacy cache metadata", err), cleanupErr,
		)
	}
	if err := s.renamePath(metadataStagePath, record.metadataPath); err != nil {
		restoreErr := s.restoreLegacyMetadata(record.metadataPath, backupPath, hadMetadata)
		cleanupErr := s.removeTree(metadataStagePath)
		return "", false, joinLegacyRepairErrors(
			legacyRepairInterruption("replace legacy cache metadata", err),
			joinLegacyRepairErrors(restoreErr, cleanupErr),
		)
	}
	return backupPath, hadMetadata, nil
}

func (s *service) removeLegacyMetadataBackup(path, backupPath string, hadMetadata bool) error {
	if !hadMetadata {
		return nil
	}
	backup := filepath.Join(path, filepath.Base(backupPath))
	if err := s.removePath(backup); err != nil {
		return legacyRepairInterruption("clean legacy cache metadata backup", err)
	}
	return nil
}

func legacyGenericCacheRecord(
	cached map[string]genericCachePath,
	artifacts []genericArtifact,
) (genericCacheRecord, bool) {
	var record genericCacheRecord
	for _, artifact := range artifacts {
		found, ok := cached[artifact.requirement.Name]
		if !ok || found.legacySnapshotPath == "" || found.legacyMetadataPath == "" {
			return genericCacheRecord{}, false
		}
		if record.snapshotPath == "" {
			record = genericCacheRecord{
				root:         found.legacyRoot,
				snapshotPath: found.legacySnapshotPath,
				metadataPath: found.legacyMetadataPath,
				metadata:     found.legacyMetadata,
			}
			continue
		}
		if record.snapshotPath != found.legacySnapshotPath || record.metadataPath != found.legacyMetadataPath {
			return genericCacheRecord{}, false
		}
		if record.root != found.legacyRoot {
			return genericCacheRecord{}, false
		}
	}
	return record, record.snapshotPath != ""
}

func (s *service) reverifyLegacyGenericArtifacts(
	ctx context.Context,
	record genericCacheRecord,
	artifacts []genericArtifact,
) ([]genericArtifact, error) {
	observed := make([]genericArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.Join(record.snapshotPath, filepath.FromSlash(artifact.requirement.Name))
		found, ok, err := s.verifyGenericCachedArtifact(ctx, path, artifact.requirement)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf(
				"%w: legacy asset %q changed before repair", models.ErrAssetIntegrityFailed,
				artifact.requirement.Name,
			)
		}
		observed = append(observed, genericArtifact{
			requirement: models.AssetRequirement{
				Name:   artifact.requirement.Name,
				Bytes:  found.artifact.Bytes,
				SHA256: found.artifact.SHA256,
			},
			metadataResolved: true,
		})
	}
	return observed, nil
}

func (s *service) ensureLegacyRepairDestinationAbsent(path string) error {
	info, err := s.inspectPath(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return legacyRepairInterruption("inspect legacy cache destination", err)
	}
	if info == nil {
		return legacyRepairInterruption("inspect legacy cache destination", errors.New("destination info unavailable"))
	}
	return fmt.Errorf("%w: legacy cache identity collision", models.ErrAssetIntegrityFailed)
}

func (s *service) reverifyRepairedGenericArtifacts(
	ctx context.Context,
	root string,
	artifacts []genericArtifact,
) (map[string]genericCachePath, error) {
	repaired := make(map[string]genericCachePath, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.Join(root, filepath.FromSlash(artifact.requirement.Name))
		found, ok, err := s.verifyGenericCachedArtifact(ctx, path, artifact.requirement)
		if err != nil {
			return repaired, err
		}
		if !ok {
			return repaired, legacyRepairInterruption(
				"validate repaired legacy cache artifact", errors.New("verified file changed"),
			)
		}
		repaired[artifact.requirement.Name] = found
	}
	return repaired, nil
}

func (s *service) rollbackLegacyRepair(
	currentPath string,
	legacyPath string,
	backupPath string,
	hadMetadata bool,
) error {
	if err := s.renamePath(currentPath, legacyPath); err != nil {
		return legacyRepairInterruption("restore legacy cache snapshot", err)
	}
	return s.restoreLegacyMetadata(filepath.Join(legacyPath, assetMetadataName), backupPath, hadMetadata)
}

func (s *service) restoreLegacyMetadata(path, backupPath string, hadMetadata bool) error {
	if !hadMetadata {
		return nil
	}
	if err := s.removePath(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return legacyRepairInterruption("remove replacement legacy metadata", err)
	}
	if err := s.renamePath(backupPath, path); err != nil {
		return legacyRepairInterruption("restore legacy cache metadata", err)
	}
	return nil
}

func legacyRepairInterruption(operation string, cause error) error {
	return pullsupport.WrapPullStage(
		models.PullStageCacheInstallation, "", operation, "",
		interruptedAssetError(operation, cause),
	)
}

func joinLegacyRepairErrors(primary, cleanup error) error {
	if primary == nil {
		return cleanup
	}
	if cleanup == nil {
		return primary
	}
	return errors.Join(primary, legacyRepairInterruption("clean legacy cache repair", cleanup))
}
