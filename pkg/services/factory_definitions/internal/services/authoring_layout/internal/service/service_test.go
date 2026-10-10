package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/internal/service"
)

type stubPersistenceFileSystem struct{ t *testing.T }

func (s stubPersistenceFileSystem) MkdirTemp(string, string) (string, error) {
	s.t.Helper()
	s.t.Fatal("unexpected MkdirTemp")
	return "", nil
}
func (s stubPersistenceFileSystem) RemoveAll(string) error {
	s.t.Helper()
	s.t.Fatal("unexpected RemoveAll")
	return nil
}
func (s stubPersistenceFileSystem) Rename(string, string) error {
	s.t.Helper()
	s.t.Fatal("unexpected Rename")
	return nil
}
func (s stubPersistenceFileSystem) Stat(string) (fs.FileInfo, error) {
	s.t.Helper()
	s.t.Fatal("unexpected Stat")
	return nil, nil
}
func (s stubPersistenceFileSystem) MkdirAll(string, fs.FileMode) error {
	s.t.Helper()
	s.t.Fatal("unexpected MkdirAll")
	return nil
}

type stubDirectoryReplacementStore struct{}

func (stubDirectoryReplacementStore) Commit(string, string, string) (string, error) {
	return "", nil
}
func (stubDirectoryReplacementStore) Restore(string, string) {}

// Only the authoring orchestrator is real. Validation, representation and durable
// effects are controlled ports; composed layout round trips live in F-DEF.
type stubValidator struct {
	factorydefinitions.Validator
	validate func(context.Context, factorydefinitions.DefinitionValidationRequest) (factorydefinitions.ValidationResult, error)
}

func (v stubValidator) ValidateDefinition(ctx context.Context, request factorydefinitions.DefinitionValidationRequest) (factorydefinitions.ValidationResult, error) {
	if v.validate != nil {
		return v.validate(ctx, request)
	}
	return factorydefinitions.ValidationResult{}, nil
}
func (stubValidator) PruneLayout(context.Context, *factorydefinitions.FactoryConfig, factorydefinitions.PendingFactoryGraphTopology) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}

func newAuthoringLayoutService(t *testing.T) authoringlayout.Service {
	t.Helper()
	return newControlledAuthoringService(stubValidator{}, stubPersistenceFileSystem{t: t}, stubDirectoryReplacementStore{},
		func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error {
			t.Fatal("unexpected write")
			return nil
		},
		func(string) error { t.Fatal("unexpected staging validation"); return nil })
}
func newControlledAuthoringService(validator stubValidator, fileSystem factorydefinitions.PersistenceFileSystem,
	directories factorydefinitions.DirectoryReplacementStore,
	write func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error, validate func(string) error,
) authoringlayout.Service {
	return authoringlayoutservice.New(validator,
		func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return factorydefinitions.DefinitionValidationRequest{CanonicalPayload: payload}, nil
		},
		func(payload []byte) (*factorydefinitions.FactoryConfig, error) {
			config := &factorydefinitions.FactoryConfig{}
			return config, json.Unmarshal(payload, config)
		},
		func(config *factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error) {
			return config, nil
		},
		func(*factorydefinitions.FactoryConfig) ([]byte, error) { return []byte("controlled canonical"), nil },
		write, validate,
		func(path string) ([]byte, error) { return []byte("flattened:" + path), nil },
		func(path string) (string, factorydefinitions.LayoutExpansionReport, error) {
			return path + "-expanded", factorydefinitions.LayoutExpansionReport{FactoryConfigPaths: 1}, nil
		}, fileSystem, func(string) error { return nil }, directories)
}

func validAlphaPayload(t *testing.T) []byte {
	t.Helper()
	return []byte(`{"name":"alpha"}`)
}

func TestPrepareFactoryLayout_ReturnsPreparedAggregateForValidPayload(t *testing.T) {
	t.Parallel()

	payload := validAlphaPayload(t)
	svc := newAuthoringLayoutService(t)

	result, err := svc.PrepareFactoryLayout(
		context.Background(),
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout: %v", err)
	}
	if result.Prepared.Config == nil {
		t.Fatal("PrepareFactoryLayout prepared config is nil")
	}
	if result.Prepared.Config.Name != "alpha" || string(result.Prepared.Canonical) != "controlled canonical" {
		t.Fatalf("prepared output = %#v / %q", result.Prepared.Config, result.Prepared.Canonical)
	}
}

func TestPrepareFactoryLayout_RejectsMalformedPayloadWithoutFilesystemEffects(t *testing.T) {
	t.Parallel()

	svc := newAuthoringLayoutService(t)
	_, err := svc.PrepareFactoryLayout(
		context.Background(),
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: []byte("{")},
	)
	if !errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
		t.Fatalf("PrepareFactoryLayout malformed error = %v, want %v", err, factorydefinitions.ErrInvalidNamedFactory)
	}
}

func TestFlattenFactoryLayout_ReturnsCanonicalBytes(t *testing.T) {
	t.Parallel()

	svc := newAuthoringLayoutService(t)
	result, err := svc.FlattenFactoryLayout(
		context.Background(),
		factorydefinitions.FlattenFactoryLayoutRequest{Path: "/factories/alpha"},
	)
	if err != nil {
		t.Fatalf("FlattenFactoryLayout: %v", err)
	}
	if string(result.Canonical) != "flattened:/factories/alpha" {
		t.Fatalf("FlattenFactoryLayout canonical = %q, want flattened path marker", result.Canonical)
	}
}

func TestExpandFactoryLayout_ReturnsFactoryDirAndReport(t *testing.T) {
	t.Parallel()

	svc := newAuthoringLayoutService(t)
	result, err := svc.ExpandFactoryLayout(
		context.Background(),
		factorydefinitions.ExpandFactoryLayoutRequest{Path: "/factories/alpha"},
	)
	if err != nil {
		t.Fatalf("ExpandFactoryLayout: %v", err)
	}
	if result.FactoryDir != "/factories/alpha-expanded" {
		t.Fatalf("ExpandFactoryLayout factoryDir = %q, want expanded path", result.FactoryDir)
	}
	if result.Report.FactoryConfigPaths != 1 {
		t.Fatalf("ExpandFactoryLayout report = %#v, want one factory config path", result.Report)
	}
}

func TestFlattenExpandFactoryLayout_RejectsEmptyPath(t *testing.T) {
	t.Parallel()

	svc := newAuthoringLayoutService(t)
	_, flattenErr := svc.FlattenFactoryLayout(
		context.Background(),
		factorydefinitions.FlattenFactoryLayoutRequest{},
	)
	if flattenErr == nil || flattenErr.Error() != "factory path is required" {
		t.Fatalf("FlattenFactoryLayout empty path error = %v, want required path failure", flattenErr)
	}

	_, expandErr := svc.ExpandFactoryLayout(
		context.Background(),
		factorydefinitions.ExpandFactoryLayoutRequest{},
	)
	if expandErr == nil || expandErr.Error() != "factory path is required" {
		t.Fatalf("ExpandFactoryLayout empty path error = %v, want required path failure", expandErr)
	}
}

// memoryLayoutEffects implements only the external ports used by the authoring
// orchestrator. Each leaf owns its state; no peer implementation or OS IO runs.
type memoryLayoutEffects struct {
	directories map[string][]byte
	removed     []string
	commitErr   error
}

func newMemoryLayoutEffects() *memoryLayoutEffects {
	return &memoryLayoutEffects{directories: map[string][]byte{}}
}
func (m *memoryLayoutEffects) MkdirTemp(root, _ string) (string, error) {
	path := filepath.Join(root, ".owned-stage")
	m.directories[path] = nil
	return path, nil
}
func (m *memoryLayoutEffects) RemoveAll(path string) error {
	m.removed = append(m.removed, path)
	delete(m.directories, path)
	return nil
}
func (m *memoryLayoutEffects) Rename(from, to string) error {
	m.directories[to] = m.directories[from]
	delete(m.directories, from)
	return nil
}
func (m *memoryLayoutEffects) Stat(path string) (fs.FileInfo, error) {
	if _, ok := m.directories[path]; ok {
		return nil, nil
	}
	return nil, fs.ErrNotExist
}
func (*memoryLayoutEffects) MkdirAll(string, fs.FileMode) error { return nil }
func (m *memoryLayoutEffects) Commit(root, target, stage string) (string, error) {
	if m.commitErr != nil {
		return "", m.commitErr
	}
	backup := filepath.Join(root, ".owned-backup")
	m.directories[backup] = m.directories[target]
	_ = m.Rename(stage, target)
	return backup, nil
}
func (*memoryLayoutEffects) Restore(string, string) {}

func TestNamedFactory_CommitsPreparedPayloadAndCleansOwnedBackup(t *testing.T) {
	t.Parallel()
	effects := newMemoryLayoutEffects()
	root := filepath.Join("factories", "owned")
	target := filepath.Join(root, "alpha")
	stage := filepath.Join(root, ".owned-stage")
	prepared := factorydefinitions.PreparedFactoryLayoutPayload{Canonical: []byte("new canonical")}
	write := func(path string, got *factorydefinitions.PreparedFactoryLayoutPayload, source string) error {
		if path != stage || source != filepath.Join(target, factorydefinitions.FactoryConfigFile) {
			t.Fatalf("write destination/source = %q/%q", path, source)
		}
		effects.directories[path] = append([]byte(nil), got.Canonical...)
		return nil
	}
	svc := newControlledAuthoringService(stubValidator{}, effects, effects, write, func(path string) error {
		if string(effects.directories[path]) != string(prepared.Canonical) {
			t.Fatal("validation did not receive staged payload")
		}
		return nil
	})
	if len(effects.directories) != 0 {
		t.Fatal("construction performed durable effects")
	}
	created, err := svc.CreateNamedFactory(context.Background(), factorydefinitions.CreateNamedFactoryRequest{
		RootDir: root, Name: "alpha", Prepared: prepared,
	})
	if err != nil || created.Name != "alpha" || created.FactoryDir != target {
		t.Fatalf("create = %#v, %v", created, err)
	}
	if string(effects.directories[target]) != "new canonical" {
		t.Fatal("created payload not committed")
	}
	prepared.Canonical = []byte("replacement canonical")
	replaced, err := svc.ReplaceNamedFactory(context.Background(), factorydefinitions.ReplaceNamedFactoryRequest{
		RootDir: root, Name: "alpha", Prepared: prepared,
	})
	if err != nil || replaced.FactoryDir != target || replaced.Name != "alpha" {
		t.Fatalf("replace = %#v, %v", replaced, err)
	}
	if string(effects.directories[target]) != "replacement canonical" {
		t.Fatal("replacement payload not committed")
	}
	assertOwnedBackupCleanup(t, effects, root)
}

func TestNamedFactory_FailedWriteRetainsPreviousAndCleansOwnedStage(t *testing.T) {
	t.Parallel()
	for _, replace := range []bool{false, true} {
		for _, failure := range []string{"write", "validate", "commit"} {
			if !replace && failure == "commit" {
				continue
			}
			name := failure + "-create"
			if replace {
				name = failure + "-replace"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				effects := newMemoryLayoutEffects()
				root := filepath.Join("factories", "owned")
				target := filepath.Join(root, "alpha")
				peer := filepath.Join(root, "peer")
				effects.directories[peer] = []byte("peer payload")
				if replace {
					effects.directories[target] = []byte("old payload")
				}
				cause := errors.New("controlled " + failure + " failure")
				if failure == "commit" {
					effects.commitErr = cause
				}
				svc := newControlledAuthoringService(stubValidator{}, effects, effects,
					func(path string, prepared *factorydefinitions.PreparedFactoryLayoutPayload, _ string) error {
						effects.directories[path] = prepared.Canonical
						if failure == "write" {
							return cause
						}
						return nil
					}, func(string) error {
						if failure == "validate" {
							return cause
						}
						return nil
					})
				prepared := factorydefinitions.PreparedFactoryLayoutPayload{Canonical: []byte("new payload")}
				var err error
				if replace {
					_, err = svc.ReplaceNamedFactory(context.Background(), factorydefinitions.ReplaceNamedFactoryRequest{
						RootDir: root, Name: "alpha", Prepared: prepared,
					})
				} else {
					_, err = svc.CreateNamedFactory(context.Background(), factorydefinitions.CreateNamedFactoryRequest{
						RootDir: root, Name: "alpha", Prepared: prepared,
					})
				}
				assertAtomicFailure(t, err, cause)
				if failure != "commit" && !errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
					t.Fatalf("error = %v, want invalid named Factory", err)
				}
				assertRejectedLayoutRetainsTargetsAndCleansStage(t, effects, root, replace)
			})
		}
	}
}
func assertAtomicFailure(t *testing.T, err, cause error) {
	t.Helper()
	var failure *factorydefinitions.AtomicFactoryWriteFailure
	if !errors.As(err, &failure) || !errors.Is(err, cause) {
		t.Fatalf("error = %v, want typed atomic failure preserving %v", err, cause)
	}
	if !failure.PreviousPreserved || failure.Name != "alpha" {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestAuthoringLayout_CanceledRequestDoesNotCallPorts(t *testing.T) {
	t.Parallel()
	svc := newAuthoringLayoutService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, prepareErr := svc.PrepareFactoryLayout(ctx, factorydefinitions.PrepareFactoryLayoutRequest{Payload: []byte("{}")})
	_, flattenErr := svc.FlattenFactoryLayout(ctx, factorydefinitions.FlattenFactoryLayoutRequest{Path: "alpha"})
	_, expandErr := svc.ExpandFactoryLayout(ctx, factorydefinitions.ExpandFactoryLayoutRequest{Path: "alpha"})
	_, createErr := svc.CreateNamedFactory(ctx, factorydefinitions.CreateNamedFactoryRequest{RootDir: "factories", Name: "alpha"})
	_, replaceErr := svc.ReplaceNamedFactory(ctx, factorydefinitions.ReplaceNamedFactoryRequest{RootDir: "factories", Name: "alpha"})
	for _, err := range []error{prepareErr, flattenErr, expandErr, createErr, replaceErr} {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want cancellation", err)
		}
		var failure *factorydefinitions.AtomicFactoryWriteFailure
		if errors.As(err, &failure) {
			t.Fatal("cancellation wrapped as atomic write failure")
		}
	}
}

func TestPrepareFactoryLayout_UsesPrePersistValidationAndRetainsFailure(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{factorydefinitions.ErrInvalidNamedFactory, errors.New("validation failed")} {
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()
			payload := validAlphaPayload(t)
			called := false
			validator := stubValidator{validate: func(ctx context.Context, request factorydefinitions.DefinitionValidationRequest) (factorydefinitions.ValidationResult, error) {
				called = true
				if ctx != t.Context() || request.Profile != factorydefinitions.ValidationProfilePrePersist || string(request.CanonicalPayload) != string(payload) {
					t.Fatalf("validation request/context = %#v/%v", request, ctx)
				}
				return factorydefinitions.ValidationResult{}, cause
			}}
			svc := newControlledAuthoringService(validator, stubPersistenceFileSystem{t: t}, stubDirectoryReplacementStore{},
				func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error {
					t.Fatal("unexpected write")
					return nil
				},
				func(string) error { t.Fatal("unexpected staging validation"); return nil })
			result, err := svc.PrepareFactoryLayout(t.Context(), factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload})
			if !called || !errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) || result.Prepared.Config != nil {
				t.Fatalf("rejected preparation = %#v, %v; validation called = %v", result, err, called)
			}
			if errors.Is(cause, factorydefinitions.ErrInvalidNamedFactory) && err != cause { //nolint:errorlint // The domain sentinel must be returned unchanged, without wrapping.
				t.Fatal("typed validation sentinel identity lost")
			}
		})
	}
}

func assertOwnedBackupCleanup(t *testing.T, effects *memoryLayoutEffects, root string) {
	t.Helper()
	if len(effects.directories) != 1 || len(effects.removed) != 1 || effects.removed[0] != filepath.Join(root, ".owned-backup") {
		t.Fatalf("owned backup cleanup = %#v; retained = %#v", effects.removed, effects.directories)
	}
}

func assertRejectedLayoutRetainsTargetsAndCleansStage(t *testing.T, effects *memoryLayoutEffects, root string, replace bool) {
	t.Helper()
	target := filepath.Join(root, "alpha")
	peer := filepath.Join(root, "peer")
	if replace {
		if string(effects.directories[target]) != "old payload" {
			t.Fatal("rejected replacement changed old payload")
		}
	} else if _, exists := effects.directories[target]; exists {
		t.Fatal("rejected create left partial target")
	}
	if string(effects.directories[peer]) != "peer payload" {
		t.Fatal("failure changed peer payload")
	}
	if len(effects.removed) != 1 || effects.removed[0] != filepath.Join(root, ".owned-stage") {
		t.Fatalf("owned stage cleanup = %#v", effects.removed)
	}
}
