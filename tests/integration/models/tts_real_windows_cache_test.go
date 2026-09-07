//go:build windows

package models_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func localAICacheAdmissionFailure(request localAIRealRunRequest, cache localAIRealCache) *localAIObservationFailure {
	if request.RequireFreshCache && (!cache.FreshAtStart || cache.PartialArtifacts != 0) {
		return &localAIObservationFailure{Owner: "harness", Assertion: "first-use cache freshness", Expected: "model and HF cache are empty before first use", Observed: "cache was not fresh"}
	}
	if request.RequireCacheReuse {
		if cache.BeforeFiles == 0 || cache.BeforeBytes == 0 || cache.PartialArtifacts != 0 || !isLocalAISHA256(cache.BeforeContentIdentitySHA256) {
			return &localAIObservationFailure{Owner: "harness", Assertion: "offline cache reuse admission", Expected: "complete cache content is available without network", Observed: "offline cache content miss"}
		}
		if request.ExpectedCacheContentSHA256 != "" && !strings.EqualFold(request.ExpectedCacheContentSHA256, cache.BeforeContentIdentitySHA256) {
			return &localAIObservationFailure{Owner: "harness", Assertion: "offline cache content identity", Expected: "pinned cache content identity", Observed: "offline cache content mismatch"}
		}
	}
	return nil
}

func finalizeLocalAIReport(request localAIRealRunRequest, report *localAIRealReport) error {
	if len(report.Journeys) != 1 {
		return errors.New("report journey cardinality changed")
	}
	journey := &report.Journeys[0]
	roots := localAIRealRootsForRequest(request)
	after, cacheErr := localAIRealCacheSnapshotForRoots(roots)
	journey.Cache.AfterIdentitySHA256 = after.IdentitySHA256
	journey.Cache.AfterContentIdentitySHA256 = after.ContentIdentitySHA256
	journey.Cache.AfterEntries = after.Entries
	journey.Cache.AfterFiles = after.Files
	journey.Cache.AfterBytes = after.Bytes
	journey.Cache.PartialArtifacts = after.PartialArtifacts
	journey.CacheIdentitySHA256 = after.IdentitySHA256
	journey.Cache.ContentBacked = journey.Cache.BeforeFiles > 0 && journey.Cache.BeforeBytes > 0 && journey.Cache.PartialArtifacts == 0 && isLocalAISHA256(journey.Cache.BeforeContentIdentitySHA256) && (request.ExpectedCacheContentSHA256 == "" || strings.EqualFold(request.ExpectedCacheContentSHA256, journey.Cache.BeforeContentIdentitySHA256))
	journey.Cache.ContentConsumed = request.RequireCacheReuse && journey.Cache.ContentBacked && report.Status == "PASS" && cacheErr == nil && after.Files > 0 && after.Bytes > 0 && after.PartialArtifacts == 0 && strings.EqualFold(after.ContentIdentitySHA256, journey.Cache.BeforeContentIdentitySHA256)
	if journey.Cache.ContentConsumed {
		journey.Cache.ConsumedContentIdentitySHA256 = after.ContentIdentitySHA256
	}
	journey.Cache.Reused = request.RequireCacheReuse && journey.Cache.ContentConsumed
	journey.Release = localAIReleaseFromExecution(journey.Execution)
	partial, partialErr := localAIPartialArtifactCount(roots)
	journey.Release.PartialArtifacts = partial
	if cacheErr != nil {
		journey.Cache.AfterInspectionFailed = true
		journey.Release.Checked = false
		journey.Release.InspectionFailed = true
		return &localAIFinalInspectionError{Stage: "cache snapshot inspection failed", Err: cacheErr}
	}
	if partialErr != nil {
		journey.Release.Checked = false
		journey.Release.InspectionFailed = true
		return &localAIFinalInspectionError{Stage: "partial artifact inspection failed", Err: partialErr}
	}
	return partialErr
}

func localAIHasCacheConsumptionMarker(observation localAICommandObservation, expected string) bool {
	marker := localAIControlledCacheConsumptionMarker + strings.ToLower(strings.TrimSpace(expected))
	return strings.Contains(strings.ToLower(string(observation.Stderr)), marker)
}

func localAIFinalInspectionObservation(err error) string {
	var inspectionErr *localAIFinalInspectionError
	if errors.As(err, &inspectionErr) {
		return inspectionErr.Stage
	}
	return "final inspection failed"
}

type localAIFinalInspectionError struct {
	Stage string
	Err   error
}

func (err *localAIFinalInspectionError) Error() string {
	return err.Stage
}

func (err *localAIFinalInspectionError) Unwrap() error {
	return err.Err
}

func localAIReleaseFromExecution(execution *localAIRealExecution) localAIRealRelease {
	release := localAIRealRelease{Checked: true, ProcessTreeClosed: true}
	if execution == nil {
		return release
	}
	for _, command := range execution.Commands {
		release.OwnedProcesses += command.OwnedProcesses
		release.OwnedListeners += command.OwnedListeners
		release.OwnedLeases += command.OwnedLeases
		if command.Started && (!command.ProcessExited || !command.ProcessTreeClosed) {
			release.ProcessTreeClosed = false
		}
	}
	return release
}

func localAIReadBoundedFile(path string, maxBytes int64) ([]byte, error) {
	return localAIReadBoundedFileWithOpen(path, maxBytes, os.Open)
}

func localAIReadBoundedFileWithOpen(path string, maxBytes int64, open func(string) (*os.File, error)) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errors.New("bounded file limit is invalid")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxBytes {
		return nil, errors.New("file is outside the bounded output limit")
	}
	file, err := open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes || int64(len(body)) != info.Size() {
		return nil, errors.New("file changed while it was read")
	}
	finalInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !finalInfo.Mode().IsRegular() || finalInfo.Size() != info.Size() {
		return nil, errors.New("file changed while it was read")
	}
	return body, nil
}

type localAIRealCacheSnapshot struct {
	IdentitySHA256        string
	ContentIdentitySHA256 string
	Entries               int
	Files                 int
	Bytes                 int64
	PartialArtifacts      int
}

type localAIRealCacheTree struct {
	IdentitySHA256        string
	ContentIdentitySHA256 string
	Entries               int
	Files                 int
	Bytes                 int64
	PartialArtifacts      int
}

func localAIRealCacheSnapshotForRoots(roots localAIRealRoots) (localAIRealCacheSnapshot, error) {
	model, err := readLocalAIRealCacheTree(roots.Cache)
	if err != nil {
		return localAIRealCacheSnapshot{}, err
	}
	huggingFace, err := readLocalAIRealCacheTree(roots.HFCache)
	if err != nil {
		return localAIRealCacheSnapshot{}, err
	}
	hasher := sha256.New()
	_, _ = io.WriteString(hasher, "managed\x00")
	_, _ = io.WriteString(hasher, model.IdentitySHA256)
	_, _ = io.WriteString(hasher, "\nhuggingface\x00")
	_, _ = io.WriteString(hasher, huggingFace.IdentitySHA256)
	contentHasher := sha256.New()
	_, _ = io.WriteString(contentHasher, "managed\x00")
	_, _ = io.WriteString(contentHasher, model.ContentIdentitySHA256)
	_, _ = io.WriteString(contentHasher, "\nhuggingface\x00")
	_, _ = io.WriteString(contentHasher, huggingFace.ContentIdentitySHA256)
	return localAIRealCacheSnapshot{
		IdentitySHA256: hex.EncodeToString(hasher.Sum(nil)), ContentIdentitySHA256: hex.EncodeToString(contentHasher.Sum(nil)),
		Entries: model.Entries + huggingFace.Entries, Files: model.Files + huggingFace.Files,
		Bytes: model.Bytes + huggingFace.Bytes, PartialArtifacts: model.PartialArtifacts + huggingFace.PartialArtifacts,
	}, nil
}

func readLocalAIRealCacheTree(root string) (localAIRealCacheTree, error) {
	entries := map[string]string{}
	contentEntries := map[string]string{}
	var totalBytes int64
	files := 0
	partialArtifacts := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return fs.SkipDir
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("cache symlink is not allowed")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		lower := strings.ToLower(relative)
		if strings.HasSuffix(lower, ".partial") || strings.Contains(lower, "/.partial/") {
			partialArtifacts++
		}
		if entry.IsDir() {
			entries[relative] = "dir"
			return nil
		}
		identity, ok := localAIReadFileIdentity(path)
		if !ok {
			return errors.New("cache file identity could not be read")
		}
		entries[relative] = fmt.Sprintf("file:%d:%s", identity.Bytes, identity.SHA256)
		contentEntries[relative] = entries[relative]
		files++
		totalBytes += identity.Bytes
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, fs.SkipDir) {
		return localAIRealCacheTree{}, err
	}
	paths := make([]string, 0, len(entries))
	for path := range entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	hasher := sha256.New()
	for _, path := range paths {
		_, _ = io.WriteString(hasher, path)
		_, _ = io.WriteString(hasher, "\x00")
		_, _ = io.WriteString(hasher, entries[path])
		_, _ = io.WriteString(hasher, "\n")
	}
	contentPaths := make([]string, 0, len(contentEntries))
	for path := range contentEntries {
		contentPaths = append(contentPaths, path)
	}
	sort.Strings(contentPaths)
	contentHasher := sha256.New()
	for _, path := range contentPaths {
		_, _ = io.WriteString(contentHasher, path)
		_, _ = io.WriteString(contentHasher, "\x00")
		_, _ = io.WriteString(contentHasher, contentEntries[path])
		_, _ = io.WriteString(contentHasher, "\n")
	}
	return localAIRealCacheTree{
		IdentitySHA256: hex.EncodeToString(hasher.Sum(nil)), ContentIdentitySHA256: hex.EncodeToString(contentHasher.Sum(nil)),
		Entries: len(entries), Files: files, Bytes: totalBytes, PartialArtifacts: partialArtifacts,
	}, nil
}

func localAIPartialArtifactCount(roots localAIRealRoots) (int, error) {
	count := 0
	for _, root := range []string{roots.Cache, roots.HFCache, roots.Temp, roots.Output, roots.Streams} {
		current, err := localAIPartialArtifactCountIn(root)
		if err != nil {
			return 0, err
		}
		count += current
	}
	return count, nil
}

func localAIPartialArtifactCountIn(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return fs.SkipDir
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		lower := strings.ToLower(filepath.ToSlash(relative))
		if strings.HasSuffix(lower, ".partial") || strings.Contains(lower, "/.partial/") {
			count++
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, fs.SkipDir) {
		return 0, err
	}
	return count, nil
}

func validateLocalAIRequest(request localAIRealRunRequest) error {
	if strings.TrimSpace(request.Selector) == "" || strings.ContainsAny(request.Selector, "\\/\r\n") {
		return errors.New("selector is not bounded")
	}
	if request.Kind != localAIJourneyTTS && request.Kind != localAIJourneyASR && request.Kind != localAIJourneyTTSASR {
		return errors.New("journey kind is not bounded")
	}
	if strings.TrimSpace(request.RunID) == "" || len(request.RunID) > 128 {
		return errors.New("run identity is not bounded")
	}
	for _, path := range []string{request.Root, request.ReportPath, request.LedgerPath, request.Command.BinaryPath} {
		if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
			return errors.New("isolated path is not absolute")
		}
	}
	if request.CacheRoot != "" && !filepath.IsAbs(request.CacheRoot) {
		return errors.New("cache root is not absolute")
	}
	if request.ReservationKind != "modelCall" && request.ReservationKind != "downloadBytes" {
		return errors.New("reservation kind is not bounded")
	}
	if request.ReservationAmount <= 0 || request.Limits.ModelCalls < 0 || request.Limits.DownloadBytes < 0 {
		return errors.New("reservation or limits are invalid")
	}
	if request.RequireFreshCache && request.RequireCacheReuse {
		return errors.New("cache admission requirements conflict")
	}
	if request.ChildProcessLimit < 0 || request.SemanticRetries < 0 || len(request.NetworkPolicy) > localAIRealMaxFailureBytes {
		return errors.New("run policy is not bounded")
	}
	if request.Command.OutputName == "" || filepath.Base(request.Command.OutputName) != request.Command.OutputName {
		return errors.New("output name is not a file identity")
	}
	if request.Kind == localAIJourneyTTSASR && request.FollowUp == nil {
		return errors.New("TTS-to-ASR journey is missing its ASR command")
	}
	if request.FollowUp != nil && (request.FollowUp.BinaryPath == "" || request.FollowUp.OutputName == "" || filepath.Base(request.FollowUp.OutputName) != request.FollowUp.OutputName) {
		return errors.New("follow-up command is not bounded")
	}
	if request.Kind == localAIJourneyASR && (strings.TrimSpace(request.InputPath) == "" || !filepath.IsAbs(request.InputPath)) {
		return errors.New("ASR input path is not absolute")
	}
	if request.ExpectedInputSHA256 != "" && !isLocalAISHA256(request.ExpectedInputSHA256) {
		return errors.New("expected input identity is not a SHA-256")
	}
	if len(request.ExpectedTranscript) > localAIRealMaxFailureBytes {
		return errors.New("expected transcript is not bounded")
	}
	if request.CacheIdentitySHA256 != "" && !isLocalAISHA256(request.CacheIdentitySHA256) {
		return errors.New("cache identity is not a SHA-256")
	}
	if request.ExpectedCacheContentSHA256 != "" && !isLocalAISHA256(request.ExpectedCacheContentSHA256) {
		return errors.New("expected cache content identity is not a SHA-256")
	}
	return nil
}

func validateLocalAIRealReport(report localAIRealReport) error {
	if report.Schema != localAIRealEvidenceSchema || !localAIStatus(report.Status) || report.Platform == "" || report.Architecture == "" || !report.Redacted {
		return errors.New("report identity or status is invalid")
	}
	if report.RunID == "" || len(report.RunID) > 128 || len(report.Journeys) != 1 {
		return errors.New("report cardinality is invalid")
	}
	if err := validateLocalAIBuild(report.Build); err != nil {
		return err
	}
	if err := validateLocalAIPolicy(report.Policy); err != nil {
		return err
	}
	journey := report.Journeys[0]
	if journey.Selector == "" || journey.Status != report.Status || !localAIStatus(journey.Status) {
		return errors.New("journey identity or status is invalid")
	}
	if journey.CacheIdentitySHA256 != "" && !isLocalAISHA256(journey.CacheIdentitySHA256) {
		return errors.New("journey cache identity is invalid")
	}
	if err := validateLocalAICache(journey.Cache); err != nil {
		return err
	}
	if journey.Execution == nil {
		return errors.New("execution evidence is missing")
	}
	if err := validateLocalAIExecution(*journey.Execution); err != nil {
		return err
	}
	if report.Status == "PASS" {
		if len(journey.Artifacts) == 0 || journey.Failure != nil || !journey.Semantic.Passed || !journey.Release.Checked || !journey.Release.ProcessTreeClosed || journey.Release.OwnedProcesses != 0 || journey.Release.OwnedListeners != 0 || journey.Release.OwnedLeases != 0 || journey.Release.PartialArtifacts != 0 {
			return errors.New("pass report omitted semantic or release proof")
		}
		for _, artifact := range journey.Artifacts {
			if err := validateLocalAIArtifact(artifact); err != nil {
				return err
			}
		}
	} else if journey.Failure == nil || !localAIFailureOwner(journey.Failure.Owner) {
		return errors.New("failure report omitted bounded ownership")
	} else if err := validateLocalAIFailure(*journey.Failure); err != nil {
		return err
	}
	if journey.Release.InspectionFailed && journey.Release.Checked {
		return errors.New("release inspection failure was inconsistent")
	}
	inspectionFailure := report.Status == "INCONCLUSIVE" && journey.Failure != nil && journey.Failure.Owner == "harness" && journey.Failure.Assertion == "runner release and cache inspection"
	if !journey.Release.Checked && !inspectionFailure {
		return errors.New("release evidence was not checked")
	}
	if report.BudgetLedger.PathIdentity == "" {
		return errors.New("budget ledger identity is missing")
	}
	if report.BudgetLedger.SHA256 != "" && !isLocalAISHA256(report.BudgetLedger.SHA256) {
		return errors.New("budget ledger identity is invalid")
	}
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	if localAIStreamViolation(body, nil) != "" {
		return errors.New("report contained unredacted evidence")
	}
	return nil
}

func validateLocalAIBuild(build localAIRealBuildIdentity) error {
	if !localAIPathIdentity(build.PathIdentity) || build.Bytes <= 0 || !isLocalAISHA256(build.SHA256) || build.Commit == "" || build.Tree == "" {
		return errors.New("build identity is invalid")
	}
	return nil
}

func validateLocalAIPolicy(policy localAIRealPolicy) error {
	for _, identity := range []string{policy.WorkRoot, policy.StateRoot, policy.CacheRoot, policy.TempRoot, policy.OutputRoot, policy.StreamsRoot, policy.PortState, policy.NetworkPolicy, policy.Timeout} {
		if strings.TrimSpace(identity) == "" || len(identity) > localAIRealMaxFailureBytes {
			return errors.New("run policy identity is invalid")
		}
	}
	if policy.ModelCallLimit < 0 || policy.DownloadLimit < 0 || policy.ChildProcessLimit <= 0 || policy.SemanticRetries < 0 {
		return errors.New("run policy limits are invalid")
	}
	return nil
}

func validateLocalAICache(cache localAIRealCache) error {
	for _, identity := range []string{cache.BeforeIdentitySHA256, cache.BeforeContentIdentitySHA256} {
		if !isLocalAISHA256(identity) {
			return errors.New("cache snapshot identity is invalid")
		}
	}
	if !cache.AfterInspectionFailed && (!isLocalAISHA256(cache.AfterIdentitySHA256) || !isLocalAISHA256(cache.AfterContentIdentitySHA256)) {
		return errors.New("cache after identity is invalid")
	}
	if cache.AfterInspectionFailed && (cache.AfterIdentitySHA256 != "" || cache.AfterContentIdentitySHA256 != "") {
		return errors.New("failed cache inspection reported an after identity")
	}
	if cache.BeforeEntries < 0 || cache.AfterEntries < 0 || cache.BeforeFiles < 0 || cache.AfterFiles < 0 || cache.BeforeBytes < 0 || cache.AfterBytes < 0 || cache.PartialArtifacts < 0 {
		return errors.New("cache snapshot counters are invalid")
	}
	if cache.FreshAtStart && (cache.BeforeEntries != 0 || cache.PartialArtifacts != 0) {
		return errors.New("fresh cache evidence is inconsistent")
	}
	if cache.ContentBacked && (cache.BeforeFiles == 0 || cache.BeforeBytes == 0) {
		return errors.New("content-backed cache evidence is inconsistent")
	}
	if cache.ContentConsumed && (!cache.ContentBacked || cache.AfterFiles == 0 || cache.AfterBytes == 0 || !isLocalAISHA256(cache.ConsumedContentIdentitySHA256)) {
		return errors.New("cache consumption evidence is inconsistent")
	}
	if cache.Reused && !cache.ContentConsumed {
		return errors.New("cache reuse evidence is inconsistent")
	}
	return nil
}

func validateLocalAIExecution(execution localAIRealExecution) error {
	if len(execution.Commands) > 2 {
		return errors.New("execution command count exceeds journey bound")
	}
	for _, command := range execution.Commands {
		if command.ExitCode < -1 || command.OwnedProcesses < 0 || command.OwnedListeners < 0 || command.OwnedLeases < 0 || command.StdoutBytes < 0 || command.StdoutBytes > localAIRealMaxStreamBytes || command.StderrBytes < 0 || command.StderrBytes > localAIRealMaxStreamBytes || !isLocalAISHA256(command.StdoutSHA256) || !isLocalAISHA256(command.StderrSHA256) {
			return errors.New("execution evidence is invalid")
		}
	}
	return nil
}

func validateLocalAIArtifact(artifact localAIRealArtifact) error {
	if artifact.Kind == "" || !localAIPathIdentity(artifact.Path) || artifact.MediaType == "" || artifact.Bytes <= 0 || !isLocalAISHA256(artifact.SHA256) {
		return errors.New("artifact identity is invalid")
	}
	if artifact.Kind == "output-audio" && artifact.Bytes > localAIRealMaxAudioBytes {
		return errors.New("audio artifact exceeds bounded output size")
	}
	return nil
}

func validateLocalAIFailure(failure localAIRealFailure) error {
	if failure.Assertion == "" || failure.Expected == "" || failure.Observed == "" || len(failure.Observed) > localAIRealMaxFailureBytes {
		return errors.New("failure observation is not bounded")
	}
	if localAIStreamViolation([]byte(failure.Observed), nil) != "" {
		return errors.New("failure observation was not redacted")
	}
	return nil
}

func localAIStatus(value string) bool {
	return value == "PASS" || value == "FAIL" || value == "INCONCLUSIVE"
}

func localAIFailureOwner(value string) bool {
	switch value {
	case "product", "harness", "environment", "unresolved":
		return true
	default:
		return false
	}
}

func localAIPathIdentity(value string) bool {
	return value != "" && filepath.Base(value) == value && !strings.ContainsAny(value, ":\r\n")
}

func boundedLocalAIValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= localAIRealMaxFailureBytes {
		return value
	}
	return "sha256=" + sha256Hex([]byte(value))
}

func isLocalAISHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
