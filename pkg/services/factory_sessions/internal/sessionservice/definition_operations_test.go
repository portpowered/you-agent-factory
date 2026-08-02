package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

func TestReadCurrentFactoryForSessionReturnsDetachedFacts(t *testing.T) {
	root := t.TempDir()
	when := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	definitions := newDefinitionOperationsDefinitions(root, factorydefinitions.DefaultCurrentFactoryName, &factorydefinitions.FactoryConfig{
		Name:    "current",
		Version: &factorydefinitions.FactoryVersion{Logical: 7, Physical: when},
	})
	runtime := newDefinitionOperationsRuntime(t, definitions, factoryruntime.ObservationStatusIdle)

	got, err := runtime.ReadCurrentFactoryForSession(context.Background(), factorysessions.DefaultSessionID)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryForSession() error = %v", err)
	}
	if got.Name != factorydefinitions.DefaultCurrentFactoryName {
		t.Fatalf("current Factory name = %q, want %q", got.Name, factorydefinitions.DefaultCurrentFactoryName)
	}
	if got.Version == nil || got.Version.Logical != 7 || !got.Version.Physical.Equal(when) {
		t.Fatalf("current Factory version = %#v, want logical=7 physical=%s", got.Version, when)
	}
	if got.Snapshot == nil || len(*got.Snapshot) == 0 {
		t.Fatal("current Factory snapshot is empty")
	}
	if definitions.captureCalls != 1 {
		t.Fatalf("snapshot capture calls = %d, want 1", definitions.captureCalls)
	}
}

func TestWithDefinitionVersionWritesFactoryConfigCompatibleJSON(t *testing.T) {
	when := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	canonical, err := withDefinitionVersion([]byte(`{"name":"current"}`), factorydefinitions.FactoryVersion{Logical: 9, Physical: when})
	if err != nil {
		t.Fatalf("withDefinitionVersion() error = %v", err)
	}
	var config factorydefinitions.FactoryConfig
	if err := json.Unmarshal(canonical, &config); err != nil {
		t.Fatalf("persisted Factory JSON unmarshal error = %v", err)
	}
	if config.Version == nil || config.Version.Logical != 9 || !config.Version.Physical.Equal(when) {
		t.Fatalf("persisted version = %#v, want logical=9 physical=%s", config.Version, when)
	}
}

func TestReadCurrentFactoryForSessionPropagatesCurrentPointerErrors(t *testing.T) {
	root := t.TempDir()
	pointerErr := errors.New("current pointer unreadable")
	definitions := newDefinitionOperationsDefinitions(root, factorydefinitions.DefaultCurrentFactoryName, &factorydefinitions.FactoryConfig{Name: "current"})
	definitions.currentPointerErr = pointerErr
	runtime := newDefinitionOperationsRuntime(t, definitions, factoryruntime.ObservationStatusIdle)

	_, err := runtime.ReadCurrentFactoryForSession(context.Background(), factorysessions.DefaultSessionID)
	if !errors.Is(err, pointerErr) {
		t.Fatalf("ReadCurrentFactoryForSession() error = %v, want current pointer error", err)
	}
}

func TestSaveFactoryForSessionReplacesCurrentAndActivates(t *testing.T) {
	root := t.TempDir()
	when := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	definitions := newDefinitionOperationsDefinitions(root, factorydefinitions.DefaultCurrentFactoryName, &factorydefinitions.FactoryConfig{
		Name:    "current",
		Version: &factorydefinitions.FactoryVersion{Logical: 1, Physical: when},
	})
	runtime := newDefinitionOperationsRuntime(t, definitions, factoryruntime.ObservationStatusIdle)
	next := &factorydefinitions.FactoryConfig{
		Name:    "current",
		Version: &factorydefinitions.FactoryVersion{Logical: 2, Physical: when.Add(time.Second)},
	}
	runtime.runtimeBuild = definitionOperationsBuilder{build: func(_ string) factoryruntime.HostedInstance {
		return newDefinitionOperationsInstance(root, next, factoryruntime.ObservationStatusIdle)
	}}

	got, err := runtime.SaveFactoryForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		factorydefinitions.SaveModeReplaceCurrent,
		factorydefinitions.EditableFactory{
			Snapshot: definitionOperationsSnapshot(t, map[string]any{"name": "edited", "work_types": []any{}, "workers": []any{}, "workstations": []any{}}),
			Version:  &factorydefinitions.FactoryVersion{Logical: 2, Physical: when.Add(time.Second)},
		},
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession() error = %v", err)
	}
	if got.Name != factorydefinitions.DefaultCurrentFactoryName || got.Version == nil || got.Version.Logical != 2 {
		t.Fatalf("saved current Factory = %#v, want current name and logical version 2", got)
	}
	if definitions.replaceCalls != 1 || definitions.restoreCalls != 0 || definitions.discardCalls != 1 {
		t.Fatalf("layout transaction counts = replace:%d restore:%d discard:%d, want 1/0/1", definitions.replaceCalls, definitions.restoreCalls, definitions.discardCalls)
	}
	if runtime.runtimeState.ActiveHandle() == nil || runtime.runtimeState.ActiveHandle().RuntimeInstance().Directory() != root {
		t.Fatalf("active runtime after save = %#v, want replacement at %q", runtime.runtimeState.ActiveHandle(), root)
	}
}

func TestSaveFactoryForSessionRejectsStaleVersionBeforeLayoutMutation(t *testing.T) {
	root := t.TempDir()
	when := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	definitions := newDefinitionOperationsDefinitions(root, factorydefinitions.DefaultCurrentFactoryName, &factorydefinitions.FactoryConfig{
		Name:    "current",
		Version: &factorydefinitions.FactoryVersion{Logical: 5, Physical: when},
	})
	runtime := newDefinitionOperationsRuntime(t, definitions, factoryruntime.ObservationStatusIdle)

	_, err := runtime.SaveFactoryForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		factorydefinitions.SaveModeReplaceCurrent,
		factorydefinitions.EditableFactory{
			Snapshot: definitionOperationsSnapshot(t, map[string]any{"name": "stale"}),
			Version:  &factorydefinitions.FactoryVersion{Logical: 4, Physical: when.Add(time.Second)},
		},
	)
	if !errors.Is(err, factorydefinitions.ErrFactoryVersionStale) {
		t.Fatalf("SaveFactoryForSession() error = %v, want ErrFactoryVersionStale", err)
	}
	if definitions.replaceCalls != 0 {
		t.Fatalf("layout replacement calls = %d, want 0 for stale input", definitions.replaceCalls)
	}
}

func TestSaveFactoryForSessionRestoresCurrentLayoutWhenRuntimeIsNotIdle(t *testing.T) {
	root := t.TempDir()
	when := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	definitions := newDefinitionOperationsDefinitions(root, factorydefinitions.DefaultCurrentFactoryName, &factorydefinitions.FactoryConfig{
		Name:    "current",
		Version: &factorydefinitions.FactoryVersion{Logical: 1, Physical: when},
	})
	runtime := newDefinitionOperationsRuntime(t, definitions, factoryruntime.ObservationStatusActive)
	runtime.runtimeBuild = definitionOperationsBuilder{build: func(dir string) factoryruntime.HostedInstance {
		return newDefinitionOperationsInstance(dir, definitions.config, factoryruntime.ObservationStatusIdle)
	}}

	_, err := runtime.SaveFactoryForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		factorydefinitions.SaveModeReplaceCurrent,
		factorydefinitions.EditableFactory{
			Snapshot: definitionOperationsSnapshot(t, map[string]any{"name": "edited"}),
			Version:  &factorydefinitions.FactoryVersion{Logical: 2, Physical: when.Add(time.Second)},
		},
	)
	if !errors.Is(err, factorydefinitions.ErrFactoryActivationRequiresIdle) {
		t.Fatalf("SaveFactoryForSession() error = %v, want ErrFactoryActivationRequiresIdle", err)
	}
	if definitions.restoreCalls != 1 || definitions.discardCalls != 0 {
		t.Fatalf("failed current save transaction = restore:%d discard:%d, want 1/0", definitions.restoreCalls, definitions.discardCalls)
	}
}

func TestSaveFactoryForSessionRollsBackNewNamedFactoryAndPointer(t *testing.T) {
	root := t.TempDir()
	when := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	definitions := newDefinitionOperationsDefinitions(root, factorydefinitions.DefaultCurrentFactoryName, &factorydefinitions.FactoryConfig{
		Name:    "current",
		Version: &factorydefinitions.FactoryVersion{Logical: 1, Physical: when},
	})
	runtime := newDefinitionOperationsRuntime(t, definitions, factoryruntime.ObservationStatusActive)

	_, err := runtime.SaveFactoryForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		factorydefinitions.SaveModeUpsertNamedAndActivate,
		factorydefinitions.EditableFactory{
			Name:     "alpha",
			Snapshot: definitionOperationsSnapshot(t, map[string]any{"name": "alpha"}),
		},
	)
	if !errors.Is(err, factorydefinitions.ErrFactoryActivationRequiresIdle) {
		t.Fatalf("SaveFactoryForSession() error = %v, want idle failure", err)
	}
	if definitions.createdName != "alpha" || definitions.deletedName != "alpha" {
		t.Fatalf("named create/delete = (%q, %q), want alpha/alpha", definitions.createdName, definitions.deletedName)
	}
	if _, exists := definitions.namedFactories["alpha"]; exists {
		t.Fatal("newly created named Factory survived rollback")
	}
	if definitions.currentName != factorydefinitions.DefaultCurrentFactoryName {
		t.Fatalf("current pointer after rollback = %q, want %q", definitions.currentName, factorydefinitions.DefaultCurrentFactoryName)
	}
	if runtime.runtimeState.ActiveHandle() == nil || runtime.runtimeState.ActiveHandle().RuntimeInstance().Directory() != root {
		t.Fatal("active Factory runtime was lost while rolling back named activation")
	}
}

func TestSaveFactoryForSessionRestoresExistingNamedLayoutAndKeepsActivationPrimary(t *testing.T) {
	root := t.TempDir()
	when := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	alphaDir := filepath.Join(root, "alpha")
	activationErr := errors.New("activation failed")
	definitions := newDefinitionOperationsDefinitions(root, factorydefinitions.DefaultCurrentFactoryName, &factorydefinitions.FactoryConfig{
		Name:    "alpha",
		Version: &factorydefinitions.FactoryVersion{Logical: 3, Physical: when},
	})
	definitions.namedFactories["alpha"] = alphaDir
	definitions.configByDir[alphaDir] = definitions.config
	runtime := newDefinitionOperationsRuntime(t, definitions, factoryruntime.ObservationStatusIdle)
	runtime.runtimeBuild = definitionOperationsBuilder{err: activationErr}

	_, err := runtime.SaveFactoryForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		factorydefinitions.SaveModeUpsertNamedAndActivate,
		factorydefinitions.EditableFactory{
			Name:     "alpha",
			Snapshot: definitionOperationsSnapshot(t, map[string]any{"name": "alpha", "revision": "next"}),
			Version:  &factorydefinitions.FactoryVersion{Logical: 4, Physical: when.Add(time.Second)},
		},
	)
	if !errors.Is(err, activationErr) {
		t.Fatalf("SaveFactoryForSession() error = %v, want activation error to remain primary", err)
	}
	if definitions.restoreCalls != 1 || definitions.discardCalls != 0 {
		t.Fatalf("existing named rollback = restore:%d discard:%d, want 1/0", definitions.restoreCalls, definitions.discardCalls)
	}
	if definitions.currentName != factorydefinitions.DefaultCurrentFactoryName {
		t.Fatalf("current pointer after existing named rollback = %q, want %q", definitions.currentName, factorydefinitions.DefaultCurrentFactoryName)
	}
	if definitions.deletedName != "" {
		t.Fatalf("existing named rollback deleted %q, want no deletion", definitions.deletedName)
	}
}

func TestSaveFactoryForSessionKeepsFirstRollbackFailure(t *testing.T) {
	root := t.TempDir()
	activationErr := errors.New("activation failed")
	clearErr := errors.New("clear pointer failed")
	deleteErr := errors.New("delete named Factory failed")
	definitions := newDefinitionOperationsDefinitions(root, "", &factorydefinitions.FactoryConfig{
		Name:    "current",
		Version: &factorydefinitions.FactoryVersion{Logical: 1, Physical: time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)},
	})
	definitions.clearErr = clearErr
	definitions.deleteErr = deleteErr
	runtime := newDefinitionOperationsRuntime(t, definitions, factoryruntime.ObservationStatusIdle)
	runtime.runtimeBuild = definitionOperationsBuilder{err: activationErr}

	_, err := runtime.SaveFactoryForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		factorydefinitions.SaveModeUpsertNamedAndActivate,
		factorydefinitions.EditableFactory{
			Name:     "alpha",
			Snapshot: definitionOperationsSnapshot(t, map[string]any{"name": "alpha"}),
		},
	)
	if !errors.Is(err, activationErr) {
		t.Fatalf("SaveFactoryForSession() error = %v, want activation error", err)
	}
	if !stringsContains(err.Error(), clearErr.Error()) || stringsContains(err.Error(), deleteErr.Error()) {
		t.Fatalf("SaveFactoryForSession() error = %v, want first rollback error only", err)
	}
	if definitions.clearCalls != 1 || definitions.deletedName != "alpha" {
		t.Fatalf("rollback attempts = clear:%d delete:%q, want 1/alpha", definitions.clearCalls, definitions.deletedName)
	}
}

func TestActivateFactoryPersistsPointerThroughDefinitionsAndRebindsRuntime(t *testing.T) {
	root := t.TempDir()
	definitions := newDefinitionOperationsDefinitions(root, factorydefinitions.DefaultCurrentFactoryName, &factorydefinitions.FactoryConfig{Name: "current"})
	alphaDir := filepath.Join(root, "alpha")
	definitions.namedFactories["alpha"] = alphaDir
	runtime := newDefinitionOperationsRuntime(t, definitions, factoryruntime.ObservationStatusIdle)
	runtime.runtimeBuild = definitionOperationsBuilder{build: func(dir string) factoryruntime.HostedInstance {
		return newDefinitionOperationsInstance(dir, &factorydefinitions.FactoryConfig{Name: "alpha"}, factoryruntime.ObservationStatusIdle)
	}}

	if err := runtime.ActivateFactory(context.Background(), "alpha"); err != nil {
		t.Fatalf("ActivateFactory() error = %v", err)
	}
	if definitions.currentName != "alpha" {
		t.Fatalf("current pointer = %q, want alpha", definitions.currentName)
	}
	if runtime.runtimeState.ActiveHandle() == nil || runtime.runtimeState.ActiveHandle().RuntimeInstance().Directory() != alphaDir {
		t.Fatalf("active runtime = %#v, want replacement at %q", runtime.runtimeState.ActiveHandle(), alphaDir)
	}
}

func TestActivateFactoryRestoresPointerWhenRuntimeReplacementFails(t *testing.T) {
	root := t.TempDir()
	activationErr := errors.New("runtime replacement failed")
	definitions := newDefinitionOperationsDefinitions(root, factorydefinitions.DefaultCurrentFactoryName, &factorydefinitions.FactoryConfig{Name: "current"})
	alphaDir := filepath.Join(root, "alpha")
	definitions.namedFactories["alpha"] = alphaDir
	runtime := newDefinitionOperationsRuntime(t, definitions, factoryruntime.ObservationStatusIdle)
	runtime.runtimeBuild = definitionOperationsBuilder{build: func(dir string) factoryruntime.HostedInstance {
		return newDefinitionOperationsInstance(dir, &factorydefinitions.FactoryConfig{Name: "alpha"}, factoryruntime.ObservationStatusIdle)
	}}
	runtime.runtimeLifecycle = definitionOperationsLifecycle{startErr: activationErr}

	if err := runtime.ActivateFactory(context.Background(), "alpha"); !errors.Is(err, activationErr) {
		t.Fatalf("ActivateFactory() error = %v, want runtime replacement failure", err)
	}
	if definitions.currentName != factorydefinitions.DefaultCurrentFactoryName {
		t.Fatalf("current pointer after failed activation = %q, want %q", definitions.currentName, factorydefinitions.DefaultCurrentFactoryName)
	}
}

func TestDefinitionOperationsActivationLockUsesDeterministicBarrier(t *testing.T) {
	root := t.TempDir()
	definitions := newDefinitionOperationsDefinitions(root, factorydefinitions.DefaultCurrentFactoryName, &factorydefinitions.FactoryConfig{Name: "current"})
	runtime := newDefinitionOperationsRuntime(t, definitions, factoryruntime.ObservationStatusIdle)
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- runtime.sessionState.WithActivationLock(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- runtime.sessionState.WithActivationLock(func() error { return nil })
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("second activation lock completed early with %v", err)
	default:
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first activation lock error = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second activation lock error = %v", err)
	}
}

type definitionOperationsDefinitions struct {
	factorydefinitions.UnimplementedService
	rootDir           string
	currentName       string
	currentPointerErr error
	config            *factorydefinitions.FactoryConfig
	configByDir       map[string]*factorydefinitions.FactoryConfig
	namedFactories    map[string]string
	setErrors         map[string]error
	clearErr          error
	deleteErr         error
	captureCalls      int
	replaceCalls      int
	restoreCalls      int
	discardCalls      int
	createdName       string
	deletedName       string
	clearCalls        int
}

func newDefinitionOperationsDefinitions(root, currentName string, config *factorydefinitions.FactoryConfig) *definitionOperationsDefinitions {
	return &definitionOperationsDefinitions{
		rootDir:        root,
		currentName:    currentName,
		config:         config,
		configByDir:    map[string]*factorydefinitions.FactoryConfig{root: config},
		namedFactories: map[string]string{},
		setErrors:      map[string]error{},
	}
}

func (d *definitionOperationsDefinitions) GetCurrentFactoryPointer(context.Context, factorydefinitions.GetCurrentFactoryPointerRequest) (factorydefinitions.GetCurrentFactoryPointerResult, error) {
	if d.currentPointerErr != nil {
		return factorydefinitions.GetCurrentFactoryPointerResult{}, d.currentPointerErr
	}
	if d.currentName == "" {
		return factorydefinitions.GetCurrentFactoryPointerResult{}, factorydefinitions.ErrCurrentFactoryNotFound
	}
	dir := d.rootDir
	if named, ok := d.namedFactories[d.currentName]; ok {
		dir = named
	}
	return factorydefinitions.GetCurrentFactoryPointerResult{Name: d.currentName, FactoryDir: dir}, nil
}

func (d *definitionOperationsDefinitions) SetCurrentFactoryPointer(_ context.Context, request factorydefinitions.SetCurrentFactoryPointerRequest) (factorydefinitions.SetCurrentFactoryPointerResult, error) {
	if err := d.setErrors[request.Name]; err != nil {
		return factorydefinitions.SetCurrentFactoryPointerResult{}, err
	}
	d.currentName = request.Name
	return factorydefinitions.SetCurrentFactoryPointerResult{Name: request.Name}, nil
}

func (d *definitionOperationsDefinitions) ClearCurrentFactoryPointer(context.Context, factorydefinitions.ClearCurrentFactoryPointerRequest) (factorydefinitions.ClearCurrentFactoryPointerResult, error) {
	d.clearCalls++
	if d.clearErr != nil {
		return factorydefinitions.ClearCurrentFactoryPointerResult{}, d.clearErr
	}
	d.currentName = ""
	return factorydefinitions.ClearCurrentFactoryPointerResult{}, nil
}

func (d *definitionOperationsDefinitions) GetNamedFactory(_ context.Context, request factorydefinitions.GetNamedFactoryRequest) (factorydefinitions.GetNamedFactoryResult, error) {
	dir, ok := d.namedFactories[request.Name]
	if !ok {
		return factorydefinitions.GetNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
	}
	return factorydefinitions.GetNamedFactoryResult{Entry: factorydefinitions.NamedFactoryListEntry{Name: request.Name, FactoryDir: dir}}, nil
}

func (d *definitionOperationsDefinitions) ValidateStructuralFactoryDefinition(context.Context, factorydefinitions.ValidateStructuralFactoryDefinitionRequest) (factorydefinitions.ValidateStructuralFactoryDefinitionResult, error) {
	return factorydefinitions.ValidateStructuralFactoryDefinitionResult{}, nil
}

func (d *definitionOperationsDefinitions) PrepareFactoryLayout(_ context.Context, request factorydefinitions.PrepareFactoryLayoutRequest) (factorydefinitions.PrepareFactoryLayoutResult, error) {
	return factorydefinitions.PrepareFactoryLayoutResult{Prepared: factorydefinitions.PreparedFactoryLayoutPayload{
		Canonical: append([]byte(nil), request.Payload...), RootFileName: factorydefinitions.FactoryConfigFile,
	}}, nil
}

func (d *definitionOperationsDefinitions) ReplaceFactoryLayoutAtDir(context.Context, factorydefinitions.ReplaceFactoryLayoutAtDirRequest) (factorydefinitions.ReplaceFactoryLayoutAtDirResult, error) {
	d.replaceCalls++
	return factorydefinitions.ReplaceFactoryLayoutAtDirResult{Replacement: &factorydefinitions.FactorySplitLayoutReplaceResult{
		Restore: func() { d.restoreCalls++ }, DiscardBackup: func() { d.discardCalls++ },
	}}, nil
}

func (d *definitionOperationsDefinitions) CreateNamedFactory(_ context.Context, request factorydefinitions.CreateNamedFactoryRequest) (factorydefinitions.CreateNamedFactoryResult, error) {
	d.createdName = request.Name
	dir := filepath.Join(request.RootDir, request.Name)
	d.namedFactories[request.Name] = dir
	d.configByDir[dir] = d.config
	return factorydefinitions.CreateNamedFactoryResult{Name: request.Name, FactoryDir: dir}, nil
}

func (d *definitionOperationsDefinitions) DeleteNamedFactory(_ context.Context, request factorydefinitions.DeleteNamedFactoryRequest) (factorydefinitions.DeleteNamedFactoryResult, error) {
	d.deletedName = request.Name
	if d.deleteErr != nil {
		return factorydefinitions.DeleteNamedFactoryResult{}, d.deleteErr
	}
	delete(d.namedFactories, request.Name)
	return factorydefinitions.DeleteNamedFactoryResult{Name: request.Name}, nil
}

func (d *definitionOperationsDefinitions) CaptureFactorySnapshot(_ context.Context, request factorydefinitions.CaptureFactorySnapshotRequest) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	d.captureCalls++
	snapshot := factorydefinitions.FactorySnapshot(append([]byte(nil), request.Canonical...))
	return factorydefinitions.CaptureFactorySnapshotResult{Snapshot: &snapshot}, nil
}

func (d *definitionOperationsDefinitions) loaded(dir string) *definitionOperationsLoadedSource {
	config := d.configByDir[dir]
	if config == nil {
		config = d.config
	}
	return &definitionOperationsLoadedSource{dir: dir, config: config}
}

func newDefinitionOperationsRuntime(t *testing.T, definitions *definitionOperationsDefinitions, status factoryruntime.ObservationStatus) *SessionRuntime {
	t.Helper()
	clock := platformclock.NewDeterministic(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), time.Millisecond)
	registry := sessionregistry.New()
	responses := responsestream.NewRegistry(func() *responsestream.SessionResponseStream {
		return responsestream.NewSessionResponseStream(clock)
	}, clock)
	state := sessionruntime.New(registry, responses, nil, clock, func() string { return "response-event-test-id" }, func() string { return "session-test-id" })
	if state == nil {
		t.Fatal("session runtime state is nil")
	}
	initialDir := definitions.rootDir
	if definitions.currentName != "" && definitions.currentName != factorydefinitions.DefaultCurrentFactoryName {
		initialDir = definitions.namedFactories[definitions.currentName]
		if initialDir == "" {
			initialDir = filepath.Join(definitions.rootDir, definitions.currentName)
			definitions.configByDir[initialDir] = definitions.config
		}
	}
	initial := newDefinitionOperationsInstance(initialDir, definitions.config, status)
	handle := initial.handle
	id := runtimebinding.Register(state, runtimebinding.Registration{
		SessionID:      factorysessions.DefaultSessionID,
		FactoryRootDir: definitions.rootDir,
		Handle:         handle,
		Target:         factorysessions.Target{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, FactoryDir: initialDir, FolderPath: definitions.rootDir},
		Select:         true,
	})
	if id == "" {
		t.Fatal("runtimebinding.Register returned an empty session ID")
	}
	runtime := &SessionRuntime{
		factoryRootDir: definitions.rootDir,
		dir:            definitions.rootDir,
		sessionState:   state,
		definitions:    definitions,
		clock:          clock,
		logger:         zap.NewNop(),
		runtimeMode:    factorydefinitions.RuntimeModeBatch,
		runtimeBuild: definitionOperationsBuilder{build: func(dir string) factoryruntime.HostedInstance {
			return newDefinitionOperationsInstance(dir, definitions.config, factoryruntime.ObservationStatusIdle)
		}},
		runtimeLifecycle: definitionOperationsLifecycle{},
		identity:         definitionOperationsIdentity{},
		loadFactory: func(dir string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return definitions.loaded(dir), nil
		},
	}
	runtime.runtimeState.SetActive(context.Background(), id, handle)
	return runtime
}

func newDefinitionOperationsInstance(dir string, config *factorydefinitions.FactoryConfig, status factoryruntime.ObservationStatus) *definitionOperationsInstance {
	loaded := &definitionOperationsLoadedSource{dir: dir, config: config}
	service := &definitionOperationsRuntimeService{status: status}
	instance := &definitionOperationsInstance{config: loaded, service: service}
	instance.handle = &definitionOperationsHandle{instance: instance, done: make(chan struct{})}
	return instance
}

func definitionOperationsSnapshot(t *testing.T, value any) *factorydefinitions.FactorySnapshot {
	t.Helper()
	snapshot, err := factorydefinitions.NewFactorySnapshot(value)
	if err != nil {
		t.Fatalf("NewFactorySnapshot() error = %v", err)
	}
	return snapshot
}

func stringsContains(value, want string) bool {
	for i := 0; i+len(want) <= len(value); i++ {
		if value[i:i+len(want)] == want {
			return true
		}
	}
	return want == ""
}

type definitionOperationsLoadedSource struct {
	dir    string
	config *factorydefinitions.FactoryConfig
}

func (s *definitionOperationsLoadedSource) FactoryConfig() *factorydefinitions.FactoryConfig {
	return s.config
}
func (s *definitionOperationsLoadedSource) FactoryDir() string           { return s.dir }
func (s *definitionOperationsLoadedSource) RuntimeBaseDir() string       { return s.dir }
func (s *definitionOperationsLoadedSource) SetRuntimeBaseDir(dir string) { s.dir = dir }
func (*definitionOperationsLoadedSource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return nil
}
func (*definitionOperationsLoadedSource) MutateWorkers(func(*factorydefinitions.FactoryWorkerConfig) error) error {
	return nil
}
func (*definitionOperationsLoadedSource) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}
func (*definitionOperationsLoadedSource) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}

type definitionOperationsRuntimeService struct {
	factoryruntime.Service
	status factoryruntime.ObservationStatus
}

func (s *definitionOperationsRuntimeService) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{Observation: factoryruntime.Observation{
		Status: s.status,
		Health: factoryruntime.ObservationHealth{FactoryState: string(factorydefinitions.FactoryState(s.status))},
	}}, nil
}

type definitionOperationsInstance struct {
	config  *definitionOperationsLoadedSource
	service factoryruntime.Service
	handle  *definitionOperationsHandle
}

func (i *definitionOperationsInstance) RuntimeService() factoryruntime.Service { return i.service }
func (i *definitionOperationsInstance) Directory() string                      { return i.config.dir }
func (i *definitionOperationsInstance) FolderDirectory() string                { return i.config.dir }
func (*definitionOperationsInstance) BackendScope() string                     { return "test-scope" }
func (*definitionOperationsInstance) StartTime() time.Time                     { return time.Time{} }
func (i *definitionOperationsInstance) LoadedRuntimeConfig() factoryruntime.LoadedConfig {
	return i.config
}
func (*definitionOperationsInstance) CanonicalEvents() []factorydefinitions.FactoryEvent { return nil }
func (*definitionOperationsInstance) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {
}
func (*definitionOperationsInstance) StreamGeneration() string                      { return "test-generation" }
func (*definitionOperationsInstance) RuntimeLogger() *zap.Logger                    { return zap.NewNop() }
func (*definitionOperationsInstance) RuntimeMetrics() factoryruntime.MetricsEmitter { return nil }
func (*definitionOperationsInstance) RuntimeDiagnostics() factoryruntime.RuntimeLogDiagnostics {
	return factoryruntime.RuntimeLogDiagnostics{}
}
func (*definitionOperationsInstance) RecordingLedger() recordings.Ledger { return nil }
func (*definitionOperationsInstance) CloseArtifacts() error              { return nil }

type definitionOperationsHandle struct {
	instance factoryruntime.HostedInstance
	done     chan struct{}
	once     sync.Once
}

func (h *definitionOperationsHandle) RuntimeInstance() factoryruntime.HostedInstance {
	return h.instance
}
func (*definitionOperationsHandle) Completed() bool              { return false }
func (*definitionOperationsHandle) Result() error                { return nil }
func (*definitionOperationsHandle) Wait() error                  { return nil }
func (h *definitionOperationsHandle) CancelRun()                 { h.once.Do(func() { close(h.done) }) }
func (h *definitionOperationsHandle) RunDoneCh() <-chan struct{} { return h.done }

type definitionOperationsBuilder struct {
	build func(string) factoryruntime.HostedInstance
	err   error
}

func (b definitionOperationsBuilder) BuildReplacement(_ context.Context, _, factoryDir, _, _ string) (factoryruntime.HostedInstance, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.build == nil {
		return nil, fmt.Errorf("replacement builder is not configured")
	}
	return b.build(factoryDir), nil
}

type definitionOperationsIdentity struct{}

func (definitionOperationsIdentity) Normalize(_ context.Context, request identity.NormalizeRequest) (identity.ResolvedIdentity, error) {
	return identity.ResolvedIdentity{
		Reference:           factorysessions.CanonicalLogicalTargetReference{BackendScopeID: request.BackendScopeID, FolderPath: request.FolderPath, Kind: factorysessions.LogicalTargetKindDefault},
		LogicalSessionKeyID: "definition-operations-test-key",
		RuntimeTarget:       factorysessions.RuntimeLogicalTarget{FolderPath: request.FolderPath, Kind: string(factorysessions.LogicalTargetKindDefault)},
	}, nil
}
func (definitionOperationsIdentity) NormalizeProvider(context.Context, identity.NormalizeProviderRequest) (identity.ResolvedIdentity, error) {
	return identity.ResolvedIdentity{LogicalSessionKeyID: "definition-operations-test-key"}, nil
}
func (definitionOperationsIdentity) Discover(context.Context, identity.DiscoverRequest) ([]factorysessions.Target, error) {
	return nil, nil
}
func (definitionOperationsIdentity) ResolveFolder(folder string) (string, error) { return folder, nil }
func (definitionOperationsIdentity) Select(targets []factorysessions.Target, _ *factorysessions.TargetRef) (*factorysessions.Target, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	return &targets[0], nil
}
func (definitionOperationsIdentity) Resolve(sessionregistry.Service, string) *livesession.LiveSession {
	return nil
}
func (definitionOperationsIdentity) ResolveLogical(sessionregistry.Service, string, string) *livesession.LiveSession {
	return nil
}

type definitionOperationsLifecycle struct {
	startErr error
}

func (l definitionOperationsLifecycle) Start(_ context.Context, instance factoryruntime.HostedInstance) (factoryruntime.HostedHandle, error) {
	if l.startErr != nil {
		return nil, l.startErr
	}
	if instance == nil {
		return nil, fmt.Errorf("replacement instance is required")
	}
	return &definitionOperationsHandle{instance: instance, done: make(chan struct{})}, nil
}
func (definitionOperationsLifecycle) WaitForStart(context.Context, factoryruntime.HostedHandle) error {
	return nil
}
func (definitionOperationsLifecycle) Stop(factoryruntime.HostedHandle) error   { return nil }
func (definitionOperationsLifecycle) StopSidecars(factoryruntime.HostedHandle) {}
func (definitionOperationsLifecycle) PublishReplacement(context.Context, factoryruntime.HostedHandle, factoryruntime.HostedInstance) error {
	return nil
}

var _ factoryruntime.HostedInstance = (*definitionOperationsInstance)(nil)
var _ factoryruntime.HostedHandle = (*definitionOperationsHandle)(nil)
var _ factoryruntime.ReplacementBuilder = definitionOperationsBuilder{}
var _ factoryruntime.Lifecycle = definitionOperationsLifecycle{}
