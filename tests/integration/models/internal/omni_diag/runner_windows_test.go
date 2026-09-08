//go:build windows

package omni_diag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/locking"
	"golang.org/x/sys/windows"
)

var errLocalAIOMNIReportInterrupted = errors.New("localai omni report interruption")
var errLocalAIOMNIRealDisabled = errors.New("localai omni real selector is disabled")

type localAIOMNIControlledExecutor struct {
	Observation localAIOMNIObservation
	Calls       atomic.Int32
	RemoveRoot  bool
}

func (e *localAIOMNIControlledExecutor) Execute(_ context.Context, _ localAICommandSpec, roots localAIRealRoots) localAIOMNIObservation {
	e.Calls.Add(1)
	if e.RemoveRoot {
		_ = os.WriteFile(filepath.Join(roots.Work, ".omni-inspection-failure"), nil, 0o600)
	}
	return e.Observation
}

type localAIOMNICancellationExecutor struct {
	Started chan struct{}
	once    sync.Once
}

func (e *localAIOMNICancellationExecutor) Execute(ctx context.Context, _ localAICommandSpec, _ localAIRealRoots) localAIOMNIObservation {
	e.once.Do(func() { close(e.Started) })
	<-ctx.Done()
	return localAIOMNIObservation{localAICommandObservation: localAICommandObservation{Started: true, ProcessExited: true, ExitCode: 1, ProcessTreeClosed: true}, Cancelled: true, BackendLogs: []localAIOMNIRawLog{{Path: "backend.log", Body: []byte("cancelled")}}, RuntimeLogs: []localAIOMNIRawLog{{Path: "runtime.log", Body: []byte("cancelled")}}}
}

func localAIOMNIFailIf(t testing.TB, bad bool, format string, args ...any) {
	if bad {
		t.Fatalf(format, args...)
	}
}
func writeLocalAIOMNIReportAtomicHook(path string, report localAIOMNIReport, hook func() error) error {
	if err := localAIOMNIValidateReport(report); err != nil {
		return err
	}
	return writeLocalAIJSONAtomic(path, report, hook)
}
func localAIOMNIInvocationValues(enable, path string) (localAIOMNIInvocation, error) {
	if strings.TrimSpace(enable) != "1" {
		return localAIOMNIInvocation{}, errLocalAIOMNIRealDisabled
	}
	if err := localAIOMNIAbs(path); err != nil {
		return localAIOMNIInvocation{}, errors.New("real selector requires an absolute manifest path")
	}
	m, hash, err := localAIOMNIReadManifest(path)
	if err != nil {
		return localAIOMNIInvocation{}, err
	}
	return localAIOMNIInvocation{Manifest: m, ManifestSHA256: hash, RunID: "real-" + m.Selector + "-" + hash[:12], EvidenceKind: localAIOMNIReal, Command: localAIOMNICommand(m)}, nil
}
func localAIOMNIReadManifest(path string) (localAIOMNIManifest, string, error) {
	return localAIOMNIReadManifestWith(path, localAIOMNIWindowsPathMetadata)
}
func localAIOMNIReadManifestWith(path string, metadata localAIOMNIPathMetadataFunc) (localAIOMNIManifest, string, error) {
	if err := localAIOMNIRejectReparsePath(path, "manifest", metadata); err != nil {
		return localAIOMNIManifest{}, "", err
	}
	info, _, err := metadata(path)
	if err != nil {
		return localAIOMNIManifest{}, "", err
	}
	if info == nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > localAIOMNIMaxManifest {
		return localAIOMNIManifest{}, "", errors.New("manifest is not bounded")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return localAIOMNIManifest{}, "", err
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	var m localAIOMNIManifest
	if err := d.Decode(&m); err != nil {
		return localAIOMNIManifest{}, "", err
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return localAIOMNIManifest{}, "", errors.New("manifest contains trailing JSON")
	}
	if err := localAIOMNIValidateManifest(m); err != nil {
		return localAIOMNIManifest{}, "", err
	}
	return m, sha256Hex(body), nil
}

func localAIOMNICommand(m localAIOMNIManifest) localAICommandSpec {
	prompt, media := "Return this exact token: "+m.Text.Token, ""
	if m.Selector == localAIOMNIImage {
		prompt, media = "Name every required fact visible in the image: "+strings.Join(m.Image.RequiredFacts, ", "), "image=@"+m.Image.Path
	}
	if m.Selector == localAIOMNIVideo {
		prompt, media = fmt.Sprintf("Report %s/%s and %s/%s, and the numeric transition time in milliseconds.", m.Video.Phase1, m.Video.Phase1Color, m.Video.Phase2, m.Video.Phase2Color), "video=@"+m.Video.Path
	}
	args := []string{"--json", "models", "invoke", m.Model.Name, "--operation", "OMNI", "--input", "prompt=" + prompt}
	if media != "" {
		args = append(args, "--input", media)
	}
	return localAICommandSpec{BinaryPath: m.CLI.Path, Arguments: args}
}

func TestLocalAIOMNIReparseManifestAdmission(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name  string
		final bool
	}{
		{name: "final", final: true},
		{name: "existing-parent", final: false},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			_, originalPath := localAIOMNIRealManifestForTest(t, root, localAIOMNIText)
			manifestPath := filepath.Join(root, "nested", "manifest.json")
			if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
				t.Fatalf("manifest directory: %v", err)
			}
			body, err := os.ReadFile(originalPath)
			if err != nil {
				t.Fatalf("read manifest fixture: %v", err)
			}
			if err := os.WriteFile(manifestPath, body, 0o600); err != nil {
				t.Fatalf("write manifest fixture: %v", err)
			}
			flagged := filepath.Clean(manifestPath)
			if !testCase.final {
				flagged = filepath.Dir(flagged)
			}
			var calls []string
			metadata := func(path string) (os.FileInfo, uint32, error) {
				calls = append(calls, filepath.Clean(path))
				info, err := os.Lstat(path)
				if err != nil {
					return nil, 0, err
				}
				if filepath.Clean(path) == flagged {
					return info, windows.FILE_ATTRIBUTE_REPARSE_POINT, nil
				}
				return info, 0, nil
			}
			_, _, err = localAIOMNIReadManifestWith(manifestPath, metadata)
			localAIOMNIReparseErrorMust(t, err, "manifest", testCase.final, map[bool]int{true: 0, false: 1}[testCase.final], calls, manifestPath, flagged)
		})
	}
}

func TestLocalAIOMNIReparseArtifactAdmissionStopsBeforeHash(t *testing.T) {
	t.Parallel()
	for _, artifact := range []struct {
		label string
		path  func(localAIOMNIManifest) string
		hash  func(localAIOMNIManifest) string
	}{
		{label: "CLI", path: func(m localAIOMNIManifest) string { return m.CLI.Path }, hash: func(m localAIOMNIManifest) string { return m.CLI.SHA256 }},
		{label: "image fixture", path: func(m localAIOMNIManifest) string { return m.Image.Path }, hash: func(m localAIOMNIManifest) string { return m.Image.SHA256 }},
		{label: "video fixture", path: func(m localAIOMNIManifest) string { return m.Video.Path }, hash: func(m localAIOMNIManifest) string { return m.Video.SHA256 }},
	} {
		artifact := artifact
		t.Run(artifact.label, func(t *testing.T) {
			t.Parallel()
			for _, final := range []bool{true, false} {
				final := final
				t.Run(map[bool]string{true: "final", false: "existing-parent"}[final], func(t *testing.T) {
					root := t.TempDir()
					manifest := localAIOMNIManifestForTest(t, root)
					path, expected := artifact.path(manifest), artifact.hash(manifest)
					flagged := filepath.Clean(path)
					if !final {
						flagged = filepath.Dir(flagged)
					}
					var calls []string
					metadata := func(path string) (os.FileInfo, uint32, error) {
						calls = append(calls, filepath.Clean(path))
						info, err := os.Lstat(path)
						if err != nil {
							return nil, 0, err
						}
						if filepath.Clean(path) == flagged {
							return info, windows.FILE_ATTRIBUTE_REPARSE_POINT, nil
						}
						return info, 0, nil
					}
					var identityCalls atomic.Int32
					err := localAIOMNIFileWith(path, expected, artifact.label, metadata, func(string) (localAIFileIdentity, bool) {
						identityCalls.Add(1)
						return localAIFileIdentity{Bytes: 1, SHA256: expected}, true
					})
					localAIOMNIReparseErrorMust(t, err, artifact.label, final, map[bool]int{true: 0, false: 1}[final], calls, path, flagged)
					if got := identityCalls.Load(); got != 0 {
						t.Fatalf("identity calls=%d, want zero", got)
					}
				})
			}
		})
	}
}

func TestLocalAIOMNIReparseWalkerBounds(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatalf("root metadata: %v", err)
	}
	path := root
	for i := 0; i < 32; i++ {
		path = filepath.Join(path, fmt.Sprintf("component-%02d", i))
	}
	var calls atomic.Int32
	metadata := func(string) (os.FileInfo, uint32, error) {
		calls.Add(1)
		return info, 0, nil
	}
	if err := localAIOMNIRejectReparsePath(path, "bounds", metadata); err != nil {
		t.Fatalf("bounded walk: %v", err)
	}
	var want int32
	current := filepath.Clean(path)
	for {
		want++
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	if got := calls.Load(); got != want {
		t.Fatalf("metadata calls=%d, want %d final-to-root components", got, want)
	}
	var invalidCalls atomic.Int32
	if err := localAIOMNIRejectReparsePath("relative", "bounds", func(string) (os.FileInfo, uint32, error) {
		invalidCalls.Add(1)
		return info, 0, nil
	}); err == nil || invalidCalls.Load() != 0 {
		t.Fatalf("relative path err=%v metadata calls=%d", err, invalidCalls.Load())
	}
	missing := filepath.Join(root, "existing", "missing.json")
	missingParent := filepath.Dir(missing)
	var missingCalls []string
	missingMetadata := func(path string) (os.FileInfo, uint32, error) {
		missingCalls = append(missingCalls, filepath.Clean(path))
		if filepath.Clean(path) == filepath.Clean(missing) {
			return nil, 0, os.ErrNotExist
		}
		if filepath.Clean(path) == filepath.Clean(missingParent) {
			return info, windows.FILE_ATTRIBUTE_REPARSE_POINT, nil
		}
		return info, 0, nil
	}
	if err := localAIOMNIRejectReparsePath(missing, "bounds", missingMetadata); err == nil {
		t.Fatal("missing final path did not inspect existing parent reparse")
	} else {
		localAIOMNIReparseErrorMust(t, err, "bounds", false, 1, missingCalls, missing, missingParent)
	}
}

func localAIOMNIReparseErrorMust(t *testing.T, err error, label string, final bool, index int, calls []string, path, flagged string) {
	t.Helper()
	var reparseErr *localAIOMNIReparsePathError
	if !errors.As(err, &reparseErr) || reparseErr.Label != label || reparseErr.Final != final || reparseErr.ComponentIndex != index {
		t.Fatalf("reparse error=%v, want label=%q final=%t index=%d", err, label, final, index)
	}
	if strings.Contains(err.Error(), filepath.Clean(path)) || strings.Contains(err.Error(), filepath.Clean(flagged)) {
		t.Fatalf("reparse error leaked path: %q", err)
	}
	wantCalls := []string{filepath.Clean(path)}
	if !final {
		wantCalls = append(wantCalls, filepath.Clean(flagged))
	}
	if len(calls) != len(wantCalls) {
		t.Fatalf("metadata calls=%v, want ordered prefix=%v", calls, wantCalls)
	}
	for i := range wantCalls {
		if calls[i] != wantCalls[i] {
			t.Fatalf("metadata calls=%v, want %v", calls, wantCalls)
		}
	}
}
func localAIOMNIValidateReport(r localAIOMNIReport) error {
	if r.Schema != localAIOMNIEvidenceSchema || (r.EvidenceKind != localAIOMNIControlled && r.EvidenceKind != localAIOMNIReal) || r.RunID == "" || r.Platform == "" || r.Architecture == "" || !r.Redacted || (r.Status != "PASS" && r.Status != "FAIL" && r.Status != "INCONCLUSIVE") {
		return errors.New("report identity or status is invalid")
	}
	admission := r.Failure != nil && r.Failure.Assertion == "manifest admission"
	if err := errors.Join(localAIOMNIRequire((r.Selector == localAIOMNIText || r.Selector == localAIOMNIImage || r.Selector == localAIOMNIVideo) || admission, "report selector or manifest identity is invalid"), localAIOMNIRequire(r.ManifestSHA256 == "" || isLocalAISHA256(r.ManifestSHA256), "report selector or manifest identity is invalid"), localAIOMNIRequire(len(r.Command.Arguments) <= localAIOMNIMaxArguments && r.Command.SecretsRedacted, "command evidence is invalid")); err != nil {
		return err
	}
	if r.EvidenceKind == localAIOMNIControlled && r.Activity != (localAIOMNIActivity{}) {
		return errors.New("controlled evidence recorded real activity")
	}
	if r.Status == "PASS" && (r.Failure != nil || !r.Semantic.Passed || r.Reservation == nil || r.Reservation.State != "COMMITTED" || len(r.Artifacts) == 0 || len(r.Logs.Backend) == 0 || len(r.Logs.Runtime) == 0 || !r.Release.Checked || r.Release.InspectionFailed || !r.Release.ProcessTreeClosed || r.Release.OwnedProcesses != 0 || r.Release.OwnedListeners != 0 || r.Release.OwnedLeases != 0 || r.Release.PartialArtifacts != 0 || r.Cache.AfterInspectionFailed) {
		return errors.New("pass report omitted semantic or release proof")
	}
	if r.Status != "PASS" && r.Failure == nil {
		return errors.New("non-pass report omitted failure")
	}
	if !r.Release.Checked && r.Failure != nil && r.Failure.Assertion != "manifest admission" && r.Failure.Assertion != "final cache and release inspection" {
		return errors.New("release inspection was not established")
	}
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return localAIOMNIRequire(!localAIOMNISecret(body) && !localAIOMNIPathPattern.Match(body), "report contains unredacted sensitive evidence")
}
func mustLocalAIOMNIRunner(t testing.TB, e localAIOMNIExecutor) localAIOMNIRunner {
	locks, err := locking.New(locking.LocalFileSystem{})
	if err != nil {
		t.Fatalf("new omni locks: %v", err)
	}
	return localAIOMNIRunner{executor: e, locks: locks, writeReport: func(path string, report localAIOMNIReport) error {
		return writeLocalAIOMNIReportAtomicHook(path, report, nil)
	}}
}
func localAIOMNIInvocationForTest(t testing.TB, m localAIOMNIManifest, id, kind string) localAIOMNIInvocation {
	body, _ := json.Marshal(m)
	return localAIOMNIInvocation{Manifest: m, ManifestSHA256: sha256Hex(body), RunID: id, EvidenceKind: kind, Command: localAIOMNICommand(m)}
}
func localAIOMNIManifestForTest(t testing.TB, root string) localAIOMNIManifest {
	cli, image, video := filepath.Join(root, "bin", "you.exe"), filepath.Join(root, "fixtures", "infinite-you.png"), filepath.Join(root, "fixtures", "groundtruth-fixture.mp4")
	for path, body := range map[string][]byte{cli: []byte("controlled CLI identity"), image: []byte("controlled image fixture: infinity symbol / INFINITE YOU"), video: []byte("controlled video fixture: PHASE 1 red then PHASE 2 blue")} {
		localAIOMNIFailIf(t, os.MkdirAll(filepath.Dir(path), 0o700) != nil, "fixture directory creation failed")
		localAIOMNIFailIf(t, os.WriteFile(path, body, 0o600) != nil, "fixture write failed")
	}
	hash := func(path string) string {
		identity, ok := localAIReadFileIdentity(path)
		localAIOMNIFailIf(t, !ok, "hash fixture %s", path)
		return identity.SHA256
	}
	return localAIOMNIManifest{Schema: localAIOMNIInputSchema, Selector: localAIOMNIText, CLI: localAIOMNICLI{Path: cli, SHA256: hash(cli)}, Model: localAIOMNIModel{Name: "llm", Identity: "model-revision-controlled"}, Backend: localAIOMNIBackend{Identity: "backend-revision-controlled"}, Text: localAIOMNITextFixture{Token: "COBALT-17"}, Image: localAIOMNIImageFixture{Path: image, SHA256: hash(image), RequiredFacts: []string{"infinity symbol", "INFINITE YOU"}}, Video: localAIOMNIVideoFixture{Path: video, SHA256: hash(video), Phase1: "PHASE 1", Phase1Color: "red", Phase2: "PHASE 2", Phase2Color: "blue", TransitionStartMilliseconds: 1500, TransitionEndMilliseconds: 2500}, Isolation: localAIOMNIIsolation{WorkRoot: filepath.Join(root, "work"), StateRoot: filepath.Join(root, "state"), CacheRoot: filepath.Join(root, "cache"), TempRoot: filepath.Join(root, "temp"), OutputRoot: filepath.Join(root, "output"), StreamsRoot: filepath.Join(root, "streams"), Host: "127.0.0.1", Port: 54321, NetworkPolicy: localAIOMNINetworkPolicy}, Limits: localAIOMNILimits{TimeoutSeconds: 1200, Processes: 4, ModelCalls: 1}, Evidence: localAIOMNIEvidencePaths{ReportPath: filepath.Join(root, "evidence", "report.json"), LedgerPath: filepath.Join(root, "evidence", "ledger.json")}}
}
func localAIOMNIPass(root, output string) localAIOMNIObservation {
	body := []byte(output)
	return localAIOMNIObservation{localAICommandObservation: localAICommandObservation{ProcessExited: true, ExitCode: 0, ProcessTreeClosed: true, Stdout: body}, BackendLogs: []localAIOMNIRawLog{{Path: filepath.Join(root, "backend.log"), Body: []byte("backend completed")}}, RuntimeLogs: []localAIOMNIRawLog{{Path: filepath.Join(root, "runtime.log"), Body: []byte("runtime completed")}}, Artifacts: []localAIOMNIRawArtifact{{Kind: "text", Path: filepath.Join(root, "output.txt"), MediaType: "text/plain", Body: body}}}
}
func localAIOMNIReportMust(t testing.TB, m localAIOMNIManifest, id string, o localAIOMNIObservation, e localAIOMNIExecutor) localAIOMNIReport {
	r, err := mustLocalAIOMNIRunner(t, e).Run(t.Context(), localAIOMNIInvocationForTest(t, m, id, localAIOMNIControlled))
	localAIOMNIFailIf(t, err != nil, "runner error: %v", err)
	return r
}
func localAIOMNIStatusFor(t testing.TB, m localAIOMNIManifest, id, output, want string) localAIOMNIReport {
	r := localAIOMNIReportMust(t, m, id, localAIOMNIPass(filepath.Dir(m.Evidence.ReportPath), output), &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(filepath.Dir(m.Evidence.ReportPath), output)})
	localAIOMNIFailIf(t, r.Status != want, "status=%s want=%s", r.Status, want)
	return r
}
func TestLocalAIOMNIRealDiagnosticRunnerControlled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		run  func(*testing.T)
	}{{"I01-controlled-default-zero-real-activity", localAIOMNII01}, {"I02-incomplete-manifest-rejects-before-launch", localAIOMNII02}, {"I03-durable-budget-and-race-safe-reservation", localAIOMNII03}, {"I04-fixture-hash-rejection-before-launch", localAIOMNII04}, {"I05-text-exact-token-pass", localAIOMNISemanticCase}, {"I06-text-token-absence-fail", localAIOMNISemanticCase}, {"I07-image-grounded-facts-pass", localAIOMNISemanticCase}, {"I08-video-grounded-numeric-window-pass", localAIOMNISemanticCase}, {"I09-video-vague-midpoint-inconclusive", localAIOMNISemanticCase}, {"I10-video-prompt-echo-inconclusive", localAIOMNISemanticCase}, {"I11-video-numeric-window-fail", localAIOMNISemanticCase}, {"I12-bounded-redacted-evidence", localAIOMNII12}, {"I13-atomic-report-interruption", localAIOMNII13}, {"I14-cancellation-and-timeout-cleanup", localAIOMNII14}, {"I15-owned-resource-leak-never-passes", localAIOMNII15}}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { t.Parallel(); c.run(t) })
	}
}
func localAIOMNII01(t *testing.T) {
	_, err := localAIOMNIInvocationValues("", "")
	localAIOMNIFailIf(t, !errors.Is(err, errLocalAIOMNIRealDisabled), "disabled admission=%v", err)
	_, err = localAIOMNIInvocationValues("1", "relative.json")
	localAIOMNIFailIf(t, err == nil, "relative real manifest accepted")
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	path := filepath.Join(root, "manifest.json")
	body, _ := json.Marshal(m)
	localAIOMNIFailIf(t, os.WriteFile(path, body, 0o600) != nil, "manifest write failed")
	in, err := localAIOMNIInvocationValues("1", path)
	localAIOMNIFailIf(t, err != nil || in.EvidenceKind != localAIOMNIReal, "real admission=%#v err=%v", in, err)
	r := localAIOMNIStatusFor(t, m, "I01", m.Text.Token, "PASS")
	localAIOMNIFailIf(t, r.Activity != (localAIOMNIActivity{}) || r.Reservation == nil || r.Reservation.State != "COMMITTED", "report=%#v", r)
	data, _ := os.ReadFile(m.Evidence.ReportPath)
	localAIOMNIFailIf(t, bytes.Contains(data, []byte(m.Isolation.WorkRoot)) || bytes.Contains(data, []byte(m.CLI.Path)), "report leaked an owned path")
}
func localAIOMNII02(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	valid, _ := json.Marshal(m)
	m.Text.Token = ""
	localAIOMNIAdmissionFailure(t, m, "I02", true)
	cases := map[string][]byte{"malformed": []byte("{"), "unknown": append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"unknown":1}`)...), "trailing": append(append([]byte(nil), valid...), []byte(" {}")...)}
	for name, body := range cases {
		path := filepath.Join(root, name+".json")
		localAIOMNIFailIf(t, os.WriteFile(path, body, 0o600) != nil, "manifest fixture write failed")
		_, _, err := localAIOMNIReadManifest(path)
		localAIOMNIFailIf(t, err == nil, "%s manifest was accepted", name)
	}
}
func localAIOMNIAdmissionFailure(t *testing.T, m localAIOMNIManifest, id string, ledgerAbsent bool) {
	e := &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(m.Isolation.WorkRoot, "unused")}
	r := localAIOMNIReportMust(t, m, id, localAIOMNIPass(m.Isolation.WorkRoot, "unused"), e)
	localAIOMNIFailIf(t, r.Status != "FAIL" || r.Failure == nil || r.Failure.Assertion != "manifest admission" || e.Calls.Load() != 0, "report=%#v calls=%d", r, e.Calls.Load())
	if ledgerAbsent {
		_, err := os.Stat(m.Evidence.LedgerPath)
		localAIOMNIFailIf(t, !errors.Is(err, os.ErrNotExist), "ledger=%v", err)
	}
}
func localAIOMNII03(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	_ = localAIOMNIStatusFor(t, m, "I03", m.Text.Token, "PASS")
	m.Evidence.ReportPath = filepath.Join(root, "evidence", "second.json")
	e := &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(root, m.Text.Token)}
	r := localAIOMNIReportMust(t, m, "I03", localAIOMNIPass(root, m.Text.Token), e)
	localAIOMNIFailIf(t, r.Status != "FAIL" || r.Failure == nil || r.Failure.Assertion != "durable model-call budget" || e.Calls.Load() != 0, "exhausted report=%#v calls=%d", r, e.Calls.Load())
	corruptRoot := t.TempDir()
	cm := localAIOMNIManifestForTest(t, corruptRoot)
	localAIOMNIFailIf(t, os.MkdirAll(filepath.Dir(cm.Evidence.LedgerPath), 0o700) != nil, "ledger directory creation failed")
	localAIOMNIFailIf(t, os.WriteFile(cm.Evidence.LedgerPath, []byte(`{"schema":"corrupt"}`), 0o600) != nil, "corrupt ledger write failed")
	ce := &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(corruptRoot, cm.Text.Token)}
	cr := localAIOMNIReportMust(t, cm, "I03-corrupt", localAIOMNIPass(corruptRoot, cm.Text.Token), ce)
	localAIOMNIFailIf(t, cr.Status != "FAIL" || cr.Failure == nil || cr.Failure.Assertion != "durable model-call ledger" || ce.Calls.Load() != 0, "corrupt ledger report=%#v calls=%d", cr, ce.Calls.Load())
	raceRoot := t.TempDir()
	ledger := filepath.Join(raceRoot, "ledger.json")
	const attempts = 9
	results := make(chan localAIOMNIReport, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			cell := filepath.Join(raceRoot, fmt.Sprintf("cell-%d", i))
			cm := localAIOMNIManifestForTest(t, cell)
			cm.Limits.ModelCalls = 3
			cm.Evidence.LedgerPath = ledger
			cm.Evidence.ReportPath = filepath.Join(cell, "report.json")
			rr, e := mustLocalAIOMNIRunner(t, &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(cell, cm.Text.Token)}).Run(t.Context(), localAIOMNIInvocationForTest(t, cm, "I03-race", localAIOMNIControlled))
			localAIOMNIFailIf(t, e != nil, "race report: %v", e)
			results <- rr
		}()
	}
	wg.Wait()
	close(results)
	pass, exhausted := 0, 0
	for r := range results {
		if r.Status == "PASS" {
			pass++
		} else if r.Status == "FAIL" && r.Failure != nil && r.Failure.Assertion == "durable model-call budget" {
			exhausted++
		} else {
			t.Fatalf("race report=%#v", r)
		}
	}
	localAIOMNIFailIf(t, pass != 3 || exhausted != attempts-3, "race pass=%d exhausted=%d", pass, exhausted)
}
func localAIOMNII04(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	m.Selector = localAIOMNIImage
	m.Image.SHA256 = strings.Repeat("0", 64)
	localAIOMNIAdmissionFailure(t, m, "I04", false)
	localAIOMNIFailIf(t, os.Remove(m.Image.Path) != nil, "fixture removal failed")
	localAIOMNIAdmissionFailure(t, m, "I04-missing-fixture", false)
}

var localAIOMNISemantics = map[string]struct{ selector, output, want string }{
	"I05": {localAIOMNIText, "COBALT-17", "PASS"}, "I06": {localAIOMNIText, "COBALT-18", "FAIL"},
	"I07": {localAIOMNIImage, "The image shows an infinity symbol and the words INFINITE YOU.", "PASS"},
	"I08": {localAIOMNIVideo, "PHASE 1 is red, then PHASE 2 is blue; the transition occurred at 2000 ms.", "PASS"},
	"I09": {localAIOMNIVideo, "PHASE 1 is red, then PHASE 2 is blue around the midpoint.", "INCONCLUSIVE"},
	"I10": {localAIOMNIVideo, "", "INCONCLUSIVE"}, "I11": {localAIOMNIVideo, "PHASE 1 red; PHASE 2 blue; transition at 3000 ms.", "FAIL"},
}

func localAIOMNISemanticCase(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	name := t.Name()
	key := strings.SplitN(name, "/", 2)[1][:3]
	spec := localAIOMNISemantics[key]
	selector, output, want := spec.selector, spec.output, spec.want
	if key == "I10" {
		selector = localAIOMNIVideo
		m.Selector = selector
		cmd := localAIOMNICommand(m)
		for i, a := range cmd.Arguments {
			if a == "--input" && i+1 < len(cmd.Arguments) && strings.HasPrefix(cmd.Arguments[i+1], "prompt=") {
				output = strings.TrimPrefix(cmd.Arguments[i+1], "prompt=")
			}
		}
		want = "INCONCLUSIVE"
	}
	m.Selector = selector
	r := localAIOMNIStatusFor(t, m, name, output, want)
	localAIOMNIFailIf(t, key == "I08" && (r.Semantic.NumericTransitionMilliseconds == nil || *r.Semantic.NumericTransitionMilliseconds != 2000), "video semantic=%#v", r.Semantic)
	localAIOMNIFailIf(t, key == "I10" && !r.Semantic.PromptEcho, "prompt echo was not classified inconclusive")
}
func localAIOMNII12(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	body := append([]byte("HF_TOKEN=controlled-secret "), bytes.Repeat([]byte("bounded-output "), localAIOMNIMaxStream)...)
	o := localAIOMNIPass(root, m.Text.Token)
	o.Stderr, o.BackendLogs[0].Body, o.RuntimeLogs[0].Body = body, body, body
	r := localAIOMNIReportMust(t, m, "I12", o, &localAIOMNIControlledExecutor{Observation: o})
	localAIOMNIFailIf(t, r.Status != "FAIL" || r.Failure == nil || r.Failure.Owner != "harness" || !r.Command.Stderr.Truncated || !r.Logs.Backend[0].Truncated || !r.Logs.Runtime[0].Truncated, "evidence report=%#v", r)
	data, _ := os.ReadFile(m.Evidence.ReportPath)
	localAIOMNIFailIf(t, bytes.Contains(data, []byte("controlled-secret")), "secret leaked")
	observationRoot := t.TempDir()
	om := localAIOMNIManifestForTest(t, observationRoot)
	actual := localAIOMNIPass(observationRoot, om.Text.Token)
	actual.Stdout = bytes.Repeat([]byte("X"), localAIOMNIMaxStream)
	actual.Stderr = bytes.Repeat([]byte("Y"), localAIOMNIMaxStream)
	actual.StdoutTruncated, actual.StderrTruncated = true, true
	truncated := localAIOMNIReportMust(t, om, "I12-observation", actual, &localAIOMNIControlledExecutor{Observation: actual})
	localAIOMNIFailIf(t, truncated.Status != "FAIL" || truncated.Failure == nil || truncated.Failure.Assertion != "bounded command streams", "observation report=%#v", truncated)
}
func localAIOMNII13(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	m.Limits.ModelCalls = 2
	_ = localAIOMNIStatusFor(t, m, "I13", m.Text.Token, "PASS")
	old, _ := os.ReadFile(m.Evidence.ReportPath)
	runner := mustLocalAIOMNIRunner(t, &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(root, m.Text.Token)})
	runner.writeReport = func(path string, r localAIOMNIReport) error {
		return writeLocalAIOMNIReportAtomicHook(path, r, func() error { return errLocalAIOMNIReportInterrupted })
	}
	r, err := runner.Run(t.Context(), localAIOMNIInvocationForTest(t, m, "I13", localAIOMNIControlled))
	localAIOMNIFailIf(t, !errors.Is(err, errLocalAIOMNIReportInterrupted) || r.Status != "PASS", "interrupted report=%#v err=%v", r, err)
	now, _ := os.ReadFile(m.Evidence.ReportPath)
	localAIOMNIFailIf(t, !bytes.Equal(old, now), "atomic interruption replaced canonical report")
}
func localAIOMNII14(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	e := &localAIOMNICancellationExecutor{Started: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan localAIOMNIReport, 1)
	go func() {
		r, err := mustLocalAIOMNIRunner(t, e).Run(ctx, localAIOMNIInvocationForTest(t, m, "I14", localAIOMNIControlled))
		localAIOMNIFailIf(t, err != nil, "cancellation report: %v", err)
		done <- r
	}()
	<-e.Started
	cancel()
	out := <-done
	localAIOMNIFailIf(t, out.Status != "FAIL" || out.Failure == nil || out.Failure.Owner != "harness" || !out.Release.Checked || !out.Release.ProcessTreeClosed, "cancellation report=%#v", out)
}
func localAIOMNII15(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	_ = os.MkdirAll(m.Isolation.OutputRoot, 0o700)
	_ = os.WriteFile(filepath.Join(m.Isolation.OutputRoot, "response.partial"), []byte("partial"), 0o600)
	o := localAIOMNIPass(root, m.Text.Token)
	o.Started, o.ProcessExited, o.ProcessTreeClosed, o.OwnedProcesses = true, false, false, 1
	r := localAIOMNIReportMust(t, m, "I15", o, &localAIOMNIControlledExecutor{Observation: o})
	localAIOMNIFailIf(t, r.Status != "FAIL" || r.Failure == nil || r.Failure.Owner != "harness" || !r.Release.Checked || r.Release.PartialArtifacts == 0 || r.Release.ProcessTreeClosed, "cleanup report=%#v", r)
	inspectionRoot := t.TempDir()
	im := localAIOMNIManifestForTest(t, inspectionRoot)
	runner := mustLocalAIOMNIRunner(t, &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(inspectionRoot, im.Text.Token), RemoveRoot: true})
	final, err := runner.Run(t.Context(), localAIOMNIInvocationForTest(t, im, "I15-inspection", localAIOMNIControlled))
	localAIOMNIFailIf(t, err != nil || final.Status != "INCONCLUSIVE" || final.Failure == nil || final.Failure.Assertion != "final cache and release inspection" || final.Release.Checked || !final.Release.InspectionFailed, "final inspection report=%#v err=%v", final, err)
}
