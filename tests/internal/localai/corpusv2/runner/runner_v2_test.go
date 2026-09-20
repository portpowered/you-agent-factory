package runner

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
)

func TestCorpusRunnerV2PreflightRecordsPortableSyntheticCorpusWithoutExecutorCalls(t *testing.T) {
	authority, manifest := portableCorpusV2Fixture(t)
	input, inputPath, reportPath := validProbeInputV2(t, "corpus-ready")
	executor := &probeV2ScriptedExecutor{}
	report, err := newPortableRunnerV2(t, executor).Preflight(context.Background(), inputPath, reportPath)
	if err != nil {
		t.Fatalf("run corpus v2 preflight: %v", err)
	}
	if report.Mode != "PREFLIGHT" || report.Status != "READY" || report.Failure != nil {
		t.Fatalf("preflight mode/status/failure = %s/%s/%#v", report.Mode, report.Status, report.Failure)
	}
	if report.Corpus.Commit != authority.Commit || report.Corpus.IndexSHA256 != authority.IndexSHA256 || report.Corpus.UniqueClips != authority.PairCount || report.Corpus.UniquePrompts != authority.PairCount {
		t.Fatalf("corpus authority = %#v, want portable %d-pair corpus", report.Corpus, authority.PairCount)
	}
	assertPortableCorpusV2ReportSamples(t, report.Corpus.SelectedSamples, manifest)
	if len(report.Calls) != 0 || len(report.Outputs) != 0 || len(report.Processes) != 0 || report.Cleanup != (CleanupEvidence{Checked: true}) {
		t.Fatalf("preflight effects = calls=%d outputs=%d processes=%d cleanup=%#v", len(report.Calls), len(report.Outputs), len(report.Processes), report.Cleanup)
	}
	if report.Policy.Port == ProbeForbiddenPort || report.Policy.PerCallTimeoutSeconds != 180 || report.Policy.MaxCalls != 10 || report.Policy.MaxRetries != 0 || len(report.Policy.RootIdentities) != 9 {
		t.Fatalf("preflight policy = %#v, want dynamic port, bounded call policy, and nine isolated roots", report.Policy)
	}
	if _, err := os.Stat(input.ProbeRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight created probe root: %v", err)
	}
	persisted, err := readProbeReportV2(reportPath, authority)
	if err != nil || persisted.Status != "READY" {
		t.Fatalf("persisted v2 report = status %q, err=%v", persisted.Status, err)
	}
	body, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read persisted report: %v", err)
	}
	for _, leaked := range []string{input.Build.Path, input.Dependencies.Model.Path, input.Dependencies.Projector.Path, input.Dependencies.Backend.Path, input.CorpusInput.Path, input.ProbeRoot, authority.RepositoryRoot} {
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
			executor := &probeV2ScriptedExecutor{}
			readerCalls := 0
			runner := newPortableRunnerV2(t, executor)
			runner.corpusReader = func(context.Context, CorpusV2Authority) (CorpusV2Manifest, error) {
				readerCalls++
				return CorpusV2Manifest{}, nil
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
	authority, base := portableCorpusV2Fixture(t)
	cases := []struct {
		name     string
		wantCode CorpusV2ValidationCode
		mutate   func(*CorpusV2Manifest)
	}{
		{name: "source commit drift", wantCode: CorpusV2CodeSourceCommitMismatch, mutate: func(m *CorpusV2Manifest) { m.Commit = strings.Repeat("0", 40) }},
		{name: "index path drift", wantCode: CorpusV2CodePathIdentity, mutate: func(m *CorpusV2Manifest) { m.IndexPath = "docs/changed-index.md" }},
		{name: "index digest drift", wantCode: CorpusV2CodeIndexHashMismatch, mutate: func(m *CorpusV2Manifest) { m.IndexSHA256 = strings.Repeat("0", sha256.Size*2) }},
		{name: "pair count drift", wantCode: CorpusV2CodeCountMismatch, mutate: func(m *CorpusV2Manifest) { m.UniqueClips-- }},
		{name: "sample path drift", wantCode: CorpusV2CodePathIdentity, mutate: func(m *CorpusV2Manifest) {
			m.Samples[0].Clip.Path = filepath.Join(t.TempDir(), "different.mp4")
		}},
		{name: "sample hash drift", wantCode: CorpusV2CodeHashMismatch, mutate: func(m *CorpusV2Manifest) { m.Samples[0].Clip.SHA256 = strings.Repeat("0", sha256.Size*2) }},
		{name: "sample selection drift", wantCode: CorpusV2CodeSelectionMismatch, mutate: func(m *CorpusV2Manifest) { m.Samples[0].Attempt = "D000-a0000" }},
		{name: "sample metadata drift", wantCode: CorpusV2CodeMetadataMismatch, mutate: func(m *CorpusV2Manifest) { m.Samples[0].Stream.Width++ }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, inputPath, reportPath := validProbeInputV2(t, testCase.name)
			manifest := cloneCorpusV2Manifest(base)
			testCase.mutate(&manifest)
			executor := &probeV2ScriptedExecutor{}
			readerCalls := 0
			runner := newPortableRunnerV2(t, executor)
			runner.authority = authority
			runner.corpusReader = func(context.Context, CorpusV2Authority) (CorpusV2Manifest, error) {
				readerCalls++
				return manifest, nil
			}
			_, err := runner.Preflight(context.Background(), inputPath, reportPath)
			var validation *CorpusV2ValidationError
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
	corpusAuthority, _ := portableCorpusV2Fixture(t)
	corpusInput := CorpusInputV2{
		SchemaVersion: CorpusV2SchemaVersion,
		RunID:         runID,
		Build:         buildFile,
		Dependencies:  dependencies,
		Corpus:        CorpusAuthorityV2{Repository: corpusAuthority.RepositoryRoot, Commit: corpusAuthority.Commit, IndexPath: corpusAuthority.IndexPath, IndexSHA256: corpusAuthority.IndexSHA256, Mode: corpusAuthority.Mode},
		SamplePolicy:  CorpusSamplePolicyV2{Studies: append([]string(nil), corpusAuthority.RequiredStudies...), RepresentativesPerStudy: 3, Ordering: CorpusV2Ordering},
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

func assertNoV2ExecutorOrReport(t *testing.T, executor *probeV2ScriptedExecutor, reportPath string) {
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

func cloneCorpusV2Manifest(manifest CorpusV2Manifest) CorpusV2Manifest {
	manifest.Pairs = append([]CorpusV2Pair(nil), manifest.Pairs...)
	manifest.Samples = append([]CorpusV2Sample(nil), manifest.Samples...)
	return manifest
}
