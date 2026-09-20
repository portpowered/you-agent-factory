package omni_media_probe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/tests/internal/localai/corpusv2"
)

func TestCorpusRunnerV2PreflightRecordsPinnedCorpusWithoutExecutorCalls(t *testing.T) {
	requireLocalCorpusV2(t)
	input, inputPath, reportPath := validProbeInputV2(t, "corpus-ready")
	executor := &recordingExecutor{}
	report, err := NewRunnerV2(executor).Preflight(context.Background(), inputPath, reportPath)
	if err != nil {
		t.Fatalf("run corpus v2 preflight: %v", err)
	}
	if report.Mode != "PREFLIGHT" || report.Status != "READY" || report.Failure != nil {
		t.Fatalf("preflight mode/status/failure = %s/%s/%#v", report.Mode, report.Status, report.Failure)
	}
	if report.Corpus.Commit != corpusv2.CorpusV2Commit || report.Corpus.IndexSHA256 != corpusv2.CorpusV2IndexSHA256 || report.Corpus.UniqueClips != 370 || report.Corpus.UniquePrompts != 370 {
		t.Fatalf("corpus authority = %#v, want pinned 370-pair corpus", report.Corpus)
	}
	assertCorpusV2ReportSamples(t, report.Corpus.SelectedSamples)
	if len(report.Calls) != 0 || len(report.Outputs) != 0 || len(report.Processes) != 0 || report.Cleanup != (CleanupEvidence{Checked: true}) {
		t.Fatalf("preflight effects = calls=%d outputs=%d processes=%d cleanup=%#v", len(report.Calls), len(report.Outputs), len(report.Processes), report.Cleanup)
	}
	if report.Policy.Port == ProbeForbiddenPort || report.Policy.PerCallTimeoutSeconds != 180 || report.Policy.MaxCalls != 10 || report.Policy.MaxRetries != 0 || len(report.Policy.RootIdentities) != 9 {
		t.Fatalf("preflight policy = %#v, want dynamic port, bounded call policy, and nine isolated roots", report.Policy)
	}
	if _, err := os.Stat(input.ProbeRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight created probe root: %v", err)
	}
	persisted, err := ReadProbeReportV2(reportPath)
	if err != nil || persisted.Status != "READY" {
		t.Fatalf("persisted v2 report = status %q, err=%v", persisted.Status, err)
	}
	body, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read persisted report: %v", err)
	}
	for _, leaked := range []string{input.Build.Path, input.Dependencies.Model.Path, input.Dependencies.Projector.Path, input.Dependencies.Backend.Path, input.CorpusInput.Path, input.ProbeRoot, corpusv2.CorpusV2Repository} {
		if strings.Contains(string(body), leaked) {
			t.Fatalf("report leaked source path %q", leaked)
		}
	}
	executor.mu.Lock()
	requests := append([]ExecutionRequest(nil), executor.requests...)
	executor.mu.Unlock()
	if len(requests) != 0 {
		t.Fatalf("preflight made %d executor calls, want none", len(requests))
	}
}

func TestCorpusRunnerV2RejectsInputDriftBeforeExecutorOrReport(t *testing.T) {
	cases := []struct {
		name     string
		wantCode ValidationCode
		mutate   func(*testing.T, *ProbeInputV2, string)
	}{
		{
			name: "authority commit drift", wantCode: CodeProbeCorpusAuthority,
			mutate: func(t *testing.T, input *ProbeInputV2, inputPath string) {
				rewriteCorpusInputV2(t, input, inputPath, func(document *CorpusInputV2) { document.Corpus.Commit = strings.Repeat("0", 40) }, true)
			},
		},
		{
			name: "sample policy drift", wantCode: CodeProbeCorpusPolicy,
			mutate: func(t *testing.T, input *ProbeInputV2, inputPath string) {
				rewriteCorpusInputV2(t, input, inputPath, func(document *CorpusInputV2) { document.SamplePolicy.Ordering = "attempt" }, true)
			},
		},
		{
			name: "corpus input path missing", wantCode: CodeProbeMissingIdentity,
			mutate: func(t *testing.T, input *ProbeInputV2, inputPath string) {
				input.CorpusInput.Path = filepath.Join(filepath.Dir(input.CorpusInput.Path), "missing-corpus-input.json")
				writeRawProbeInputV2(t, inputPath, *input)
			},
		},
		{
			name: "corpus input digest drift", wantCode: CodeProbeIdentityMismatch,
			mutate: func(t *testing.T, input *ProbeInputV2, inputPath string) {
				input.CorpusInput.SHA256 = strings.Repeat("0", sha256.Size*2)
				writeRawProbeInputV2(t, inputPath, *input)
			},
		},
		{
			name: "build digest drift", wantCode: CodeProbeIdentityMismatch,
			mutate: func(t *testing.T, input *ProbeInputV2, inputPath string) {
				input.Build.SHA256 = strings.Repeat("0", sha256.Size*2)
				writeRawProbeInputV2(t, inputPath, *input)
			},
		},
		{
			name: "call budget drift", wantCode: CodeProbeInvalidInput,
			mutate: func(t *testing.T, input *ProbeInputV2, inputPath string) {
				input.Limits.MaxCalls = 9
				writeRawProbeInputV2(t, inputPath, *input)
			},
		},
		{
			name: "existing root", wantCode: CodeProbeRootNotFresh,
			mutate: func(t *testing.T, input *ProbeInputV2, inputPath string) {
				if err := os.Mkdir(input.ProbeRoot, 0o700); err != nil {
					t.Fatalf("create pre-existing root: %v", err)
				}
				writeRawProbeInputV2(t, inputPath, *input)
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			input, inputPath, reportPath := validProbeInputV2(t, testCase.name)
			testCase.mutate(t, &input, inputPath)
			executor := &recordingExecutor{}
			readerCalls := 0
			runner := NewRunnerV2(executor)
			runner.corpusReader = func(context.Context, corpusv2.CorpusV2Authority) (corpusv2.CorpusV2Manifest, error) {
				readerCalls++
				return corpusv2.CorpusV2Manifest{}, nil
			}
			_, err := runner.Preflight(context.Background(), inputPath, reportPath)
			if !hasValidationCode(err, testCase.wantCode) {
				t.Fatalf("preflight error = %v, want %q", err, testCase.wantCode)
			}
			assertNoV2ExecutorOrReport(t, executor, reportPath)
			if readerCalls != 0 {
				t.Fatalf("invalid input reached corpus reader %d times", readerCalls)
			}
		})
	}
}

func TestCorpusRunnerV2RejectsManifestDriftBeforeExecutorOrReport(t *testing.T) {
	requireLocalCorpusV2(t)
	base, err := corpusv2.ReadCorpusV2Manifest(context.Background(), corpusv2.DefaultCorpusV2Authority())
	if err != nil {
		t.Fatalf("read pinned corpus for controlled drift cases: %v", err)
	}
	cases := []struct {
		name     string
		wantCode corpusv2.CorpusV2ValidationCode
		mutate   func(*corpusv2.CorpusV2Manifest)
	}{
		{name: "source commit drift", wantCode: corpusv2.CorpusV2CodeSourceCommitMismatch, mutate: func(m *corpusv2.CorpusV2Manifest) { m.Commit = strings.Repeat("0", 40) }},
		{name: "index path drift", wantCode: corpusv2.CorpusV2CodePathIdentity, mutate: func(m *corpusv2.CorpusV2Manifest) { m.IndexPath = "docs/changed-index.md" }},
		{name: "index digest drift", wantCode: corpusv2.CorpusV2CodeIndexHashMismatch, mutate: func(m *corpusv2.CorpusV2Manifest) { m.IndexSHA256 = strings.Repeat("0", sha256.Size*2) }},
		{name: "pair count drift", wantCode: corpusv2.CorpusV2CodeCountMismatch, mutate: func(m *corpusv2.CorpusV2Manifest) { m.UniqueClips-- }},
		{name: "sample path drift", wantCode: corpusv2.CorpusV2CodePathIdentity, mutate: func(m *corpusv2.CorpusV2Manifest) {
			m.Samples[0].Clip.Path = filepath.Join(t.TempDir(), "different.mp4")
		}},
		{name: "sample hash drift", wantCode: corpusv2.CorpusV2CodeHashMismatch, mutate: func(m *corpusv2.CorpusV2Manifest) { m.Samples[0].Clip.SHA256 = strings.Repeat("0", sha256.Size*2) }},
		{name: "sample selection drift", wantCode: corpusv2.CorpusV2CodeSelectionMismatch, mutate: func(m *corpusv2.CorpusV2Manifest) { m.Samples[0].Attempt = "D000-a0000" }},
		{name: "sample metadata drift", wantCode: corpusv2.CorpusV2CodeMetadataMismatch, mutate: func(m *corpusv2.CorpusV2Manifest) { m.Samples[0].Stream.Width++ }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, inputPath, reportPath := validProbeInputV2(t, testCase.name)
			manifest := cloneCorpusV2Manifest(base)
			testCase.mutate(&manifest)
			executor := &recordingExecutor{}
			readerCalls := 0
			runner := NewRunnerV2(executor)
			runner.corpusReader = func(context.Context, corpusv2.CorpusV2Authority) (corpusv2.CorpusV2Manifest, error) {
				readerCalls++
				return manifest, nil
			}
			_, err := runner.Preflight(context.Background(), inputPath, reportPath)
			var validation *corpusv2.CorpusV2ValidationError
			if err == nil || !errors.As(err, &validation) || validation.Code != testCase.wantCode {
				t.Fatalf("preflight error = %v, want corpus validation code %q", err, testCase.wantCode)
			}
			assertNoV2ExecutorOrReport(t, executor, reportPath)
			if readerCalls != 1 {
				t.Fatalf("manifest drift reader calls = %d, want one", readerCalls)
			}
		})
	}
}

func TestProbeInputV2RejectsUnknownAndDuplicateJSONKeys(t *testing.T) {
	_, inputPath, _ := validProbeInputV2(t, "strict-v2")
	body, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read strict v2 input: %v", err)
	}
	unknown := bytes.Replace(body, []byte("\"limits\":"), []byte("\"unexpected\":true,\"limits\":"), 1)
	if err := os.WriteFile(inputPath, unknown, 0o600); err != nil {
		t.Fatalf("write unknown-field v2 input: %v", err)
	}
	if _, err := ReadProbeInputV2(inputPath); !hasValidationCode(err, CodeUnknownField) {
		t.Fatalf("unknown-field input error = %v, want %q", err, CodeUnknownField)
	}
	duplicate := bytes.Replace(body, []byte("\"runId\":"), []byte("\"runId\":\"strict-v2\",\"runId\":"), 1)
	if err := os.WriteFile(inputPath, duplicate, 0o600); err != nil {
		t.Fatalf("write duplicate-key v2 input: %v", err)
	}
	if _, err := ReadProbeInputV2(inputPath); !hasValidationCode(err, CodeDuplicateJSONKey) {
		t.Fatalf("duplicate-key input error = %v, want %q", err, CodeDuplicateJSONKey)
	}
}

func validProbeInputV2(t *testing.T, runID string) (ProbeInputV2, string, string) {
	t.Helper()
	root := t.TempDir()
	buildFile := writeProbeV2File(t, root, "you.exe", "test-build", []byte("controlled candidate placeholder"), 0o700)
	model := writeProbeV2File(t, root, "model.gguf", "test-model", []byte("controlled model identity"), 0o600)
	projector := writeProbeV2File(t, root, "projector.gguf", "test-projector", []byte("controlled projector identity"), 0o600)
	backend := writeProbeV2File(t, root, "backend", "test-backend", []byte("controlled backend identity"), 0o600)
	dependencies := ProbeDependencies{Model: model, Projector: projector, Backend: backend}
	inputPath := filepath.Join(root, "runner-input.json")
	corpusInputPath := filepath.Join(root, "corpus-input.json")
	corpusAuthority := corpusv2.DefaultCorpusV2Authority()
	corpusInput := CorpusInputV2{
		SchemaVersion: corpusv2.CorpusV2SchemaVersion,
		RunID:         runID,
		Build:         buildFile,
		Dependencies:  dependencies,
		Corpus:        CorpusAuthorityV2{Repository: corpusAuthority.RepositoryRoot, Commit: corpusAuthority.Commit, IndexPath: corpusAuthority.IndexPath, IndexSHA256: corpusAuthority.IndexSHA256, Mode: corpusAuthority.Mode},
		SamplePolicy:  CorpusSamplePolicyV2{Studies: append([]string(nil), corpusAuthority.RequiredStudies...), RepresentativesPerStudy: 3, Ordering: corpusv2.CorpusV2Ordering},
		Limits:        CorpusInputLimitsV2{PerInvocationTimeoutSeconds: 180, Retries: 0, MaxHeavyProcesses: 1, MaxDiskBytes: 1 << 30, MaxDownloadBytes: 0, MaxPaidUSD: 0, ForbiddenPort: ProbeForbiddenPort, NetworkPolicy: ProbeNetworkPolicy},
	}
	writeCorpusInputDocumentV2(t, corpusInputPath, corpusInput)
	input := ProbeInputV2{
		SchemaVersion: ProbeInputSchemaV2,
		RunID:         runID,
		Build:         ProbeBuildIdentity{Path: buildFile.Path, Identity: buildFile.Identity, SHA256: buildFile.SHA256},
		Dependencies:  dependencies,
		CorpusInput:   probeV2FileIdentity(t, corpusInputPath, "test-corpus-input"),
		ProbeRoot:     filepath.Join(root, "probe-root"),
		Limits:        ProbeLimitsV2{PerCallTimeoutSeconds: 180, MaxHeavyProcesses: 1, MaxCompilerTestProcesses: 4, MaxDiskBytes: 3 << 30, MaxDownloadBytes: 0, MaxPaidUSD: 0, MaxCalls: 10, MaxRetries: 0, ForbiddenPort: ProbeForbiddenPort, NetworkPolicy: ProbeNetworkPolicy},
	}
	if err := WriteProbeInputV2Atomic(inputPath, input); err != nil {
		t.Fatalf("write v2 probe input: %v", err)
	}
	return input, inputPath, filepath.Join(root, "runner-report.json")
}

func writeProbeV2File(t *testing.T, root, name, identity string, body []byte, mode os.FileMode) ProbeFileIdentity {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, body, mode); err != nil {
		t.Fatalf("write v2 identity file %s: %v", name, err)
	}
	return probeV2FileIdentity(t, path, identity)
}

func probeV2FileIdentity(t *testing.T, path, identity string) ProbeFileIdentity {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read v2 identity file: %v", err)
	}
	digest := sha256.Sum256(body)
	return ProbeFileIdentity{Path: path, Identity: identity, Bytes: int64(len(body)), SHA256: hex.EncodeToString(digest[:])}
}

func writeCorpusInputDocumentV2(t *testing.T, path string, document CorpusInputV2) {
	t.Helper()
	body, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("encode v2 corpus input: %v", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatalf("write v2 corpus input: %v", err)
	}
}

func rewriteCorpusInputV2(t *testing.T, input *ProbeInputV2, inputPath string, mutate func(*CorpusInputV2), refreshIdentity bool) {
	t.Helper()
	document, err := readCorpusInputV2(input.CorpusInput.Path)
	if err != nil {
		t.Fatalf("read v2 corpus input fixture: %v", err)
	}
	mutate(&document)
	writeCorpusInputDocumentV2(t, input.CorpusInput.Path, document)
	if refreshIdentity {
		input.CorpusInput = probeV2FileIdentity(t, input.CorpusInput.Path, input.CorpusInput.Identity)
	}
	writeRawProbeInputV2(t, inputPath, *input)
}

func writeRawProbeInputV2(t *testing.T, path string, input ProbeInputV2) {
	t.Helper()
	body, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		t.Fatalf("encode raw v2 probe input: %v", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatalf("write raw v2 probe input: %v", err)
	}
}

func assertNoV2ExecutorOrReport(t *testing.T, executor *recordingExecutor, reportPath string) {
	t.Helper()
	executor.mu.Lock()
	requestCount := len(executor.requests)
	executor.mu.Unlock()
	if requestCount != 0 {
		t.Fatalf("failed admission made %d executor calls, want none", requestCount)
	}
	if _, err := os.Stat(reportPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed admission published a report: %v", err)
	}
}

func cloneCorpusV2Manifest(manifest corpusv2.CorpusV2Manifest) corpusv2.CorpusV2Manifest {
	manifest.Pairs = append([]corpusv2.CorpusV2Pair(nil), manifest.Pairs...)
	manifest.Samples = append([]corpusv2.CorpusV2Sample(nil), manifest.Samples...)
	return manifest
}

func requireLocalCorpusV2(t *testing.T) {
	t.Helper()
	authority := corpusv2.DefaultCorpusV2Authority()
	indexPath := filepath.Join(authority.RepositoryRoot, filepath.FromSlash(authority.IndexPath))
	if _, err := os.Stat(indexPath); err != nil {
		t.Skipf("pinned external corpus unavailable at %s: %v", indexPath, err)
	}
}

func assertCorpusV2ReportSamples(t *testing.T, samples []CorpusSampleReportV2) {
	t.Helper()
	want := []struct {
		study, band, attempt   string
		clipBytes              int64
		clipSHA256             string
		promptBytes            int64
		promptSHA256           string
		codec                  string
		width, height          int
		frameRate              string
		durationMillis, frames int64
	}{
		{"selfie-jessie-duration-study", "minimum", "D007-a2001", 393020, "0adf4be0b74415f8aad7c59aa8c2ecb83b4e23abaa03a1c42effe8fd8ea9b27e", 1290, "e2c04f0ef9ae5a92208e73dd9b0ccb6f96ad00163d392c7df030fd19ca62bffe", "avc1", 480, 864, "24/1", 5167, 124},
		{"selfie-jessie-duration-study", "median", "D010-a2001", 455857, "196f96370be7bc0a5fb365ef86ad46244e2c46310f6b73d6c1ee9a67068d50c5", 1399, "f0b8aaf1efaa389d8efc8e1902e5037a649446450218fb9f128d5dcf8109e4ef", "avc1", 480, 864, "24/1", 5167, 124},
		{"selfie-jessie-duration-study", "maximum", "D040-a2001", 939599, "58cd074f1c963ea9ff7322620a2d178b4b580934c2ee6d23dd24e64e11bf52a3", 1481, "e8106ca828dedd0bf528687393c6c2abc31ccf16c7fa2cf54cc7a13f11ae6d02", "avc1", 480, 864, "24/1", 10125, 243},
		{"selfie-jessie-prompt-study", "minimum", "P004-a1007", 395702, "eacdb3f7564deeea1da693b8b219082d9ee4ac70067d565ef8901242b4ec5140", 1009, "8580aaf3c7ba60e44ebb59a3c35036828cc9cafd9ba20cf6ac98a2ad9f957cee", "avc1", 480, 864, "24/1", 5167, 124},
		{"selfie-jessie-prompt-study", "median", "P090-a1008", 451560, "0c90abe1cedb117772c0a4b47bf49b1a252bf78de5921082198583b5843e5233", 2228, "a04b9875576c47b242fe41916145d42cbadc2b4aae612ddbd490872fd39101be", "avc1", 480, 864, "24/1", 5167, 124},
		{"selfie-jessie-prompt-study", "maximum", "P099-a1003", 483760, "f7567326e76bdf7cbdad5652b39cae1000d904be2a3d6274d371832e8883ca98", 4410, "10a4937d251595c8744223a83f1f52f26bd38318911e2a0150acd73d5d0a713a", "avc1", 480, 864, "24/1", 5167, 124},
		{"selfie-jessie-quality", "minimum", "Q08-a18", 372001, "89e2df6b356cf26e305b860bab711b29d6ca6d3c897c17ffee2fcc832608e871", 5972, "f6d35918cd69d1047ab9012a47ad7b5e2480005f2e0ee8191cb6e5481eded2f7", "avc1", 480, 864, "25/1", 2640, 66},
		{"selfie-jessie-quality", "median", "Q02-a1", 470854, "3361b4b3bb818d7678569ab6786f32e0a2087719f506b32a6c6287cad5012790", 3771, "bd55340b2a7b45bedf3a653357d31e4dc6f7efaf0666443caa8f495c0228cbed", "avc1", 480, 864, "24/1", 5167, 124},
		{"selfie-jessie-quality", "maximum", "Q02-a18", 599629, "eb06cf35dc6e0db810d2b63766cc82d41682394cf94d60415c87e228977b2fb7", 5989, "a25019a0080cb560c1cbdb258aca9fe6313a00ab4b7fd09078e7b7c5cef67f38", "avc1", 480, 864, "24/1", 5167, 124},
	}
	if len(samples) != len(want) {
		t.Fatalf("selected samples = %d, want %d", len(samples), len(want))
	}
	for index, expected := range want {
		got := samples[index]
		if got.Study != expected.study || got.Band != expected.band || got.Attempt != expected.attempt || got.SourceCommit != corpusv2.CorpusV2Commit || got.Clip.Bytes != expected.clipBytes || got.Clip.SHA256 != expected.clipSHA256 || got.Prompt.Bytes != expected.promptBytes || got.Prompt.SHA256 != expected.promptSHA256 || got.Stream.Codec != expected.codec || got.Stream.Width != expected.width || got.Stream.Height != expected.height || got.Stream.FrameRate != expected.frameRate || got.Stream.DurationMillis != expected.durationMillis || got.Stream.Frames != expected.frames {
			t.Errorf("selected sample[%d] = %#v, want identity for %s/%s/%s", index, got, expected.study, expected.band, expected.attempt)
		}
	}
}
