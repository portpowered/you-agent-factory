package internal

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

const defaultFactorySessionID = "~default"

type runtimeInstance struct {
	runtimeID        string
	factorySessionID string
	snapshot         interfaces.RuntimeSnapshot
	activationInputs runtimeActivationInputIdentity
	owner            *Service
	runtimeConfig    *runtimeSnapshotConfig
	watcher          automations.FilesystemWatcher
	submit           automations.WorkRequestSubmitter
	startSchedulers  bool
	ctx              context.Context
	cancel           context.CancelFunc
	sidecars         sync.WaitGroup

	mu       sync.Mutex
	starting bool
	started  bool
}

type runtimeSnapshotConfig struct {
	factoryDir     string
	runtimeBaseDir string
	runtimeID      string
	config         interfaces.FactoryConfig
}

func (c *runtimeSnapshotConfig) FactoryDir() string {
	if c == nil {
		return ""
	}
	return c.factoryDir
}

func (c *runtimeSnapshotConfig) RuntimeBaseDir() string {
	if c == nil {
		return ""
	}
	return c.runtimeBaseDir
}

// RuntimeInstanceID is an optional private lookup used by durable source
// effects that must keep checkpoints isolated even when two runtimes share a
// Factory directory and source names.
func (c *runtimeSnapshotConfig) RuntimeInstanceID() string {
	if c == nil {
		return ""
	}
	return c.runtimeID
}

func (c *runtimeSnapshotConfig) FactoryConfig() *interfaces.FactoryConfig {
	if c == nil {
		return nil
	}
	return &c.config
}

func (c *runtimeSnapshotConfig) Worker(name string) (*interfaces.FactoryWorkerConfig, bool) {
	if c == nil {
		return nil, false
	}
	for index := range c.config.Workers {
		if c.config.Workers[index].Name == name {
			return &c.config.Workers[index], true
		}
	}
	return nil, false
}

func (c *runtimeSnapshotConfig) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	if c == nil {
		return nil, false
	}
	for index := range c.config.Workstations {
		if c.config.Workstations[index].Name == name {
			return &c.config.Workstations[index], true
		}
	}
	return nil, false
}

var _ interfaces.RuntimeConfigLookup = (*runtimeSnapshotConfig)(nil)

func (s *Service) ActivateRuntime(
	ctx context.Context,
	request automations.RuntimeActivationRequest,
) (automations.RuntimeActivationResult, error) {
	normalized, err := normalizeRuntimeActivationRequest(request)
	if err != nil {
		return automations.RuntimeActivationResult{}, err
	}

	s.runtimeMu.Lock()
	if existing := s.runtimes[normalized.RuntimeID]; existing != nil {
		s.runtimeMu.Unlock()
		if runtimeActivationMatches(existing, normalized) {
			return automations.RuntimeActivationResult{
				RuntimeID:  normalized.RuntimeID,
				State:      automations.RuntimeLifecycleActivated,
				Idempotent: true,
			}, nil
		}
		return automations.RuntimeActivationResult{}, runtimeLifecycleError(
			"ActivateRuntime", automations.ErrorCodeConflict,
			fmt.Errorf("runtime %q is already activated with different inputs", normalized.RuntimeID),
		)
	}
	if _, pending := s.runtimeActivating[normalized.RuntimeID]; pending {
		s.runtimeMu.Unlock()
		return automations.RuntimeActivationResult{}, runtimeLifecycleError(
			"ActivateRuntime", automations.ErrorCodeConflict,
			fmt.Errorf("runtime %q activation is already in progress", normalized.RuntimeID),
		)
	}
	s.runtimeActivating[normalized.RuntimeID] = struct{}{}
	s.runtimeMu.Unlock()

	instance, err := s.buildRuntimeInstance(ctx, normalized)
	if err != nil {
		s.clearRuntimeActivation(normalized.RuntimeID)
		return automations.RuntimeActivationResult{}, err
	}

	s.runtimeMu.Lock()
	delete(s.runtimeActivating, normalized.RuntimeID)
	if existing := s.runtimes[normalized.RuntimeID]; existing != nil {
		s.runtimeMu.Unlock()
		instance.stop(context.Background())
		if runtimeActivationMatches(existing, normalized) {
			return automations.RuntimeActivationResult{
				RuntimeID:  normalized.RuntimeID,
				State:      automations.RuntimeLifecycleActivated,
				Idempotent: true,
			}, nil
		}
		return automations.RuntimeActivationResult{}, runtimeLifecycleError(
			"ActivateRuntime", automations.ErrorCodeConflict,
			fmt.Errorf("runtime %q is already activated with different inputs", normalized.RuntimeID),
		)
	}
	s.runtimes[normalized.RuntimeID] = instance
	s.runtimeMu.Unlock()

	return automations.RuntimeActivationResult{
		RuntimeID: normalized.RuntimeID,
		State:     automations.RuntimeLifecycleActivated,
	}, nil
}

func (s *Service) StartRuntime(ctx context.Context, runtimeID string) error {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return runtimeLifecycleError("StartRuntime", automations.ErrorCodeInvalid, automations.ErrInvalidRequest)
	}

	s.runtimeMu.Lock()
	instance := s.runtimes[runtimeID]
	s.runtimeMu.Unlock()
	if instance == nil {
		return runtimeLifecycleError("StartRuntime", automations.ErrorCodeNotFound, automations.ErrNotFound)
	}

	instance.mu.Lock()
	if instance.started {
		instance.mu.Unlock()
		return nil
	}
	if instance.starting {
		instance.mu.Unlock()
		return runtimeLifecycleError("StartRuntime", automations.ErrorCodeConflict, automations.ErrConflict)
	}
	instance.starting = true
	instance.mu.Unlock()

	startErr := instance.start(ctx)
	instance.mu.Lock()
	instance.starting = false
	if startErr == nil {
		instance.started = true
	}
	instance.mu.Unlock()
	if startErr == nil {
		return nil
	}

	s.removeRuntime(runtimeID, instance)
	instance.stop(context.Background())
	return startErr
}

func (s *Service) DeactivateRuntime(
	ctx context.Context,
	request automations.RuntimeDeactivationRequest,
) (automations.RuntimeDeactivationResult, error) {
	runtimeID := strings.TrimSpace(request.RuntimeID)
	if runtimeID == "" {
		return automations.RuntimeDeactivationResult{}, runtimeLifecycleError(
			"DeactivateRuntime", automations.ErrorCodeInvalid, automations.ErrInvalidRequest,
		)
	}

	s.runtimeMu.Lock()
	if _, pending := s.runtimeActivating[runtimeID]; pending {
		s.runtimeMu.Unlock()
		return automations.RuntimeDeactivationResult{}, runtimeLifecycleError(
			"DeactivateRuntime", automations.ErrorCodeConflict,
			fmt.Errorf("runtime %q activation is still in progress", runtimeID),
		)
	}
	instance := s.runtimes[runtimeID]
	s.runtimeMu.Unlock()
	if instance == nil {
		return automations.RuntimeDeactivationResult{
			RuntimeID:  runtimeID,
			State:      automations.RuntimeLifecycleStopped,
			Idempotent: true,
		}, nil
	}

	if err := instance.stop(ctx); err != nil {
		return automations.RuntimeDeactivationResult{}, runtimeLifecycleError(
			"DeactivateRuntime", automations.ErrorCodeFailed, err,
		)
	}
	s.runtimeMu.Lock()
	if s.runtimes[runtimeID] == instance {
		delete(s.runtimes, runtimeID)
	}
	s.runtimeMu.Unlock()
	return automations.RuntimeDeactivationResult{
		RuntimeID: runtimeID,
		State:     automations.RuntimeLifecycleStopped,
	}, nil
}

func normalizeRuntimeActivationRequest(
	request automations.RuntimeActivationRequest,
) (automations.RuntimeActivationRequest, error) {
	request.RuntimeID = strings.TrimSpace(request.RuntimeID)
	if request.RuntimeID == "" {
		return automations.RuntimeActivationRequest{}, runtimeLifecycleError(
			"ActivateRuntime", automations.ErrorCodeInvalid, fmt.Errorf("runtime ID is required"),
		)
	}
	request.FactorySessionID = strings.TrimSpace(request.FactorySessionID)
	if request.FactorySessionID == "" {
		request.FactorySessionID = strings.TrimSpace(request.Snapshot.Invocation.FactorySessionID)
	}
	if request.FactorySessionID == "" {
		request.FactorySessionID = defaultFactorySessionID
	}
	if snapshotSession := strings.TrimSpace(request.Snapshot.Invocation.FactorySessionID); snapshotSession != "" && snapshotSession != request.FactorySessionID {
		return automations.RuntimeActivationRequest{}, runtimeLifecycleError(
			"ActivateRuntime", automations.ErrorCodeInvalid,
			fmt.Errorf("Factory Session ID does not match the snapshot invocation"),
		)
	}
	request.Snapshot.Invocation.FactorySessionID = request.FactorySessionID
	request.Snapshot.FactoryDir = strings.TrimSpace(request.Snapshot.FactoryDir)
	if request.Snapshot.FactoryDir == "" {
		return automations.RuntimeActivationRequest{}, runtimeLifecycleError(
			"ActivateRuntime", automations.ErrorCodeInvalid, fmt.Errorf("Factory Directory is required"),
		)
	}
	request.Snapshot.RuntimeBaseDir = strings.TrimSpace(request.Snapshot.RuntimeBaseDir)
	if request.Snapshot.RuntimeBaseDir == "" {
		request.Snapshot.RuntimeBaseDir = request.Snapshot.FactoryDir
	}
	if strings.TrimSpace(request.Snapshot.EffectiveFactory.Name) == "" {
		return automations.RuntimeActivationRequest{}, runtimeLifecycleError(
			"ActivateRuntime", automations.ErrorCodeInvalid, fmt.Errorf("Factory Definition name is required"),
		)
	}
	if len(request.Snapshot.Workers) == 0 {
		request.Snapshot.Workers = append([]interfaces.FactoryWorkerConfig(nil), request.Snapshot.EffectiveFactory.Workers...)
	}
	if len(request.Snapshot.Workstations) == 0 {
		request.Snapshot.Workstations = append([]interfaces.FactoryWorkstationConfig(nil), request.Snapshot.EffectiveFactory.Workstations...)
	}
	cloned, err := request.Snapshot.Clone()
	if err != nil {
		return automations.RuntimeActivationRequest{}, runtimeLifecycleError(
			"ActivateRuntime", automations.ErrorCodeInvalid, err,
		)
	}
	request.Snapshot = cloned
	request.Inputs.Filesystem.KnownWorkTypes = append([]string(nil), request.Inputs.Filesystem.KnownWorkTypes...)
	request.Inputs.Filesystem.ValidStatesByType = cloneValidStates(request.Inputs.Filesystem.ValidStatesByType)
	return request, nil
}

func (s *Service) buildRuntimeInstance(
	ctx context.Context,
	request automations.RuntimeActivationRequest,
) (*runtimeInstance, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	config, err := newRuntimeSnapshotConfig(request.RuntimeID, request.Snapshot)
	if err != nil {
		return nil, runtimeLifecycleError("ActivateRuntime", automations.ErrorCodeInvalid, err)
	}
	workflowID := request.Snapshot.Invocation.WorkflowID
	if strings.TrimSpace(workflowID) == "" {
		workflowID = s.workflowID
	}
	owner := NewWithCursorFileSystem(
		s.logger(), s.clock, s.commandRunner(), workflowID, request.Snapshot.FactoryDir,
		s.hostedPollers, s.resolveTemplates, s.executionPolicy, s.cursorFileSystem,
	)
	if owner == nil {
		return nil, runtimeLifecycleError(
			"ActivateRuntime", automations.ErrorCodeFailed, fmt.Errorf("runtime Automations owner is unavailable"),
		)
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	instance := &runtimeInstance{
		runtimeID:        request.RuntimeID,
		factorySessionID: request.FactorySessionID,
		snapshot:         request.Snapshot,
		activationInputs: newRuntimeActivationInputIdentity(request.Inputs),
		owner:            owner,
		runtimeConfig:    config,
		submit:           request.Inputs.Submitter,
		startSchedulers:  request.Inputs.StartSchedulers,
		ctx:              runtimeCtx,
		cancel:           cancel,
	}
	filesystemInputs := request.Inputs.Filesystem
	if filesystemInputs.Files != nil && filesystemInputs.WalkDirectory != nil &&
		filesystemInputs.WorkRequestIDs != nil && request.Inputs.Submitter != nil {
		instance.watcher = owner.NewFilesystemWatcher(runtimeFilesystemConfig(
			request.Snapshot.FactoryDir,
			filesystemInputs,
			s.logger(),
			request.Inputs.Submitter,
		))
	}
	if instance.watcher != nil {
		if err := instance.watcher.PreseedInputs(ctx); err != nil {
			instance.stop(context.Background())
			return nil, runtimeLifecycleError("ActivateRuntime", automations.ErrorCodeFailed, fmt.Errorf("preseed inputs: %w", err))
		}
	}
	return instance, nil
}

func newRuntimeSnapshotConfig(runtimeID string, snapshot interfaces.RuntimeSnapshot) (*runtimeSnapshotConfig, error) {
	config, err := interfaces.CloneFactoryConfig(&snapshot.EffectiveFactory)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, fmt.Errorf("Factory Definition configuration is required")
	}
	if len(snapshot.Workers) > 0 {
		config.Workers = make([]interfaces.FactoryWorkerConfig, len(snapshot.Workers))
		for index, worker := range snapshot.Workers {
			config.Workers[index] = interfaces.CloneWorkerConfig(worker)
		}
	}
	if len(snapshot.Workstations) > 0 {
		config.Workstations = make([]interfaces.FactoryWorkstationConfig, len(snapshot.Workstations))
		for index, workstation := range snapshot.Workstations {
			config.Workstations[index] = interfaces.CloneWorkstationConfig(workstation)
		}
	}
	return &runtimeSnapshotConfig{
		factoryDir:     snapshot.FactoryDir,
		runtimeBaseDir: snapshot.RuntimeBaseDir,
		runtimeID:      runtimeID,
		config:         *config,
	}, nil
}

func runtimeFilesystemConfig(
	factoryDir string,
	inputs automations.RuntimeFilesystemInputs,
	logger *zap.Logger,
	submit automations.WorkRequestSubmitter,
) automations.FilesystemWatcherConfig {
	var runtimeLogger *zap.Logger
	if logger != nil {
		runtimeLogger = logger.Named("filesystem")
	}
	return automations.FilesystemWatcherConfig{
		Dir:               filepath.Join(factoryDir, interfaces.InputsDir),
		Logger:            runtimeLogger,
		KnownWorkTypes:    append([]string(nil), inputs.KnownWorkTypes...),
		ValidStatesByType: cloneValidStates(inputs.ValidStatesByType),
		Files:             inputs.Files,
		WalkDirectory:     inputs.WalkDirectory,
		WorkRequestIDs:    inputs.WorkRequestIDs,
		Submitter:         submit,
	}
}

func (instance *runtimeInstance) start(ctx context.Context) error {
	if instance == nil || instance.owner == nil {
		return runtimeLifecycleError("StartRuntime", automations.ErrorCodeFailed, fmt.Errorf("runtime Automations owner is unavailable"))
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if instance.startSchedulers {
		if err := instance.owner.StartSchedulerSidecarsForRuntime(
			instance.ctx,
			&instance.sidecars,
			instance.snapshot.FactoryDir,
			instance.runtimeConfig.FactoryConfig(),
			instance.runtimeConfig,
			instance.submit,
		); err != nil {
			instance.stop(context.Background())
			return runtimeLifecycleError("StartRuntime", automations.ErrorCodeFailed, err)
		}
	}
	if instance.watcher != nil {
		instance.sidecars.Add(1)
		go func() {
			defer instance.sidecars.Done()
			_ = instance.watcher.Watch(instance.ctx)
		}()
	}
	return nil
}

func (instance *runtimeInstance) stop(ctx context.Context) error {
	if instance == nil || instance.cancel == nil {
		return nil
	}
	instance.cancel()
	done := make(chan struct{})
	go func() {
		instance.sidecars.Wait()
		close(done)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) clearRuntimeActivation(runtimeID string) {
	s.runtimeMu.Lock()
	delete(s.runtimeActivating, runtimeID)
	s.runtimeMu.Unlock()
}

func (s *Service) removeRuntime(runtimeID string, expected *runtimeInstance) {
	s.runtimeMu.Lock()
	if s.runtimes[runtimeID] == expected {
		delete(s.runtimes, runtimeID)
	}
	s.runtimeMu.Unlock()
}

func runtimeActivationMatches(instance *runtimeInstance, request automations.RuntimeActivationRequest) bool {
	if instance == nil {
		return false
	}
	return instance.factorySessionID == request.FactorySessionID &&
		reflect.DeepEqual(instance.snapshot, request.Snapshot) &&
		instance.activationInputs.matches(request.Inputs)
}

// runtimeActivationInputIdentity is the value-level identity retained for one
// activated runtime. Runtime inputs contain effect interfaces and callbacks,
// so they cannot be compared with reflect.DeepEqual as a whole: non-nil
// functions are never deeply equal, while an omitted callback is materially
// different from an installed one. Keep presence and type identity for opaque
// effects and deep-copy the plain filesystem policy values. Effect instances
// are intentionally not compared by address: callers commonly construct
// equivalent ports for each activation request.
type runtimeActivationInputIdentity struct {
	startSchedulers bool
	submitter       opaqueActivationIdentity
	filesystem      runtimeFilesystemInputIdentity
}

type runtimeFilesystemInputIdentity struct {
	files          opaqueActivationIdentity
	walkDirectory  opaqueActivationIdentity
	workRequestIDs opaqueActivationIdentity
	knownWorkTypes []string
	validStates    map[string]map[string]bool
}

type opaqueActivationIdentity struct {
	present  bool
	typeName string
}

func newRuntimeActivationInputIdentity(inputs automations.RuntimeActivationInputs) runtimeActivationInputIdentity {
	return runtimeActivationInputIdentity{
		startSchedulers: inputs.StartSchedulers,
		submitter:       newOpaqueActivationIdentity(inputs.Submitter),
		filesystem: runtimeFilesystemInputIdentity{
			files:          newOpaqueActivationIdentity(inputs.Filesystem.Files),
			walkDirectory:  newOpaqueActivationIdentity(inputs.Filesystem.WalkDirectory),
			workRequestIDs: newOpaqueActivationIdentity(inputs.Filesystem.WorkRequestIDs),
			knownWorkTypes: append([]string(nil), inputs.Filesystem.KnownWorkTypes...),
			validStates:    cloneValidStates(inputs.Filesystem.ValidStatesByType),
		},
	}
}

func newOpaqueActivationIdentity(value any) opaqueActivationIdentity {
	if value == nil {
		return opaqueActivationIdentity{}
	}
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return opaqueActivationIdentity{}
	}
	identity := opaqueActivationIdentity{
		present:  true,
		typeName: reflected.Type().String(),
	}
	switch reflected.Kind() {
	case reflect.Func:
		if reflected.IsNil() {
			return opaqueActivationIdentity{}
		}
	case reflect.Pointer, reflect.Chan, reflect.UnsafePointer:
		if reflected.IsNil() {
			return opaqueActivationIdentity{}
		}
	}
	return identity
}

func (identity opaqueActivationIdentity) matches(other opaqueActivationIdentity) bool {
	if identity.present != other.present || identity.typeName != other.typeName {
		return false
	}
	if !identity.present {
		return true
	}
	return true
}

func (identity runtimeActivationInputIdentity) matches(inputs automations.RuntimeActivationInputs) bool {
	other := newRuntimeActivationInputIdentity(inputs)
	return identity.startSchedulers == other.startSchedulers &&
		identity.submitter.matches(other.submitter) &&
		identity.filesystem.matches(other.filesystem)
}

func (identity runtimeFilesystemInputIdentity) matches(other runtimeFilesystemInputIdentity) bool {
	return identity.files.matches(other.files) &&
		identity.walkDirectory.matches(other.walkDirectory) &&
		identity.workRequestIDs.matches(other.workRequestIDs) &&
		reflect.DeepEqual(identity.knownWorkTypes, other.knownWorkTypes) &&
		reflect.DeepEqual(identity.validStates, other.validStates)
}

func runtimeLifecycleError(op string, code automations.ErrorCode, err error) *automations.Error {
	sentinel := runtimeLifecycleSentinel(code)
	if err != nil && sentinel != nil && !errors.Is(err, sentinel) {
		err = fmt.Errorf("%w: %v", sentinel, err)
	}
	return &automations.Error{Op: op, Code: code, Err: err}
}

func runtimeLifecycleSentinel(code automations.ErrorCode) error {
	switch code {
	case automations.ErrorCodeInvalid:
		return automations.ErrInvalidRequest
	case automations.ErrorCodeNotFound:
		return automations.ErrNotFound
	case automations.ErrorCodeConflict:
		return automations.ErrConflict
	case automations.ErrorCodeFailed:
		return automations.ErrSupervisionFailed
	default:
		return nil
	}
}

func cloneValidStates(source map[string]map[string]bool) map[string]map[string]bool {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]map[string]bool, len(source))
	for workType, states := range source {
		clonedStates := make(map[string]bool, len(states))
		for state, valid := range states {
			clonedStates[state] = valid
		}
		cloned[workType] = clonedStates
	}
	return cloned
}
