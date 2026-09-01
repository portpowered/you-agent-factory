package inference_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type wsrFT006Case struct {
	name                    string
	provider                modelprovider.Provider
	fixtureProvider         string
	fixture                 string
	wantDelta               bool
	wantSnapshot            bool
	wantFinalOnly           bool
	wantNoProviderSessionID bool
}

// TestWSRFT006PortableWorkerRecordingParity proves the portable Worker
// contract through the root-built application path. Each fixture is loaded by
// the sanitized provider-session harness, selected by a production
// MODEL_WORKER registration, and executed only through the injected provider
// command-runner edge.
//
// WSR-FT-006: portable round-trip parity across streaming, snapshot, and
// final-only fidelity, including a provider history without a Provider Session
// reference and rejection of tampered ordering and overstated fidelity.
func TestWSRFT006PortableWorkerRecordingParity(t *testing.T) {
	t.Parallel()
	cases := []wsrFT006Case{
		{
			name:            "streaming",
			provider:        modelprovider.ProviderClaude,
			fixtureProvider: "claude",
			fixture:         "full-stream-text-success",
			wantDelta:       true,
			wantSnapshot:    true,
		},
		{
			name:            "snapshot",
			provider:        modelprovider.ProviderCodex,
			fixtureProvider: "codex",
			fixture:         "success",
			wantSnapshot:    true,
		},
		{
			name:                    "final-only-without-provider-session",
			provider:                modelprovider.ProviderAntigravity,
			fixtureProvider:         "agy",
			fixture:                 "final-only-success",
			wantFinalOnly:           true,
			wantNoProviderSessionID: true,
		},
	}

	portableByClass := make([]recordings.WorkerPortableRecording, len(cases))
	t.Run("portable fidelity cases", func(t *testing.T) {
		for index, testCase := range cases {
			index, testCase := index, testCase
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()
				portableByClass[index] = runWSRFT006Case(t, testCase)
			})
		}
	})

	streaming := portableByClass[0]
	assertWSRFT006OrderingTamperRejected(t, streaming)
	assertWSRFT006FidelityTamperRejected(t, streaming)
}

// TestWSRFT006PortableExportSelectsRootBuiltWorkerSession proves that one
// explicit Factory Session recording can contain more than one Worker Session,
// and that a later Factory Session gets a fresh durable recording identity.
// Both sessions use one reusable root-built host and the same writer.
func TestWSRFT006PortableExportSelectsRootBuiltWorkerSession(t *testing.T) {
	t.Parallel()
	loaded := loadOpeningRecordFixture(t, "codex", "success")
	firstDir := wsrFT006Factory(t, modelprovider.ProviderCodex, loaded)
	support.ClearSeedInputs(t, firstDir)
	secondDir := wsrFT006Factory(t, modelprovider.ProviderCodex, loaded)
	support.ClearSeedInputs(t, secondDir)
	probe := newWSRFT004RecordingProbe(t, false)
	runner := newWSRFT004ProviderRunner(t, probe)

	queueWSRFT006ProviderResult(t, loaded, runner)
	queueWSRFT006ProviderResult(t, loaded, runner)
	runSharedInferenceFactory(t, firstDir, sharedInferenceScenario{
		commandRunner:         runner,
		workerRecordingWriter: probe,
		submittedWorks: sharedInferenceWorks(
			"WSR-FT-006 first Worker Session",
			"WSR-FT-006 second Worker Session",
		),
	}, sharedInferenceScenarioTimeout)
	reader := recordings.WorkerRecordingReader(probe)
	firstRecordingID, _ := probe.RecordingIdentity(t)
	firstSnapshot, err := reader.LoadWorkerRecording(t.Context(), firstRecordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording(%q) error = %v", firstRecordingID, err)
	}
	if len(firstSnapshot.Sessions) != 2 {
		summary := make([]string, len(firstSnapshot.Sessions))
		for index, session := range firstSnapshot.Sessions {
			summary[index] = fmt.Sprintf("%s/%s/%d", session.WorkerSessionID, session.Status, len(session.Records))
		}
		t.Fatalf("durable multi-session snapshot contains %d sessions, want two: %v", len(firstSnapshot.Sessions), summary)
	}
	if _, err := (recordings.WorkerRecordingCodec{}).ExportWorkerPortableRecording(firstSnapshot); !errors.Is(err, recordings.ErrWorkerPortableRecordingIdentity) {
		t.Fatalf("multi-session export without selector = %v, want identity diagnostic", err)
	}
	selected := firstSnapshot.Sessions[1].WorkerSessionID
	portable, err := (recordings.WorkerRecordingCodec{}).ExportWorkerPortableRecording(firstSnapshot, selected)
	if err != nil {
		t.Fatalf("ExportWorkerPortableRecording(%q) error = %v", selected, err)
	}
	if portable.Identity.RecordingID != firstRecordingID || portable.Identity.WorkerSessionID != selected {
		t.Fatalf("portable identity = %#v, want recording %q and Worker Session %q", portable.Identity, firstRecordingID, selected)
	}

	queueWSRFT006ProviderResult(t, loaded, runner)
	runSharedInferenceFactory(t, secondDir, sharedInferenceScenario{
		commandRunner:         runner,
		workerRecordingWriter: probe,
		submittedWork:         sharedInferenceWork("WSR-FT-006 later Factory Session"),
	}, sharedInferenceScenarioTimeout)
	secondRecordingID, _ := probe.RecordingIdentity(t)
	if secondRecordingID == firstRecordingID {
		t.Fatalf("later Factory Session recording identity = %q, want a fresh identity", secondRecordingID)
	}
	secondSnapshot, err := reader.LoadWorkerRecording(t.Context(), secondRecordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording(%q) error = %v", secondRecordingID, err)
	}
	if len(secondSnapshot.Sessions) != 1 {
		t.Fatalf("later Factory Session snapshot contains %d Worker Sessions, want one", len(secondSnapshot.Sessions))
	}
	for _, session := range secondSnapshot.Sessions {
		for _, first := range firstSnapshot.Sessions {
			if session.WorkerSessionID == first.WorkerSessionID {
				t.Fatalf("later recording inherited Worker Session %q from recording %q", session.WorkerSessionID, firstRecordingID)
			}
		}
	}
}

func runWSRFT006Case(t *testing.T, testCase wsrFT006Case) recordings.WorkerPortableRecording {
	t.Helper()
	loaded := loadOpeningRecordFixture(t, testCase.fixtureProvider, testCase.fixture)
	dir := wsrFT006Factory(t, testCase.provider, loaded)
	probe := newWSRFT004RecordingProbe(t, false)
	runner := newWSRFT004ProviderRunner(t, probe)
	reader := runWSRFT006FactoryWithSharedProcess(t, dir, loaded, runner, probe)
	if runner.CallCount() != 1 {
		t.Fatalf("%s provider command calls = %d, want one", testCase.name, runner.CallCount())
	}

	recordingID, workerSessionID := probe.RecordingIdentity(t)
	snapshot, err := reader.LoadWorkerRecording(t.Context(), recordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording(%q) error = %v", recordingID, err)
	}
	live := probe.LiveProjection(t)
	portable, err := (recordings.WorkerRecordingCodec{}).ExportWorkerPortableRecording(snapshot)
	if err != nil {
		t.Fatalf("ExportWorkerPortableRecording(%q) error = %v", recordingID, err)
	}
	if portable.Identity.WorkerSessionID != workerSessionID {
		t.Fatalf("portable Worker Session ID = %q, want %q", portable.Identity.WorkerSessionID, workerSessionID)
	}
	if !strings.EqualFold(portable.Provider.Provider, string(testCase.provider)) {
		t.Fatalf("portable provider = %q, want %q", portable.Provider.Provider, testCase.provider)
	}
	assertWSRFT006Fidelity(t, portable, testCase)
	assertWSRFT006PortableRoundTrip(t, portable, live, runner)
	if testCase.wantNoProviderSessionID {
		assertWSRFT006NoProviderSessionReference(t, portable)
	}
	return portable
}

func wsrFT006Factory(
	t *testing.T,
	provider modelprovider.Provider,
	loaded support.ProviderSessionCase,
) string {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.ClearSeedInputs(t, dir)
	config := support.BuildModelWorkerConfig(provider, loaded.Process.Model)
	config = sharedInferenceWithExecutorProvider(config, strings.ToUpper(string(provider)))
	if provider == modelprovider.ProviderClaude || provider == modelprovider.ProviderAntigravity {
		config = strings.Replace(config, "stopToken: COMPLETE", "skipPermissions: true\nstopToken: COMPLETE", 1)
	}
	support.WriteAgentConfig(t, dir, "worker", config)
	testutil.WriteSeedFile(t, dir, "task", []byte(fmt.Sprintf(`{"title":"WSR-FT-006 %s"}`, provider)))
	return dir
}

func runWSRFT006FactoryWithSharedProcess(
	t *testing.T,
	dir string,
	loaded support.ProviderSessionCase,
	runner *wsrFT004ProviderRunner,
	probe *wsrFT004RecordingProbe,
) recordings.WorkerRecordingReader {
	t.Helper()
	support.ClearSeedInputs(t, dir)
	queueWSRFT006ProviderResult(t, loaded, runner)
	runSharedInferenceFactory(t, dir, sharedInferenceScenario{
		commandRunner:         runner,
		workerRecordingWriter: probe,
		submittedWork:         sharedInferenceWork("WSR-FT-006 portable recording parity"),
	}, sharedInferenceScenarioTimeout)
	return probe
}

func queueWSRFT006ProviderResult(
	t *testing.T,
	loaded support.ProviderSessionCase,
	runner *wsrFT004ProviderRunner,
) {
	t.Helper()
	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	runner.delegate.Queue(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})
}

func assertWSRFT006Fidelity(t *testing.T, portable recordings.WorkerPortableRecording, testCase wsrFT006Case) {
	t.Helper()
	var gotDelta, gotSnapshot, gotFinalOnly bool
	for _, record := range portable.Records {
		switch record.Provenance.Delivery {
		case workers.DeliveryNativeFinal:
			gotFinalOnly = true
		case workers.DeliveryNativeStream:
			switch {
			case record.Phase == workers.PhaseDelta || record.Provenance.NativeEventType == "message.delta":
				gotDelta = true
			case record.Provenance.NativeEventType == "message.completed":
				gotSnapshot = true
			}
		}
	}
	if gotDelta != testCase.wantDelta || gotSnapshot != testCase.wantSnapshot || gotFinalOnly != testCase.wantFinalOnly {
		t.Fatalf("portable fidelity = delta:%t snapshot:%t finalOnly:%t, want delta:%t snapshot:%t finalOnly:%t", gotDelta, gotSnapshot, gotFinalOnly, testCase.wantDelta, testCase.wantSnapshot, testCase.wantFinalOnly)
	}
}

func assertWSRFT006PortableRoundTrip(
	t *testing.T,
	portable recordings.WorkerPortableRecording,
	live recordings.WorkerRecordingProjection,
	runner *wsrFT004ProviderRunner,
) {
	t.Helper()
	providerCallsBefore := runner.CallCount()
	encoded, err := (recordings.WorkerRecordingCodec{}).EncodeWorkerPortableRecording(portable)
	if err != nil {
		t.Fatalf("EncodeWorkerPortableRecording() error = %v", err)
	}
	decoded, err := (recordings.WorkerRecordingCodec{}).DecodeWorkerPortableRecording(encoded)
	if err != nil {
		t.Fatalf("DecodeWorkerPortableRecording() error = %v", err)
	}
	if !reflect.DeepEqual(portable, decoded) {
		t.Fatalf("decoded portable recording differs from export:\nexport=%#v\ndecoded=%#v", portable, decoded)
	}
	replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerPortableRecording(decoded)
	if err != nil {
		t.Fatalf("ReplayWorkerPortableRecording() error = %v", err)
	}
	if !reflect.DeepEqual(live, replayed.Projection) {
		t.Fatalf("live projection differs from portable replay:\nlive=%#v\nreplay=%#v", live, replayed.Projection)
	}
	if !replayed.Projection.Complete || replayed.Projection.Terminal == nil {
		t.Fatalf("portable replay projection = %#v, want completed terminal history", replayed.Projection)
	}
	last := replayed.Projection.Records[len(replayed.Projection.Records)-1]
	if replayed.Projection.Terminal.Position != last.ID.Position {
		t.Fatalf("portable terminal position = %d, last record position = %d; terminal must be last", replayed.Projection.Terminal.Position, last.ID.Position)
	}
	if providerCallsBefore != 1 {
		t.Fatalf("provider calls before portable replay = %d, want one", providerCallsBefore)
	}
	if providerCallsAfter := runner.CallCount(); providerCallsAfter != providerCallsBefore {
		t.Fatalf("portable replay changed provider calls from %d to %d", providerCallsBefore, providerCallsAfter)
	}
}

func assertWSRFT006NoProviderSessionReference(t *testing.T, portable recordings.WorkerPortableRecording) {
	t.Helper()
	if portable.Provider.ProviderSessionRef != "" {
		t.Fatalf("portable Provider Session reference = %q, want absent", portable.Provider.ProviderSessionRef)
	}
	for index, record := range portable.Records {
		if record.ProviderSessionRef != "" {
			t.Fatalf("portable record[%d] Provider Session reference = %q, want absent", index, record.ProviderSessionRef)
		}
	}
}

func assertWSRFT006OrderingTamperRejected(t *testing.T, portable recordings.WorkerPortableRecording) {
	t.Helper()
	tampered := cloneWSRFT006Portable(portable)
	if len(tampered.Records) < 2 {
		t.Fatal("streaming portable recording has fewer than two records")
	}
	tampered.Records[1].Position = 3
	_, err := (recordings.WorkerRecordingCodec{}).DecodeWorkerPortableRecording(marshalWSRFT006JSON(t, tampered))
	assertWSRFT006Diagnostic(t, err, recordings.WorkerPortableCodeInvalidOrder)
}

func assertWSRFT006FidelityTamperRejected(t *testing.T, portable recordings.WorkerPortableRecording) {
	t.Helper()
	tampered := cloneWSRFT006Portable(portable)
	index := -1
	for candidate, record := range tampered.Records {
		if record.Provenance.Delivery == workers.DeliveryNativeStream &&
			(record.Phase == workers.PhaseDelta || record.Provenance.NativeEventType == "message.delta") {
			index = candidate
			break
		}
	}
	if index < 0 {
		t.Fatal("streaming portable recording has no delta record to tamper")
	}
	var draft workers.Draft
	if err := json.Unmarshal(tampered.Records[index].Payload, &draft); err != nil {
		t.Fatalf("decode streaming Worker draft: %v", err)
	}
	draft.Provenance.Fidelity = workers.FidelityFinalOnly
	tampered.Records[index].Payload = marshalWSRFT006JSON(t, draft)
	tampered.Records[index].Provenance = draft.Provenance
	_, err := (recordings.WorkerRecordingCodec{}).DecodeWorkerPortableRecording(marshalWSRFT006JSON(t, tampered))
	assertWSRFT006Diagnostic(t, err, recordings.WorkerPortableCodeInvalidFidelity)
}

func assertWSRFT006Diagnostic(t *testing.T, err error, want recordings.WorkerPortableRecordingDiagnosticCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("portable tamper validation error = nil, want %s", want)
	}
	var diagnostic *recordings.WorkerPortableRecordingDiagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("portable tamper error = %v, want actionable diagnostic", err)
	}
	if diagnostic.Code != want || diagnostic.Path == "" || diagnostic.Message == "" {
		t.Fatalf("portable diagnostic = %#v, want code %s with path and message", diagnostic, want)
	}
}

func cloneWSRFT006Portable(portable recordings.WorkerPortableRecording) recordings.WorkerPortableRecording {
	clone := portable
	clone.Records = make([]recordings.WorkerPortableRecord, len(portable.Records))
	for index, record := range portable.Records {
		clone.Records[index] = record
		clone.Records[index].Payload = append(json.RawMessage(nil), record.Payload...)
	}
	return clone
}

func marshalWSRFT006JSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal WSR-FT-006 JSON: %v", err)
	}
	return payload
}
