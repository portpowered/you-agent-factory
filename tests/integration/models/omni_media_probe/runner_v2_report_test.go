package omni_media_probe

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/portpowered/infinite-you/tests/internal/localai/corpusv2"
)

func ReadProbeReportV2(path string) (ProbeReportV2, error) {
	path, err := normalizeProbePath(path, "report path")
	if err != nil {
		return ProbeReportV2{}, err
	}
	if err := validateExistingRegularFile(path, "report"); err != nil {
		return ProbeReportV2{}, err
	}
	data, err := readProbeJSON(path, probeInputMaxBytes)
	if err != nil {
		return ProbeReportV2{}, fmt.Errorf("read corpus runner v2 report: %w", err)
	}
	var report ProbeReportV2
	if err := decodeStrictJSON(data, &report); err != nil {
		return ProbeReportV2{}, strictJSONError("report", err)
	}
	if err := report.Validate(); err != nil {
		return ProbeReportV2{}, err
	}
	return report, nil
}

func WriteProbeReportV2Atomic(path string, report ProbeReportV2) error {
	if err := report.Validate(); err != nil {
		return fmt.Errorf("validate corpus runner v2 report: %w", err)
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode corpus runner v2 report: %w", err)
	}
	if err := writeProbeJSONNoReplaceAtomic(path, append(body, '\n')); err != nil {
		if errors.Is(err, errProbeV2ReportExists) {
			return validationError(CodeProbeInvalidReport, "reportPath", "fresh absent report destination", "already exists", nil)
		}
		return validationError(CodeProbeReportPersist, "report", "complete atomic v2 report", "atomic persistence failed", nil)
	}
	return nil
}

var errProbeV2ReportExists = errors.New("v2 report destination exists")

func (report ProbeReportV2) Validate() error {
	if report.SchemaVersion != ProbeReportSchemaV2 {
		return validationError(CodeProbeInvalidReport, "schemaVersion", ProbeReportSchemaV2, report.SchemaVersion, nil)
	}
	if err := validateProbeLabel(report.RunID, "runId"); err != nil {
		return err
	}
	for _, item := range []struct {
		name     string
		identity RecordedIdentity
	}{
		{name: "build", identity: report.Build},
		{name: "dependencies.model", identity: report.Dependencies.Model},
		{name: "dependencies.projector", identity: report.Dependencies.Projector},
		{name: "dependencies.backend", identity: report.Dependencies.Backend},
	} {
		if err := validateRecordedIdentity(item.identity, item.name, true); err != nil {
			return err
		}
	}
	if err := validateCorpusReportV2(report.Corpus); err != nil {
		return err
	}
	if err := validateProbePolicyV2(report.Policy); err != nil {
		return err
	}
	if report.Calls == nil || report.Outputs == nil || report.Processes == nil || report.Corpus.SelectedSamples == nil {
		return validationError(CodeProbeInvalidReport, "report", "arrays present as JSON arrays", "null array", nil)
	}
	if !report.Cleanup.Checked || report.Cleanup.OwnedProcessSurvivors != 0 || report.Cleanup.OwnedListenerSurvivors != 0 || report.Cleanup.PartialOutputs != 0 {
		return validationError(CodeProbeInvalidReport, "cleanup", "checked cleanup with zero survivors and outputs", "cleanup is not proven zero", nil)
	}
	switch report.Mode {
	case "PREFLIGHT":
		if report.Status != "READY" || len(report.Calls) != 0 || len(report.Outputs) != 0 || len(report.Processes) != 0 || report.Failure != nil {
			return validationError(CodeProbeInvalidReport, "mode", "PREFLIGHT READY without calls, outputs, processes, or failure", "inconsistent preflight evidence", nil)
		}
	case "EXECUTE":
		if err := validateProbeExecutionReportV2(report); err != nil {
			return err
		}
	default:
		return validationError(CodeProbeInvalidReport, "mode", "PREFLIGHT or EXECUTE", report.Mode, nil)
	}
	return validateProbeReportV2Redaction(report)
}

func validateProbeExecutionReportV2(report ProbeReportV2) error {
	if len(report.Calls) == 0 || len(report.Calls) > int(ProbeV2MaxCalls) {
		return validationError(CodeProbeInvalidReport, "calls", "one to ten executed calls", fmt.Sprint(len(report.Calls)), nil)
	}
	if len(report.Processes) != len(report.Calls) {
		return validationError(CodeProbeInvalidReport, "processes", "one process observation per call", fmt.Sprint(len(report.Processes)), nil)
	}
	expectedOutputs := make([]RecordedIdentity, 0, len(report.Calls))
	failedAt := -1
	for index, call := range report.Calls {
		if call.Ordinal != index+1 {
			return validationError(CodeProbeInvalidReport, fmt.Sprintf("calls[%d].ordinal", index), fmt.Sprint(index+1), fmt.Sprint(call.Ordinal), nil)
		}
		if err := validateProbeCallV2(call, index, report.Corpus.SelectedSamples); err != nil {
			return err
		}
		process := report.Processes[index]
		if err := validateProcessEvidence(process, index); err != nil {
			return err
		}
		if (process.Started && !process.Exited) || (!process.Started && (process.PID != 0 || process.Exited)) {
			return validationError(CodeProbeInvalidReport, fmt.Sprintf("processes[%d]", index), "terminal process or a non-started attempt", "process lifetime is incomplete", nil)
		}
		if call.Status == "PASS" {
			if failedAt >= 0 {
				return validationError(CodeProbeInvalidReport, "calls", "no calls after a non-PASS outcome", "execution expanded after failure", nil)
			}
			expectedOutputs = append(expectedOutputs, *call.Output)
		} else {
			if failedAt >= 0 || index != len(report.Calls)-1 {
				return validationError(CodeProbeInvalidReport, "calls", "one terminal non-PASS outcome at the end", "failure prefix is not minimal", nil)
			}
			failedAt = index
		}
	}
	if failedAt < 0 {
		if len(report.Calls) != int(ProbeV2MaxCalls) || report.Status != "PASS" || report.Failure != nil {
			return validationError(CodeProbeInvalidReport, "status", "PASS only after all ten calls", "incomplete successful run", nil)
		}
	} else {
		last := report.Calls[failedAt]
		if report.Status != last.Status || report.Failure == nil || *report.Failure != *last.Failure {
			return validationError(CodeProbeInvalidReport, "status", "terminal status and failure match the final call", "report outcome mismatch", nil)
		}
	}
	if report.Failure != nil {
		if err := validateReportFailure(*report.Failure, "failure"); err != nil {
			return err
		}
	}
	if len(report.Outputs) != len(expectedOutputs) {
		return validationError(CodeProbeInvalidReport, "outputs", "exactly the gradeable outputs from completed PASS calls", fmt.Sprint(len(report.Outputs)), nil)
	}
	for index, expected := range expectedOutputs {
		if report.Outputs[index] != expected {
			return validationError(CodeProbeInvalidReport, fmt.Sprintf("outputs[%d]", index), "ordered call output identity", "output ledger mismatch", nil)
		}
	}
	return nil
}

func validateProbeCallV2(call ProbeCallReportV2, index int, samples []CorpusSampleReportV2) error {
	field := fmt.Sprintf("calls[%d]", index)
	if call.Status != "PASS" && call.Status != "FAIL" && call.Status != "INCONCLUSIVE" {
		return validationError(CodeProbeInvalidReport, field+".status", "PASS, FAIL, or INCONCLUSIVE", call.Status, nil)
	}
	if call.Status == "PASS" {
		if call.Failure != nil || call.Output == nil {
			return validationError(CodeProbeInvalidReport, field, "PASS with one output and no failure", "output or failure mismatch", nil)
		}
	} else if call.Failure == nil || call.Output != nil {
		return validationError(CodeProbeInvalidReport, field, "non-PASS with typed failure and no output", "output or failure mismatch", nil)
	}
	if call.Failure != nil {
		if err := validateReportFailure(*call.Failure, field+".failure"); err != nil {
			return err
		}
	}
	if index == 0 {
		if call.Kind != probeV2CallWarmup || call.SampleIdentity != nil || len(call.Inputs) != 1 {
			return validationError(CodeProbeInvalidReport, field, "text-only WARMUP with one input and no sample", "warm-up shape mismatch", nil)
		}
	} else {
		sampleIndex := index - 1
		if sampleIndex >= len(samples) || len(call.Inputs) != 2 {
			return validationError(CodeProbeInvalidReport, field, "CANARY/SAMPLE bound to a selected video", "sample input shape mismatch", nil)
		}
		wantKind := probeV2CallSample
		if sampleIndex == 0 {
			wantKind = probeV2CallCanary
		}
		wantSampleIdentity := corpusSampleIdentityV2(samples[sampleIndex])
		if call.Kind != wantKind || call.SampleIdentity == nil || *call.SampleIdentity != wantSampleIdentity || call.Inputs[1] != samples[sampleIndex].Clip {
			return validationError(CodeProbeInvalidReport, field, "exact canary/sample selection and clip identity", "sample order or identity mismatch", nil)
		}
	}
	for inputIndex, input := range call.Inputs {
		if err := validateRecordedIdentity(input, fmt.Sprintf("%s.inputs[%d]", field, inputIndex), true); err != nil {
			return err
		}
	}
	if call.Inputs[0].Identity != "sha256:"+call.Inputs[0].SHA256 {
		return validationError(CodeProbeInvalidReport, field+".inputs[0]", "redacted prompt digest identity", "prompt identity is not a digest", nil)
	}
	wantLength := 7
	if index != 0 {
		wantLength = 9
	}
	if len(call.Command) != wantLength {
		return validationError(CodeProbeInvalidReport, field+".command", fmt.Sprintf("%d redacted public CLI arguments", wantLength), fmt.Sprint(len(call.Command)), nil)
	}
	wantPrefix := []string{"models", "invoke", "llm", "--operation", "OMNI", "--input"}
	for commandIndex, expected := range wantPrefix {
		if call.Command[commandIndex] != expected {
			return validationError(CodeProbeInvalidReport, field+".command", "shipped models invoke llm OMNI grammar", "command prefix mismatch", nil)
		}
	}
	if call.Command[6] != "prompt=sha256:"+call.Inputs[0].SHA256 {
		return validationError(CodeProbeInvalidReport, field+".command", "prompt argument replaced by its digest", "prompt argument mismatch", nil)
	}
	if index != 0 && (call.Command[7] != "--input" || call.Command[8] != "video=@<"+call.Inputs[1].PathIdentity+">") {
		return validationError(CodeProbeInvalidReport, field+".command", "video @ input represented by its path identity", "video argument mismatch", nil)
	}
	if call.Output != nil {
		if err := validateRecordedIdentity(*call.Output, field+".output", true); err != nil {
			return err
		}
		if call.Output.Identity != "sha256:"+call.Output.SHA256 {
			return validationError(CodeProbeInvalidReport, field+".output", "redacted output digest identity", "output identity is not a digest", nil)
		}
	}
	return nil
}

func corpusSampleIdentityV2(sample CorpusSampleReportV2) string {
	return sample.Study + "/" + sample.Band + "/" + sample.Attempt
}

func validateCorpusReportV2(corpus CorpusReportV2) error {
	authority := corpusv2.DefaultCorpusV2Authority()
	indexPath := filepath.Join(authority.RepositoryRoot, filepath.FromSlash(authority.IndexPath))
	if corpus.RepositoryIdentity != pathIdentity(authority.RepositoryRoot) || corpus.IndexPathIdentity != pathIdentity(indexPath) {
		return validationError(CodeProbeInvalidReport, "corpus.pathIdentity", "hashed pinned corpus paths", "identity mismatch", nil)
	}
	if corpus.Commit != authority.Commit || !strings.EqualFold(corpus.IndexSHA256, authority.IndexSHA256) || corpus.UniqueClips != authority.PairCount || corpus.UniquePrompts != authority.PairCount {
		return validationError(CodeProbeInvalidReport, "corpus", "pinned commit, index digest, and 370 pairs", "corpus identity mismatch", nil)
	}
	if !corpus.ReadOnly || corpus.CopiedBytes != 0 || corpus.UploadedBytes != 0 || corpus.SelectedSamples == nil || len(corpus.SelectedSamples) != len(authority.ExpectedSamples) {
		return validationError(CodeProbeInvalidReport, "corpus", "read-only 370-pair corpus with nine selected samples and zero copied/uploaded bytes", "corpus accounting mismatch", nil)
	}
	for index, expected := range authority.ExpectedSamples {
		if err := validateCorpusSampleReportV2(corpus.SelectedSamples[index], expected, authority.RepositoryRoot, authority.Commit); err != nil {
			return err
		}
	}
	return nil
}

func validateCorpusSampleReportV2(sample CorpusSampleReportV2, expected corpusv2.CorpusV2SampleExpectation, root, commit string) error {
	field := fmt.Sprintf("corpus.selectedSamples.%s.%s", expected.Study, expected.Band)
	if sample.Study != expected.Study || sample.Band != expected.Band || sample.Attempt != expected.Attempt || sample.SourceCommit != commit {
		return validationError(CodeProbeInvalidReport, field, "exact pinned sample order and source commit", "selection mismatch", nil)
	}
	clipPath := filepath.Join(root, "production", expected.Study, "attempts", expected.Attempt, "clip.mp4")
	promptPath := filepath.Join(root, "production", expected.Study, "attempts", expected.Attempt, "prompt.md")
	if !expectedRecordedIdentity(sample.Clip, clipPath, expected.ClipBytes, expected.ClipSHA256) || !expectedRecordedIdentity(sample.Prompt, promptPath, expected.PromptBytes, expected.PromptSHA256) {
		return validationError(CodeProbeInvalidReport, field+".identity", "pinned redacted clip and prompt identities", "identity mismatch", nil)
	}
	want := CorpusStreamReportV2{Codec: expected.Codec, Width: expected.Width, Height: expected.Height, FrameRate: expected.FrameRate, DurationMillis: expected.DurationMillis, Frames: expected.Frames}
	if sample.Stream != want {
		return validationError(CodeProbeInvalidReport, field+".stream", "pinned stream metadata", fmt.Sprintf("%+v", sample.Stream), nil)
	}
	return nil
}

func expectedRecordedIdentity(identity RecordedIdentity, path string, bytes int64, digest string) bool {
	return identity.Identity == fmt.Sprintf("file:%d:%s", bytes, digest) && identity.PathIdentity == pathIdentity(path) && identity.Bytes == bytes && strings.EqualFold(identity.SHA256, digest)
}

func validateProbePolicyV2(policy ProbePolicyV2) error {
	if len(policy.RootIdentities) < 6 || !uniqueStrings(policy.RootIdentities) {
		return validationError(CodeProbeInvalidReport, "policy.rootIdentities", "at least six unique redacted root identities", fmt.Sprint(policy.RootIdentities), nil)
	}
	if !validProbeV2Platform(policy.Platform) || policy.Port <= 0 || policy.Port > 65535 || policy.Port == ProbeForbiddenPort || policy.PerCallTimeoutSeconds != ProbeV2PerCallTimeoutSeconds || policy.NetworkPolicy != ProbeNetworkPolicy || policy.DownloadBytes != 0 || policy.PaidUSD != 0 || policy.MaxHeavyProcesses != ProbeV2MaxHeavyProcesses || policy.MaxCalls != ProbeV2MaxCalls || policy.MaxRetries != ProbeV2MaxRetries {
		return validationError(CodeProbeInvalidReport, "policy", "bounded no-download/no-retry policy on a dynamic non-7437 port", "policy mismatch", nil)
	}
	return nil
}

func validProbeV2Platform(platform string) bool {
	parts := strings.Split(platform, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, part := range parts {
		for _, character := range part {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
				return false
			}
		}
	}
	return true
}

func validateProbeReportV2Redaction(report ProbeReportV2) error {
	values := []string{report.Build.Identity, report.Dependencies.Model.Identity, report.Dependencies.Projector.Identity, report.Dependencies.Backend.Identity, report.Corpus.RepositoryIdentity, report.Corpus.IndexPathIdentity}
	values = append(values, report.Policy.RootIdentities...)
	for _, sample := range report.Corpus.SelectedSamples {
		values = append(values, sample.Study, sample.Band, sample.Attempt, sample.SourceCommit, sample.Clip.Identity, sample.Clip.PathIdentity, sample.Prompt.Identity, sample.Prompt.PathIdentity)
	}
	for _, call := range report.Calls {
		values = append(values, call.Kind)
		values = append(values, call.Command...)
		if call.SampleIdentity != nil {
			values = append(values, *call.SampleIdentity)
		}
		for _, input := range call.Inputs {
			values = append(values, input.Identity, input.PathIdentity)
		}
		if call.Output != nil {
			values = append(values, call.Output.Identity, call.Output.PathIdentity)
		}
		if call.Failure != nil {
			values = append(values, call.Failure.Owner, call.Failure.Code, call.Failure.Expected, call.Failure.Observed, call.Failure.NextAction)
		}
	}
	for _, output := range report.Outputs {
		values = append(values, output.Identity, output.PathIdentity)
	}
	for _, process := range report.Processes {
		values = append(values, process.Identity, process.Kind, process.Owner)
	}
	if report.Failure != nil {
		values = append(values, report.Failure.Owner, report.Failure.Code, report.Failure.Expected, report.Failure.Observed, report.Failure.NextAction)
	}
	for _, value := range values {
		if reportStringContainsAbsolutePath(value) {
			return validationError(CodeProbeInvalidReport, "report", "path-redacted evidence", "absolute path found", nil)
		}
	}
	return nil
}

func newProbeReportV2(prepared admittedProbeInputV2, port int) ProbeReportV2 {
	return ProbeReportV2{
		SchemaVersion: ProbeReportSchemaV2,
		RunID:         prepared.input.RunID,
		Mode:          "PREFLIGHT",
		Status:        "READY",
		Build:         prepared.build,
		Dependencies:  prepared.dependencies,
		Corpus:        corpusReportV2(prepared.manifest),
		Policy: ProbePolicyV2{
			RootIdentities:        probeV2RootIdentities(prepared.input.ProbeRoot),
			Platform:              runtime.GOOS + "/" + runtime.GOARCH,
			Port:                  port,
			PerCallTimeoutSeconds: prepared.input.Limits.PerCallTimeoutSeconds,
			NetworkPolicy:         prepared.input.Limits.NetworkPolicy,
			DownloadBytes:         prepared.input.Limits.MaxDownloadBytes,
			PaidUSD:               prepared.input.Limits.MaxPaidUSD,
			MaxHeavyProcesses:     prepared.input.Limits.MaxHeavyProcesses,
			MaxCalls:              prepared.input.Limits.MaxCalls,
			MaxRetries:            prepared.input.Limits.MaxRetries,
		},
		Calls: []ProbeCallReportV2{}, Outputs: []RecordedIdentity{}, Processes: []ProcessEvidence{}, Cleanup: CleanupEvidence{Checked: true},
	}
}

func corpusReportV2(manifest corpusv2.CorpusV2Manifest) CorpusReportV2 {
	indexPath := filepath.Join(manifest.Repository, filepath.FromSlash(manifest.IndexPath))
	samples := make([]CorpusSampleReportV2, 0, len(manifest.Samples))
	for _, sample := range manifest.Samples {
		samples = append(samples, CorpusSampleReportV2{
			Study: sample.Study, Band: sample.Band, Attempt: sample.Attempt,
			Clip: recordedCorpusV2File(sample.Clip), Prompt: recordedCorpusV2File(sample.Prompt),
			Stream:       CorpusStreamReportV2{Codec: sample.Stream.Codec, Width: sample.Stream.Width, Height: sample.Stream.Height, FrameRate: sample.Stream.FrameRate, DurationMillis: sample.Stream.DurationMillis, Frames: sample.Stream.Frames},
			SourceCommit: sample.SourceCommit,
		})
	}
	return CorpusReportV2{RepositoryIdentity: pathIdentity(manifest.Repository), Commit: manifest.Commit, IndexPathIdentity: pathIdentity(indexPath), IndexSHA256: manifest.IndexSHA256, UniqueClips: manifest.UniqueClips, UniquePrompts: manifest.UniquePrompts, SelectedSamples: samples, CopiedBytes: manifest.CopiedBytes, UploadedBytes: manifest.UploadedBytes, ReadOnly: manifest.ReadOnly}
}

func recordedCorpusV2File(identity corpusv2.CorpusV2FileIdentity) RecordedIdentity {
	return RecordedIdentity{Identity: identity.Identity, PathIdentity: pathIdentity(identity.Path), Bytes: identity.Bytes, SHA256: identity.SHA256}
}

func writeProbeJSONNoReplaceAtomic(path string, body []byte) error {
	path, err := normalizeProbePath(path, "atomic JSON path")
	if err != nil {
		return err
	}
	if err := rejectProbeSymlinkComponents(path); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return errProbeV2ReportExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".omni-video-runner-v2-*.tmp")
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
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errProbeV2ReportExists
		}
		return err
	}
	if err := os.Remove(temporaryPath); err != nil {
		_ = os.Remove(path)
		return err
	}
	removeTemporary = false
	return nil
}

func probeV2RootIdentities(root string) []string {
	paths := RootPaths{
		Work: filepath.Join(root, "work"), Profile: filepath.Join(root, "profile"), Cache: filepath.Join(root, "cache"),
		Model: filepath.Join(root, "model"), Projector: filepath.Join(root, "projector"), Backend: filepath.Join(root, "backend"),
		Output: filepath.Join(root, "output"), Streams: filepath.Join(root, "streams"),
	}
	return rootIdentities(probeRoots{Root: root, Paths: paths})
}
