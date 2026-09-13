package tts_clean_install

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"
)

const (
	ManifestSchemaVersion = 1
	ReportSchemaVersion   = 1
	ExitSuccess           = 0
	ExitInputFailure      = 2
	ExitJourneyFailure    = 3
	maxManifestBytes      = 1 << 20
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Invocation struct {
	ArtifactPath     string
	ArtifactIdentity string
	ArtifactSHA256   string
	ManifestPath     string
	ReportPath       string
}

type Manifest struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Build         Build                `json:"build"`
	Distribution  Distribution         `json:"distribution"`
	Acceptance    ImmutableFile        `json:"acceptance"`
	PublicDocs    []NamedImmutableFile `json:"publicDocs"`
	Fixtures      Fixtures             `json:"fixtures"`
	Limits        Limits               `json:"limits"`
}

type Build struct {
	Identity     string `json:"identity"`
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
	ArtifactPath string `json:"artifactPath"`
	ArtifactKind string `json:"artifactKind"`
	SHA256       string `json:"sha256"`
}

type Distribution struct {
	Installer ImmutableFile `json:"installer"`
	Archive   ImmutableFile `json:"archive"`
	Checksums ImmutableFile `json:"checksums"`
}

type ImmutableFile struct {
	Path     string `json:"path"`
	Identity string `json:"identity"`
	SHA256   string `json:"sha256"`
}

type NamedImmutableFile struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Identity string `json:"identity"`
	SHA256   string `json:"sha256"`
}

type Fixtures struct {
	Text  NamedImmutableFile  `json:"text"`
	Voice *NamedImmutableFile `json:"voice"`
}

type Limits struct {
	TimeoutSeconds    int     `json:"timeoutSeconds"`
	DiskBytes         int64   `json:"diskBytes"`
	DownloadBytes     int64   `json:"downloadBytes"`
	PaidUSD           float64 `json:"paidUsd"`
	MaxOwnedProcesses int     `json:"maxOwnedProcesses"`
	NetworkPolicy     string  `json:"networkPolicy"`
}

type Identity struct {
	Name     string `json:"name"`
	Identity string `json:"identity"`
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
}

type IsolationPlan struct {
	OutputRoot  string
	WorkRoot    string
	ProfileRoot string
	StateRoot   string
	CacheRoot   string
	TempRoot    string
	StreamsRoot string
	RuntimeRoot string
	Environment map[string]string
}

type SealedPlan struct {
	Invocation   Invocation
	Manifest     Manifest
	ManifestHash string
	Identities   []Identity
	Isolation    IsolationPlan
}

type PreflightCheck struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Expected string `json:"expected"`
	Observed string `json:"observed"`
}

type PreflightEvidence struct {
	CompletedBeforeEffects bool             `json:"completedBeforeEffects"`
	ChildStarts            int              `json:"childStarts"`
	ListenerOpens          int              `json:"listenerOpens"`
	NetworkAttempts        int              `json:"networkAttempts"`
	Checks                 []PreflightCheck `json:"checks"`
}

type Environment struct {
	Platform     string           `json:"platform"`
	Architecture string           `json:"architecture"`
	Roots        EnvironmentRoots `json:"roots"`
	Limits       Limits           `json:"limits"`
}

type EnvironmentRoots struct {
	Output  string `json:"output"`
	Work    string `json:"work"`
	Profile string `json:"profile"`
	State   string `json:"state"`
	Cache   string `json:"cache"`
	Temp    string `json:"temp"`
	Streams string `json:"streams"`
	Runtime string `json:"runtime"`
}

type CommandEvidence struct {
	Phase        string   `json:"phase"`
	Argv         []string `json:"argv"`
	ExitCode     int      `json:"exitCode"`
	StartedAt    string   `json:"startedAt"`
	EndedAt      string   `json:"endedAt"`
	StdoutBytes  int64    `json:"stdoutBytes"`
	StderrBytes  int64    `json:"stderrBytes"`
	StdoutSHA256 string   `json:"stdoutSha256"`
	StderrSHA256 string   `json:"stderrSha256"`
	TimedOut     bool     `json:"timedOut"`
	Cancelled    bool     `json:"cancelled"`
}

type JourneyEvidence struct {
	Install     PhaseEvidence `json:"install"`
	Discovery   PhaseEvidence `json:"discovery"`
	Cold        PhaseEvidence `json:"cold"`
	WarmOffline PhaseEvidence `json:"warmOffline"`
	Removal     PhaseEvidence `json:"removal"`
	Steps       []JourneyStep `json:"steps"`
}

type PhaseEvidence struct {
	Status       string   `json:"status"`
	Evidence     []string `json:"evidence"`
	UnprovenEdge string   `json:"unprovenEdge"`
}

type AudioEvidence struct {
	Name           string `json:"name"`
	MediaType      string `json:"mediaType"`
	Bytes          int64  `json:"bytes"`
	SHA256         string `json:"sha256"`
	RIFFDecoded    bool   `json:"riffDecoded"`
	DurationMillis int64  `json:"durationMillis"`
	NonSilent      int64  `json:"nonSilentSamples"`
}

type ReadinessEvidence struct {
	Phase          string `json:"phase"`
	ReadinessState string `json:"readinessState"`
	LifecycleState string `json:"lifecycleState"`
	CacheBytes     int64  `json:"cacheBytes"`
	CacheReused    bool   `json:"cacheReused"`
}

type NetworkEvidence struct {
	Policy              string `json:"policy"`
	Attempts            int    `json:"attempts"`
	WarmOfflineAttempts int    `json:"warmOfflineAttempts"`
}

type CleanupEvidence struct {
	OwnedProcesses      int `json:"ownedProcesses"`
	OwnedListeners      int `json:"ownedListeners"`
	OwnedRoots          int `json:"ownedRoots"`
	SurvivingProcesses  int `json:"survivingProcesses"`
	SurvivingListeners  int `json:"survivingListeners"`
	RemovedRuntimeRoots int `json:"removedRuntimeRoots"`
}

type CriterionEvidence struct {
	ID           string   `json:"id"`
	Verdict      string   `json:"verdict"`
	Evidence     []string `json:"evidence"`
	UnprovenEdge *string  `json:"unprovenEdge"`
}

type Finding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Check    string `json:"check"`
	Expected string `json:"expected"`
	Observed string `json:"observed"`
}

type Report struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Verdict       string              `json:"verdict"`
	Environment   Environment         `json:"environment"`
	Identities    []Identity          `json:"identities"`
	Preflight     PreflightEvidence   `json:"preflight"`
	Commands      []CommandEvidence   `json:"commands"`
	Journeys      JourneyEvidence     `json:"journeys"`
	Audio         []AudioEvidence     `json:"audio"`
	Readiness     []ReadinessEvidence `json:"readiness"`
	Network       NetworkEvidence     `json:"network"`
	Cleanup       CleanupEvidence     `json:"cleanup"`
	Criteria      []CriterionEvidence `json:"criteria"`
	Findings      []Finding           `json:"findings"`
}

type PreflightResult struct {
	Plan   SealedPlan
	Report Report
}

type PreflightError struct {
	Check    string
	Expected string
	Observed string
}

func (e *PreflightError) Error() string {
	return fmt.Sprintf("preflight %s failed: expected %s; observed %s", e.Check, e.Expected, e.Observed)
}

func Preflight(in Invocation) (PreflightResult, error) {
	report := newReport()
	result := PreflightResult{Report: report}
	if err := validateInvocationShape(in); err != nil {
		return reject(&result, "invocation", "all required arguments are absolute and well-formed", err.Error())
	}
	reportPath, err := absoluteClean(in.ReportPath)
	if err != nil {
		return reject(&result, "report-path", "an absolute report path", err.Error())
	}
	outputRoot := filepath.Dir(reportPath)
	if err := validateEmptyOutputRoot(outputRoot, reportPath); err != nil {
		return reject(&result, "empty-output-root", "an existing empty task-owned output root", err.Error())
	}
	if err := validateNoPathAlias(in.ManifestPath, reportPath, "manifest", "report"); err != nil {
		return reject(&result, "path-alias", "distinct manifest and report paths", err.Error())
	}

	manifestPath, err := absoluteClean(in.ManifestPath)
	if err != nil {
		return reject(&result, "manifest-path", "an absolute manifest path", err.Error())
	}
	return continuePreflight(&result, in, manifestPath, reportPath, outputRoot)
}

func continuePreflight(result *PreflightResult, in Invocation, manifestPath, reportPath, outputRoot string) (PreflightResult, error) {
	manifest, manifestBody, err := readManifest(manifestPath)
	if err != nil {
		return reject(result, "manifest", "a strict schema-v1 UTF-8 manifest", err.Error())
	}
	manifestHash := sha256Hex(manifestBody)
	if err := validateManifest(manifest); err != nil {
		return reject(result, "manifest-schema", "a complete schema-v1 manifest", err.Error())
	}
	artifactPath, err := absoluteClean(in.ArtifactPath)
	if err != nil {
		return reject(result, "artifact-path", "an absolute artifact path", err.Error())
	}
	if !samePath(artifactPath, manifest.Build.ArtifactPath) {
		return reject(result, "artifact-path-match", "the invocation artifact path matches manifest.build.artifactPath", artifactPath)
	}
	if in.ArtifactIdentity != manifest.Build.Identity {
		return reject(result, "artifact-identity-match", "the invocation identity matches manifest.build.identity", in.ArtifactIdentity)
	}
	if in.ArtifactSHA256 != manifest.Build.SHA256 {
		return reject(result, "artifact-hash-match", "the invocation SHA-256 matches manifest.build.sha256", in.ArtifactSHA256)
	}

	identities, totalBytes, err := inspectManifestInputs(manifest, manifestPath, reportPath)
	if err != nil {
		var issue *PreflightError
		if errors.As(err, &issue) {
			return reject(result, issue.Check, issue.Expected, issue.Observed)
		}
		return reject(result, "input-identity", "all immutable inputs are readable and match their declarations", err.Error())
	}
	if totalBytes > manifest.Limits.DiskBytes {
		return reject(result, "disk-limit", "declared immutable inputs fit within limits.diskBytes", fmt.Sprintf("input bytes %d exceed limit %d", totalBytes, manifest.Limits.DiskBytes))
	}
	identities = append([]Identity{{
		Name: "manifest", Identity: "schema-v1", Path: pathIdentity(manifestPath), Bytes: int64(len(manifestBody)), SHA256: manifestHash,
	}}, identities...)

	isolation, err := sealIsolation(outputRoot)
	if err != nil {
		return reject(result, "isolation", "child-only roots remain beneath the empty output root", err.Error())
	}
	result.Plan = SealedPlan{
		Invocation: in, Manifest: manifest, ManifestHash: manifestHash,
		Identities: identities, Isolation: isolation,
	}
	result.Report = reportForPlan(result.Plan)
	return *result, nil
}

func reject(result *PreflightResult, check, expected, observed string) (PreflightResult, error) {
	observed = bounded(observed)
	result.Report.Preflight.Checks = append(result.Report.Preflight.Checks, PreflightCheck{
		ID: check, Status: "FAIL", Expected: expected, Observed: observed,
	})
	result.Report.Verdict = "FAIL"
	result.Report.Criteria = preflightCriteria("FAIL", check)
	result.Report.Findings = []Finding{{
		ID: "TTS-PROBE-01", Severity: "FAIL", Check: check,
		Expected: expected, Observed: observed,
	}}
	return *result, &PreflightError{Check: check, Expected: expected, Observed: observed}
}

func inspectManifestInputs(manifest Manifest, manifestPath, reportPath string) ([]Identity, int64, error) {
	refs := manifestReferences(manifest)
	seen := make(map[string]string, len(refs))
	identities := make([]Identity, 0, len(refs))
	var totalBytes int64
	for _, ref := range refs {
		path, err := absoluteClean(ref.Path)
		if err != nil {
			return nil, 0, &PreflightError{Check: "absolute-input-path", Expected: ref.Name + " is an absolute path", Observed: err.Error()}
		}
		key := pathKey(path)
		if previous, ok := seen[key]; ok {
			return nil, 0, &PreflightError{Check: "duplicate-input", Expected: "every immutable input path is unique", Observed: ref.Name + " duplicates " + previous}
		}
		seen[key] = ref.Name
		if samePath(path, manifestPath) || samePath(path, reportPath) {
			return nil, 0, &PreflightError{Check: "input-path-alias", Expected: "immutable inputs do not alias manifest or report", Observed: ref.Name}
		}
		identity, bytesRead, err := inspectFile(ref.Name, path, ref.Identity, ref.SHA256, ref.Text)
		if err != nil {
			return nil, 0, &PreflightError{Check: "input-identity", Expected: ref.Name + " exists, is readable, and matches its identity", Observed: err.Error()}
		}
		totalBytes += bytesRead
		identities = append(identities, identity)
	}
	return identities, totalBytes, nil
}

func validateInvocationShape(in Invocation) error {
	if strings.TrimSpace(in.ArtifactPath) == "" || strings.TrimSpace(in.ManifestPath) == "" || strings.TrimSpace(in.ReportPath) == "" {
		return errors.New("artifact, manifest, and report paths are required")
	}
	if strings.TrimSpace(in.ArtifactIdentity) == "" {
		return errors.New("artifact identity is required")
	}
	if !sha256Pattern.MatchString(in.ArtifactSHA256) {
		return errors.New("artifact SHA-256 must be 64 lowercase hexadecimal characters")
	}
	return nil
}

func absoluteClean(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is empty")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("path is relative")
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", errors.New("cleaned path is not absolute")
	}
	return clean, nil
}

func validateEmptyOutputRoot(root, reportPath string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("output root is not a directory")
	}
	if _, err := os.Lstat(reportPath); err == nil {
		return errors.New("report path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("output root contains %d existing entries", len(entries))
	}
	return nil
}

func validateNoPathAlias(first, second, firstName, secondName string) error {
	firstPath, err := absoluteClean(first)
	if err != nil {
		return err
	}
	secondPath, err := absoluteClean(second)
	if err != nil {
		return err
	}
	if samePath(firstPath, secondPath) {
		return fmt.Errorf("%s aliases %s", firstName, secondName)
	}
	return nil
}

func readManifest(path string) (Manifest, []byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Manifest{}, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Manifest{}, nil, errors.New("manifest is not a regular file")
	}
	if info.Size() <= 0 || info.Size() > maxManifestBytes {
		return Manifest{}, nil, errors.New("manifest size is outside the bounded range")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, nil, err
	}
	if !utf8.Valid(body) {
		return Manifest{}, nil, errors.New("manifest is not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Manifest{}, nil, errors.New("manifest contains trailing JSON")
	}
	return manifest, body, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("schemaVersion=%d, want %d", manifest.SchemaVersion, ManifestSchemaVersion)
	}
	if err := requireText("build.identity", manifest.Build.Identity); err != nil {
		return err
	}
	if manifest.Build.Platform != "windows" || manifest.Build.Architecture != "amd64" {
		return errors.New("build platform and architecture must be windows/amd64")
	}
	if err := validatePathField("build.artifactPath", manifest.Build.ArtifactPath); err != nil {
		return err
	}
	if manifest.Build.ArtifactKind != "installer" && manifest.Build.ArtifactKind != "archive" && manifest.Build.ArtifactKind != "executable" {
		return errors.New("build.artifactKind is unsupported")
	}
	if err := validateSHA("build.sha256", manifest.Build.SHA256); err != nil {
		return err
	}
	files := []struct {
		name string
		file ImmutableFile
	}{
		{name: "distribution.installer", file: manifest.Distribution.Installer},
		{name: "distribution.archive", file: manifest.Distribution.Archive},
		{name: "distribution.checksums", file: manifest.Distribution.Checksums},
		{name: "acceptance", file: manifest.Acceptance},
	}
	for _, item := range files {
		if err := validateImmutableFile(item.name, item.file); err != nil {
			return err
		}
	}
	if len(manifest.PublicDocs) == 0 {
		return errors.New("publicDocs must not be empty")
	}
	for index, file := range manifest.PublicDocs {
		if err := validateNamedFile(fmt.Sprintf("publicDocs[%d]", index), file); err != nil {
			return err
		}
	}
	if err := validateNamedFile("fixtures.text", manifest.Fixtures.Text); err != nil {
		return err
	}
	if manifest.Fixtures.Voice != nil {
		if err := validateNamedFile("fixtures.voice", *manifest.Fixtures.Voice); err != nil {
			return err
		}
	}
	if err := validateLimits(manifest.Limits); err != nil {
		return err
	}
	return nil
}

func validateLimits(limits Limits) error {
	if limits.TimeoutSeconds < 1 || limits.DiskBytes < 1 || limits.DownloadBytes < 0 || limits.PaidUSD != 0 || limits.MaxOwnedProcesses < 1 || limits.MaxOwnedProcesses > 4 {
		return errors.New("limits are outside the declared bounded range")
	}
	if limits.NetworkPolicy != "none" && limits.NetworkPolicy != "staged-loopback-and-declared-public-origins" {
		return errors.New("limits.networkPolicy is unsupported")
	}
	return nil
}

func validateImmutableFile(name string, file ImmutableFile) error {
	if err := validatePathField(name+".path", file.Path); err != nil {
		return err
	}
	if err := requireText(name+".identity", file.Identity); err != nil {
		return err
	}
	return validateSHA(name+".sha256", file.SHA256)
}

func validateNamedFile(name string, file NamedImmutableFile) error {
	if err := requireText(name+".name", file.Name); err != nil {
		return err
	}
	return validateImmutableFile(name, ImmutableFile{Path: file.Path, Identity: file.Identity, SHA256: file.SHA256})
}

func validatePathField(name, path string) error {
	if _, err := absoluteClean(path); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func requireText(name, value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is empty or contains forbidden control characters", name)
	}
	return nil
}

func validateSHA(name, value string) error {
	if !sha256Pattern.MatchString(value) {
		return fmt.Errorf("%s must be 64 lowercase hexadecimal characters", name)
	}
	return nil
}

type fileReference struct {
	Name     string
	Path     string
	Identity string
	SHA256   string
	Text     bool
}

func manifestReferences(manifest Manifest) []fileReference {
	refs := []fileReference{{
		Name: "artifact", Path: manifest.Build.ArtifactPath,
		Identity: manifest.Build.Identity, SHA256: manifest.Build.SHA256,
	}}
	refs = append(refs,
		fileReference{Name: "distribution.installer", Path: manifest.Distribution.Installer.Path, Identity: manifest.Distribution.Installer.Identity, SHA256: manifest.Distribution.Installer.SHA256},
		fileReference{Name: "distribution.archive", Path: manifest.Distribution.Archive.Path, Identity: manifest.Distribution.Archive.Identity, SHA256: manifest.Distribution.Archive.SHA256},
		fileReference{Name: "distribution.checksums", Path: manifest.Distribution.Checksums.Path, Identity: manifest.Distribution.Checksums.Identity, SHA256: manifest.Distribution.Checksums.SHA256, Text: true},
		fileReference{Name: "acceptance", Path: manifest.Acceptance.Path, Identity: manifest.Acceptance.Identity, SHA256: manifest.Acceptance.SHA256, Text: true},
	)
	for _, doc := range manifest.PublicDocs {
		refs = append(refs, fileReference{Name: "publicDocs." + doc.Name, Path: doc.Path, Identity: doc.Identity, SHA256: doc.SHA256, Text: true})
	}
	refs = append(refs, fileReference{Name: "fixtures.text", Path: manifest.Fixtures.Text.Path, Identity: manifest.Fixtures.Text.Identity, SHA256: manifest.Fixtures.Text.SHA256, Text: true})
	if manifest.Fixtures.Voice != nil {
		voice := manifest.Fixtures.Voice
		refs = append(refs, fileReference{Name: "fixtures.voice." + voice.Name, Path: voice.Path, Identity: voice.Identity, SHA256: voice.SHA256})
	}
	return refs
}

func inspectFile(name, path, expectedIdentity, expectedSHA string, textFile bool) (Identity, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Identity{}, 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Identity{}, 0, errors.New("input is not a regular non-reparse file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Identity{}, 0, err
	}
	defer file.Close()
	hasher := sha256.New()
	var body bytes.Buffer
	var destination io.Writer = hasher
	if textFile {
		destination = io.MultiWriter(hasher, &body)
	}
	bytesRead, err := io.Copy(destination, file)
	if err != nil {
		return Identity{}, 0, err
	}
	if bytesRead != info.Size() {
		return Identity{}, 0, errors.New("input changed while it was being hashed")
	}
	if textFile && !utf8.Valid(body.Bytes()) {
		return Identity{}, 0, errors.New("text input is not valid UTF-8")
	}
	digest := hex.EncodeToString(hasher.Sum(nil))
	if digest != expectedSHA {
		return Identity{}, 0, fmt.Errorf("sha256=%s, want %s", digest, expectedSHA)
	}
	return Identity{Name: name, Identity: expectedIdentity, Path: pathIdentity(path), Bytes: bytesRead, SHA256: digest}, bytesRead, nil
}

func sealIsolation(outputRoot string) (IsolationPlan, error) {
	root, err := absoluteClean(outputRoot)
	if err != nil {
		return IsolationPlan{}, err
	}
	paths := map[string]string{
		"work": filepath.Join(root, "work"), "profile": filepath.Join(root, "profile"),
		"state": filepath.Join(root, "state"), "cache": filepath.Join(root, "cache"),
		"temp": filepath.Join(root, "temp"), "streams": filepath.Join(root, "streams"),
		"runtime": filepath.Join(root, "runtime"),
	}
	for name, path := range paths {
		if !pathWithin(root, path) {
			return IsolationPlan{}, fmt.Errorf("%s root escapes output root", name)
		}
	}
	profile := paths["profile"]
	volume := filepath.VolumeName(profile)
	homePath := strings.TrimPrefix(profile, volume)
	environment := map[string]string{
		"APPDATA":      paths["state"],
		"HF_HOME":      paths["cache"],
		"HF_HUB_CACHE": filepath.Join(paths["cache"], "hub"),
		"HOME":         profile,
		"LOCALAPPDATA": paths["state"],
		"TEMP":         paths["temp"],
		"TMP":          paths["temp"],
		"USERPROFILE":  profile,
		"HOMEDRIVE":    volume,
		"HOMEPATH":     homePath,
	}
	return IsolationPlan{
		OutputRoot: root, WorkRoot: paths["work"], ProfileRoot: profile,
		StateRoot: paths["state"], CacheRoot: paths["cache"], TempRoot: paths["temp"],
		StreamsRoot: paths["streams"], RuntimeRoot: paths["runtime"], Environment: environment,
	}, nil
}

func reportForPlan(plan SealedPlan) Report {
	report := newReport()
	report.Verdict = "INCONCLUSIVE"
	report.Environment = Environment{
		Platform: runtime.GOOS, Architecture: runtime.GOARCH,
		Roots: EnvironmentRoots{
			Output: pathIdentity(plan.Isolation.OutputRoot), Work: pathIdentity(plan.Isolation.WorkRoot),
			Profile: pathIdentity(plan.Isolation.ProfileRoot), State: pathIdentity(plan.Isolation.StateRoot),
			Cache: pathIdentity(plan.Isolation.CacheRoot), Temp: pathIdentity(plan.Isolation.TempRoot),
			Streams: pathIdentity(plan.Isolation.StreamsRoot), Runtime: pathIdentity(plan.Isolation.RuntimeRoot),
		},
		Limits: plan.Manifest.Limits,
	}
	report.Identities = append([]Identity(nil), plan.Identities...)
	report.Criteria = preflightCriteria("PASS", "preflight")
	report.Findings = []Finding{}
	return report
}

func newReport() Report {
	emptySHA := sha256Hex(nil)
	return Report{
		SchemaVersion: ReportSchemaVersion, Verdict: "INCONCLUSIVE",
		Environment: Environment{
			Platform: runtime.GOOS, Architecture: runtime.GOARCH,
			Roots: EnvironmentRoots{
				Output: "not-established", Work: "not-established", Profile: "not-established",
				State: "not-established", Cache: "not-established", Temp: "not-established",
				Streams: "not-established", Runtime: "not-established",
			},
			Limits: Limits{TimeoutSeconds: 1, DiskBytes: 1, MaxOwnedProcesses: 1, NetworkPolicy: "none"},
		},
		Identities: []Identity{{Name: "preflight", Identity: "not-established", Path: "not-established", SHA256: emptySHA}},
		Preflight:  PreflightEvidence{CompletedBeforeEffects: true, Checks: []PreflightCheck{}},
		Commands:   []CommandEvidence{},
		Journeys: JourneyEvidence{
			Install: phaseNotRun(), Discovery: phaseNotRun(), Cold: phaseNotRun(),
			WarmOffline: phaseNotRun(), Removal: phaseNotRun(), Steps: []JourneyStep{},
		},
		Audio: []AudioEvidence{
			{Name: "cold-tts.wav", MediaType: "audio/wav", SHA256: emptySHA},
			{Name: "warm-offline-tts.wav", MediaType: "audio/wav", SHA256: emptySHA},
		},
		Readiness: []ReadinessEvidence{},
		Network:   NetworkEvidence{Policy: "none"},
		Criteria:  preflightCriteria("INCONCLUSIVE", "preflight"),
		Findings:  []Finding{},
	}
}

func phaseNotRun() PhaseEvidence {
	return PhaseEvidence{Status: "NOT_RUN", Evidence: []string{}, UnprovenEdge: "later TTS journey story"}
}

func preflightCriteria(verdict, check string) []CriterionEvidence {
	edge := "later TTS journey and OS execution stories"
	if verdict == "PASS" {
		none := ""
		return []CriterionEvidence{
			{ID: "TTS-PROBE-01", Verdict: "PASS", Evidence: []string{"sealed immutable inputs and empty output root"}, UnprovenEdge: &none},
			{ID: "TTS-PROBE-02", Verdict: "INCONCLUSIVE", Evidence: []string{}, UnprovenEdge: &edge},
			{ID: "TTS-PROBE-03", Verdict: "INCONCLUSIVE", Evidence: []string{}, UnprovenEdge: &edge},
		}
	}
	return []CriterionEvidence{
		{ID: "TTS-PROBE-01", Verdict: verdict, Evidence: []string{"preflight rejected the input before effects"}, UnprovenEdge: nil},
		{ID: "TTS-PROBE-02", Verdict: "INCONCLUSIVE", Evidence: []string{}, UnprovenEdge: &edge},
		{ID: "TTS-PROBE-03", Verdict: "INCONCLUSIVE", Evidence: []string{}, UnprovenEdge: &edge},
	}
}

func ValidateReport(report Report) error {
	if report.SchemaVersion != ReportSchemaVersion {
		return fmt.Errorf("schemaVersion=%d, want %d", report.SchemaVersion, ReportSchemaVersion)
	}
	if report.Verdict != "PASS" && report.Verdict != "FAIL" && report.Verdict != "INCONCLUSIVE" {
		return errors.New("verdict is unsupported")
	}
	if report.Environment.Platform == "" || report.Environment.Architecture == "" {
		return errors.New("environment platform and architecture are required")
	}
	if report.Environment.Roots.Output == "" || report.Environment.Roots.Work == "" || report.Environment.Roots.Profile == "" || report.Environment.Roots.State == "" || report.Environment.Roots.Cache == "" || report.Environment.Roots.Temp == "" || report.Environment.Roots.Streams == "" || report.Environment.Roots.Runtime == "" {
		return errors.New("environment roots are incomplete")
	}
	if err := validateLimits(report.Environment.Limits); err != nil {
		return fmt.Errorf("environment limits: %w", err)
	}
	if len(report.Identities) == 0 {
		return errors.New("at least one identity is required")
	}
	for _, identity := range report.Identities {
		if identity.Name == "" || identity.Identity == "" || identity.Path == "" || identity.Bytes < 0 || !sha256Pattern.MatchString(identity.SHA256) {
			return errors.New("identity record is incomplete")
		}
	}
	if !report.Preflight.CompletedBeforeEffects || report.Preflight.ChildStarts < 0 || report.Preflight.ListenerOpens < 0 || report.Preflight.NetworkAttempts < 0 {
		return errors.New("preflight effect counters cannot be negative")
	}
	if len(report.Audio) < 2 {
		return errors.New("two audio records are required")
	}
	for _, audio := range report.Audio {
		if audio.Name == "" || audio.MediaType == "" || audio.Bytes < 0 || !sha256Pattern.MatchString(audio.SHA256) || audio.DurationMillis < 0 || audio.NonSilent < 0 {
			return errors.New("audio record is incomplete")
		}
	}
	phases := []PhaseEvidence{report.Journeys.Install, report.Journeys.Discovery, report.Journeys.Cold, report.Journeys.WarmOffline, report.Journeys.Removal}
	for _, phase := range phases {
		if phase.Status == "" || phase.Evidence == nil {
			return errors.New("journey phase evidence is incomplete")
		}
	}
	for _, readiness := range report.Readiness {
		if readiness.Phase == "" || readiness.ReadinessState == "" || readiness.LifecycleState == "" || readiness.CacheBytes < 0 {
			return errors.New("readiness evidence is incomplete")
		}
	}
	for _, command := range report.Commands {
		if command.Phase == "" || !sha256Pattern.MatchString(command.StdoutSHA256) || !sha256Pattern.MatchString(command.StderrSHA256) || command.StdoutBytes < 0 || command.StderrBytes < 0 {
			return errors.New("command evidence is incomplete")
		}
	}
	if report.Network.Policy != "none" && report.Network.Policy != "staged-loopback-and-declared-public-origins" {
		return errors.New("network policy is unsupported")
	}
	if report.Network.Attempts < 0 || report.Network.WarmOfflineAttempts < 0 {
		return errors.New("network evidence is incomplete")
	}
	if report.Cleanup.OwnedProcesses < 0 || report.Cleanup.OwnedListeners < 0 || report.Cleanup.OwnedRoots < 0 || report.Cleanup.SurvivingProcesses < 0 || report.Cleanup.SurvivingListeners < 0 || report.Cleanup.RemovedRuntimeRoots < 0 {
		return errors.New("cleanup evidence cannot be negative")
	}
	for _, criterion := range report.Criteria {
		if criterion.ID == "" || criterion.Verdict == "" || criterion.Evidence == nil {
			return errors.New("criterion evidence is incomplete")
		}
	}
	if report.Findings == nil || report.Preflight.Checks == nil || report.Commands == nil || report.Readiness == nil {
		return errors.New("report arrays must be explicit, complete arrays")
	}
	if report.Journeys.Steps == nil {
		return errors.New("journey steps must be an explicit array")
	}
	if report.Verdict == "PASS" {
		if err := validatePassReport(report); err != nil {
			return err
		}
	}
	return nil
}

func WriteReportAtomic(path string, report Report, beforeReplace func() error) error {
	reportPath, err := absoluteClean(path)
	if err != nil {
		return err
	}
	root := filepath.Dir(reportPath)
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("report parent is not a directory")
	}
	body, err := MarshalReport(report)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(root, ".tts-report-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(body); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if beforeReplace != nil {
		if err := beforeReplace(); err != nil {
			return err
		}
	}
	if err := os.Rename(tempPath, reportPath); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func samePath(first, second string) bool {
	firstPath, firstErr := absoluteClean(first)
	secondPath, secondErr := absoluteClean(second)
	if firstErr != nil || secondErr != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(firstPath, secondPath)
	}
	return firstPath == secondPath
}

func pathKey(path string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func pathIdentity(path string) string {
	return "sha256:" + sha256Hex([]byte(path))
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func bounded(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 256 {
		return value
	}
	return value[:256]
}
