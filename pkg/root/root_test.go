package root

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	"github.com/spf13/cobra"

	"github.com/portpowered/infinite-you/internal/testutil"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	inference "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestMain(m *testing.M) {
	// Process tests execute Cobra commands in-process. Explorer launch behavior
	// is outside this package's contract, and its Windows process scan dominates
	// the cost of repeated Execute calls.
	cobra.MousetrapHelpText = ""
	m.Run()
}

func TestBuildProcessPreservesCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	process, err := BuildProcess(ctx, serviceedges.Edges{})
	if process != nil {
		t.Fatal("BuildProcess() returned a process for a canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BuildProcess() error = %v, want context.Canceled", err)
	}
}

func TestBuildProcessConstructionFailureDoesNotStartExternalLifecycle(t *testing.T) {
	t.Parallel()

	apiStarts := 0
	process, err := BuildProcess(nil, serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return nil
		},
	})
	if process != nil {
		t.Fatal("BuildProcess() returned a process for a missing context")
	}
	if err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("BuildProcess() error = %v, want required-context diagnostic", err)
	}
	if apiStarts != 0 {
		t.Fatalf("construction failure started API lifecycle %d times, want zero", apiStarts)
	}
}

func TestBuildStatelessWorkersValidatesContextBeforeComposition(t *testing.T) {
	t.Parallel()

	if service, err := BuildStatelessWorkers(nil, serviceedges.Edges{}); service != nil ||
		err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("BuildStatelessWorkers(nil) = (%#v, %v), want required-context failure", service, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if service, err := BuildStatelessWorkers(ctx, serviceedges.Edges{}); service != nil ||
		!errors.Is(err, context.Canceled) {
		t.Fatalf("BuildStatelessWorkers(canceled) = (%#v, %v), want context.Canceled", service, err)
	}
}

func TestBuildStatelessWorkersComposesAndPropagatesProviderValidation(t *testing.T) {
	t.Parallel()

	service, err := BuildStatelessWorkers(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildStatelessWorkers() error = %v", err)
	}
	if service == nil {
		t.Fatal("BuildStatelessWorkers() returned nil service")
	}

	_, err = BuildStatelessWorkers(context.Background(), serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{{
			Manifest:    rootExternalManifest(t, "claude", "collision"),
			Integration: &rootRecordingIntegration{identity: "claude"},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "provider registry validation failed") {
		t.Fatalf("BuildStatelessWorkers(invalid provider) error = %v, want provider validation failure", err)
	}
}

func TestBuildMockStatelessWorkersValidatesContextBeforeComposition(t *testing.T) {
	t.Parallel()

	mockWorkers := workers.NewEmptyMockWorkersConfig()
	if service, err := BuildMockStatelessWorkers(nil, serviceedges.Edges{}, mockWorkers); service != nil ||
		err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("BuildMockStatelessWorkers(nil) = (%#v, %v), want required-context failure", service, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if service, err := BuildMockStatelessWorkers(ctx, serviceedges.Edges{}, mockWorkers); service != nil ||
		!errors.Is(err, context.Canceled) {
		t.Fatalf("BuildMockStatelessWorkers(canceled) = (%#v, %v), want context.Canceled", service, err)
	}
}

func TestBuildMockStatelessWorkersComposesAndPropagatesProviderValidation(t *testing.T) {
	t.Parallel()

	service, err := BuildMockStatelessWorkers(
		context.Background(),
		serviceedges.Edges{},
		workers.NewEmptyMockWorkersConfig(),
	)
	if err != nil {
		t.Fatalf("BuildMockStatelessWorkers() error = %v", err)
	}
	if service == nil {
		t.Fatal("BuildMockStatelessWorkers() returned nil service")
	}

	_, err = BuildMockStatelessWorkers(
		context.Background(),
		serviceedges.Edges{
			ProviderRegistrations: []inference.Registration{{
				Manifest:    rootExternalManifest(t, "claude", "collision"),
				Integration: &rootRecordingIntegration{identity: "claude"},
			}},
		},
		workers.NewEmptyMockWorkersConfig(),
	)
	if err == nil || !strings.Contains(err.Error(), "provider registry validation failed") {
		t.Fatalf("BuildMockStatelessWorkers(invalid provider) error = %v, want provider validation failure", err)
	}
}

func TestWorkerRecordingReaderFromProcessUsesComposedReader(t *testing.T) {
	t.Parallel()

	if reader := WorkerRecordingReaderFromProcess(nil); reader != nil {
		t.Fatalf("WorkerRecordingReaderFromProcess(nil) = %#v, want nil", reader)
	}
	if operations := DetachedOperationsFromProcess(nil); operations != nil {
		t.Fatalf("DetachedOperationsFromProcess(nil) = %#v, want nil", operations)
	}
	processWithoutReader, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil, nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewProcess(without reader) error = %v", err)
	}
	if reader := WorkerRecordingReaderFromProcess(processWithoutReader); reader != nil {
		t.Fatalf("WorkerRecordingReaderFromProcess(without reader) = %#v, want nil", reader)
	}
	if operations := DetachedOperationsFromProcess(processWithoutReader); operations != nil {
		t.Fatalf("DetachedOperationsFromProcess(without capability) = %#v, want nil", operations)
	}
	if query := RuntimeMetricsQueryFromProcess(processWithoutReader); query != nil {
		t.Fatalf("RuntimeMetricsQueryFromProcess(without capability) = %#v, want nil", query)
	}

	readerWriter := &rootWorkerRecordingReaderProbe{}
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		WorkerRecordingWriter: readerWriter,
	})
	if err != nil {
		t.Fatalf("BuildProcess(reader writer) error = %v", err)
	}
	got := WorkerRecordingReaderFromProcess(process)
	if got == nil {
		t.Fatal("WorkerRecordingReaderFromProcess() returned nil")
	}
	if snapshot, err := got.LoadWorkerRecording(t.Context(), "root-recording"); err != nil {
		t.Fatalf("WorkerRecordingReaderFromProcess.LoadWorkerRecording() error = %v", err)
	} else if snapshot.RecordingID != "" || len(snapshot.Sessions) != 0 {
		t.Fatalf("WorkerRecordingReaderFromProcess snapshot = %#v, want empty snapshot", snapshot)
	}
	if operations := DetachedOperationsFromProcess(process); operations == nil {
		t.Fatal("DetachedOperationsFromProcess(composed process) returned nil, want the composed view")
	}
	writeOnlyProcess, err := BuildProcess(context.Background(), serviceedges.Edges{
		WorkerRecordingWriter: recordings.WorkerRecordingWriterFunc(func(context.Context, recordings.WorkerRecordingRecord) error {
			return nil
		}),
	})
	if writeOnlyProcess != nil {
		t.Fatal("BuildProcess(write-only writer) returned a process")
	}
	if !errors.Is(err, recordings.ErrMissingWorkerRecordingReader) {
		t.Fatalf("BuildProcess(write-only writer) error = %v, want ErrMissingWorkerRecordingReader", err)
	}
}

func TestDetachedOperationsFromProcessResolvesTypedCapability(t *testing.T) {
	t.Parallel()

	want := &factorysessions.DetachedOperations{}
	process, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil, nil,
		rootDetachedOperationsCapabilityProbe{operations: want},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewProcess(detached capability) error = %v", err)
	}
	if got := DetachedOperationsFromProcess(process); got != want {
		t.Fatalf("DetachedOperationsFromProcess() = %#v, want %#v", got, want)
	}

	wrongType, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil, nil,
		rootDetachedOperationsCapabilityProbe{operations: struct{}{}},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewProcess(wrong-type capability) error = %v", err)
	}
	if got := DetachedOperationsFromProcess(wrongType); got != nil {
		t.Fatalf("DetachedOperationsFromProcess(wrong type) = %#v, want nil", got)
	}
}

func TestProcessCapabilityRootsReifyOpaqueValuesAtTheRootBoundary(t *testing.T) {
	t.Parallel()

	if got := RecordingsProjectionFromProcess(nil); got != nil {
		t.Fatalf("RecordingsProjectionFromProcess(nil) = %#v, want nil", got)
	}
	if got := OperatorSettingsFromProcess(nil); got != nil {
		t.Fatalf("OperatorSettingsFromProcess(nil) = %#v, want nil", got)
	}

	withoutCapabilities, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewProcess() error = %v", err)
	}
	if got := RecordingsProjectionFromProcess(withoutCapabilities); got != nil {
		t.Fatalf("RecordingsProjectionFromProcess(without capability) = %#v, want nil", got)
	}
	if got := OperatorSettingsFromProcess(withoutCapabilities); got != nil {
		t.Fatalf("OperatorSettingsFromProcess(without capability) = %#v, want nil", got)
	}

	wrongType, err := initializerapplication.NewProcessWithRuntimeCostsAndExecutionAndCapabilities(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		rootRecordingsProjectionCapabilityProbe{value: struct{}{}},
		rootOperatorSettingsCapabilityProbe{value: struct{}{}},
	)
	if err != nil {
		t.Fatalf("NewProcessWithRuntimeCostsAndExecutionAndCapabilities() error = %v", err)
	}
	if got := RecordingsProjectionFromProcess(wrongType); got != nil {
		t.Fatalf("RecordingsProjectionFromProcess(wrong type) = %#v, want nil", got)
	}
	if got := OperatorSettingsFromProcess(wrongType); got != nil {
		t.Fatalf("OperatorSettingsFromProcess(wrong type) = %#v, want nil", got)
	}

	typedProjection := &rootRecordingsProjectionProbe{}
	typedProcess, err := initializerapplication.NewProcessWithRuntimeCostsAndExecutionAndCapabilities(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		rootRecordingsProjectionCapabilityProbe{value: typedProjection},
		rootOperatorSettingsCapabilityProbe{value: struct{}{}},
	)
	if err != nil {
		t.Fatalf("NewProcessWithRuntimeCostsAndExecutionAndCapabilities(typed) error = %v", err)
	}
	if got := RecordingsProjectionFromProcess(typedProcess); got != typedProjection {
		t.Fatalf("RecordingsProjectionFromProcess(typed) = %#v, want %#v", got, typedProjection)
	}

	composed, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if got := RecordingsProjectionFromProcess(composed); got == nil {
		t.Fatal("RecordingsProjectionFromProcess(composed) = nil, want composed projection")
	}
	if got := OperatorSettingsFromProcess(composed); got == nil {
		t.Fatal("OperatorSettingsFromProcess(composed) = nil, want composed service")
	}
}

func TestWorkerRecordingReaderFromProcessPropagatesReaderError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("worker recording reader unavailable")
	process, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil,
		rootWorkerProcessReader{err: wantErr},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewProcess() error = %v", err)
	}

	reader := WorkerRecordingReaderFromProcess(process)
	if reader == nil {
		t.Fatal("WorkerRecordingReaderFromProcess() returned nil")
	}
	if _, err := reader.LoadWorkerRecording(t.Context(), "root-recording"); !errors.Is(err, wantErr) {
		t.Fatalf("LoadWorkerRecording() error = %v, want %v", err, wantErr)
	}
}

func TestWorkerRecordingReaderFromProcessRejectsMalformedSnapshot(t *testing.T) {
	t.Parallel()

	process, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil,
		rootWorkerProcessReader{payload: json.RawMessage(`{"recordingId":`)},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewProcess() error = %v", err)
	}

	reader := WorkerRecordingReaderFromProcess(process)
	if reader == nil {
		t.Fatal("WorkerRecordingReaderFromProcess() returned nil")
	}
	if _, err := reader.LoadWorkerRecording(t.Context(), "root-recording"); err == nil ||
		!strings.Contains(err.Error(), "decode Worker recording snapshot") {
		t.Fatalf("LoadWorkerRecording() error = %v, want decode diagnostic", err)
	}
}

type rootWorkerRecordingReaderProbe struct{}

type rootDetachedOperationsCapabilityProbe struct {
	operations any
}

func (probe rootDetachedOperationsCapabilityProbe) DetachedOperations() any {
	return probe.operations
}

type rootRecordingsProjectionCapabilityProbe struct {
	value any
}

func (probe rootRecordingsProjectionCapabilityProbe) RecordingsProjection() any {
	return probe.value
}

type rootOperatorSettingsCapabilityProbe struct {
	value any
}

func (probe rootOperatorSettingsCapabilityProbe) OperatorSettings() any {
	return probe.value
}

type rootRecordingsProjectionProbe struct{}

func (rootRecordingsProjectionProbe) ReconstructWorldState(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
	return recordings.ReconstructWorldStateResult{}, nil
}

func (rootRecordingsProjectionProbe) QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest) (recordings.WorkstationRequestsQueryResult, error) {
	return recordings.WorkstationRequestsQueryResult{}, nil
}

func (rootRecordingsProjectionProbe) ReconstructFactoryWorldState([]recordings.FactoryEvent, int) (recordings.FactoryWorldState, error) {
	return recordings.FactoryWorldState{}, nil
}

func (rootRecordingsProjectionProbe) ProjectWorkstationRequests(recordings.FactoryWorldState) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{}
}

func (rootRecordingsProjectionProbe) SimpleDashboardRenderData(recordings.FactoryWorldState) recordings.SimpleDashboardRenderData {
	return recordings.SimpleDashboardRenderData{}
}

func (rootRecordingsProjectionProbe) ProjectActiveThrottlePauses(
	recordings.InitialStructurePayload,
	[]recordings.ActiveThrottlePause,
) []recordings.FactoryWorldThrottlePause {
	return nil
}

func (rootRecordingsProjectionProbe) ValidateReconnectReplay(
	[]recordings.FactoryEvent,
	recordings.FactoryEventReconnectCursor,
	recordings.FactoryEventReconnectScope,
) error {
	return nil
}

type rootWorkerProcessRegistry struct{}

func (rootWorkerProcessRegistry) CanonicalIdentity(identity string) (string, error) {
	return identity, nil
}

type rootWorkerProcessLifecycle struct{}

func (rootWorkerProcessLifecycle) Close(context.Context) error { return nil }

type rootWorkerProcessReader struct {
	payload json.RawMessage
	err     error
}

func (reader rootWorkerProcessReader) LoadWorkerRecording(context.Context, string) (json.RawMessage, error) {
	return reader.payload, reader.err
}

func (*rootWorkerRecordingReaderProbe) PersistWorkerRecord(context.Context, recordings.WorkerRecordingRecord) error {
	return nil
}

func (*rootWorkerRecordingReaderProbe) LoadWorkerRecording(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
	return recordings.WorkerRecordingSnapshot{}, nil
}

func TestBuildProcessComposesInertModelsRuntimeHost(t *testing.T) {
	t.Parallel()

	launcher := &rootRecordingModelHostLauncher{}
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		ModelHostProcessLauncher: launcher,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if process == nil {
		t.Fatal("BuildProcess() returned nil process")
	}
	if launcher.starts != 0 {
		t.Fatalf("model host process starts during construction = %d, want 0", launcher.starts)
	}
}

type rootRecordingModelHostLauncher struct {
	starts int
}

func (launcher *rootRecordingModelHostLauncher) Start(
	context.Context,
	serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.starts++
	panic("model host process launcher called during inert construction")
}

func TestBuildProcessComposesDetachedExternalProviderWithBuiltInsInertly(t *testing.T) {
	t.Parallel()

	manifest := rootExternalManifest(t, "customer.provider", "customer")
	integration := &rootRecordingIntegration{identity: "customer.provider"}
	registrations := []inference.Registration{
		{Manifest: manifest, Integration: integration},
	}
	apiStarts := 0
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		ProviderRegistrations: registrations,
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	registrations[0] = inference.Registration{
		Manifest:    rootExternalManifest(t, "mutated.provider", "mutated"),
		Integration: &rootRecordingIntegration{identity: "mutated.provider"},
	}

	assertProviderLookup(t, process.ProviderRegistry(), "customer.provider", "customer.provider")
	assertProviderLookup(t, process.ProviderRegistry(), "customer", "customer.provider")
	assertProviderLookup(t, process.ProviderRegistry(), "claude", "claude")
	assertProviderLookup(t, process.ProviderRegistry(), "codex", "codex")
	if apiStarts != 0 || integration.discoverCalls != 0 ||
		integration.capabilityCalls != 0 || integration.invokeCalls != 0 {
		t.Fatalf(
			"construction side effects = api:%d discover:%d capabilities:%d invoke:%d, want zero",
			apiStarts,
			integration.discoverCalls,
			integration.capabilityCalls,
			integration.invokeCalls,
		)
	}

	independent, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("independent BuildProcess() error = %v", err)
	}
	if _, err := independent.ProviderRegistry().CanonicalIdentity("customer.provider"); err == nil {
		t.Fatal("independent process retained another build's external registration")
	}
}

func TestBuildProcessReportsCanonicalRegistryValidationFailure(t *testing.T) {
	t.Parallel()

	manifest := rootExternalManifest(t, "claude", "customer-claude")
	registration := inference.Registration{
		Manifest:    manifest,
		Integration: &rootRecordingIntegration{identity: "claude"},
	}

	process, buildErr := BuildProcess(context.Background(), serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{registration},
	})
	if process != nil {
		t.Fatal("BuildProcess() returned process for invalid provider registration")
	}
	if buildErr == nil ||
		!strings.Contains(buildErr.Error(), "provider registry validation failed") ||
		!strings.Contains(buildErr.Error(), `"claude": identity collision`) {
		t.Fatalf("BuildProcess() error = %v, want canonical identity-collision diagnostic", buildErr)
	}
}

func TestBuildProcessOpensFactoryWithRegisteredExternalProviderWithoutProviderIO(t *testing.T) {
	t.Parallel()

	manifest := rootExternalManifest(t, "customer.provider", "customer")
	integration := &rootRecordingIntegration{identity: "customer.provider"}
	factoryDir := rootFactoryWithProvider(t, "customer")

	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{{
			Manifest:    manifest,
			Integration: integration,
		}},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	err = process.Execute(Input{
		Args: []string{
			"you", "run", "--dir", factoryDir, "--with-mock-workers", "--quiet", "--no-record",
		},
		Env:              homeEnvironment(t.TempDir()),
		Context:          context.Background(),
		WorkingDirectory: factoryDir,
	})
	if err != nil {
		t.Fatalf("Process.Execute(run) error = %v", err)
	}
	if integration.discoverCalls != 0 ||
		integration.capabilityCalls != 0 ||
		integration.invokeCalls != 0 {
		t.Fatalf(
			"Factory opening provider I/O = discover:%d capabilities:%d invoke:%d, want zero",
			integration.discoverCalls,
			integration.capabilityCalls,
			integration.invokeCalls,
		)
	}
}

// TestBuildProcessDefersAndOpensOneFactoryRuntime proves that the canonical
// process graph remains inert across repeated construction, then opens one
// Factory Session runtime for one selected live application execution.
func TestBuildProcessDefersAndOpensOneFactoryRuntime(t *testing.T) {
	t.Parallel()

	var openedRuntimeIDs atomic.Int32
	edges := serviceedges.Edges{
		FactorySessionRuntimeInstanceIDGenerator: func() string {
			openedRuntimeIDs.Add(1)
			return "root-runtime-opening"
		},
	}
	process, err := BuildProcess(context.Background(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	second, err := BuildProcess(context.Background(), edges)
	if err != nil {
		t.Fatalf("second BuildProcess() error = %v", err)
	}
	if got := openedRuntimeIDs.Load(); got != 0 {
		t.Fatalf("runtime IDs generated during process construction = %d, want 0", got)
	}

	factoryDir := rootFactoryWithProvider(t, "codex")
	if err := process.Execute(Input{
		Args: []string{
			"you", "run", "--dir", factoryDir, "--with-mock-workers", "--quiet", "--no-record",
		},
		Env:              homeEnvironment(t.TempDir()),
		Context:          context.Background(),
		WorkingDirectory: factoryDir,
	}); err != nil {
		t.Fatalf("Process.Execute(run) error = %v", err)
	}
	if got := openedRuntimeIDs.Load(); got != 1 {
		t.Fatalf("runtime IDs generated during one process execution = %d, want 1", got)
	}
	if err := process.Close(context.Background()); err != nil {
		t.Fatalf("Process.Close() error = %v", err)
	}
	if err := second.Close(context.Background()); err != nil {
		t.Fatalf("second Process.Close() error = %v", err)
	}
}

// TestBuildProcessReusesCanonicalRootsAcrossTwoIsolatedExecutions proves that
// one inert process graph can open two Factory Sessions without rebuilding a
// Work or Recordings root or leaking one execution's runtime identity into the
// other.
func TestBuildProcessReusesCanonicalRootsAcrossTwoIsolatedExecutions(t *testing.T) {
	t.Parallel()

	var openedRuntimeIDs []string
	edges := serviceedges.Edges{
		FactorySessionRuntimeInstanceIDGenerator: func() string {
			id := fmt.Sprintf("root-runtime-%d", len(openedRuntimeIDs)+1)
			openedRuntimeIDs = append(openedRuntimeIDs, id)
			return id
		},
	}
	process, err := BuildProcess(context.Background(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	for index := 0; index < 2; index++ {
		factoryDir := rootFactoryWithProvider(t, "codex")
		if err := process.Execute(Input{
			Args: []string{
				"you", "run", "--dir", factoryDir, "--with-mock-workers", "--quiet", "--no-record",
			},
			Env:              homeEnvironment(t.TempDir()),
			Context:          context.Background(),
			WorkingDirectory: factoryDir,
		}); err != nil {
			t.Fatalf("Process.Execute(run %d) error = %v", index+1, err)
		}
	}
	if !slices.Equal(openedRuntimeIDs, []string{"root-runtime-1", "root-runtime-2"}) {
		t.Fatalf("runtime IDs = %v, want two isolated runtime identities", openedRuntimeIDs)
	}
	if err := process.Close(context.Background()); err != nil {
		t.Fatalf("Process.Close() error = %v", err)
	}
}

func TestBuildProcessRejectsUnknownAndNonSelectableFactoryProvidersWithoutFallback(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		provider string
		want     string
	}{
		{name: "unknown", provider: "unknown.provider", want: `provider is unknown: "unknown.provider"`},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			process, err := BuildProcess(context.Background(), serviceedges.Edges{})
			if err != nil {
				t.Fatalf("BuildProcess() error = %v", err)
			}
			factoryDir := rootFactoryWithProvider(t, test.provider)
			err = process.Execute(Input{
				Args: []string{
					"you", "run", "--dir", factoryDir, "--with-mock-workers", "--quiet", "--no-record",
				},
				Env:              homeEnvironment(t.TempDir()),
				Context:          context.Background(),
				WorkingDirectory: factoryDir,
			})
			if err == nil ||
				!strings.Contains(err.Error(), "workers[0].modelProvider") ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("Process.Execute(run) error = %v, want field-local %s", err, test.want)
			}
		})
	}
}

func TestProcessRoutesHelpAndExplicitCommandsToSuppliedStreams(t *testing.T) {
	t.Parallel()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	var help bytes.Buffer
	err = process.Execute(Input{
		Args:             []string{"renamed-binary", "--help"},
		Env:              homeEnvironment(t.TempDir()),
		Stdout:           &help,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Process.Execute(help) error = %v", err)
	}
	if !strings.HasPrefix(help.String(), "Run and manage CPN-based workflow factories") {
		t.Fatalf("help output = %q", help.String())
	}

	var docs bytes.Buffer
	err = process.Execute(Input{
		Args:             []string{"you", "docs", "agents"},
		Env:              homeEnvironment(t.TempDir()),
		Stdout:           &docs,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Process.Execute(docs agents) error = %v", err)
	}
	if !strings.Contains(docs.String(), "# Agents") {
		t.Fatalf("docs output does not contain agents topic: %q", docs.String())
	}
	if strings.Contains(help.String(), "# Agents") {
		t.Fatal("sequential execution leaked the second command's output into the first stream")
	}
}

func TestProcessSubmitBatchUsesInjectedFileAndStdinEdges(t *testing.T) {
	t.Parallel()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	const batch = `{"requestId":"root-process-batch","type":"FACTORY_REQUEST_BATCH","works":[{"name":"alpha","workTypeName":"task","payload":{"title":"A"}}]}`
	home := t.TempDir()
	batchPath := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(batchPath, []byte(batch), 0o600); err != nil {
		t.Fatalf("write batch fixture: %v", err)
	}

	for _, tc := range []struct {
		name  string
		args  []string
		stdin io.Reader
	}{
		{name: "file", args: []string{"you", "submit", "batch", "--dry-run", batchPath}},
		{name: "stdin", args: []string{"you", "submit", "batch", "--dry-run"}, stdin: strings.NewReader(batch)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := process.Execute(Input{
				Args: tc.args, Env: homeEnvironment(home), Stdin: tc.stdin,
				Stdout: &stdout, Context: context.Background(), WorkingDirectory: home,
			})
			if err != nil {
				t.Fatalf("Process.Execute(submit batch %s) error = %v", tc.name, err)
			}
			if !strings.Contains(stdout.String(), "requestId: root-process-batch") ||
				!strings.Contains(stdout.String(), "dry-run: no request sent") {
				t.Fatalf("submit batch %s output = %q", tc.name, stdout.String())
			}
		})
	}
}

func TestProcessInvalidArgumentsReturnsDiagnostic(t *testing.T) {
	t.Parallel()

	process, buildErr := BuildProcess(context.Background(), serviceedges.Edges{})
	if buildErr != nil {
		t.Fatalf("BuildProcess() error = %v", buildErr)
	}
	var stderr bytes.Buffer
	err := process.Execute(Input{
		Args:             []string{"you", "definitely-not-a-command"},
		Env:              homeEnvironment(t.TempDir()),
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	})
	if err == nil {
		t.Fatal("Process.Execute(invalid command) error = nil")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("Process.Execute(invalid command) error = %q", err)
	}
}

func TestProcessSequentialHomesKeepEffectiveListingReadOnly(t *testing.T) {
	ambientHome := t.TempDir()
	t.Setenv("HOME", ambientHome)
	t.Setenv("USERPROFILE", ambientHome)

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	homes := []string{t.TempDir(), t.TempDir()}
	for _, home := range homes {
		var output bytes.Buffer
		if err := process.Execute(Input{
			Args: []string{
				"you", "--json", "factory", "list", "--dir",
				filepath.Join(home, ".you-agent-factory", "factories"),
			},
			Env: homeEnvironment(home), Stdout: &output, Context: context.Background(), WorkingDirectory: home,
		}); err != nil {
			t.Fatalf("Process.Execute(factory list, home %q) error = %v", home, err)
		}
		if output.Len() == 0 {
			t.Fatalf("factory list output for supplied home %q is empty", home)
		}
		configPath := filepath.Join(home, ".you-agent-factory", "config.json")
		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Fatalf("Stat(config for supplied home %q) error = %v, want not-exist", home, err)
		}
	}
	ambientEntries, err := os.ReadDir(ambientHome)
	if err != nil {
		t.Fatalf("ReadDir(ambient home) error = %v", err)
	}
	if len(ambientEntries) != 0 {
		t.Fatalf("ambient home contains %d entries after supplied-home invocations, want none", len(ambientEntries))
	}
}

func TestProcessConcurrentCommandsKeepInvocationStateIndependent(t *testing.T) {
	t.Parallel()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	var help bytes.Buffer
	var docs bytes.Buffer
	start := make(chan struct{})
	errs := make(chan error, 2)
	var commands sync.WaitGroup
	for _, input := range []Input{
		{
			Args:             []string{"you", "--help"},
			Env:              homeEnvironment(t.TempDir()),
			Stdout:           &help,
			Context:          context.Background(),
			WorkingDirectory: t.TempDir(),
		},
		{
			Args:             []string{"you", "docs", "agents"},
			Env:              homeEnvironment(t.TempDir()),
			Stdout:           &docs,
			Context:          context.Background(),
			WorkingDirectory: t.TempDir(),
		},
	} {
		input := input
		commands.Add(1)
		go func() {
			defer commands.Done()
			<-start
			errs <- process.Execute(input)
		}()
	}
	close(start)
	commands.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Process.Execute(concurrent command) error = %v", err)
		}
	}

	if !strings.HasPrefix(help.String(), "Run and manage CPN-based workflow factories") {
		t.Fatalf("help output = %q", help.String())
	}
	if strings.Contains(help.String(), "# Agents") {
		t.Fatalf("help output contains docs output: %q", help.String())
	}
	if !strings.Contains(docs.String(), "# Agents") {
		t.Fatalf("docs output = %q", docs.String())
	}
	if strings.Contains(docs.String(), "Usage:\n  you [command]") {
		t.Fatalf("docs output contains help output: %q", docs.String())
	}
}

func TestProcessFactoryListDiscoversPackagedFactoriesWithoutMaterialization(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	var listOutput bytes.Buffer
	if err := process.Execute(Input{
		Args: []string{
			"you", "--json", "factory", "list", "--dir",
			filepath.Join(home, ".you-agent-factory", "factories"),
		},
		Env:              homeEnvironment(home),
		Stdout:           &listOutput,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	}); err != nil {
		t.Fatalf("Process.Execute(factory list) error = %v", err)
	}

	if !strings.Contains(listOutput.String(), `"factoryDirectory":"-"`) {
		t.Fatalf("factory list output = %q, want unmaterialized packaged location", listOutput.String())
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("home entries after factory list = (%v, %v), want empty", entries, err)
	}
}

func TestProcessNormalInitializationAndFactoryValidationThroughProductionComposition(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	factoryDir := filepath.Join(home, ".you-agent-factory", "factories", "@you", "goal")
	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	runNormalInitialization(t, process, home)

	var validateOutput bytes.Buffer
	if err := process.Execute(Input{
		Args:             []string{"you", "factory", "config", "validate", factoryDir},
		Env:              homeEnvironment(home),
		Stdout:           &validateOutput,
		Context:          context.Background(),
		WorkingDirectory: filepath.Dir(factoryDir),
	}); err != nil {
		t.Fatalf("Process.Execute(factory config validate) error = %v", err)
	}
	if !strings.Contains(validateOutput.String(), "Factory validation passed") {
		t.Fatalf("factory validation output = %q", validateOutput.String())
	}

	missingPath := filepath.Join(t.TempDir(), "missing-factory")
	if err := process.Execute(Input{
		Args:             []string{"you", "factory", "config", "validate", missingPath},
		Env:              homeEnvironment(home),
		Context:          context.Background(),
		WorkingDirectory: filepath.Dir(factoryDir),
	}); err == nil || !strings.Contains(err.Error(), "find factory config") {
		t.Fatalf("Process.Execute(factory config validate missing path) error = %v", err)
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("missing factory path Stat error = %v, want not-exist", err)
	}
}

func homeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}

type rootRecordingIntegration struct {
	identity        inference.Identity
	discoverCalls   int
	capabilityCalls int
	invokeCalls     int
}

func (i *rootRecordingIntegration) Identity() inference.Identity { return i.identity }

func (*rootRecordingIntegration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(inference.CapabilityPromptSubmission)
}

func (i *rootRecordingIntegration) Discover(context.Context) (inference.Discovery, error) {
	i.discoverCalls++
	panic("process construction must not discover external providers")
}

func (i *rootRecordingIntegration) Capabilities(
	context.Context,
	inference.InvocationRequest,
) (inference.CapabilitySet, error) {
	i.capabilityCalls++
	panic("process construction must not negotiate external provider capabilities")
}

func (i *rootRecordingIntegration) Invoke(
	_ context.Context,
	_ inference.InvocationRequest,
	_ inference.ResponseWriter,
) error {
	i.invokeCalls++
	panic("process construction must not invoke external providers")
}

func rootExternalManifest(t *testing.T, identity, alias string) inference.Manifest {
	t.Helper()
	var catalog struct {
		Providers []inference.Manifest `json:"providers"`
	}
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("decode embedded provider catalog: %v", err)
	}
	manifest := catalog.Providers[0]
	manifest.ID = identity
	manifest.Aliases = []string{alias}
	manifest.ImplementationAvailability = inference.ImplementationExternallySupplied
	manifest.TechnicalSupportLevel = inference.SupportProduction
	manifest.Deprecation = nil
	manifest.MaximumExecutionCapabilities = inference.ExecutionCapabilities{
		PromptSubmission: true,
	}
	manifest.MaximumResponseFidelityCapabilities = inference.ResponseFidelityCapabilities{}
	return manifest
}

func rootFactoryWithProvider(t *testing.T, provider string) string {
	t.Helper()
	factoryDir := testutil.CopyFixtureDir(
		t,
		testutil.MustRepoPath(t, filepath.Join("tests", "functional_test", "testdata", "executor_success")),
	)
	workerPath := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	worker := strings.Join([]string{
		"---",
		"model: test-model",
		"modelProvider: " + provider,
		"stopToken: COMPLETE",
		"type: MODEL_WORKER",
		"---",
		"",
		"Test worker.",
		"",
	}, "\n")
	if err := os.WriteFile(workerPath, []byte(worker), 0o600); err != nil {
		t.Fatalf("write provider worker: %v", err)
	}
	return factoryDir
}

func assertProviderLookup(
	t *testing.T,
	registry interface {
		CanonicalIdentity(string) (string, error)
	},
	identity string,
	want inference.Identity,
) {
	t.Helper()
	canonical, err := registry.CanonicalIdentity(identity)
	if err != nil {
		t.Fatalf("CanonicalIdentity(%q) error = %v", identity, err)
	}
	if canonical != string(want) {
		t.Fatalf("CanonicalIdentity(%q) = %q, want %q", identity, canonical, want)
	}
}
