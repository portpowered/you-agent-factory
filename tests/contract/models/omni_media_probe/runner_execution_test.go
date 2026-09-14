package omni_media_probe

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
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

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

// reserveProbePort obtains an ephemeral loopback port and closes the probing
// listener before returning. The returned port is therefore available for the
// executor to bind; the listener is only a short-lived collision check.
func reserveProbePort(forbidden int) (int, error) {
	for attempt := 0; attempt < 8; attempt++ {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		address, ok := listener.Addr().(*net.TCPAddr)
		if !ok || address.Port <= 0 {
			_ = listener.Close()
			return 0, errors.New("loopback listener did not expose a port")
		}
		if address.Port != forbidden {
			if err := listener.Close(); err != nil {
				return 0, fmt.Errorf("release loopback port reservation: %w", err)
			}
			return address.Port, nil
		}
		_ = listener.Close()
	}
	return 0, fmt.Errorf("ephemeral port repeatedly selected forbidden port %d", forbidden)
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
			TimeoutSeconds:           prepared.input.Limits.TimeoutSeconds,
			NetworkPolicy:            prepared.input.Limits.NetworkPolicy,
			DownloadBytes:            prepared.input.Limits.MaxDownloadBytes,
			PaidUSD:                  prepared.input.Limits.MaxPaidUSD,
			MaxHeavyProcesses:        prepared.input.Limits.MaxHeavyProcesses,
			MaxCompilerTestProcesses: prepared.input.Limits.MaxCompilerTestProcesses,
			MaxDiskBytes:             prepared.input.Limits.MaxDiskBytes,
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

func (runner Runner) executeJourneys(ctx context.Context, report *ProbeReport, roots probeRoots) error {
	for index := range report.Journeys {
		if index == 1 && report.Journeys[0].Status != JourneyPass {
			continue
		}
		outcome := runner.executeJourney(ctx, &report.Journeys[index], roots)
		report.Processes = append(report.Processes, outcome.process)
		report.Outputs = append(report.Outputs, outcome.outputs...)
		if outcome.failure != nil && report.Failure == nil {
			report.Failure = outcome.failure
		}
		if outcome.cleanupFailure != nil && report.Failure == nil {
			report.Failure = outcome.cleanupFailure
		}
		if outcome.cleanupFailure != nil {
			return fmt.Errorf("journey %s cleanup was not proven: %w", report.Journeys[index].Name, validationError(CodeProbeCleanupFailure, "cleanup", "zero owned survivors", outcome.cleanupFailure.Observed, nil))
		}
		if report.Journeys[index].Status != JourneyPass {
			if report.Journeys[index].Status == JourneyInconclusive {
				report.Status = "INCONCLUSIVE"
			} else {
				report.Status = "FAIL"
			}
			return nil
		}
	}
	report.Status = "PASS"
	return nil
}

type journeyOutcome struct {
	process        ProcessEvidence
	outputs        []RecordedIdentity
	failure        *ReportFailure
	cleanupFailure *ReportFailure
}

func (runner Runner) executeJourney(ctx context.Context, journey *JourneyReport, roots probeRoots) journeyOutcome {
	request := ExecutionRequest{
		Journey: journey.Name, Command: append([]string(nil), journey.Command...),
		Inputs: append([]RequestInput(nil), journey.RequestInputs...), Roots: roots.Paths,
		Port: roots.Port,
	}
	return runner.executeJourneyRequest(ctx, journey, request)
}

func (runner Runner) executeJourneyRequest(ctx context.Context, journey *JourneyReport, request ExecutionRequest) journeyOutcome {
	observation, err := runner.Executor.Execute(ctx, request)
	process := observation.Process
	if process.Identity == "" {
		process.Identity = "not-started-" + string(journey.Name)
	}
	if process.Kind == "" {
		process.Kind = "controlled-executor"
	}
	if process.Owner == "" {
		process.Owner = "omni-media-probe"
	}
	if observation.OwnedProcessSurvivors != 0 || observation.OwnedListenerSurvivors != 0 || observation.PartialOutputs != 0 {
		journey.Status = JourneyFail
		failure := probeFailure("harness", string(CodeProbeCleanupFailure), "owned processes, listeners, and partial outputs are zero", "owned resource survivors were reported", "repair executor cleanup before retrying")
		journey.Failure = failure
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: failure, cleanupFailure: failure}
	}
	if err != nil {
		journey.Status = JourneyFail
		journey.Failure = probeFailure("executor", string(CodeProbeExecutionFailure), "executor returns a bounded observation", "executor returned an error", "inspect the controlled or real executor evidence")
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: journey.Failure}
	}
	if observation.TimedOut || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		journey.Status = JourneyInconclusive
		journey.Failure = probeFailure("harness", string(CodeProbeTimedOut), "journey finishes within the declared timeout", "journey timed out", "inspect the bounded timeout evidence and retry the real gate")
		process.TimedOut = true
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: journey.Failure}
	}
	if observation.Cancelled || errors.Is(ctx.Err(), context.Canceled) {
		journey.Status = JourneyInconclusive
		journey.Failure = probeFailure("harness", string(CodeProbeCancelled), "journey completes before cancellation", "journey was cancelled", "inspect cleanup evidence before retrying")
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: journey.Failure}
	}
	if observation.Failure != nil {
		journey.Status = JourneyFail
		journey.Failure = normalizeReportFailure(*observation.Failure, "executor")
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: journey.Failure}
	}
	if !process.Started || !process.Exited || process.ExitCode != 0 {
		journey.Status = JourneyFail
		journey.Failure = probeFailure("executor", string(CodeProbeExecutionFailure), "started process exits with code zero", "process observation was not successful", "inspect process evidence before retrying")
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: journey.Failure}
	}
	for index, output := range observation.Outputs {
		if err := validateRecordedIdentity(output, fmt.Sprintf("outputs[%d]", index), false); err != nil {
			journey.Status = JourneyFail
			journey.Failure = probeFailure("harness", string(CodeProbeOutputFailure), "output identity is complete and redacted", "output identity was invalid", "repair output recording before retrying")
			return journeyOutcome{process: process, failure: journey.Failure}
		}
	}
	journey.Status = JourneyPass
	return journeyOutcome{process: process, outputs: observation.Outputs}
}

func recordedArtifact(artifact Artifact) RecordedIdentity {
	return RecordedIdentity{Identity: artifact.ID, PathIdentity: pathIdentity(artifact.ResolvedPath), Bytes: artifact.Bytes, SHA256: artifact.SHA256}
}

func rootIdentities(roots probeRoots) []string {
	paths := []string{roots.Root, roots.Paths.Work, roots.Paths.Profile, roots.Paths.Cache, roots.Paths.Model, roots.Paths.Projector, roots.Paths.Backend, roots.Paths.Output, roots.Paths.Streams}
	identities := make([]string, 0, len(paths))
	for _, path := range paths {
		identities = append(identities, pathIdentity(path))
	}
	return identities
}

func pathIdentity(path string) string {
	digest := sha256.Sum256([]byte(filepath.Clean(path)))
	return "sha256=" + hex.EncodeToString(digest[:])
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (report ProbeReport) journeyFailureCount() int {
	count := 0
	for _, journey := range report.Journeys {
		if journey.Status == JourneyFail || journey.Status == JourneyInconclusive {
			count++
		}
	}
	return count
}

func validateProbePolicy(policy ProbePolicy) error {
	if len(policy.RootIdentities) < 6 || !uniqueStrings(policy.RootIdentities) {
		return validationError(CodeProbeInvalidReport, "policy.rootIdentities", "at least six unique redacted roots", fmt.Sprint(policy.RootIdentities), nil)
	}
	if policy.Port <= 0 || policy.Port > 65535 || policy.Port == ProbeForbiddenPort {
		return validationError(CodeProbeInvalidReport, "policy.port", "valid non-7437 port", fmt.Sprint(policy.Port), nil)
	}
	if policy.TimeoutSeconds < 1 || policy.NetworkPolicy != ProbeNetworkPolicy || policy.DownloadBytes != 0 || policy.PaidUSD != 0 || policy.MaxHeavyProcesses != ProbeMaxHeavyProcesses || policy.MaxCompilerTestProcesses != ProbeMaxCompilerTestProcesses || policy.MaxDiskBytes <= 0 || policy.MaxDiskBytes > ProbeMaxDiskBytes {
		return validationError(CodeProbeInvalidReport, "policy", "declared zero-download one-heavy local policy", fmt.Sprintf("%+v", policy), nil)
	}
	return nil
}

func validateJourneyReport(journey JourneyReport, index int) error {
	if journey.Status != JourneyNotRun && journey.Status != JourneyPass && journey.Status != JourneyFail && journey.Status != JourneyInconclusive {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].status", index), "known journey status", string(journey.Status), nil)
	}
	if len(journey.Command) == 0 {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].command", index), "non-empty redacted command", "empty", nil)
	}
	for _, argument := range journey.Command {
		if reportStringContainsAbsolutePath(argument) {
			return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].command", index), "redacted command without absolute paths", "absolute path", nil)
		}
	}
	if len(journey.RequestInputs) != 2 {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].requestInputs", index), "prompt and one ordered media input", fmt.Sprint(len(journey.RequestInputs)), nil)
	}
	for inputIndex, input := range journey.RequestInputs {
		if input.Order != inputIndex || input.Bytes < 0 || validateSHA256(input.SHA256, fmt.Sprintf("journeys[%d].requestInputs[%d].sha256", index, inputIndex)) != nil {
			return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].requestInputs[%d]", index, inputIndex), "ordered complete request input", fmt.Sprint(input), nil)
		}
	}
	wantMediaName, wantModality, wantMediaType := "image", "IMAGE", "image/png"
	if journey.Name == probeJourneyVideo {
		wantMediaName, wantModality, wantMediaType = "video", "VIDEO", "video/mp4"
	}
	if journey.RequestInputs[0].Name != "prompt" || journey.RequestInputs[0].Modality != "TEXT" || journey.RequestInputs[0].MediaType != "text/plain" || journey.RequestInputs[1].Name != wantMediaName || journey.RequestInputs[1].Modality != wantModality || journey.RequestInputs[1].MediaType != wantMediaType {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].requestInputs", index), "prompt followed by the named media input", fmt.Sprint(journey.RequestInputs), nil)
	}
	if len(bytes.TrimSpace(journey.SemanticRubric)) == 0 || journey.SemanticRubric[0] != '{' {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].semanticRubric", index), "non-empty JSON object rubric", "missing", nil)
	}
	var rubric map[string]json.RawMessage
	if err := json.Unmarshal(journey.SemanticRubric, &rubric); err != nil || len(rubric) == 0 {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].semanticRubric", index), "non-empty JSON object rubric", "invalid", err)
	}
	if journey.Status == JourneyNotRun && journey.Failure != nil {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].failure", index), "null for NOT_RUN", "failure present", nil)
	}
	if journey.Status == JourneyPass && journey.Failure != nil {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].failure", index), "null for PASS", "failure present", nil)
	}
	if (journey.Status == JourneyFail || journey.Status == JourneyInconclusive) && journey.Failure == nil {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].failure", index), "typed failure for failed journey", "missing", nil)
	}
	if journey.Failure != nil {
		return validateReportFailure(*journey.Failure, fmt.Sprintf("journeys[%d].failure", index))
	}
	return nil
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

func validateReportState(report ProbeReport) error {
	if report.Status == "READY" {
		if report.Failure != nil || report.journeyFailureCount() != 0 || report.Journeys[0].Status != JourneyNotRun || report.Journeys[1].Status != JourneyNotRun {
			return validationError(CodeProbeInvalidReport, "status", "READY with no executed journey or failure", "inconsistent", nil)
		}
	}
	if report.Status == "PASS" && (report.Failure != nil || report.Journeys[0].Status != JourneyPass || report.Journeys[1].Status != JourneyPass) {
		return validationError(CodeProbeInvalidReport, "status", "PASS with two passing journeys", "inconsistent", nil)
	}
	if (report.Status == "FAIL" || report.Status == "INCONCLUSIVE") && report.Failure == nil {
		return validationError(CodeProbeInvalidReport, "failure", "typed report failure", "missing", nil)
	}
	if report.Journeys[0].Status != JourneyPass && report.Journeys[1].Status != JourneyNotRun {
		return validationError(CodeProbeInvalidReport, "journeys[1].status", "video NOT_RUN after image failure", string(report.Journeys[1].Status), nil)
	}
	return nil
}

func validateReportRedaction(report ProbeReport) error {
	values := []string{report.Build.Identity, report.Dependencies.Model.Identity, report.Dependencies.Projector.Identity, report.Dependencies.Backend.Identity}
	for _, fixture := range report.Fixtures {
		values = append(values, fixture.Identity, fixture.PathIdentity)
	}
	for _, journey := range report.Journeys {
		values = append(values, journey.Command...)
	}
	for _, process := range report.Processes {
		values = append(values, process.Identity, process.Kind, process.Owner)
	}
	if report.Failure != nil {
		values = append(values, report.Failure.Owner, report.Failure.Code, report.Failure.Expected, report.Failure.Observed, report.Failure.NextAction)
	}
	for _, value := range values {
		if reportStringContainsAbsolutePath(value) {
			return validationError(CodeProbeInvalidReport, "report", "redacted path values", "absolute path", nil)
		}
	}
	return nil
}

func reportStringContainsAbsolutePath(value string) bool {
	if filepath.IsAbs(value) {
		return true
	}
	if len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		return true
	}
	return false
}

func normalizeReportFailure(failure ReportFailure, defaultOwner string) *ReportFailure {
	if failure.Owner == "" {
		failure.Owner = defaultOwner
	}
	if failure.Code == "" {
		failure.Code = string(CodeProbeExecutionFailure)
	}
	if failure.Expected == "" {
		failure.Expected = "bounded journey observation"
	}
	if failure.Observed == "" {
		failure.Observed = "executor reported failure"
	}
	if failure.NextAction == "" {
		failure.NextAction = "inspect the bounded evidence before retrying"
	}
	return &failure
}

func probeFailure(owner, code, expected, observed, nextAction string) *ReportFailure {
	return &ReportFailure{Owner: owner, Code: code, Expected: expected, Observed: observed, NextAction: nextAction}
}

func probeContextError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return validationError(CodeProbeTimedOut, "context", "live bounded context", "deadline exceeded", err)
	}
	return validationError(CodeProbeCancelled, "context", "live bounded context", "cancelled", err)
}

func wrapProbeCleanupError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, validationError(CodeProbeCleanupFailure, "cleanup", "zero owned survivors", "cleanup failed", err))
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
