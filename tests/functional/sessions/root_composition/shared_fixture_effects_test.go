package root_composition_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

type rootCompositionCleanupLedger struct {
	mu      sync.Mutex
	closed  bool
	next    uint64
	actions map[uint64]*rootCompositionCleanupAction
}

type rootCompositionCleanupAction struct {
	label string
	once  sync.Once
	fn    func() error
	err   error
}

func newRootCompositionCleanupLedger() *rootCompositionCleanupLedger {
	return &rootCompositionCleanupLedger{actions: make(map[uint64]*rootCompositionCleanupAction)}
}

func (ledger *rootCompositionCleanupLedger) register(label string, fn func() error) (func() error, error) {
	if strings.TrimSpace(label) == "" {
		return nil, errors.New("cleanup label is empty")
	}
	if fn == nil {
		return nil, errors.New("cleanup function is nil")
	}
	action := &rootCompositionCleanupAction{label: label, fn: fn}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.closed {
		return nil, errors.New("cleanup ledger is closed")
	}
	ledger.next++
	id := ledger.next
	ledger.actions[id] = action
	return func() error {
		ledger.mu.Lock()
		delete(ledger.actions, id)
		ledger.mu.Unlock()
		return action.run()
	}, nil
}

func (ledger *rootCompositionCleanupLedger) cleanup() error {
	ledger.mu.Lock()
	if ledger.closed && len(ledger.actions) == 0 {
		ledger.mu.Unlock()
		return nil
	}
	ledger.closed = true
	ids := make([]uint64, 0, len(ledger.actions))
	for id := range ledger.actions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	actions := make([]*rootCompositionCleanupAction, 0, len(ids))
	for _, id := range ids {
		actions = append(actions, ledger.actions[id])
	}
	ledger.actions = make(map[uint64]*rootCompositionCleanupAction)
	ledger.mu.Unlock()

	var errs []error
	for _, action := range actions {
		if err := action.run(); err != nil {
			errs = append(errs, fmt.Errorf("cleanup %s: %w", action.label, err))
		}
	}
	return errors.Join(errs...)
}

func (action *rootCompositionCleanupAction) run() error {
	action.once.Do(func() { action.err = action.fn() })
	return action.err
}

type rootCompositionEffectCounts struct {
	workingDirectory atomic.Int64
	executionGetwd   atomic.Int64
	executionStat    atomic.Int64
	directoryStat    atomic.Int64
	directoryReadDir atomic.Int64
	resolveHome      atomic.Int64
	resolveSymlinks  atomic.Int64
	sessionID        atomic.Int64
	runtimeID        atomic.Int64
	responseEventID  atomic.Int64
	cursorMkdirAll   atomic.Int64
	cursorReadFile   atomic.Int64
	cursorRemove     atomic.Int64
	cursorRename     atomic.Int64
	cursorTempFile   atomic.Int64
	runtimeMkdirAll  atomic.Int64
	runtimeReadFile  atomic.Int64
	runtimeWriteFile atomic.Int64
	contractFixture  atomic.Int64
	replayRecording  atomic.Int64
	invocationInput  atomic.Int64
	initialWork      atomic.Int64
	invocationMetric atomic.Int64
	runtimeHost      atomic.Int64
	workRequestID    atomic.Int64
}

type rootCompositionConstructionSnapshot struct {
	providerCalls    int64
	scriptCalls      int64
	apiStarts        int64
	routeRejections  int64
	workingDirectory int64
	executionGetwd   int64
	executionStat    int64
	directoryStat    int64
	directoryReadDir int64
	resolveHome      int64
	resolveSymlinks  int64
	sessionID        int64
	runtimeID        int64
	responseEventID  int64
	cursorMkdirAll   int64
	cursorReadFile   int64
	cursorRemove     int64
	cursorRename     int64
	cursorTempFile   int64
	runtimeMkdirAll  int64
	runtimeReadFile  int64
	runtimeWriteFile int64
	contractFixture  int64
	replayRecording  int64
	invocationInput  int64
	initialWork      int64
	invocationMetric int64
	runtimeHost      int64
	workRequestID    int64
}

func (snapshot rootCompositionConstructionSnapshot) totalLifecycle() int64 {
	return snapshot.resolveHome + snapshot.resolveSymlinks + snapshot.sessionID + snapshot.runtimeID +
		snapshot.directoryStat + snapshot.directoryReadDir + snapshot.cursorMkdirAll + snapshot.cursorReadFile +
		snapshot.cursorRemove + snapshot.cursorRename + snapshot.cursorTempFile + snapshot.runtimeMkdirAll +
		snapshot.runtimeReadFile + snapshot.runtimeWriteFile + snapshot.runtimeHost
}

func (snapshot rootCompositionConstructionSnapshot) totalRuntimeOpening() int64 {
	return snapshot.workingDirectory + snapshot.executionGetwd + snapshot.executionStat + snapshot.contractFixture + snapshot.replayRecording
}

func (snapshot rootCompositionConstructionSnapshot) totalWorkAdmission() int64 {
	return snapshot.invocationInput + snapshot.initialWork
}

func (snapshot rootCompositionConstructionSnapshot) totalResponseStream() int64 {
	return snapshot.responseEventID + snapshot.invocationMetric
}

func (snapshot rootCompositionConstructionSnapshot) total() int64 {
	return snapshot.providerCalls + snapshot.scriptCalls + snapshot.apiStarts + snapshot.routeRejections + snapshot.totalLifecycle() +
		snapshot.totalRuntimeOpening() + snapshot.totalWorkAdmission() + snapshot.totalResponseStream() + snapshot.workRequestID
}

type rootCompositionSessionEffects struct {
	routes     *rootCompositionRouteRegistry
	apiStarts  *atomic.Int64
	counts     rootCompositionEffectCounts
	snapshot   atomic.Value
	replayPath atomic.Value
	sequence   atomic.Uint64
}

func newRootCompositionSessionEffects(routes *rootCompositionRouteRegistry, apiStarts *atomic.Int64) *rootCompositionSessionEffects {
	return &rootCompositionSessionEffects{routes: routes, apiStarts: apiStarts}
}

func (effects *rootCompositionSessionEffects) captureConstructionSnapshot() {
	effects.snapshot.Store(effects.constructionSnapshot())
}

func (effects *rootCompositionSessionEffects) constructionSnapshot() rootCompositionConstructionSnapshot {
	if snapshot := effects.snapshot.Load(); snapshot != nil {
		return snapshot.(rootCompositionConstructionSnapshot)
	}
	return rootCompositionConstructionSnapshot{
		providerCalls:    effects.routes.providerCalls.Load(),
		scriptCalls:      effects.routes.scriptCalls.Load(),
		apiStarts:        effects.apiStarts.Load(),
		routeRejections:  effects.routes.unmatchedCount(),
		workingDirectory: effects.counts.workingDirectory.Load(),
		executionGetwd:   effects.counts.executionGetwd.Load(),
		executionStat:    effects.counts.executionStat.Load(),
		directoryStat:    effects.counts.directoryStat.Load(),
		directoryReadDir: effects.counts.directoryReadDir.Load(),
		resolveHome:      effects.counts.resolveHome.Load(),
		resolveSymlinks:  effects.counts.resolveSymlinks.Load(),
		sessionID:        effects.counts.sessionID.Load(),
		runtimeID:        effects.counts.runtimeID.Load(),
		responseEventID:  effects.counts.responseEventID.Load(),
		cursorMkdirAll:   effects.counts.cursorMkdirAll.Load(),
		cursorReadFile:   effects.counts.cursorReadFile.Load(),
		cursorRemove:     effects.counts.cursorRemove.Load(),
		cursorRename:     effects.counts.cursorRename.Load(),
		cursorTempFile:   effects.counts.cursorTempFile.Load(),
		runtimeMkdirAll:  effects.counts.runtimeMkdirAll.Load(),
		runtimeReadFile:  effects.counts.runtimeReadFile.Load(),
		runtimeWriteFile: effects.counts.runtimeWriteFile.Load(),
		contractFixture:  effects.counts.contractFixture.Load(),
		replayRecording:  effects.counts.replayRecording.Load(),
		invocationInput:  effects.counts.invocationInput.Load(),
		initialWork:      effects.counts.initialWork.Load(),
		invocationMetric: effects.counts.invocationMetric.Load(),
		runtimeHost:      effects.counts.runtimeHost.Load(),
		workRequestID:    effects.counts.workRequestID.Load(),
	}
}

func (effects *rootCompositionSessionEffects) liveSnapshot() rootCompositionConstructionSnapshot {
	return rootCompositionConstructionSnapshot{
		providerCalls:    effects.routes.providerCalls.Load(),
		scriptCalls:      effects.routes.scriptCalls.Load(),
		apiStarts:        effects.apiStarts.Load(),
		routeRejections:  effects.routes.unmatchedCount(),
		workingDirectory: effects.counts.workingDirectory.Load(),
		executionGetwd:   effects.counts.executionGetwd.Load(),
		executionStat:    effects.counts.executionStat.Load(),
		directoryStat:    effects.counts.directoryStat.Load(),
		directoryReadDir: effects.counts.directoryReadDir.Load(),
		resolveHome:      effects.counts.resolveHome.Load(),
		resolveSymlinks:  effects.counts.resolveSymlinks.Load(),
		sessionID:        effects.counts.sessionID.Load(),
		runtimeID:        effects.counts.runtimeID.Load(),
		responseEventID:  effects.counts.responseEventID.Load(),
		cursorMkdirAll:   effects.counts.cursorMkdirAll.Load(),
		cursorReadFile:   effects.counts.cursorReadFile.Load(),
		cursorRemove:     effects.counts.cursorRemove.Load(),
		cursorRename:     effects.counts.cursorRename.Load(),
		cursorTempFile:   effects.counts.cursorTempFile.Load(),
		runtimeMkdirAll:  effects.counts.runtimeMkdirAll.Load(),
		runtimeReadFile:  effects.counts.runtimeReadFile.Load(),
		runtimeWriteFile: effects.counts.runtimeWriteFile.Load(),
		contractFixture:  effects.counts.contractFixture.Load(),
		replayRecording:  effects.counts.replayRecording.Load(),
		invocationInput:  effects.counts.invocationInput.Load(),
		initialWork:      effects.counts.initialWork.Load(),
		invocationMetric: effects.counts.invocationMetric.Load(),
		runtimeHost:      effects.counts.runtimeHost.Load(),
		workRequestID:    effects.counts.workRequestID.Load(),
	}
}

func (effects *rootCompositionSessionEffects) lastReplayPath() string {
	value, _ := effects.replayPath.Load().(string)
	return value
}

func (effects *rootCompositionSessionEffects) effectRoute(path string) (*rootCompositionRoute, error) {
	return effects.routes.routeForEffectPath(path)
}

type rootCompositionExecutionOpeningFileSystem struct {
	effects *rootCompositionSessionEffects
}

func (filesystem *rootCompositionExecutionOpeningFileSystem) Getwd() (string, error) {
	filesystem.effects.counts.executionGetwd.Add(1)
	// The route-bearing Process input supplies the scenario working directory.
	// This legacy argument-free probe is retained only as a read-only host
	// fallback and never selects or mutates a scenario route.
	return os.Getwd()
}

func (filesystem *rootCompositionExecutionOpeningFileSystem) Stat(path string) (fs.FileInfo, error) {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return nil, err
	}
	filesystem.effects.counts.executionStat.Add(1)
	return os.Stat(path)
}

type rootCompositionDirectoryInspection struct {
	effects *rootCompositionSessionEffects
}

func (filesystem *rootCompositionDirectoryInspection) Stat(path string) (fs.FileInfo, error) {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return nil, err
	}
	filesystem.effects.counts.directoryStat.Add(1)
	return os.Stat(path)
}

func (filesystem *rootCompositionDirectoryInspection) ReadDir(path string) ([]fs.DirEntry, error) {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return nil, err
	}
	filesystem.effects.counts.directoryReadDir.Add(1)
	return os.ReadDir(path)
}

type rootCompositionCursorPersistenceFileSystem struct {
	effects *rootCompositionSessionEffects
}

func (filesystem *rootCompositionCursorPersistenceFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return err
	}
	filesystem.effects.counts.cursorMkdirAll.Add(1)
	return os.MkdirAll(path, mode)
}

func (filesystem *rootCompositionCursorPersistenceFileSystem) ReadFile(path string) ([]byte, error) {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return nil, err
	}
	filesystem.effects.counts.cursorReadFile.Add(1)
	return os.ReadFile(path)
}

func (filesystem *rootCompositionCursorPersistenceFileSystem) Remove(path string) error {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return err
	}
	filesystem.effects.counts.cursorRemove.Add(1)
	return os.Remove(path)
}

func (filesystem *rootCompositionCursorPersistenceFileSystem) Rename(oldPath, newPath string) error {
	oldRoute, err := filesystem.effects.effectRoute(oldPath)
	if err != nil {
		return err
	}
	newRoute, err := filesystem.effects.effectRoute(newPath)
	if err != nil {
		return err
	}
	if oldRoute != newRoute {
		return fmt.Errorf("%w: rename crosses routes", errRootCompositionRouteCrossing)
	}
	filesystem.effects.counts.cursorRename.Add(1)
	return os.Rename(oldPath, newPath)
}

type rootCompositionRuntimePersistenceFileSystem struct {
	effects *rootCompositionSessionEffects
}

func (filesystem *rootCompositionRuntimePersistenceFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return err
	}
	filesystem.effects.counts.runtimeMkdirAll.Add(1)
	return os.MkdirAll(path, mode)
}

func (filesystem *rootCompositionRuntimePersistenceFileSystem) ReadFile(path string) ([]byte, error) {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return nil, err
	}
	filesystem.effects.counts.runtimeReadFile.Add(1)
	return os.ReadFile(path)
}

func (filesystem *rootCompositionRuntimePersistenceFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	if _, err := filesystem.effects.effectRoute(path); err != nil {
		return err
	}
	filesystem.effects.counts.runtimeWriteFile.Add(1)
	return os.WriteFile(path, data, mode)
}

func (effects *rootCompositionSessionEffects) createCursorTemporaryFile(dir, pattern string) (factorysessions.CursorPersistenceTemporaryFile, error) {
	route, err := effects.effectRoute(dir)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	route.mu.Lock()
	route.temporaryFiles[file.Name()] = struct{}{}
	route.mu.Unlock()
	effects.counts.cursorTempFile.Add(1)
	return file, nil
}

func (effects *rootCompositionSessionEffects) resolveHomeDirectory() (string, error) {
	return "", errors.New("route-specific home resolution is not available without a selected Process input")
}

func (effects *rootCompositionSessionEffects) resolveLogicalTargetSymlinks(path string) (string, error) {
	if _, err := effects.effectRoute(path); err != nil {
		return "", err
	}
	effects.counts.resolveSymlinks.Add(1)
	return filepath.EvalSymlinks(path)
}

func (effects *rootCompositionSessionEffects) nextSessionID() string {
	effects.counts.sessionID.Add(1)
	// Durable execution snapshots accept UUID-shaped identities (and the
	// durable-session prefix is added by the session service). Keep the
	// generator deterministic without leaking a scenario label into that
	// public identity grammar.
	hex := fmt.Sprintf("%032x", effects.sequence.Add(1))
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex[0:8], hex[8:12], hex[12:16], hex[16:20], hex[20:])
}

func (effects *rootCompositionSessionEffects) nextRuntimeInstanceID() string {
	effects.counts.runtimeID.Add(1)
	return fmt.Sprintf("runtime-%d", effects.sequence.Add(1))
}

func (effects *rootCompositionSessionEffects) nextResponseEventID() string {
	effects.counts.responseEventID.Add(1)
	return fmt.Sprintf("response-%d", effects.sequence.Add(1))
}

func (effects *rootCompositionSessionEffects) nextWorkRequestID() string {
	effects.counts.workRequestID.Add(1)
	return fmt.Sprintf("request-%d", effects.sequence.Add(1))
}

func (effects *rootCompositionSessionEffects) readContractFixture(path string) ([]byte, error) {
	resolved, err := effects.resolveEffectPath(path)
	if err != nil {
		return nil, err
	}
	effects.counts.contractFixture.Add(1)
	return os.ReadFile(resolved)
}

func (effects *rootCompositionSessionEffects) readInvocationInput(path string) ([]byte, error) {
	resolved, err := effects.resolveEffectPath(path)
	if err != nil {
		return nil, err
	}
	effects.counts.invocationInput.Add(1)
	return os.ReadFile(resolved)
}

func (effects *rootCompositionSessionEffects) readReplayRecording(path string) ([]byte, error) {
	// Preserve the caller's source token even when a relative token cannot be
	// safely resolved without an explicit path selector. Absolute replay
	// artifacts are the routed, successful path; the relative failure witness
	// still proves the loader boundary without guessing another scenario's
	// working directory.
	effects.replayPath.Store(path)
	resolved, err := effects.resolveEffectPath(path)
	if err != nil {
		return nil, err
	}
	effects.counts.replayRecording.Add(1)
	return os.ReadFile(resolved)
}

func (effects *rootCompositionSessionEffects) readInitialWork(path string) ([]byte, error) {
	resolved, err := effects.resolveEffectPath(path)
	if err != nil {
		return nil, err
	}
	effects.counts.initialWork.Add(1)
	return os.ReadFile(resolved)
}

func (effects *rootCompositionSessionEffects) resolveEffectPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: relative path %q has no route selector", errRootCompositionRouteNotFound, path)
	}
	if _, err := effects.effectRoute(path); err != nil {
		return "", err
	}
	return path, nil
}

func (effects *rootCompositionSessionEffects) RecordInvocationMetric(factorysessions.InvocationMetric) {
	effects.counts.invocationMetric.Add(1)
}

func (effects *rootCompositionSessionEffects) observeRuntimeHost(factorysessions.RuntimeHostBinding) {
	effects.counts.runtimeHost.Add(1)
}

var _ factorysessions.ExecutionOpeningFileSystem = (*rootCompositionExecutionOpeningFileSystem)(nil)
var _ factorysessions.DirectoryInspection = (*rootCompositionDirectoryInspection)(nil)
var _ factorysessions.CursorPersistenceFileSystem = (*rootCompositionCursorPersistenceFileSystem)(nil)
var _ factorysessions.RuntimePersistenceFileSystem = (*rootCompositionRuntimePersistenceFileSystem)(nil)
var _ factorysessions.InvocationMetricsRecorder = (*rootCompositionSessionEffects)(nil)
var _ platformprocess.CommandRunner = (*rootCompositionCommandRouter)(nil)
var _ io.Writer = (*os.File)(nil)

type rootCompositionCountingCommandRunner struct {
	calls  atomic.Int64
	result platformprocess.CommandResult
}

func (runner *rootCompositionCountingCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return runner.result, nil
}

var _ platformprocess.CommandRunner = (*rootCompositionCountingCommandRunner)(nil)
