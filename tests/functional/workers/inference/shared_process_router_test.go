package inference_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type inferenceCommandRoute struct {
	runner       platformprocess.CommandRunner
	env          []string
	scenarioName string
	sessionID    string
}

type inferenceRouteContext struct {
	scenarioName string
	sessionID    string
}

type inferenceResponseCaptureRunner struct {
	delegate platformprocess.CommandRunner
	release  <-chan struct{}
}

func (runner *inferenceResponseCaptureRunner) wait(ctx context.Context) error {
	select {
	case <-runner.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runner *inferenceResponseCaptureRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := runner.wait(ctx); err != nil {
		return platformprocess.CommandResult{}, err
	}
	return runner.delegate.Run(ctx, request)
}

func (runner *inferenceResponseCaptureRunner) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if err := runner.wait(ctx); err != nil {
		return platformprocess.CommandResult{}, err
	}
	streaming, ok := runner.delegate.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	})
	if ok {
		return streaming.RunStreaming(ctx, request, observer)
	}
	result, err := runner.delegate.Run(ctx, request)
	if observer != nil {
		if len(result.Stdout) > 0 {
			observer(platformprocess.OutputStreamStdout, append([]byte(nil), result.Stdout...))
		}
		if len(result.Stderr) > 0 {
			observer(platformprocess.OutputStreamStderr, append([]byte(nil), result.Stderr...))
		}
	}
	return result, err
}

type inferenceCommandRouter struct {
	mu     sync.Mutex
	routes map[string]inferenceCommandRoute
}

// inferenceWorkerRecordingRouter keeps the root-built process's durable
// recording port stable while allowing each explicit session to observe the
// exact Worker records it produced. The fallback and routed delegates
// use production journals for both replay and captured inspection.
type inferenceWorkerRecordingRouter struct {
	mu          sync.RWMutex
	fallback    recordings.WorkerRecordingWriter
	bySession   map[string]recordings.WorkerRecordingWriter
	byWorker    map[string]inferenceWorkerRecordingRoute
	byRecording map[string]inferenceWorkerRecordingRoute
}

type inferenceWorkerRecordingRoute struct {
	sessionID string
	writer    recordings.WorkerRecordingWriter
}

func (router *inferenceWorkerRecordingRouter) setSession(
	sessionID string,
	delegate recordings.WorkerRecordingWriter,
) {
	if router == nil {
		return
	}
	router.mu.Lock()
	if router.bySession == nil {
		router.bySession = make(map[string]recordings.WorkerRecordingWriter)
	}
	router.bySession[sessionID] = delegate
	router.mu.Unlock()
}

func (router *inferenceWorkerRecordingRouter) clearSession(sessionID string) {
	if router == nil {
		return
	}
	router.mu.Lock()
	delete(router.bySession, sessionID)
	for workerSessionID, route := range router.byWorker {
		if route.sessionID == sessionID {
			delete(router.byWorker, workerSessionID)
		}
	}
	for recordingID, route := range router.byRecording {
		if route.sessionID == sessionID {
			delete(router.byRecording, recordingID)
		}
	}
	router.mu.Unlock()
}

func (router *inferenceWorkerRecordingRouter) routeRecord(
	record recordings.WorkerRecordingRecord,
) recordings.WorkerRecordingWriter {
	if router == nil {
		return nil
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if route := router.byWorker[record.WorkerSessionID]; route.writer != nil {
		return route.writer
	}
	if route := router.byRecording[record.RecordingID]; route.writer != nil {
		router.byWorker[record.WorkerSessionID] = route
		return route.writer
	}
	sessionID := record.FactorySessionID
	writer := router.bySession[sessionID]
	if writer == nil {
		return router.fallback
	}
	route := inferenceWorkerRecordingRoute{sessionID: sessionID, writer: writer}
	router.byWorker[record.WorkerSessionID] = route
	router.byRecording[record.RecordingID] = route
	return writer
}

func (router *inferenceWorkerRecordingRouter) routeIdentity(
	recordingID string,
	workerSessionID string,
) recordings.WorkerRecordingWriter {
	if router == nil {
		return nil
	}
	router.mu.RLock()
	defer router.mu.RUnlock()
	if route := router.byWorker[workerSessionID]; route.writer != nil {
		return route.writer
	}
	if route := router.byRecording[recordingID]; route.writer != nil {
		return route.writer
	}
	return router.fallback
}

func (router *inferenceWorkerRecordingRouter) PersistWorkerRecord(
	ctx context.Context,
	record recordings.WorkerRecordingRecord,
) error {
	writer := router.routeRecord(record)
	if writer == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	return writer.PersistWorkerRecord(ctx, record)
}

func (router *inferenceWorkerRecordingRouter) PersistWorkerRecordingFailure(
	ctx context.Context,
	failure recordings.WorkerRecordingFailure,
) error {
	writer := router.routeIdentity(failure.RecordingID, failure.WorkerSessionID)
	failureWriter, ok := writer.(recordings.WorkerRecordingFailureWriter)
	if !ok || failureWriter == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	return failureWriter.PersistWorkerRecordingFailure(ctx, failure)
}

func (router *inferenceWorkerRecordingRouter) LoadWorkerRecording(
	ctx context.Context,
	recordingID string,
) (recordings.WorkerRecordingSnapshot, error) {
	reader, ok := router.routeIdentity(recordingID, "").(recordings.WorkerRecordingReader)
	if !ok || reader == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.LoadWorkerRecording(ctx, recordingID)
}

func (router *inferenceWorkerRecordingRouter) controlStore(recordingID, workerID string) (recordings.WorkerControlOperationStore, error) {
	store, ok := router.routeIdentity(recordingID, workerID).(recordings.WorkerControlOperationStore)
	if !ok || store == nil {
		return nil, recordings.ErrMissingWorkerControlOperationStore
	}
	return store, nil
}

func (router *inferenceWorkerRecordingRouter) BeginWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	store, err := router.controlStore(record.Target.RecordingID, record.Target.WorkerSessionID)
	if err != nil {
		return recordings.WorkerControlOperationRecord{}, false, err
	}
	return store.BeginWorkerControlOperation(ctx, record)
}

func (router *inferenceWorkerRecordingRouter) AdvanceWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord, revision uint64) (recordings.WorkerControlOperationRecord, error) {
	store, err := router.controlStore(record.Target.RecordingID, record.Target.WorkerSessionID)
	if err != nil {
		return recordings.WorkerControlOperationRecord{}, err
	}
	return store.AdvanceWorkerControlOperation(ctx, record, revision)
}

func (router *inferenceWorkerRecordingRouter) LoadWorkerControlOperation(ctx context.Context, key recordings.WorkerControlOperationKey) (recordings.WorkerControlOperationRecord, error) {
	store, err := router.controlStore(key.RecordingID, key.WorkerSessionID)
	if err != nil {
		return recordings.WorkerControlOperationRecord{}, err
	}
	return store.LoadWorkerControlOperation(ctx, key)
}

func (router *inferenceWorkerRecordingRouter) ListWorkerControlOperations(ctx context.Context, target recordings.WorkerControlTarget) ([]recordings.WorkerControlOperationRecord, error) {
	store, err := router.controlStore(target.RecordingID, target.WorkerSessionID)
	if err != nil {
		return nil, err
	}
	return store.ListWorkerControlOperations(ctx, target)
}

func (router *inferenceWorkerRecordingRouter) PersistWorkerControlInput(ctx context.Context, key recordings.WorkerControlOperationKey, input json.RawMessage) (string, error) {
	store, err := router.controlStore(key.RecordingID, key.WorkerSessionID)
	if err != nil {
		return "", err
	}
	return store.PersistWorkerControlInput(ctx, key, input)
}

func (router *inferenceWorkerRecordingRouter) ReadWorkerControlInput(ctx context.Context, key recordings.WorkerControlOperationKey, ref string) (json.RawMessage, error) {
	store, err := router.controlStore(key.RecordingID, key.WorkerSessionID)
	if err != nil {
		return nil, err
	}
	return store.ReadWorkerControlInput(ctx, key, ref)
}

func (router *inferenceWorkerRecordingRouter) restartStore(recordingID, workerID string) (recordings.WorkerRestartInputStore, error) {
	store, ok := router.routeIdentity(recordingID, workerID).(recordings.WorkerRestartInputStore)
	if !ok || store == nil {
		return nil, recordings.ErrMissingWorkerRestartInputStore
	}
	return store, nil
}

func (router *inferenceWorkerRecordingRouter) ValidateWorkerRestartRecipe(ctx context.Context, workerID string, execution workers.WorkstationDispatchRequest) error {
	store, err := router.restartStore(execution.Execution.RecordingID, workerID)
	if err != nil {
		return err
	}
	return store.ValidateWorkerRestartRecipe(ctx, workerID, execution)
}

func (router *inferenceWorkerRecordingRouter) SaveWorkerRestartRecipe(ctx context.Context, target recordings.WorkerControlTarget, execution workers.WorkstationDispatchRequest) error {
	store, err := router.restartStore(target.RecordingID, target.WorkerSessionID)
	if err != nil {
		return err
	}
	return store.SaveWorkerRestartRecipe(ctx, target, execution)
}

func (router *inferenceWorkerRecordingRouter) ReadWorkerRestartRecipe(ctx context.Context, target recordings.WorkerControlTarget) (workers.WorkstationDispatchRequest, error) {
	store, err := router.restartStore(target.RecordingID, target.WorkerSessionID)
	if err != nil {
		return workers.WorkstationDispatchRequest{}, err
	}
	return store.ReadWorkerRestartRecipe(ctx, target)
}

func (router *inferenceWorkerRecordingRouter) LookupPreparedWorkerContinuationSource(ctx context.Context, target recordings.WorkerControlTarget) (recordings.WorkerContinuationSource, error) {
	store, err := router.restartStore(target.RecordingID, target.WorkerSessionID)
	if err != nil {
		return recordings.WorkerContinuationSource{}, err
	}
	return store.LookupPreparedWorkerContinuationSource(ctx, target)
}

func (router *inferenceWorkerRecordingRouter) ReadWorkerContinuationInput(ctx context.Context, key recordings.WorkerControlOperationKey) (json.RawMessage, error) {
	store, err := router.restartStore(key.RecordingID, key.WorkerSessionID)
	if err != nil {
		return nil, err
	}
	return store.ReadWorkerContinuationInput(ctx, key)
}

func (router *inferenceCommandRouter) set(
	dir string,
	runner platformprocess.CommandRunner,
	env []string,
	context inferenceRouteContext,
) error {
	if router == nil {
		return errors.New("shared inference command router is nil")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	key := cleanInferencePath(dir)
	if existing, ok := router.routes[key]; ok {
		return fmt.Errorf(
			"shared inference route selector %q is already registered for scenario %q session %q",
			key,
			existing.scenarioName,
			existing.sessionID,
		)
	}
	router.routes[key] = inferenceCommandRoute{
		runner:       runner,
		env:          append([]string(nil), env...),
		scenarioName: context.scenarioName,
		sessionID:    context.sessionID,
	}
	return nil
}

func (router *inferenceCommandRouter) updateContext(
	dir string,
	routeContext inferenceRouteContext,
) error {
	if router == nil {
		return errors.New("shared inference command router is nil")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	key := cleanInferencePath(dir)
	route, ok := router.routes[key]
	if !ok {
		return fmt.Errorf("shared inference route selector %q is not registered", key)
	}
	route.scenarioName = routeContext.scenarioName
	route.sessionID = routeContext.sessionID
	router.routes[key] = route
	return nil
}

func (router *inferenceCommandRouter) clear(dir string) {
	if router == nil {
		return
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	delete(router.routes, cleanInferencePath(dir))
}

func (router *inferenceCommandRouter) routeCount() int {
	if router == nil {
		return 0
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *inferenceCommandRouter) route(request platformprocess.CommandRequest) (inferenceCommandRoute, error) {
	router.mu.Lock()
	defer router.mu.Unlock()
	key := cleanInferencePath(request.WorkDir)
	route, ok := router.routes[key]
	if ok {
		if route.runner != nil {
			return route, nil
		}
		return inferenceCommandRoute{}, fmt.Errorf(
			"shared inference route selector %q has no command runner (scenario %q, session %q)",
			key,
			route.scenarioName,
			route.sessionID,
		)
	}
	selectors := make([]string, 0, len(router.routes))
	for selector, registered := range router.routes {
		selectors = append(selectors, fmt.Sprintf(
			"%q (scenario %q, session %q)",
			selector,
			registered.scenarioName,
			registered.sessionID,
		))
	}
	sort.Strings(selectors)
	return inferenceCommandRoute{}, fmt.Errorf(
		"shared inference route selector %q is not registered; registered selectors: %s",
		key,
		strings.Join(selectors, ", "),
	)
}

func (router *inferenceCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	route, err := router.route(request)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	request.Env = overlayInferenceEnvironment(request.Env, route.env)
	return route.runner.Run(ctx, request)
}

func (router *inferenceCommandRouter) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	route, err := router.route(request)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	request.Env = overlayInferenceEnvironment(request.Env, route.env)
	streaming, ok := route.runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	})
	if ok {
		return streaming.RunStreaming(ctx, request, observer)
	}
	result, err := route.runner.Run(ctx, request)
	if observer != nil {
		if len(result.Stdout) > 0 {
			observer(platformprocess.OutputStreamStdout, append([]byte(nil), result.Stdout...))
		}
		if len(result.Stderr) > 0 {
			observer(platformprocess.OutputStreamStderr, append([]byte(nil), result.Stderr...))
		}
	}
	return result, err
}

func overlayInferenceEnvironment(base, overlay []string) []string {
	if len(overlay) == 0 {
		return base
	}
	result := make([]string, 0, len(base)+len(overlay))
	for _, value := range base {
		name := strings.SplitN(value, "=", 2)[0]
		duplicate := false
		for _, replacement := range overlay {
			if strings.EqualFold(name, strings.SplitN(replacement, "=", 2)[0]) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, value)
		}
	}
	return append(result, overlay...)
}

// Catalog reads gather only the actual routed journals. This fixture deliberately
// refuses oversized/paged catalogs rather than inventing cursor semantics; its
// independently bounded scenarios remain below one production catalog page.
func (router *inferenceWorkerRecordingRouter) ListWorkerSessionCaptures(ctx context.Context, request recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	if request.NextToken != "" {
		return recordings.WorkerCapturedCatalogPage{}, recordings.ErrWorkerRecordingPersistence
	}
	router.mu.RLock()
	writers := []recordings.WorkerRecordingWriter{router.fallback}
	for _, writer := range router.bySession {
		writers = append(writers, writer)
	}
	router.mu.RUnlock()
	result := recordings.WorkerCapturedCatalogPage{}
	var generations []string
	seen := make(map[string]bool)
	for _, writer := range writers {
		reader, ok := writer.(recordings.WorkerCapturedActivityReader)
		if !ok {
			return result, recordings.ErrMissingWorkerRecordingReader
		}
		page, err := reader.ListWorkerSessionCaptures(ctx, recordings.WorkerCapturedCatalogRequest{Limit: 1000, PreparedSummariesOnly: request.PreparedSummariesOnly, RequireCompleteMembership: request.RequireCompleteMembership})
		if err != nil {
			return result, err
		}
		if page.NextToken != "" {
			return result, recordings.ErrWorkerRecordingPersistence
		}
		generations = append(generations, page.GenerationID)
		for _, item := range page.Items {
			if !seen[item.Catalog.WorkerSessionID] {
				result.Items = append(result.Items, item)
				seen[item.Catalog.WorkerSessionID] = true
			}
		}
	}
	sort.Strings(generations)
	result.GenerationID = strings.Join(generations, ":")
	if request.Limit > 0 && len(result.Items) > request.Limit {
		return result, recordings.ErrWorkerRecordingPersistence
	}
	return result, nil
}

func (router *inferenceWorkerRecordingRouter) LookupWorkerSessionCapture(ctx context.Context, id string) (recordings.WorkerSessionCatalogEntry, error) {
	reader, ok := router.routeIdentity("", id).(recordings.WorkerCapturedActivityReader)
	if !ok {
		return recordings.WorkerSessionCatalogEntry{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.LookupWorkerSessionCapture(ctx, id)
}

func (router *inferenceWorkerRecordingRouter) ReadWorkerCapturedActivity(ctx context.Context, request recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	reader, ok := router.routeIdentity("", request.WorkerSessionID).(recordings.WorkerCapturedActivityReader)
	if !ok {
		return recordings.WorkerCapturedActivityPage{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.ReadWorkerCapturedActivity(ctx, request)
}

// Select through the same routed journal that accepted this identity. Scoped
// observations must retain the real store's prepared health and committed facts.
func (router *inferenceWorkerRecordingRouter) CurrentWorkerRecordingHealth(ctx context.Context, recordingID string, ids []string) (recordings.WorkerRecordingSnapshot, error) {
	reader, ok := router.routeIdentity(recordingID, "").(recordings.WorkerRecordingHealthReader)
	if !ok {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.CurrentWorkerRecordingHealth(ctx, recordingID, ids)
}

func (router *inferenceWorkerRecordingRouter) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	reader, ok := router.routeIdentity("", id).(recordings.WorkerCapturedSummaryReader)
	if !ok {
		return recordings.WorkerCapturedSummary{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.LookupWorkerSessionSummary(ctx, id)
}

func (router *inferenceWorkerRecordingRouter) ListPreparedWorkerSessionCaptures(ctx context.Context, request recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	request.PreparedSummariesOnly = true
	return router.ListWorkerSessionCaptures(ctx, request)
}
