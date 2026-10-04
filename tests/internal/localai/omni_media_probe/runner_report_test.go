package omni_media_probe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	if policy.TimeoutSeconds < 1 || policy.NetworkPolicy != ProbeNetworkPolicy || policy.DownloadBytes != 0 || policy.PaidUSD != 0 || policy.MaxHeavyProcesses != ProbeMaxHeavyProcesses || policy.MaxCalls < 1 || policy.MaxCalls > ProbeMaxCalls || policy.MaxRetries != ProbeMaxRetries {
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
	for _, output := range report.Outputs {
		values = append(values, output.Identity, output.PathIdentity)
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
