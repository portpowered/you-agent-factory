package tts_clean_install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestU01RejectsUntrustedInputsBeforeEffects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*testing.T, *fixture)
	}{
		{name: "relative artifact", mutate: func(_ *testing.T, f *fixture) { f.invocation.ArtifactPath = "artifact.bin" }},
		{name: "missing artifact", mutate: func(t *testing.T, f *fixture) { mustRemove(t, f.manifest.Build.ArtifactPath) }},
		{name: "duplicate input", mutate: func(t *testing.T, f *fixture) {
			f.manifest.Distribution.Archive.Path = f.manifest.Distribution.Installer.Path
			f.manifest.Distribution.Archive.SHA256 = f.manifest.Distribution.Installer.SHA256
			f.writeManifest(t)
		}},
		{name: "identity mismatch", mutate: func(_ *testing.T, f *fixture) { f.invocation.ArtifactIdentity = "wrong-build" }},
		{name: "hash mismatch", mutate: func(_ *testing.T, f *fixture) { f.invocation.ArtifactSHA256 = strings.Repeat("0", 64) }},
		{name: "invalid UTF-8", mutate: func(t *testing.T, f *fixture) {
			if err := os.WriteFile(f.manifest.Fixtures.Text.Path, []byte{0xff, 0xfe}, 0o600); err != nil {
				t.Fatal(err)
			}
			f.manifest.Fixtures.Text.SHA256 = sha256Hex([]byte{0xff, 0xfe})
			f.writeManifest(t)
		}},
		{name: "non-empty output root", mutate: func(t *testing.T, f *fixture) {
			if err := os.WriteFile(filepath.Join(f.outputRoot, "sentinel"), []byte("owned by another task"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "invalid limits", mutate: func(t *testing.T, f *fixture) {
			f.manifest.Limits.MaxOwnedProcesses = 5
			f.writeManifest(t)
		}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			testCase.mutate(t, f)
			result, err := Preflight(f.invocation)
			if err == nil {
				t.Fatal("Preflight succeeded for an untrusted input")
			}
			var preflightErr *PreflightError
			if !errors.As(err, &preflightErr) {
				t.Fatalf("error = %T %v, want PreflightError", err, err)
			}
			if result.Report.Verdict != "FAIL" {
				t.Fatalf("report verdict = %q, want FAIL", result.Report.Verdict)
			}
			if got := result.Report.Preflight.ChildStarts; got != 0 {
				t.Fatalf("child starts = %d, want zero", got)
			}
			if got := result.Report.Preflight.ListenerOpens; got != 0 {
				t.Fatalf("listener opens = %d, want zero", got)
			}
			if got := result.Report.Preflight.NetworkAttempts; got != 0 {
				t.Fatalf("network attempts = %d, want zero", got)
			}
			if !result.Report.Preflight.CompletedBeforeEffects {
				t.Fatal("preflight did not record completion before effects")
			}
			if err := ValidateReport(result.Report); err != nil {
				t.Fatalf("zero-effect failure report is invalid: %v", err)
			}
		})
	}
}

func TestU02MatchingInputsProduceSealedPlan(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	result, err := Preflight(f.invocation)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if result.Report.Verdict != "INCONCLUSIVE" {
		t.Fatalf("report verdict = %q, want INCONCLUSIVE until journey execution exists", result.Report.Verdict)
	}
	if result.Plan.ManifestHash != sha256Hex(f.manifestBody) {
		t.Fatalf("manifest hash = %q, want hash of exact manifest bytes", result.Plan.ManifestHash)
	}
	if len(result.Plan.Identities) != 9 {
		t.Fatalf("identity count = %d, want manifest plus eight immutable inputs", len(result.Plan.Identities))
	}
	if result.Plan.Isolation.OutputRoot != f.outputRoot {
		t.Fatalf("output root = %q, want %q", result.Plan.Isolation.OutputRoot, f.outputRoot)
	}
	for name, path := range map[string]string{
		"work": result.Plan.Isolation.WorkRoot, "profile": result.Plan.Isolation.ProfileRoot,
		"state": result.Plan.Isolation.StateRoot, "cache": result.Plan.Isolation.CacheRoot,
		"temp": result.Plan.Isolation.TempRoot, "streams": result.Plan.Isolation.StreamsRoot,
		"runtime": result.Plan.Isolation.RuntimeRoot,
	} {
		if !pathWithin(f.outputRoot, path) {
			t.Fatalf("%s root %q escaped output root %q", name, path, f.outputRoot)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s root was touched during preflight: stat err=%v", name, err)
		}
	}
	if result.Plan.Isolation.Environment["HOME"] != result.Plan.Isolation.ProfileRoot || result.Plan.Isolation.Environment["TEMP"] != result.Plan.Isolation.TempRoot {
		t.Fatalf("child environment is not rooted in sealed plan: %#v", result.Plan.Isolation.Environment)
	}
	if result.Report.Preflight.ChildStarts != 0 || result.Report.Preflight.ListenerOpens != 0 || result.Report.Preflight.NetworkAttempts != 0 {
		t.Fatalf("matching preflight crossed an effect boundary: %#v", result.Report.Preflight)
	}
	if err := ValidateReport(result.Report); err != nil {
		t.Fatalf("sealed-plan report is invalid: %v", err)
	}
	if got := result.Report.Criteria[0].Verdict; got != "PASS" {
		t.Fatalf("TTS-PROBE-01 = %q, want PASS", got)
	}
}

func TestU03RejectsMalformedOrUnknownManifestBeforeEffects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body func(*testing.T, *fixture) []byte
	}{
		{name: "unknown field", body: func(t *testing.T, f *fixture) []byte {
			body, err := json.Marshal(struct {
				Manifest
				Unexpected string `json:"unexpected"`
			}{Manifest: f.manifest, Unexpected: "must reject"})
			if err != nil {
				t.Fatal(err)
			}
			return body
		}},
		{name: "trailing JSON", body: func(t *testing.T, f *fixture) []byte {
			return append(append([]byte(nil), f.manifestBody...), []byte("{}")...)
		}},
		{name: "wrong schema version", body: func(t *testing.T, f *fixture) []byte {
			f.manifest.SchemaVersion = 2
			body, err := json.Marshal(f.manifest)
			if err != nil {
				t.Fatal(err)
			}
			return body
		}},
		{name: "invalid manifest UTF-8", body: func(_ *testing.T, _ *fixture) []byte { return []byte{'{', 0xff, '}'} }},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			body := testCase.body(t, f)
			if err := os.WriteFile(f.invocation.ManifestPath, body, 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := Preflight(f.invocation)
			if err == nil {
				t.Fatal("Preflight accepted malformed manifest")
			}
			if result.Report.Preflight.ChildStarts != 0 || result.Report.Preflight.ListenerOpens != 0 || result.Report.Preflight.NetworkAttempts != 0 {
				t.Fatalf("malformed manifest crossed an effect boundary: %#v", result.Report.Preflight)
			}
			if err := ValidateReport(result.Report); err != nil {
				t.Fatalf("failure report is invalid: %v", err)
			}
		})
	}
}

func TestU04RejectsIncompleteLimitsAndReportShape(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.manifest.Limits.DiskBytes = 1
	f.writeManifest(t)
	result, err := Preflight(f.invocation)
	if err == nil {
		t.Fatal("Preflight accepted inputs over the declared disk limit")
	}
	if result.Report.Preflight.ChildStarts != 0 || result.Report.Preflight.ListenerOpens != 0 || result.Report.Preflight.NetworkAttempts != 0 {
		t.Fatalf("disk limit failure crossed an effect boundary: %#v", result.Report.Preflight)
	}
	if err := ValidateReport(result.Report); err != nil {
		t.Fatalf("disk-limit report is invalid: %v", err)
	}

	incomplete := newReport()
	incomplete.Identities = nil
	if err := ValidateReport(incomplete); err == nil {
		t.Fatal("ValidateReport accepted a report without identities")
	}
}

func TestU06AtomicReportPublicationDoesNotExposePartialPass(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	reportPath := filepath.Join(root, "report.json")
	report := newReport()
	report.Verdict = "PASS"
	interrupted := errors.New("controlled publication interruption")
	if err := WriteReportAtomic(reportPath, report, func() error { return interrupted }); !errors.Is(err, interrupted) {
		t.Fatalf("WriteReportAtomic error = %v, want interruption", err)
	}
	if _, err := os.Stat(reportPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interrupted report stat error = %v, want no published report", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary report artifacts remain: %v", entries)
	}
	if err := WriteReportAtomic(reportPath, report, nil); err != nil {
		t.Fatalf("WriteReportAtomic successful publication: %v", err)
	}
	body, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var published Report
	if err := json.Unmarshal(body, &published); err != nil {
		t.Fatalf("published JSON: %v", err)
	}
	if err := ValidateReport(published); err != nil {
		t.Fatalf("published report validation: %v", err)
	}
	if published.Verdict != "PASS" {
		t.Fatalf("published verdict = %q, want PASS", published.Verdict)
	}

	incomplete := report
	incomplete.Audio = nil
	badPath := filepath.Join(root, "incomplete.json")
	if err := WriteReportAtomic(badPath, incomplete, nil); err == nil {
		t.Fatal("WriteReportAtomic accepted incomplete report")
	}
	if _, err := os.Stat(badPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("incomplete report was published: %v", err)
	}
}

type fixture struct {
	root         string
	outputRoot   string
	manifest     Manifest
	manifestBody []byte
	invocation   Invocation
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	outputRoot := filepath.Join(root, "output")
	if err := os.Mkdir(outputRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, body []byte) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	artifact := write("artifact.bin", []byte("fake immutable executable\n"))
	installer := write("installer.exe", []byte("fake installer\n"))
	archive := write("archive.zip", []byte("fake archive\n"))
	checksums := write("checksums.txt", []byte("artifact  sha256\n"))
	acceptance := write("acceptance.md", []byte("acceptance witness\n"))
	doc := write("models.md", []byte("public models docs\n"))
	text := write("prompt.txt", []byte("Read the release summary.\n"))
	voice := write("voice.bin", []byte{0, 1, 2, 3})
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Build:         Build{Identity: "build-test-001", Platform: "windows", Architecture: "amd64", ArtifactPath: artifact, ArtifactKind: "executable", SHA256: fileSHA(t, artifact)},
		Distribution: Distribution{
			Installer: ImmutableFile{Path: installer, Identity: "installer-test-001", SHA256: fileSHA(t, installer)},
			Archive:   ImmutableFile{Path: archive, Identity: "archive-test-001", SHA256: fileSHA(t, archive)},
			Checksums: ImmutableFile{Path: checksums, Identity: "checksums-test-001", SHA256: fileSHA(t, checksums)},
		},
		Acceptance: ImmutableFile{Path: acceptance, Identity: "acceptance-test-001", SHA256: fileSHA(t, acceptance)},
		PublicDocs: []NamedImmutableFile{{Name: "models", Path: doc, Identity: "docs-test-001", SHA256: fileSHA(t, doc)}},
		Fixtures: Fixtures{
			Text:  NamedImmutableFile{Name: "prompt", Path: text, Identity: "fixture-text-001", SHA256: fileSHA(t, text)},
			Voice: &NamedImmutableFile{Name: "voice", Path: voice, Identity: "fixture-voice-001", SHA256: fileSHA(t, voice)},
		},
		Limits: Limits{TimeoutSeconds: 30, DiskBytes: 1 << 20, DownloadBytes: 0, PaidUSD: 0, MaxOwnedProcesses: 2, NetworkPolicy: "none"},
	}
	f := &fixture{root: root, outputRoot: outputRoot, manifest: manifest}
	f.writeManifest(t)
	f.invocation = Invocation{ArtifactPath: artifact, ArtifactIdentity: manifest.Build.Identity, ArtifactSHA256: manifest.Build.SHA256, ManifestPath: filepath.Join(root, "manifest.json"), ReportPath: filepath.Join(outputRoot, "report.json")}
	return f
}

func (f *fixture) writeManifest(t *testing.T) {
	t.Helper()
	body, err := json.Marshal(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	f.manifestBody = body
	if err := os.WriteFile(filepath.Join(f.root, "manifest.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fileSHA(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256Hex(body)
}

func mustRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(fmt.Errorf("remove %s: %w", path, err))
	}
}
