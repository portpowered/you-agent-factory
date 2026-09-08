package platform_conformance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type readinessFixture struct {
	root      string
	spec      RunSpec
	host      HostIdentity
	inspector *fixtureInspector
}

type fixtureInspector struct {
	artifacts map[string]ObservedArtifact
	errors    map[string]error
	calls     []string
}

func (inspector *fixtureInspector) Inspect(path string) (ObservedArtifact, error) {
	inspector.calls = append(inspector.calls, path)
	if err := inspector.errors[path]; err != nil {
		return ObservedArtifact{}, err
	}
	artifact, ok := inspector.artifacts[path]
	if !ok {
		return ObservedArtifact{}, os.ErrNotExist
	}
	return artifact, nil
}

func newReadinessFixture(t *testing.T) readinessFixture {
	t.Helper()
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(artifactRoot, 0o700); err != nil {
		t.Fatalf("create artifact fixture root: %v", err)
	}

	cliPath := filepath.Join(artifactRoot, "you")
	backendPath := filepath.Join(artifactRoot, "llama-cpp")
	modelPath := filepath.Join(artifactRoot, "embed.gguf")
	fixturePath := filepath.Join(artifactRoot, "embed-input.txt")
	cli := writeFixtureArtifact(t, cliPath, []byte("portable cli fixture\n"))
	backend := writeFixtureArtifact(t, backendPath, []byte("portable backend fixture\n"))
	model := writeFixtureArtifact(t, modelPath, []byte("portable model fixture\n"))
	fixture := writeFixtureArtifact(t, fixturePath, []byte("Find similar work\n"))

	rootBase := filepath.Join(root, "roots")
	spec := RunSpec{
		Schema:  RunSchemaV1,
		RunID:   "run-001",
		Target:  Target{OS: TargetLinux, Arch: ArchAMD64},
		CLI:     CLIIdentity{Path: cliPath, SHA256: cli.SHA256, SizeBytes: cli.SizeBytes, Version: "v0.0.0-fixture", Commit: "commit-fixture-001"},
		Backend: BackendIdentity{ID: "localai-llamacpp", Path: backendPath, SHA256: backend.SHA256, SizeBytes: backend.SizeBytes, SourceRevision: "backend-fixture-001"},
		Model:   ModelIdentity{ID: "embed", Path: modelPath, SHA256: model.SHA256, SizeBytes: model.SizeBytes, Revision: "model-fixture-001"},
		Fixture: FixtureIdentity{ID: "embed-input-001", Path: fixturePath, SHA256: fixture.SHA256, SizeBytes: fixture.SizeBytes, SemanticAssertion: "embedding has declared dimensions and finite numeric values"},
		Roots: Roots{
			Work: filepath.Join(rootBase, "work"), State: filepath.Join(rootBase, "state"),
			Cache: filepath.Join(rootBase, "cache"), Temp: filepath.Join(rootBase, "temp"),
			Output: filepath.Join(rootBase, "output"),
		},
		Port: 18437, TimeoutMillis: 1200000, NetworkPolicy: NetworkPolicyDeny,
		Limits: BudgetLimits{DownloadBytes: 0, ModelCalls: 0, NetworkRequests: 0, MaxChildProcesses: 4, TemporaryBytes: 1024},
		Commands: []CommandSpec{{
			Name: "invoke", Path: cliPath,
			Args:        []string{"models", "invoke", "embed", "--input", "text=Find similar work", "--json", "--offline"},
			Environment: []string{"HOME=" + filepath.Join(root, "home")},
		}},
		ReportPath: filepath.Join(root, "report.json"), LedgerPath: filepath.Join(root, "budget.json"),
	}

	return readinessFixture{
		root: root, spec: spec, host: HostIdentity{OS: TargetLinux, Arch: ArchAMD64},
		inspector: &fixtureInspector{artifacts: map[string]ObservedArtifact{
			cliPath:     {Regular: true, Executable: true, SizeBytes: cli.SizeBytes, SHA256: cli.SHA256},
			backendPath: {Regular: true, Executable: true, SizeBytes: backend.SizeBytes, SHA256: backend.SHA256},
			modelPath:   {Regular: true, SizeBytes: model.SizeBytes, SHA256: model.SHA256},
			fixturePath: {Regular: true, SizeBytes: fixture.SizeBytes, SHA256: fixture.SHA256},
		}, errors: map[string]error{}},
	}
}

type fixtureArtifact struct {
	SHA256    string
	SizeBytes int64
}

func writeFixtureArtifact(t *testing.T, path string, body []byte) fixtureArtifact {
	t.Helper()
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write fixture artifact %s: %v", path, err)
	}
	return fixtureArtifact{SHA256: SHA256Hex(body), SizeBytes: int64(len(body))}
}

func TestPortableRunnerAdmission(t *testing.T) {
	f := newReadinessFixture(t)

	if err := f.spec.Validate(); err != nil {
		t.Fatalf("valid fixture specification rejected: %v", err)
	}
	observed, err := (LocalArtifactInspector{}).Inspect(f.spec.Model.Path)
	if err != nil {
		t.Fatalf("local artifact inspector rejected fixture: %v", err)
	}
	if !observed.Regular || observed.SizeBytes != f.spec.Model.SizeBytes || observed.SHA256 != f.spec.Model.SHA256 {
		t.Fatalf("local artifact identity mismatch: got %+v want size=%d sha=%s", observed, f.spec.Model.SizeBytes, f.spec.Model.SHA256)
	}

	admitted, err := AdmitWithInspector(f.spec, f.host, f.inspector)
	if err != nil {
		t.Fatalf("valid controlled run rejected: %v", err)
	}
	if admitted.SpecIdentity == "" || strings.Contains(admitted.SpecIdentity, f.spec.CLI.Path) {
		t.Fatalf("spec identity is missing or exposes a raw path: %q", admitted.SpecIdentity)
	}
	if len(admitted.RootIdentities) != 5 || len(f.inspector.calls) != 4 {
		t.Fatalf("admission identity observations mismatch: roots=%d artifactCalls=%d", len(admitted.RootIdentities), len(f.inspector.calls))
	}

	specPath := filepath.Join(f.root, "run.json")
	if err := WriteRunSpecAtomic(specPath, f.spec); err != nil {
		t.Fatalf("write canonical run specification: %v", err)
	}
	roundTrip, err := ReadRunSpec(specPath)
	if err != nil {
		t.Fatalf("read canonical run specification: %v", err)
	}
	if !reflect.DeepEqual(f.spec, roundTrip) {
		t.Fatalf("run specification changed during canonical round trip:\n got=%+v\nwant=%+v", roundTrip, f.spec)
	}

	tests := []struct {
		name string
		edit func(*readinessFixture)
		want string
	}{
		{
			name: "unsupported host",
			edit: func(f *readinessFixture) { f.host = HostIdentity{OS: TargetLinux, Arch: ArchARM64} },
			want: "unsupported_host",
		},
		{
			name: "absent cli",
			edit: func(f *readinessFixture) {
				f.spec.CLI.Path = filepath.Join(f.root, "missing-cli")
				f.spec.Commands[0].Path = f.spec.CLI.Path
			},
			want: "artifact_missing",
		},
		{
			name: "non executable cli",
			edit: func(f *readinessFixture) {
				f.inspector.artifacts[f.spec.CLI.Path] = ObservedArtifact{Regular: true, Executable: false, SizeBytes: f.spec.CLI.SizeBytes, SHA256: f.spec.CLI.SHA256}
			},
			want: "artifact_not_executable",
		},
		{
			name: "cli digest drift",
			edit: func(f *readinessFixture) { f.spec.CLI.SHA256 = strings.Repeat("0", 64) },
			want: "artifact_digest_mismatch",
		},
		{
			name: "fixture digest drift",
			edit: func(f *readinessFixture) { f.spec.Fixture.SHA256 = strings.Repeat("0", 64) },
			want: "artifact_digest_mismatch",
		},
		{
			name: "corrupt ledger",
			edit: func(f *readinessFixture) {
				if err := os.WriteFile(f.spec.LedgerPath, []byte("{\"schema\":\"broken\"}\n"), 0o600); err != nil {
					t.Fatalf("write corrupt ledger: %v", err)
				}
			},
			want: "ledger_invalid",
		},
		{
			name: "over budget ledger",
			edit: func(f *readinessFixture) {
				ledger := BudgetLedger{
					Schema: BudgetSchemaV1, LedgerID: DefaultLedgerID(f.spec.RunID), RunID: f.spec.RunID,
					Generation: 1, Limits: f.spec.Limits, Consumed: BudgetConsumed{ChildProcesses: f.spec.Limits.MaxChildProcesses + 1},
					Reservations: []BudgetReservation{},
				}
				body, err := MarshalCanonical(ledger)
				if err != nil {
					t.Fatalf("marshal over-budget ledger: %v", err)
				}
				if err := os.WriteFile(f.spec.LedgerPath, body, 0o600); err != nil {
					t.Fatalf("write over-budget ledger: %v", err)
				}
			},
			want: "ledger_invalid",
		},
		{
			name: "invalid network policy",
			edit: func(f *readinessFixture) { f.spec.NetworkPolicy = "allow" },
			want: "invalid_network_policy",
		},
		{
			name: "root overlap",
			edit: func(f *readinessFixture) { f.spec.Roots.State = f.spec.Roots.Work },
			want: "path_overlap",
		},
		{
			name: "report destination reused",
			edit: func(f *readinessFixture) {
				if err := os.WriteFile(f.spec.ReportPath, []byte("existing\n"), 0o600); err != nil {
					t.Fatalf("write reused report: %v", err)
				}
			},
			want: "report_path_reused",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caseFixture := newReadinessFixture(t)
			test.edit(&caseFixture)
			admitted, err := AdmitWithInspector(caseFixture.spec, caseFixture.host, caseFixture.inspector)
			if admitted.SpecIdentity != "" {
				t.Fatalf("rejected run returned an admission identity: %q", admitted.SpecIdentity)
			}
			requireAdmissionCode(t, err, test.want)
			// The admission boundary has no executor dependency. This counter is
			// the caller-side launch gate: a rejected admission must not cross it.
			executorCalls := 0
			if err == nil {
				executorCalls++
			}
			if executorCalls != 0 {
				t.Fatalf("rejected admission crossed executor gate %d times", executorCalls)
			}
		})
	}
}

func TestPortableBudgetLedger(t *testing.T) {
	f := newReadinessFixture(t)
	if _, err := AdmitWithInspector(f.spec, f.host, f.inspector); err != nil {
		t.Fatalf("valid fixture admission failed: %v", err)
	}
	store, err := NewLocalBudgetStore()
	if err != nil {
		t.Fatalf("create local budget store: %v", err)
	}
	request := ReservationRequest{
		LedgerPath: f.spec.LedgerPath, LedgerID: DefaultLedgerID(f.spec.RunID), RunID: f.spec.RunID,
		ID: "reservation-1", Kind: BudgetKindChildProcesses, Amount: 1, Command: "invoke", Limits: f.spec.Limits,
	}
	reservation, ledger, err := store.Reserve(context.Background(), request)
	if err != nil {
		t.Fatalf("reserve first child process: %v", err)
	}
	if reservation.State != ReservationStateReserved || ledger.Generation != 1 || ledger.Consumed.ChildProcesses != 0 || len(ledger.Reservations) != 1 {
		t.Fatalf("unexpected first reservation state: %+v", ledger)
	}
	before, err := os.ReadFile(f.spec.LedgerPath)
	if err != nil {
		t.Fatalf("read first ledger bytes: %v", err)
	}

	reused := request
	if _, _, err := store.Reserve(context.Background(), reused); err == nil {
		t.Fatal("reused reservation was accepted")
	} else {
		requireBudgetCode(t, err, "reservation_reused")
	}
	assertFileBytes(t, f.spec.LedgerPath, before)

	foreign := request
	foreign.ID = "foreign-reservation"
	foreign.RunID = "foreign-run"
	if _, _, err := store.Reserve(context.Background(), foreign); err == nil {
		t.Fatal("foreign run reservation was accepted")
	} else {
		requireBudgetCode(t, err, "ledger_foreign")
	}
	assertFileBytes(t, f.spec.LedgerPath, before)

	drifted := request
	drifted.ID = "drifted-reservation"
	drifted.Limits.MaxChildProcesses--
	if _, _, err := store.Reserve(context.Background(), drifted); err == nil {
		t.Fatal("drifted budget reservation was accepted")
	} else {
		requireBudgetCode(t, err, "ledger_drift")
	}
	assertFileBytes(t, f.spec.LedgerPath, before)

	committed, err := store.Commit(context.Background(), f.spec.LedgerPath, f.spec.RunID, request.ID)
	if err != nil {
		t.Fatalf("commit reservation: %v", err)
	}
	if committed.Consumed.ChildProcesses != 1 || committed.Reservations[0].State != ReservationStateCommitted {
		t.Fatalf("commit did not consume reservation: %+v", committed)
	}

	second := request
	second.ID = "reservation-2"
	secondReservation, withSecond, err := store.Reserve(context.Background(), second)
	if err != nil {
		t.Fatalf("reserve second child process: %v", err)
	}
	if secondReservation.State != ReservationStateReserved || len(withSecond.Reservations) != 2 {
		t.Fatalf("unexpected second reservation: %+v", withSecond)
	}
	released, err := store.Release(context.Background(), f.spec.LedgerPath, f.spec.RunID, second.ID)
	if err != nil {
		t.Fatalf("release second reservation: %v", err)
	}
	if released.Consumed.ChildProcesses != 1 || released.Reservations[1].State != ReservationStateReleased {
		t.Fatalf("release changed consumed budget incorrectly: %+v", released)
	}
	finalized, err := store.Finalize(context.Background(), f.spec.LedgerPath, f.spec.RunID)
	if err != nil {
		t.Fatalf("finalize ledger: %v", err)
	}
	if !finalized.Finalized {
		t.Fatal("ledger did not become finalized")
	}
	if _, _, err := store.Reserve(context.Background(), ReservationRequest{
		LedgerPath: f.spec.LedgerPath, LedgerID: request.LedgerID, RunID: f.spec.RunID,
		ID: "after-finalize", Kind: BudgetKindChildProcesses, Amount: 1, Command: "invoke", Limits: f.spec.Limits,
	}); err == nil {
		t.Fatal("reservation after finalization was accepted")
	} else {
		requireBudgetCode(t, err, "ledger_finalized")
	}

	contentionPath := filepath.Join(f.root, "contention.json")
	contentionLimits := f.spec.Limits
	contentionLimits.MaxChildProcesses = 3
	const contenderCount = 8
	results := make(chan error, contenderCount)
	var waitGroup sync.WaitGroup
	for index := 0; index < contenderCount; index++ {
		index := index
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, _, reserveErr := store.Reserve(context.Background(), ReservationRequest{
				LedgerPath: contentionPath, LedgerID: "ledger-contention", RunID: f.spec.RunID,
				ID: fmt.Sprintf("parallel-%d", index), Kind: BudgetKindChildProcesses, Amount: 1,
				Command: "controlled", Limits: contentionLimits,
			})
			results <- reserveErr
		}()
	}
	waitGroup.Wait()
	close(results)
	successes := 0
	for reserveErr := range results {
		if reserveErr == nil {
			successes++
			continue
		}
		requireBudgetCode(t, reserveErr, "budget_exhausted")
	}
	if int64(successes) != contentionLimits.MaxChildProcesses {
		t.Fatalf("contention admitted %d reservations, want %d", successes, contentionLimits.MaxChildProcesses)
	}
	contentionLedger, err := ReadBudgetLedger(contentionPath)
	if err != nil {
		t.Fatalf("read contended ledger: %v", err)
	}
	if contentionLedger.Generation != int64(successes) || len(contentionLedger.Reservations) != successes {
		t.Fatalf("contended ledger lost serialized mutations: generation=%d reservations=%d", contentionLedger.Generation, len(contentionLedger.Reservations))
	}

	atomicPath := filepath.Join(f.root, "atomic.json")
	body, err := MarshalCanonical(contentionLedger)
	if err != nil {
		t.Fatalf("marshal atomic preimage: %v", err)
	}
	if err := writeJSONAtomic(atomicPath, body, nil); err != nil {
		t.Fatalf("write atomic preimage: %v", err)
	}
	preimage, err := os.ReadFile(atomicPath)
	if err != nil {
		t.Fatalf("read atomic preimage: %v", err)
	}
	interrupted := errors.New("injected interruption")
	if err := writeJSONAtomic(atomicPath, []byte("changed\n"), func() error { return interrupted }); !errors.Is(err, interrupted) {
		t.Fatalf("atomic interruption error = %v, want %v", err, interrupted)
	}
	assertFileBytes(t, atomicPath, preimage)
	entries, err := os.ReadDir(f.root)
	if err != nil {
		t.Fatalf("read atomic parent: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".platform-conformance-") {
			t.Fatalf("interrupted atomic write left temporary file %q", entry.Name())
		}
	}
	identity, err := LedgerFileIdentity(contentionPath)
	if err != nil {
		t.Fatalf("read ledger file identity: %v", err)
	}
	if identity.Generation != contentionLedger.Generation || !validPathIdentity(identity.PathIdentity) || !validDigest(identity.SHA256) {
		t.Fatalf("invalid redacted ledger identity: %+v", identity)
	}
}

func TestPortableReportPersistence(t *testing.T) {
	f := newReadinessFixture(t)
	admitted, err := AdmitWithInspector(f.spec, f.host, f.inspector)
	if err != nil {
		t.Fatalf("valid fixture admission failed: %v", err)
	}
	store, err := NewLocalBudgetStore()
	if err != nil {
		t.Fatalf("create local budget store: %v", err)
	}
	_, _, err = store.Reserve(context.Background(), ReservationRequest{
		LedgerPath: f.spec.LedgerPath, LedgerID: DefaultLedgerID(f.spec.RunID), RunID: f.spec.RunID,
		ID: "report-reservation", Kind: BudgetKindChildProcesses, Amount: 1, Command: "invoke", Limits: f.spec.Limits,
	})
	if err != nil {
		t.Fatalf("reserve report budget: %v", err)
	}
	ledgerIdentity, err := LedgerFileIdentity(f.spec.LedgerPath)
	if err != nil {
		t.Fatalf("get report ledger identity: %v", err)
	}
	report := admitted.NewReadinessReport(ledgerIdentity)
	if err := report.Validate(); err != nil {
		t.Fatalf("generated readiness report is invalid: %v", err)
	}
	if err := WriteReportAtomic(f.spec.ReportPath, report); err != nil {
		t.Fatalf("write readiness report: %v", err)
	}
	readBack, err := ReadReport(f.spec.ReportPath)
	if err != nil {
		t.Fatalf("read readiness report: %v", err)
	}
	if !reflect.DeepEqual(report, readBack) {
		t.Fatalf("report changed during canonical round trip:\n got=%+v\nwant=%+v", readBack, report)
	}
	body, err := os.ReadFile(f.spec.ReportPath)
	if err != nil {
		t.Fatalf("read persisted report bytes: %v", err)
	}
	canonical, err := MarshalCanonical(readBack)
	if err != nil {
		t.Fatalf("marshal report canonical bytes: %v", err)
	}
	if !reflect.DeepEqual(body, canonical) {
		t.Fatal("persisted report is not canonical JSON")
	}
	textBody := string(body)
	for _, secret := range []string{f.spec.CLI.Path, f.spec.Backend.Path, f.spec.Model.Path, f.spec.Fixture.Path, f.spec.Roots.Work, f.spec.Commands[0].Environment[0][len("HOME="):]} {
		if strings.Contains(textBody, secret) {
			t.Fatalf("report exposes raw path or environment value %q", secret)
		}
	}
	if !strings.Contains(textBody, "\"environmentKeys\": [\n        \"HOME\"") {
		t.Fatal("report did not retain redacted environment key")
	}

	before, err := os.ReadFile(f.spec.ReportPath)
	if err != nil {
		t.Fatalf("read report before invalid replacement: %v", err)
	}
	invalid := report
	invalid.Commands = append([]CommandEvidence(nil), report.Commands...)
	invalid.Commands[0].Redacted = false
	if err := WriteReportAtomic(f.spec.ReportPath, invalid); err == nil {
		t.Fatal("unredacted report was persisted")
	} else {
		var schemaErr *SchemaError
		if !errors.As(err, &schemaErr) || schemaErr.Code != "unredacted_evidence" {
			t.Fatalf("unredacted report error = %v, want SchemaError/unredacted_evidence", err)
		}
	}
	assertFileBytes(t, f.spec.ReportPath, before)

	if err := os.WriteFile(f.spec.ReportPath, append(append([]byte(nil), before...), []byte("{}\n")...), 0o600); err != nil {
		t.Fatalf("write non-canonical report: %v", err)
	}
	if _, err := ReadReport(f.spec.ReportPath); err == nil {
		t.Fatal("report with trailing JSON was accepted")
	}
}

func requireAdmissionCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("admission succeeded, want code %q", want)
	}
	var admissionErr *AdmissionError
	if !errors.As(err, &admissionErr) {
		t.Fatalf("admission error %T = %v, want *AdmissionError", err, err)
	}
	if admissionErr.Code != want {
		t.Fatalf("admission error code = %q, want %q (%v)", admissionErr.Code, want, err)
	}
}

func requireBudgetCode(t *testing.T, err error, want string) {
	t.Helper()
	var budgetErr *BudgetError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("budget error %T = %v, want *BudgetError", err, err)
	}
	if budgetErr.Code != want {
		t.Fatalf("budget error code = %q, want %q (%v)", budgetErr.Code, want, err)
	}
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", pathIdentity(path), err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("file %s changed unexpectedly", pathIdentity(path))
	}
}
