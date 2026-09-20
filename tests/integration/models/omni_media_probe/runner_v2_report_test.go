package omni_media_probe

import (
	"encoding/json"
	"fmt"
	"path/filepath"
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
	return writeProbeJSONAtomic(path, append(body, '\n'))
}

func (report ProbeReportV2) Validate() error {
	if report.SchemaVersion != ProbeReportSchemaV2 || report.Mode != "PREFLIGHT" || report.Status != "READY" {
		return validationError(CodeProbeInvalidReport, "report", "v2 PREFLIGHT READY evidence", "schema, mode, or status mismatch", nil)
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
	if len(report.Calls) != 0 || len(report.Outputs) != 0 || len(report.Processes) != 0 || report.Failure != nil {
		return validationError(CodeProbeInvalidReport, "mode", "preflight without calls, outputs, processes, or failure", "unexpected execution evidence", nil)
	}
	if !report.Cleanup.Checked || report.Cleanup.OwnedProcessSurvivors != 0 || report.Cleanup.OwnedListenerSurvivors != 0 || report.Cleanup.PartialOutputs != 0 {
		return validationError(CodeProbeInvalidReport, "cleanup", "checked cleanup with zero survivors and outputs", fmt.Sprintf("%+v", report.Cleanup), nil)
	}
	return validateProbeReportV2Redaction(report)
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
	if policy.Port <= 0 || policy.Port > 65535 || policy.Port == ProbeForbiddenPort || policy.PerCallTimeoutSeconds != ProbeV2PerCallTimeoutSeconds || policy.NetworkPolicy != ProbeNetworkPolicy || policy.DownloadBytes != 0 || policy.PaidUSD != 0 || policy.MaxHeavyProcesses != ProbeV2MaxHeavyProcesses || policy.MaxCalls != ProbeV2MaxCalls || policy.MaxRetries != ProbeV2MaxRetries {
		return validationError(CodeProbeInvalidReport, "policy", "bounded no-download/no-retry policy on a dynamic non-7437 port", "policy mismatch", nil)
	}
	return nil
}

func validateProbeReportV2Redaction(report ProbeReportV2) error {
	values := []string{report.Build.Identity, report.Dependencies.Model.Identity, report.Dependencies.Projector.Identity, report.Dependencies.Backend.Identity, report.Corpus.RepositoryIdentity, report.Corpus.IndexPathIdentity}
	values = append(values, report.Policy.RootIdentities...)
	for _, sample := range report.Corpus.SelectedSamples {
		values = append(values, sample.Study, sample.Band, sample.Attempt, sample.SourceCommit, sample.Clip.Identity, sample.Clip.PathIdentity, sample.Prompt.Identity, sample.Prompt.PathIdentity)
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

func probeV2RootIdentities(root string) []string {
	paths := RootPaths{
		Work: filepath.Join(root, "work"), Profile: filepath.Join(root, "profile"), Cache: filepath.Join(root, "cache"),
		Model: filepath.Join(root, "model"), Projector: filepath.Join(root, "projector"), Backend: filepath.Join(root, "backend"),
		Output: filepath.Join(root, "output"), Streams: filepath.Join(root, "streams"),
	}
	return rootIdentities(probeRoots{Root: root, Paths: paths})
}
