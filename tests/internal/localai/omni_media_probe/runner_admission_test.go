package omni_media_probe

import (
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

	"github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

func ReadProbeInput(path string) (ProbeInput, error) {
	path, err := normalizeProbePath(path, "input path")
	if err != nil {
		return ProbeInput{}, err
	}
	data, err := readProbeJSON(path, probeInputMaxBytes)
	if err != nil {
		return ProbeInput{}, fmt.Errorf("read OMNI probe input: %w", err)
	}
	var input ProbeInput
	if err := decodeStrictJSON(data, &input); err != nil {
		var typed *ValidationError
		if errors.As(err, &typed) {
			return ProbeInput{}, err
		}
		if strings.Contains(err.Error(), "json: unknown field") {
			return ProbeInput{}, validationError(CodeUnknownField, "$", "known probe input fields", "unknown field", err)
		}
		return ProbeInput{}, validationError(CodeProbeInvalidJSON, "$", "one strict probe input JSON value", "invalid", err)
	}
	if err := input.Validate(); err != nil {
		return ProbeInput{}, err
	}
	return input, nil
}

func WriteProbeInputAtomic(path string, input ProbeInput) error {
	if err := input.Validate(); err != nil {
		return fmt.Errorf("validate OMNI probe input: %w", err)
	}
	body, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return fmt.Errorf("encode OMNI probe input: %w", err)
	}
	body = append(body, '\n')
	return writeProbeJSONAtomic(path, body)
}

func WriteProbeInput(path string, input ProbeInput) error {
	return WriteProbeInputAtomic(path, input)
}

func (input ProbeInput) Validate() error { return input.validateShape() }

func (input ProbeInput) validateShape() error {
	if input.SchemaVersion != ProbeInputSchemaV1 {
		return validationError(CodeProbeInvalidInput, "schemaVersion", ProbeInputSchemaV1, input.SchemaVersion, nil)
	}
	if err := validateProbeLabel(input.RunID, "runId"); err != nil {
		return err
	}
	if err := validateBuildShape(input.Build); err != nil {
		return err
	}
	for name, identity := range map[string]ProbeFileIdentity{
		"dependencies.model":     input.Dependencies.Model,
		"dependencies.projector": input.Dependencies.Projector,
		"dependencies.backend":   input.Dependencies.Backend,
		"fixtureManifest":        input.FixtureManifest,
	} {
		if err := validateFileIdentityShape(identity, name); err != nil {
			return err
		}
	}
	if err := validateAbsolutePathShape(input.ProbeRoot, "probeRoot"); err != nil {
		return err
	}
	if len(input.Journeys) != 2 || input.Journeys[0] != probeJourneyImage || input.Journeys[1] != probeJourneyVideo {
		return validationError(CodeProbeInvalidInput, "journeys", "[image, video]", fmt.Sprint(input.Journeys), nil)
	}
	return validateProbeLimits(input.Limits)
}

func ReadReport(path string) (Report, error) {
	path, err := normalizeProbePath(path, "report path")
	if err != nil {
		return Report{}, err
	}
	data, err := readProbeJSON(path, probeInputMaxBytes)
	if err != nil {
		return Report{}, fmt.Errorf("read OMNI probe report: %w", err)
	}
	var report Report
	if err := decodeStrictJSON(data, &report); err != nil {
		var typed *ValidationError
		if errors.As(err, &typed) {
			return Report{}, err
		}
		if strings.Contains(err.Error(), "json: unknown field") {
			return Report{}, validationError(CodeUnknownField, "$", "known probe report fields", "unknown field", err)
		}
		return Report{}, validationError(CodeProbeInvalidJSON, "$", "one strict probe report JSON value", "invalid", err)
	}
	if err := report.Validate(); err != nil {
		return Report{}, err
	}
	return report, nil
}

func WriteReportAtomic(path string, report Report) error {
	if err := report.Validate(); err != nil {
		return fmt.Errorf("validate OMNI probe report: %w", err)
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode OMNI probe report: %w", err)
	}
	body = append(body, '\n')
	return writeProbeJSONAtomic(path, body)
}

func WriteReport(path string, report Report) error { return WriteReportAtomic(path, report) }

func (report ProbeReport) Validate() error {
	if report.SchemaVersion != ProbeReportSchemaV1 {
		return validationError(CodeProbeInvalidReport, "schemaVersion", ProbeReportSchemaV1, report.SchemaVersion, nil)
	}
	if err := validateProbeLabel(report.RunID, "runId"); err != nil {
		return err
	}
	if report.Status != "READY" && report.Status != "PASS" && report.Status != "FAIL" && report.Status != "INCONCLUSIVE" {
		return validationError(CodeProbeInvalidReport, "status", "READY, PASS, FAIL, or INCONCLUSIVE", report.Status, nil)
	}
	for name, identity := range map[string]RecordedIdentity{
		"build":                  report.Build,
		"dependencies.model":     report.Dependencies.Model,
		"dependencies.projector": report.Dependencies.Projector,
		"dependencies.backend":   report.Dependencies.Backend,
	} {
		if err := validateRecordedIdentity(identity, name, true); err != nil {
			return err
		}
	}
	if len(report.Fixtures) != 2 {
		return validationError(CodeProbeInvalidReport, "fixtures", "exactly two fixture identities", fmt.Sprint(len(report.Fixtures)), nil)
	}
	for index, fixture := range report.Fixtures {
		if err := validateRecordedIdentity(fixture, fmt.Sprintf("fixtures[%d]", index), true); err != nil {
			return err
		}
	}
	if err := validateProbePolicy(report.Policy); err != nil {
		return err
	}
	if len(report.Journeys) != 2 || report.Journeys[0].Name != probeJourneyImage || report.Journeys[1].Name != probeJourneyVideo {
		return validationError(CodeProbeInvalidReport, "journeys", "ordered image and video journeys", fmt.Sprint(report.Journeys), nil)
	}
	for index := range report.Journeys {
		if err := validateJourneyReport(report.Journeys[index], index); err != nil {
			return err
		}
	}
	for index, output := range report.Outputs {
		if err := validateRecordedIdentity(output, fmt.Sprintf("outputs[%d]", index), false); err != nil {
			return err
		}
	}
	for index, process := range report.Processes {
		if err := validateProcessEvidence(process, index); err != nil {
			return err
		}
	}
	if !report.Cleanup.Checked || report.Cleanup.OwnedProcessSurvivors != 0 || report.Cleanup.OwnedListenerSurvivors != 0 || report.Cleanup.PartialOutputs != 0 {
		return validationError(CodeProbeInvalidReport, "cleanup", "checked cleanup with zero survivors and partial outputs", fmt.Sprintf("%+v", report.Cleanup), nil)
	}
	if report.Failure != nil {
		if err := validateReportFailure(*report.Failure, "failure"); err != nil {
			return err
		}
	}
	if err := validateReportState(report); err != nil {
		return err
	}
	if err := validateReportRedaction(report); err != nil {
		return err
	}
	return nil
}

type admittedProbeInput struct {
	input        ProbeInput
	reportPath   string
	build        RecordedIdentity
	dependencies ProbeReportDependencies
	manifest     Manifest
}

func admitProbeInput(ctx context.Context, input ProbeInput, reportPath string) (admittedProbeInput, error) {
	if err := input.Validate(); err != nil {
		return admittedProbeInput{}, err
	}
	reportPath, err := normalizeProbePath(reportPath, "report path")
	if err != nil {
		return admittedProbeInput{}, validationError(CodeProbeInvalidReport, "reportPath", "absolute clean report path", reportPath, err)
	}
	if err := validateReportDestination(reportPath, input.ProbeRoot); err != nil {
		return admittedProbeInput{}, err
	}
	if err := ctx.Err(); err != nil {
		return admittedProbeInput{}, probeContextError(err)
	}

	build, err := admitBuildIdentity(input.Build)
	if err != nil {
		return admittedProbeInput{}, err
	}
	dependencies := ProbeReportDependencies{}
	for name, declared := range map[string]ProbeFileIdentity{
		"model":     input.Dependencies.Model,
		"projector": input.Dependencies.Projector,
		"backend":   input.Dependencies.Backend,
	} {
		observed, observeErr := admitFileIdentity(declared, "dependencies."+name)
		if observeErr != nil {
			return admittedProbeInput{}, observeErr
		}
		switch name {
		case "model":
			dependencies.Model = observed
		case "projector":
			dependencies.Projector = observed
		case "backend":
			dependencies.Backend = observed
		}
	}
	manifestIdentity, err := admitFileIdentity(input.FixtureManifest, "fixtureManifest")
	if err != nil {
		return admittedProbeInput{}, err
	}
	manifest, err := LoadManifest(input.FixtureManifest.Path)
	if err != nil {
		return admittedProbeInput{}, fmt.Errorf("admit fixture manifest: %w", err)
	}
	if filepath.Clean(manifest.ManifestPath) != filepath.Clean(input.FixtureManifest.Path) || manifestIdentity.Bytes <= 0 {
		return admittedProbeInput{}, validationError(CodeProbeIdentityMismatch, "fixtureManifest.path", input.FixtureManifest.Path, manifest.ManifestPath, nil)
	}
	if err := validateProbeRoot(input.ProbeRoot); err != nil {
		return admittedProbeInput{}, err
	}
	if err := ensureDistinctDeclaredPaths(input); err != nil {
		return admittedProbeInput{}, err
	}
	return admittedProbeInput{input: input, reportPath: reportPath, build: build, dependencies: dependencies, manifest: manifest}, nil
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
	if filepath.IsAbs(filepath.VolumeName(path)) && filepath.Clean(path) == filepath.VolumeName(path) {
		return validationError(CodeProbeInvalidIdentity, field, "non-root absolute path", path, nil)
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

func validateProbeLimits(limits ProbeLimits) error {
	if limits.TimeoutSeconds < 1 || limits.TimeoutSeconds > ProbeMaxTimeoutSeconds {
		return validationError(CodeProbeInvalidInput, "limits.timeoutSeconds", fmt.Sprintf("1..%d", ProbeMaxTimeoutSeconds), fmt.Sprint(limits.TimeoutSeconds), nil)
	}
	if limits.MaxHeavyProcesses != ProbeMaxHeavyProcesses {
		return validationError(CodeProbeInvalidInput, "limits.maxHeavyProcesses", "1", fmt.Sprint(limits.MaxHeavyProcesses), nil)
	}
	if limits.MaxCompilerTestProcesses != ProbeMaxCompilerTestProcesses {
		return validationError(CodeProbeInvalidInput, "limits.maxCompilerTestProcesses", "4", fmt.Sprint(limits.MaxCompilerTestProcesses), nil)
	}
	if limits.MaxDiskBytes <= 0 || limits.MaxDiskBytes > ProbeMaxDiskBytes {
		return validationError(CodeProbeInvalidInput, "limits.maxDiskBytes", fmt.Sprintf("1..%d", ProbeMaxDiskBytes), fmt.Sprint(limits.MaxDiskBytes), nil)
	}
	if limits.MaxDownloadBytes != 0 {
		return validationError(CodeProbeInvalidInput, "limits.maxDownloadBytes", "0", fmt.Sprint(limits.MaxDownloadBytes), nil)
	}
	if limits.MaxPaidUSD != 0 {
		return validationError(CodeProbeInvalidInput, "limits.maxPaidUsd", "0", fmt.Sprint(limits.MaxPaidUSD), nil)
	}
	if limits.MaxCalls < 1 || limits.MaxCalls > ProbeMaxCalls {
		return validationError(CodeProbeInvalidInput, "limits.maxCalls", fmt.Sprintf("1..%d", ProbeMaxCalls), fmt.Sprint(limits.MaxCalls), nil)
	}
	if limits.MaxRetries != ProbeMaxRetries {
		return validationError(CodeProbeInvalidInput, "limits.maxRetries", fmt.Sprint(ProbeMaxRetries), fmt.Sprint(limits.MaxRetries), nil)
	}
	if limits.ForbiddenPort != ProbeForbiddenPort {
		return validationError(CodeProbeInvalidInput, "limits.forbiddenPort", fmt.Sprint(ProbeForbiddenPort), fmt.Sprint(limits.ForbiddenPort), nil)
	}
	if limits.NetworkPolicy != ProbeNetworkPolicy {
		return validationError(CodeProbeInvalidInput, "limits.networkPolicy", ProbeNetworkPolicy, limits.NetworkPolicy, nil)
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

func ensureDistinctDeclaredPaths(input ProbeInput) error {
	seen := map[string]string{}
	add := func(field, path string) error {
		clean := filepath.Clean(path)
		if previous, exists := seen[clean]; exists {
			return validationError(CodeProbeInvalidIdentity, field+".path", "unique declared path", previous, nil)
		}
		seen[clean] = field
		return nil
	}
	if err := add("build", input.Build.Path); err != nil {
		return err
	}
	for field, identity := range map[string]ProbeFileIdentity{
		"dependencies.model":     input.Dependencies.Model,
		"dependencies.projector": input.Dependencies.Projector,
		"dependencies.backend":   input.Dependencies.Backend,
		"fixtureManifest":        input.FixtureManifest,
	} {
		if err := add(field, identity.Path); err != nil {
			return err
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

type probeRoots struct {
	Root  string
	Paths RootPaths
	Owned []string
	Port  int
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

func newProbeReport(prepared admittedProbeInput, roots probeRoots, port int) (ProbeReport, error) {
	image, video, err := probeJourneyReports(prepared.manifest)
	if err != nil {
		return ProbeReport{}, err
	}
	return ProbeReport{
		SchemaVersion: ProbeReportSchemaV1,
		RunID:         prepared.input.RunID,
		Status:        "INCONCLUSIVE",
		Build:         prepared.build,
		Dependencies:  prepared.dependencies,
		Fixtures: []RecordedIdentity{
			recordedArtifact(prepared.manifest.Artifacts[0]), recordedArtifact(prepared.manifest.Artifacts[1]),
		},
		Policy: ProbePolicy{
			RootIdentities: rootIdentities(roots), Port: port,
			TimeoutSeconds:    prepared.input.Limits.TimeoutSeconds,
			NetworkPolicy:     prepared.input.Limits.NetworkPolicy,
			DownloadBytes:     prepared.input.Limits.MaxDownloadBytes,
			PaidUSD:           prepared.input.Limits.MaxPaidUSD,
			MaxHeavyProcesses: prepared.input.Limits.MaxHeavyProcesses,
			MaxCalls:          prepared.input.Limits.MaxCalls,
			MaxRetries:        prepared.input.Limits.MaxRetries,
		},
		Journeys: []JourneyReport{image, video},
		Outputs:  []RecordedIdentity{}, Processes: []ProcessEvidence{},
	}, nil
}

func probeJourneyReports(manifest Manifest) (JourneyReport, JourneyReport, error) {
	imageRubric, err := reportRubric(manifest.Artifacts[0])
	if err != nil {
		return JourneyReport{}, JourneyReport{}, err
	}
	videoRubric, err := reportRubric(manifest.Artifacts[1])
	if err != nil {
		return JourneyReport{}, JourneyReport{}, err
	}
	imagePrompt := "Identify the infinity symbol, text, and accent colors in this image."
	videoPrompt := "Describe the ordered phases and hard-cut transition in this video."
	image := makeJourneyReport(probeJourneyImage, imagePrompt, manifest.Artifacts[0], imageRubric)
	video := makeJourneyReport(probeJourneyVideo, videoPrompt, manifest.Artifacts[1], videoRubric)
	return image, video, nil
}

func makeJourneyReport(name JourneyName, prompt string, artifact Artifact, rubric json.RawMessage) JourneyReport {
	promptBytes := []byte(prompt)
	fixtureName := "image"
	modality := "IMAGE"
	mediaType := "image/png"
	if name == probeJourneyVideo {
		fixtureName, modality, mediaType = "video", "VIDEO", "video/mp4"
	}
	return JourneyReport{
		Name: name, Status: JourneyNotRun,
		Command: []string{"models", "invoke", "llm", "--operation", "OMNI", "--input", "prompt=" + prompt, "--input", fixtureName + "=@fixture:" + artifact.ID},
		RequestInputs: []RequestInput{
			{Order: 0, Name: "prompt", Modality: "TEXT", MediaType: "text/plain", Bytes: int64(len(promptBytes)), SHA256: hashBytes(promptBytes)},
			{Order: 1, Name: fixtureName, Modality: modality, MediaType: mediaType, Bytes: artifact.Bytes, SHA256: artifact.SHA256},
		},
		SemanticRubric: rubric,
	}
}

func reportRubric(artifact Artifact) (json.RawMessage, error) {
	if artifact.SemanticRubric.Image != nil {
		predicates := make([]struct {
			Kind  string          `json:"kind"`
			Exact json.RawMessage `json:"exact"`
		}, 0, len(artifact.SemanticRubric.Image.AllOf))
		for _, predicate := range artifact.SemanticRubric.Image.AllOf {
			predicates = append(predicates, struct {
				Kind  string          `json:"kind"`
				Exact json.RawMessage `json:"exact"`
			}{Kind: predicate.Kind, Exact: predicate.Exact})
		}
		return json.Marshal(struct {
			AllOf any `json:"allOf"`
		}{AllOf: predicates})
	}
	if artifact.SemanticRubric.Video != nil {
		return json.Marshal(struct {
			OrderedPhases []VideoPhase     `json:"orderedPhases"`
			Transition    *VideoTransition `json:"transition"`
		}{OrderedPhases: artifact.SemanticRubric.Video.OrderedPhases, Transition: artifact.SemanticRubric.Video.Transition})
	}
	return nil, validationError(CodeProbeInvalidReport, "semanticRubric", "one media rubric", "missing", nil)
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
