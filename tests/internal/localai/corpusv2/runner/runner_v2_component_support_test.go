package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

func portableCorpusV2Fixture(t testing.TB) (CorpusV2Authority, CorpusV2Manifest) {
	t.Helper()
	root := portableCorpusV2Root()
	indexTemplate, err := os.ReadFile(filepath.Join(root, "video-output-index.md"))
	if err != nil {
		t.Fatalf("read repository-owned portable corpus index: %v", err)
	}
	indexData := []byte(strings.ReplaceAll(string(indexTemplate), "{{ROOT}}", corpusV2Slash(root)))
	authority := CorpusV2Authority{
		RepositoryRoot:  root,
		Commit:          strings.Repeat("a", 40),
		IndexPath:       "video-output-index.md",
		IndexSHA256:     hashBytes(indexData),
		Mode:            "repository-owned-synthetic",
		PairCount:       9,
		SectionCounts:   map[string]int{"study-a": 3, "study-b": 3, "study-c": 3},
		RequiredStudies: []string{"study-a", "study-b", "study-c"},
	}
	index, err := ParseCorpusV2Index(indexData, root)
	if err != nil {
		t.Fatalf("parse repository-owned portable corpus index: %v", err)
	}
	if len(index.Pairs) != authority.PairCount || len(index.Sections) != len(authority.SectionCounts) {
		t.Fatalf("portable index counts = %d pairs across %d sections, want 9 across 3", len(index.Pairs), len(index.Sections))
	}
	pairs := append([]CorpusV2Pair(nil), index.Pairs...)
	for index := range pairs {
		pair := &pairs[index]
		clip, clipBytes, err := corpusV2ReadIdentity(root, pair.Clip.Path)
		if err != nil {
			t.Fatalf("read synthetic clip %s/%s: %v", pair.Study, pair.Attempt, err)
		}
		prompt, _, err := corpusV2ReadIdentity(root, pair.Prompt.Path)
		if err != nil {
			t.Fatalf("read synthetic prompt %s/%s: %v", pair.Study, pair.Attempt, err)
		}
		stream, err := parseCorpusV2MP4Metadata(clipBytes)
		if err != nil {
			t.Fatalf("parse synthetic clip metadata %s/%s: %v", pair.Study, pair.Attempt, err)
		}
		stream.Identity = corpusV2IdentityForStream(stream)
		pair.Clip = clip
		pair.Prompt = prompt
		pair.Stream = stream
	}
	manifest := CorpusV2Manifest{
		SchemaVersion: CorpusV2SchemaVersion,
		Repository:    root,
		Commit:        authority.Commit,
		IndexPath:     authority.IndexPath,
		IndexSHA256:   authority.IndexSHA256,
		Pairs:         pairs,
		UniqueClips:   len(pairs),
		UniquePrompts: len(pairs),
		ReadOnly:      true,
	}
	manifest.Samples, err = SelectCorpusV2Representatives(manifest, authority)
	if err != nil {
		t.Fatalf("select portable corpus representatives: %v", err)
	}
	authority.ExpectedSamples = make([]CorpusV2SampleExpectation, len(manifest.Samples))
	for index, sample := range manifest.Samples {
		authority.ExpectedSamples[index] = CorpusV2SampleExpectation{
			Study: sample.Study, Band: sample.Band, Attempt: sample.Attempt,
			ClipBytes: sample.Clip.Bytes, ClipSHA256: sample.Clip.SHA256,
			PromptBytes: sample.Prompt.Bytes, PromptSHA256: sample.Prompt.SHA256,
			Codec: sample.Stream.Codec, Width: sample.Stream.Width, Height: sample.Stream.Height,
			FrameRate: sample.Stream.FrameRate, DurationMillis: sample.Stream.DurationMillis, Frames: sample.Stream.Frames,
		}
	}
	if err := ValidateCorpusV2Manifest(manifest, authority); err != nil {
		t.Fatalf("validate portable corpus fixture against its authority: %v", err)
	}
	return authority, manifest
}

func portableCorpusV2Root() string {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		panic("locate portable corpus fixture")
	}
	return filepath.Join(filepath.Dir(source), "testdata", "portable-corpus")
}

func newPortableRunnerV2(t testing.TB, executor Executor) RunnerV2 {
	t.Helper()
	authority, manifest := portableCorpusV2Fixture(t)
	runner := NewRunnerV2(executor)
	runner.authority = authority
	runner.corpusReader = cachedCorpusReaderV2(manifest)
	return runner
}

func assertPortableCorpusV2ReportSamples(t testing.TB, got []CorpusSampleReportV2, manifest CorpusV2Manifest) {
	t.Helper()
	if len(got) != len(manifest.Samples) {
		t.Fatalf("reported samples = %d, want %d", len(got), len(manifest.Samples))
	}
	for index, sample := range manifest.Samples {
		reported := got[index]
		wantStream := CorpusStreamReportV2{
			Codec: sample.Stream.Codec, Width: sample.Stream.Width, Height: sample.Stream.Height,
			FrameRate: sample.Stream.FrameRate, DurationMillis: sample.Stream.DurationMillis, Frames: sample.Stream.Frames,
		}
		if reported.Study != sample.Study || reported.Band != sample.Band || reported.Attempt != sample.Attempt ||
			reported.Clip != recordedCorpusV2File(sample.Clip) || reported.Prompt != recordedCorpusV2File(sample.Prompt) ||
			reported.Stream != wantStream || reported.SourceCommit != sample.SourceCommit {
			t.Errorf("reported sample[%d] = %#v, want portable identity for %s/%s/%s", index, reported, sample.Study, sample.Band, sample.Attempt)
		}
	}
}

const (
	ProbeNetworkPolicy = "declared-local-assets-and-loopback-only"
	ProbeForbiddenPort = 7437
	probeInputMaxBytes = 1 << 20
)

const (
	CodeProbeInvalidJSON      ValidationCode = "probe_invalid_json"
	CodeProbeInvalidInput     ValidationCode = "probe_invalid_input"
	CodeProbeInvalidIdentity  ValidationCode = "probe_invalid_identity"
	CodeProbeMissingIdentity  ValidationCode = "probe_missing_identity"
	CodeProbeIdentityMismatch ValidationCode = "probe_identity_mismatch"
	CodeProbeInvalidRoot      ValidationCode = "probe_invalid_root"
	CodeProbeRootNotFresh     ValidationCode = "probe_root_not_fresh"
	CodeProbeInvalidReport    ValidationCode = "probe_invalid_report_path"
	CodeProbeHeavyOwnerBusy   ValidationCode = "probe_heavy_owner_busy"
	CodeProbeCancelled        ValidationCode = "probe_cancelled"
	CodeProbeTimedOut         ValidationCode = "probe_timed_out"
	CodeProbeExecutionFailure ValidationCode = "probe_execution_failure"
	CodeProbeCleanupFailure   ValidationCode = "probe_cleanup_failure"
	CodeProbeOutputFailure    ValidationCode = "probe_output_failure"
	CodeUnknownField          ValidationCode = "unknown_field"
	CodeInvalidJSON           ValidationCode = "invalid_json"
	CodeDuplicateJSONKey      ValidationCode = "duplicate_json_key"
)

type ValidationCode string

type ValidationError struct {
	Code     ValidationCode
	Field    string
	Expected string
	Observed string
	Cause    error
}

func (err *ValidationError) Error() string {
	if err == nil {
		return ""
	}
	if err.Cause != nil {
		return fmt.Sprintf("%s at %s (expected %q, observed %q): %v", err.Code, err.Field, err.Expected, err.Observed, err.Cause)
	}
	return fmt.Sprintf("%s at %s (expected %q, observed %q)", err.Code, err.Field, err.Expected, err.Observed)
}

func (err *ValidationError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func validationError(code ValidationCode, field, expected, observed string, cause error) *ValidationError {
	return &ValidationError{Code: code, Field: field, Expected: expected, Observed: observed, Cause: cause}
}

type JourneyName string

type ProbeFileIdentity struct {
	Path     string `json:"path"`
	Identity string `json:"identity"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
}

type ProbeBuildIdentity struct {
	Path     string `json:"path"`
	Identity string `json:"identity"`
	SHA256   string `json:"sha256"`
}

type ProbeDependencies struct {
	Model     ProbeFileIdentity `json:"model"`
	Projector ProbeFileIdentity `json:"projector"`
	Backend   ProbeFileIdentity `json:"backend"`
}

type RecordedIdentity struct {
	Identity     string `json:"identity"`
	PathIdentity string `json:"pathIdentity"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
}

type ProbeReportDependencies struct {
	Model     RecordedIdentity `json:"model"`
	Projector RecordedIdentity `json:"projector"`
	Backend   RecordedIdentity `json:"backend"`
}

type RequestInput struct {
	Order     int    `json:"order"`
	Name      string `json:"name"`
	Modality  string `json:"modality"`
	MediaType string `json:"mediaType"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
}

type ProcessEvidence struct {
	Identity string `json:"identity"`
	Kind     string `json:"kind"`
	PID      int    `json:"pid"`
	Owner    string `json:"owner"`
	Started  bool   `json:"started"`
	Exited   bool   `json:"exited"`
	ExitCode int    `json:"exitCode"`
	TimedOut bool   `json:"timedOut"`
}

type CleanupEvidence struct {
	Checked                bool `json:"checked"`
	OwnedProcessSurvivors  int  `json:"ownedProcessSurvivors"`
	OwnedListenerSurvivors int  `json:"ownedListenerSurvivors"`
	PartialOutputs         int  `json:"partialOutputs"`
}

type ReportFailure struct {
	Owner      string `json:"owner"`
	Code       string `json:"code"`
	Expected   string `json:"expected"`
	Observed   string `json:"observed"`
	NextAction string `json:"nextAction"`
}

type RootPaths struct {
	Work      string
	Profile   string
	Cache     string
	Model     string
	Projector string
	Backend   string
	Output    string
	Streams   string
}

type probeRoots struct {
	Root  string
	Paths RootPaths
	Owned []string
	Port  int
}

type ExecutionRequest struct {
	Journey JourneyName
	Command []string
	Inputs  []RequestInput
	Roots   RootPaths
	Port    int
}

type ExecutionObservation struct {
	Process                ProcessEvidence
	Outputs                []RecordedIdentity
	Failure                *ReportFailure
	TimedOut               bool
	Cancelled              bool
	OwnedProcessSurvivors  int
	OwnedListenerSurvivors int
	PartialOutputs         int
}

type Executor interface {
	Execute(context.Context, ExecutionRequest) (ExecutionObservation, error)
}

type probeCallBudget struct {
	maxCalls int64
	calls    int64
}

var probeHeavyOwner = make(chan struct{}, 1)

func (budget *probeCallBudget) reserve() error {
	budget.calls++
	if budget.calls > budget.maxCalls {
		return validationError(CodeProbeExecutionFailure, "limits.maxCalls", fmt.Sprintf("at most %d executor calls", budget.maxCalls), fmt.Sprintf("executor call %d", budget.calls), nil)
	}
	return nil
}

func validateBuildShape(build ProbeBuildIdentity) error {
	if err := validateAbsolutePathShape(build.Path, "build.path"); err != nil {
		return err
	}
	if err := validateProbeLabel(build.Identity, "build.identity"); err != nil {
		return err
	}
	return validateSHA256(build.SHA256, "build.sha256")
}

func validateFileIdentityShape(identity ProbeFileIdentity, field string) error {
	if err := validateAbsolutePathShape(identity.Path, field+".path"); err != nil {
		return err
	}
	if err := validateProbeLabel(identity.Identity, field+".identity"); err != nil {
		return err
	}
	if identity.Bytes <= 0 {
		return validationError(CodeProbeInvalidIdentity, field+".bytes", "positive byte count", fmt.Sprint(identity.Bytes), nil)
	}
	return validateSHA256(identity.SHA256, field+".sha256")
}

func validateAbsolutePathShape(path, field string) error {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return validationError(CodeProbeInvalidIdentity, field, "absolute clean path", path, nil)
	}
	return nil
}

func validateProbeLabel(value, field string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsRune(value, 0) || len(value) > 256 {
		return validationError(CodeProbeInvalidInput, field, "non-empty bounded label", value, nil)
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return validationError(CodeProbeInvalidInput, field, "printable label", "control character", nil)
		}
	}
	return nil
}

func validateSHA256(value, field string) error {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return validationError(CodeProbeInvalidIdentity, field, "lowercase SHA-256", value, nil)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return validationError(CodeProbeInvalidIdentity, field, "lowercase SHA-256", value, err)
	}
	return nil
}

func validateProbeRoot(path string) error {
	if err := rejectProbeSymlinkComponents(path); err != nil {
		return validationError(CodeProbeInvalidRoot, "probeRoot", "fresh path without symlink components", path, err)
	}
	info, err := os.Lstat(path)
	if err == nil {
		return validationError(CodeProbeRootNotFresh, "probeRoot", "non-existent fresh directory", info.Mode().String(), nil)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return validationError(CodeProbeInvalidRoot, "probeRoot", "inspectable fresh path", path, err)
	}
	parentInfo, err := os.Stat(filepath.Dir(path))
	if err != nil || !parentInfo.IsDir() {
		return validationError(CodeProbeInvalidRoot, "probeRoot", "existing parent directory", filepath.Dir(path), err)
	}
	return nil
}

func normalizeProbePath(path, field string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) || !filepath.IsAbs(path) {
		return "", validationError(CodeProbeInvalidIdentity, field, "absolute path", path, nil)
	}
	clean := filepath.Clean(path)
	if clean != path {
		return "", validationError(CodeProbeInvalidIdentity, field, "clean absolute path", path, nil)
	}
	return clean, nil
}

func rejectProbeSymlinkComponents(path string) error {
	current := filepath.Clean(path)
	for {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component is a symlink (%s)", pathIdentity(current))
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func readProbeJSON(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("JSON exceeds %d bytes", maximum)
	}
	return data, nil
}

func writeProbeJSONAtomic(path string, body []byte) error {
	path, err := normalizeProbePath(path, "atomic JSON path")
	if err != nil {
		return err
	}
	if err := rejectProbeSymlinkComponents(path); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("atomic JSON destination is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".omni-media-probe-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := (filesystem.Local{AllowRenameReplacement: true}).RenameReplacing(temporaryPath, path); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

func decodeStrictJSON(data []byte, destination any) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := walkJSONValue(decoder, "$", 0); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return validationError(CodeInvalidJSON, "$", "one JSON value", "trailing value", nil)
		}
		return err
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder, field string, depth int) error {
	if depth > 64 {
		return validationError(CodeInvalidJSON, field, "JSON nesting no deeper than 64 levels", "too deep", nil)
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return validationError(CodeDuplicateJSONKey, field+"."+key, "unique JSON object key", "duplicate", nil)
			}
			seen[key] = struct{}{}
			if err := walkJSONValue(decoder, field+"."+key, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("object is not closed")
		}
	case '[':
		index := 0
		for decoder.More() {
			if err := walkJSONValue(decoder, fmt.Sprintf("%s[%d]", field, index), depth+1); err != nil {
				return err
			}
			index++
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("array is not closed")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

func validateReportDestination(path, probeRoot string) error {
	if err := rejectProbeSymlinkComponents(path); err != nil {
		return validationError(CodeProbeInvalidReport, "reportPath", "path without symlink components", path, err)
	}
	if filepath.Clean(path) == filepath.Clean(probeRoot) {
		return validationError(CodeProbeInvalidReport, "reportPath", "report file distinct from probe root", path, nil)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return validationError(CodeProbeInvalidReport, "reportPath", "absent or regular report file", info.Mode().String(), nil)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return validationError(CodeProbeInvalidReport, "reportPath", "inspectable report destination", path, err)
	}
	for _, name := range []string{"work", "profile", "cache", "model", "projector", "backend", "output", "streams"} {
		if pathWithin(filepath.Join(probeRoot, name), path) {
			return validationError(CodeProbeInvalidReport, "reportPath", "report outside owned resource roots", path, nil)
		}
	}
	return nil
}

func admitBuildIdentity(build ProbeBuildIdentity) (RecordedIdentity, error) {
	info, digest, err := inspectDeclaredFile(build.Path, "build")
	if err != nil {
		return RecordedIdentity{}, err
	}
	if err := validateExecutable(build.Path, info); err != nil {
		return RecordedIdentity{}, err
	}
	if digest != build.SHA256 {
		return RecordedIdentity{}, validationError(CodeProbeIdentityMismatch, "build.sha256", build.SHA256, digest, nil)
	}
	return RecordedIdentity{Identity: build.Identity, PathIdentity: pathIdentity(build.Path), Bytes: info.Size(), SHA256: digest}, nil
}

func admitFileIdentity(declared ProbeFileIdentity, field string) (RecordedIdentity, error) {
	info, digest, err := inspectDeclaredFile(declared.Path, field)
	if err != nil {
		return RecordedIdentity{}, err
	}
	if info.Size() != declared.Bytes {
		return RecordedIdentity{}, validationError(CodeProbeIdentityMismatch, field+".bytes", fmt.Sprint(declared.Bytes), fmt.Sprint(info.Size()), nil)
	}
	if digest != declared.SHA256 {
		return RecordedIdentity{}, validationError(CodeProbeIdentityMismatch, field+".sha256", declared.SHA256, digest, nil)
	}
	return RecordedIdentity{Identity: declared.Identity, PathIdentity: pathIdentity(declared.Path), Bytes: info.Size(), SHA256: digest}, nil
}

func inspectDeclaredFile(path, field string) (os.FileInfo, string, error) {
	if err := rejectProbeSymlinkComponents(path); err != nil {
		return nil, "", validationError(CodeProbeInvalidIdentity, field+".path", "non-aliased regular file", path, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		code := CodeProbeMissingIdentity
		if !errors.Is(err, os.ErrNotExist) {
			code = CodeProbeInvalidIdentity
		}
		return nil, "", validationError(code, field+".path", "existing regular file", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 {
		return nil, "", validationError(CodeProbeInvalidIdentity, field+".path", "nonempty regular file", info.Mode().String(), nil)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", validationError(CodeProbeInvalidIdentity, field+".path", "readable regular file", path, err)
	}
	digest := sha256.New()
	_, copyErr := io.Copy(digest, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return nil, "", validationError(CodeProbeInvalidIdentity, field+".path", "readable stable file", path, errors.Join(copyErr, closeErr))
	}
	finalInfo, err := os.Lstat(path)
	if err != nil || finalInfo.Size() != info.Size() {
		return nil, "", validationError(CodeProbeIdentityMismatch, field+".path", "stable file size", fmt.Sprint(info.Size()), err)
	}
	return info, hex.EncodeToString(digest.Sum(nil)), nil
}

func validateExecutable(path string, info os.FileInfo) error {
	if runtime.GOOS == "windows" {
		if strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil
		}
		return validationError(CodeProbeInvalidIdentity, "build.path", "Windows executable with .exe suffix", filepath.Ext(path), nil)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return validationError(CodeProbeInvalidIdentity, "build.path", "executable file mode", info.Mode().String(), nil)
	}
	return nil
}

func reserveProbeHeavyOwner(ctx context.Context) (func(), error) {
	select {
	case probeHeavyOwner <- struct{}{}:
		return func() { <-probeHeavyOwner }, nil
	case <-ctx.Done():
		return nil, probeContextError(ctx.Err())
	default:
		return nil, validationError(CodeProbeHeavyOwnerBusy, "limits.maxHeavyProcesses", "one available heavy owner", "another OMNI probe owns it", nil)
	}
}

func probeContextError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return validationError(CodeProbeTimedOut, "context", "live bounded context", "deadline exceeded", err)
	}
	return validationError(CodeProbeCancelled, "context", "live bounded context", "cancelled", err)
}

func hasValidationCode(err error, want ValidationCode) bool {
	var validation *ValidationError
	return errors.As(err, &validation) && validation.Code == want
}

func pathIdentity(path string) string {
	digest := sha256.Sum256([]byte(filepath.Clean(path)))
	return "sha256=" + hex.EncodeToString(digest[:])
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func reportStringContainsAbsolutePath(value string) bool {
	if filepath.IsAbs(value) {
		return true
	}
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/')
}

func probeFailure(owner, code, expected, observed, nextAction string) *ReportFailure {
	return &ReportFailure{Owner: owner, Code: code, Expected: expected, Observed: observed, NextAction: nextAction}
}

func wrapProbeCleanupError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, validationError(CodeProbeCleanupFailure, "cleanup", "zero owned survivors", "cleanup failed", err))
}

func createProbeRoots(root, reportPath string) (probeRoots, error) {
	paths := RootPaths{
		Work: filepath.Join(root, "work"), Profile: filepath.Join(root, "profile"),
		Cache: filepath.Join(root, "cache"), Model: filepath.Join(root, "model"),
		Projector: filepath.Join(root, "projector"), Backend: filepath.Join(root, "backend"),
		Output: filepath.Join(root, "output"), Streams: filepath.Join(root, "streams"),
	}
	owned := []string{paths.Work, paths.Profile, paths.Cache, paths.Model, paths.Projector, paths.Backend, paths.Output, paths.Streams}
	if err := os.Mkdir(root, 0o700); err != nil {
		return probeRoots{}, fmt.Errorf("create fresh OMNI probe root: %w", err)
	}
	created := probeRoots{Root: root, Paths: paths, Owned: owned}
	for _, path := range owned {
		if err := os.Mkdir(path, 0o700); err != nil {
			_ = cleanupProbeRoots(created, reportPath)
			return probeRoots{}, fmt.Errorf("create isolated OMNI probe root %q: %w", filepath.Base(path), err)
		}
	}
	return created, nil
}

func cleanupProbeRoots(roots probeRoots, reportPath string) error {
	var cleanupErr error
	for _, path := range roots.Owned {
		cleanupErr = errors.Join(cleanupErr, os.RemoveAll(path))
	}
	if !pathWithin(roots.Root, reportPath) {
		cleanupErr = errors.Join(cleanupErr, os.RemoveAll(roots.Root))
	}
	return cleanupErr
}

func reserveProbePort(forbidden int) (net.Listener, int, error) {
	for attempt := 0; attempt < 8; attempt++ {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, 0, err
		}
		address, ok := listener.Addr().(*net.TCPAddr)
		if !ok || address.Port <= 0 {
			_ = listener.Close()
			return nil, 0, errors.New("loopback listener did not expose a port")
		}
		if address.Port != forbidden {
			return listener, address.Port, nil
		}
		_ = listener.Close()
	}
	return nil, 0, fmt.Errorf("ephemeral port repeatedly selected forbidden port %d", forbidden)
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func rootIdentities(roots probeRoots) []string {
	paths := []string{roots.Root, roots.Paths.Work, roots.Paths.Profile, roots.Paths.Cache, roots.Paths.Model, roots.Paths.Projector, roots.Paths.Backend, roots.Paths.Output, roots.Paths.Streams}
	identities := make([]string, 0, len(paths))
	for _, path := range paths {
		identities = append(identities, pathIdentity(path))
	}
	return identities
}

func validateRecordedIdentity(identity RecordedIdentity, field string, requireBytes bool) error {
	if err := validateProbeLabel(identity.Identity, field+".identity"); err != nil {
		return err
	}
	if !strings.HasPrefix(identity.PathIdentity, "sha256=") || len(identity.PathIdentity) != len("sha256=")+sha256.Size*2 {
		return validationError(CodeProbeInvalidReport, field+".pathIdentity", "redacted SHA-256 path identity", identity.PathIdentity, nil)
	}
	if err := validateSHA256(strings.TrimPrefix(identity.PathIdentity, "sha256="), field+".pathIdentity"); err != nil {
		return err
	}
	if identity.Bytes < 0 || (requireBytes && identity.Bytes == 0) {
		return validationError(CodeProbeInvalidReport, field+".bytes", "non-negative observed bytes", fmt.Sprint(identity.Bytes), nil)
	}
	return validateSHA256(identity.SHA256, field+".sha256")
}

func validateProcessEvidence(process ProcessEvidence, index int) error {
	if err := validateProbeLabel(process.Identity, fmt.Sprintf("processes[%d].identity", index)); err != nil {
		return err
	}
	if err := validateProbeLabel(process.Kind, fmt.Sprintf("processes[%d].kind", index)); err != nil {
		return err
	}
	if err := validateProbeLabel(process.Owner, fmt.Sprintf("processes[%d].owner", index)); err != nil {
		return err
	}
	if process.PID < 0 {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("processes[%d].pid", index), "non-negative process id", fmt.Sprint(process.PID), nil)
	}
	return nil
}

func validateReportFailure(failure ReportFailure, field string) error {
	for name, value := range map[string]string{"owner": failure.Owner, "code": failure.Code, "expected": failure.Expected, "observed": failure.Observed, "nextAction": failure.NextAction} {
		if value == "" || reportStringContainsAbsolutePath(value) {
			return validationError(CodeProbeInvalidReport, field+"."+name, "non-empty redacted failure text", value, nil)
		}
	}
	return nil
}

func uniqueStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}
