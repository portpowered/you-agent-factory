package internal_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

type source struct {
	roots    map[string][]factorydefinitions.EffectiveFactoryCatalogCandidate
	packaged []factorydefinitions.EffectiveFactoryCatalogCandidate
}

type baseService struct {
	factorydefinitions.Service
}

func (s source) discovery() factorydefinitions.EffectiveFactoryCatalogDiscovery {
	return factorydefinitions.EffectiveFactoryCatalogDiscovery{
		ListRoot:     s.listRoot,
		ListPackaged: s.listPackaged,
	}
}

func (s source) listRoot(
	_ context.Context,
	root string,
) ([]factorydefinitions.EffectiveFactoryCatalogCandidate, error) {
	return cloneCandidates(s.roots[root]), nil
}

func (s source) listPackaged(
	context.Context,
) ([]factorydefinitions.EffectiveFactoryCatalogCandidate, error) {
	return cloneCandidates(s.packaged), nil
}

func TestCatalogPrecedenceShadowingOrderingAndDetachedDefinitions(t *testing.T) {
	t.Parallel()

	projectRoot := "/project/factories"
	globalRoot := "/global/factories"
	catalog := newCatalog(t, source{
		roots: map[string][]factorydefinitions.EffectiveFactoryCatalogCandidate{
			projectRoot: {
				candidate("zeta", projectRoot, "project-zeta"),
				candidate("shared", projectRoot, "project-shared"),
			},
			globalRoot: {
				candidate("global-only", globalRoot, "global-only"),
				candidate("shared", globalRoot, "global-shared"),
				candidate("@you/goal", globalRoot, "installed-goal"),
			},
		},
		packaged: []factorydefinitions.EffectiveFactoryCatalogCandidate{
			packagedCandidate("@you/review", "packaged-review"),
			packagedCandidate("shared", "packaged-shared"),
			packagedCandidate("@you/goal", "packaged-goal"),
		},
	}.discovery())

	first := list(t, catalog, projectRoot, globalRoot)
	wantNames := []string{"@you/goal", "@you/review", "global-only", "shared", "zeta"}
	if got := names(first.Entries); !slices.Equal(got, wantNames) {
		t.Fatalf("effective names = %v, want %v", got, wantNames)
	}
	assertProject(t, first.Entries, "shared", "project-shared")
	assertProject(t, first.Entries, "@you/goal", "installed-goal")
	assertProject(t, first.Entries, "@you/review", "packaged-review")
	if catalogEntry := entry(t, first.Entries, "@you/review"); catalogEntry.Location != nil {
		t.Fatalf("packaged-only location = %q, want nil", *catalogEntry.Location)
	}

	first.Entries[0].Definition.Project = "mutated"
	first.Entries[0].InvocationSignature.Parameters[0].Name = "mutated"
	*first.Entries[2].Location = "/mutated"
	second := list(t, catalog, projectRoot, globalRoot)
	assertProject(t, second.Entries, "@you/goal", "installed-goal")
	if got := second.Entries[0].InvocationSignature.Parameters[0].Name; got != "prompt" {
		t.Fatalf("detached signature parameter = %q, want prompt", got)
	}
	if got := *entry(t, second.Entries, "global-only").Location; got == "/mutated" {
		t.Fatal("catalog location aliases a prior result")
	}
}

func TestCatalogIncludesEveryPublishedPackagedFactoryWithoutLocation(t *testing.T) {
	t.Parallel()

	published, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("load published packaged definitions: %v", err)
	}
	discovery, err := factoryinternal.NewEffectiveCatalogDiscovery(
		rootLister{}.ListNamedFactories,
		definitionFiles{}.ReadFile,
		published.All(),
	)
	if err != nil {
		t.Fatalf("new published effective catalog source: %v", err)
	}
	catalog := newCatalog(t, discovery)

	result := list(t, catalog, "/project", "/global")
	if got, want := names(result.Entries), published.Names(); !slices.Equal(got, want) {
		t.Fatalf("effective packaged names = %v, want published names %v", got, want)
	}
	for _, catalogEntry := range result.Entries {
		if catalogEntry.Location != nil {
			t.Fatalf("%s location = %q, want nil", catalogEntry.Name, *catalogEntry.Location)
		}
		if catalogEntry.Definition == nil {
			t.Fatalf("%s definition is nil", catalogEntry.Name)
		}
	}
}

func TestCatalogCoversEveryPrecedenceCombination(t *testing.T) {
	t.Parallel()

	catalog := newCatalog(t, source{
		roots: map[string][]factorydefinitions.EffectiveFactoryCatalogCandidate{
			"/project": {
				candidate("all-three", "/project", "project"),
				candidate("project-global", "/project", "project"),
				candidate("project-packaged", "/project", "project"),
			},
			"/global": {
				candidate("all-three", "/global", "global"),
				candidate("project-global", "/global", "global"),
				candidate("global-packaged", "/global", "global"),
			},
		},
		packaged: []factorydefinitions.EffectiveFactoryCatalogCandidate{
			packagedCandidate("all-three", "packaged"),
			packagedCandidate("project-packaged", "packaged"),
			packagedCandidate("global-packaged", "packaged"),
		},
	}.discovery())

	result := list(t, catalog, "/project", "/global")
	assertProject(t, result.Entries, "all-three", "project")
	assertProject(t, result.Entries, "project-global", "project")
	assertProject(t, result.Entries, "project-packaged", "project")
	assertProject(t, result.Entries, "global-packaged", "global")
	if len(result.Entries) != 4 {
		t.Fatalf("effective entries = %#v, want four completely shadowed names", result.Entries)
	}
}

func TestAttachPublishesEffectiveCatalogOnRootService(t *testing.T) {
	t.Parallel()

	catalog := newCatalog(t, source{
		roots: map[string][]factorydefinitions.EffectiveFactoryCatalogCandidate{
			"/project": {candidate("project-only", "/project", "project")},
			"/global":  nil,
		},
	}.discovery())
	service, err := factoryinternal.AttachEffectiveCatalog(baseService{}, catalog)
	if err != nil {
		t.Fatalf("attach effective catalog: %v", err)
	}

	result, err := service.ListEffectiveFactories(
		t.Context(),
		factorydefinitions.ListEffectiveFactoriesRequest{
			ProjectRoot: "/project",
			GlobalRoot:  "/global",
		},
	)
	if err != nil {
		t.Fatalf("root service ListEffectiveFactories: %v", err)
	}
	if got := names(result.Entries); !slices.Equal(got, []string{"project-only"}) {
		t.Fatalf("root service effective names = %v, want [project-only]", got)
	}
}

func newCatalog(
	t *testing.T,
	discovery factorydefinitions.EffectiveFactoryCatalogDiscovery,
) factorydefinitions.EffectiveFactoryCatalogOperation {
	t.Helper()
	catalog, err := factoryinternal.NewEffectiveCatalog(discovery, normalize)
	if err != nil {
		t.Fatalf("new effective catalog: %v", err)
	}
	return catalog
}

func normalize(
	_ context.Context,
	candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
) (*factorydefinitions.FactoryConfig, error) {
	var definition factorydefinitions.FactoryConfig
	if err := json.Unmarshal(candidate.Canonical, &definition); err != nil {
		return nil, err
	}
	return &definition, nil
}

func list(
	t *testing.T,
	catalog factorydefinitions.EffectiveFactoryCatalogOperation,
	projectRoot string,
	globalRoot string,
) factorydefinitions.ListEffectiveFactoriesResult {
	t.Helper()
	result, err := catalog(t.Context(), factorydefinitions.ListEffectiveFactoriesRequest{
		ProjectRoot: projectRoot,
		GlobalRoot:  globalRoot,
	})
	if err != nil {
		t.Fatalf("list effective Factories: %v", err)
	}
	return result
}

func candidate(name, root, project string) factorydefinitions.EffectiveFactoryCatalogCandidate {
	location := fmt.Sprintf("%s/%s", root, name)
	return factorydefinitions.EffectiveFactoryCatalogCandidate{
		Name:      name,
		Location:  &location,
		Canonical: definitionJSON(project),
	}
}

func packagedCandidate(name, project string) factorydefinitions.EffectiveFactoryCatalogCandidate {
	return factorydefinitions.EffectiveFactoryCatalogCandidate{
		Name:      name,
		Canonical: definitionJSON(project),
	}
}

func definitionJSON(project string) []byte {
	return []byte(fmt.Sprintf(
		`{"name":"%s","project":"%s","invocationSignature":{"parameters":[{"name":"prompt","typeHint":"string","binding":{"kind":"positional"}}]},"work_types":[],"resources":[],"workers":[],"workstations":[]}`,
		project,
		project,
	))
}

func cloneCandidates(
	candidates []factorydefinitions.EffectiveFactoryCatalogCandidate,
) []factorydefinitions.EffectiveFactoryCatalogCandidate {
	cloned := make([]factorydefinitions.EffectiveFactoryCatalogCandidate, len(candidates))
	for index, candidate := range candidates {
		candidate.Canonical = append([]byte(nil), candidate.Canonical...)
		if candidate.Location != nil {
			location := *candidate.Location
			candidate.Location = &location
		}
		cloned[index] = candidate
	}
	return cloned
}

func names(entries []factorydefinitions.EffectiveFactoryCatalogEntry) []string {
	result := make([]string, len(entries))
	for index, catalogEntry := range entries {
		result[index] = catalogEntry.Name
	}
	return result
}

func entry(
	t *testing.T,
	entries []factorydefinitions.EffectiveFactoryCatalogEntry,
	name string,
) factorydefinitions.EffectiveFactoryCatalogEntry {
	t.Helper()
	for _, catalogEntry := range entries {
		if catalogEntry.Name == name {
			return catalogEntry
		}
	}
	t.Fatalf("entry %q not found in %#v", name, entries)
	return factorydefinitions.EffectiveFactoryCatalogEntry{}
}

func assertProject(
	t *testing.T,
	entries []factorydefinitions.EffectiveFactoryCatalogEntry,
	name string,
	want string,
) {
	t.Helper()
	if got := entry(t, entries, name).Definition.Project; got != want {
		t.Fatalf("%s project = %q, want %q", name, got, want)
	}
}

// capturedLogCall records one structured log call made through
// logging.Logger, preserving the level so tests can distinguish start
// (Info) calls from failure (Warn) calls.
type capturedLogCall struct {
	level string
	msg   string
	kv    []any
}

// captureLogger is a logging.Logger test double that records every call
// instead of writing anywhere, so tests can assert on exactly what a
// CatalogPathsService operation logs.
type captureLogger struct {
	calls *[]capturedLogCall
}

func newCaptureLogger() (captureLogger, *[]capturedLogCall) {
	calls := &[]capturedLogCall{}
	return captureLogger{calls: calls}, calls
}

func (l captureLogger) Debug(msg string, kv ...any) {
	*l.calls = append(*l.calls, capturedLogCall{level: "debug", msg: msg, kv: kv})
}
func (l captureLogger) Info(msg string, kv ...any) {
	*l.calls = append(*l.calls, capturedLogCall{level: "info", msg: msg, kv: kv})
}
func (l captureLogger) Warn(msg string, kv ...any) {
	*l.calls = append(*l.calls, capturedLogCall{level: "warn", msg: msg, kv: kv})
}
func (l captureLogger) Error(msg string, kv ...any)   {}
func (l captureLogger) Verbose(msg string, kv ...any) {}

func hasKV(kv []any, key string, value any) bool {
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i] == key && kv[i+1] == value {
			return true
		}
	}
	return false
}

func validCatalogPathsCollaborators() (
	factorydefinitions.EffectiveFactoryCatalogOperation,
	func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error),
	factorydefinitions.CurrentFactoryDirectoryResolver,
) {
	listEffective := func(
		context.Context,
		factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		return factorydefinitions.ListEffectiveFactoriesResult{}, nil
	}
	resolveNamedFactory := func(
		context.Context,
		factorydefinitions.ResolveNamedFactoryRequest,
	) (factorydefinitions.ResolveNamedFactoryResult, error) {
		return factorydefinitions.ResolveNamedFactoryResult{}, nil
	}
	resolveCurrentDir := func(string) (string, error) {
		return "", nil
	}
	return listEffective, resolveNamedFactory, resolveCurrentDir
}

func TestNewCatalogPathsServiceRejectsMissingCollaborators(t *testing.T) {
	t.Parallel()

	listEffective, resolveNamedFactory, resolveCurrentDir := validCatalogPathsCollaborators()

	if _, err := factoryinternal.NewCatalogPathsService(nil, resolveNamedFactory, resolveCurrentDir, logging.NoopLogger{}); err == nil {
		t.Fatal("NewCatalogPathsService with nil listEffective = nil error, want error")
	}
	if _, err := factoryinternal.NewCatalogPathsService(listEffective, nil, resolveCurrentDir, logging.NoopLogger{}); err == nil {
		t.Fatal("NewCatalogPathsService with nil resolveNamedFactory = nil error, want error")
	}
	if _, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, nil, logging.NoopLogger{}); err == nil {
		t.Fatal("NewCatalogPathsService with nil resolveCurrentDir = nil error, want error")
	}
}

func TestNewCatalogPathsServiceAcceptsNilLogger(t *testing.T) {
	t.Parallel()

	listEffective, resolveNamedFactory, resolveCurrentDir := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, nil)
	if err != nil {
		t.Fatalf("NewCatalogPathsService with nil logger: unexpected error: %v", err)
	}
	if _, err := service.ListEffectiveFactories(context.Background(), factorydefinitions.ListEffectiveFactoriesRequest{}); err != nil {
		t.Fatalf("ListEffectiveFactories with nil-logger-constructed service: unexpected error: %v", err)
	}
}

func TestNewCatalogPathsServicePerformsNoIOAtConstruction(t *testing.T) {
	t.Parallel()

	panicky := func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		panic("listEffective invoked during inert construction")
	}
	panickyResolve := func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error) {
		panic("resolveNamedFactory invoked during inert construction")
	}
	panickyCurrent := func(string) (string, error) {
		panic("resolveCurrentDir invoked during inert construction")
	}

	if _, err := factoryinternal.NewCatalogPathsService(panicky, panickyResolve, panickyCurrent, logging.NoopLogger{}); err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}
}

func TestCatalogPathsServiceListEffectiveFactoriesForwardsToCollaborator(t *testing.T) {
	t.Parallel()

	want := factorydefinitions.ListEffectiveFactoriesResult{
		Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{Name: "alpha"}},
	}
	var gotRequest factorydefinitions.ListEffectiveFactoriesRequest
	listEffective := func(
		_ context.Context,
		request factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		gotRequest = request
		return want, nil
	}
	_, resolveNamedFactory, resolveCurrentDir := validCatalogPathsCollaborators()

	logger, calls := newCaptureLogger()
	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	request := factorydefinitions.ListEffectiveFactoriesRequest{ProjectRoot: "/project", GlobalRoot: "/global"}
	got, err := service.ListEffectiveFactories(context.Background(), request)
	if err != nil {
		t.Fatalf("ListEffectiveFactories: unexpected error: %v", err)
	}
	if gotRequest != request {
		t.Fatalf("ListEffectiveFactories forwarded request = %+v, want %+v", gotRequest, request)
	}
	if len(got.Entries) != 1 || got.Entries[0].Name != "alpha" {
		t.Fatalf("ListEffectiveFactories result = %+v, want the collaborator's result", got)
	}

	if len(*calls) != 2 {
		t.Fatalf("ListEffectiveFactories logged %d calls, want 2 (start, outcome): %+v", len(*calls), *calls)
	}
	if (*calls)[0].level != "info" {
		t.Fatalf("first log level = %q, want info (start)", (*calls)[0].level)
	}
	outcome := (*calls)[1]
	if outcome.level != "info" {
		t.Fatalf("second log level = %q, want info (outcome)", outcome.level)
	}
	if !hasKV(outcome.kv, "entry_count", 1) {
		t.Fatalf("outcome log missing entry_count=1: %+v", outcome.kv)
	}
}

func TestCatalogPathsServiceListEffectiveFactoriesLogsFailureClassification(t *testing.T) {
	t.Parallel()

	listEffective := func(
		context.Context,
		factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		return factorydefinitions.ListEffectiveFactoriesResult{}, errors.New("boom")
	}
	_, resolveNamedFactory, resolveCurrentDir := validCatalogPathsCollaborators()

	logger, calls := newCaptureLogger()
	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	if _, err := service.ListEffectiveFactories(context.Background(), factorydefinitions.ListEffectiveFactoriesRequest{}); err == nil {
		t.Fatal("ListEffectiveFactories: got nil error, want the collaborator's error")
	}

	if len(*calls) != 2 {
		t.Fatalf("ListEffectiveFactories logged %d calls, want 2 (start, failure): %+v", len(*calls), *calls)
	}
	failure := (*calls)[1]
	if failure.level != "warn" {
		t.Fatalf("failure log level = %q, want warn", failure.level)
	}
	if !hasKV(failure.kv, "reason", "operation_failed") {
		t.Fatalf("failure log missing reason=operation_failed: %+v", failure.kv)
	}
}

func TestCatalogPathsServiceResolveNamedFactoryForwardsToCollaborator(t *testing.T) {
	t.Parallel()

	want := factorydefinitions.ResolveNamedFactoryResult{
		Resolution: factorydefinitions.NamedFactoryResolution{Name: "alpha", FactoryDir: "/project/alpha"},
	}
	var gotRequest factorydefinitions.ResolveNamedFactoryRequest
	resolveNamedFactory := func(
		_ context.Context,
		request factorydefinitions.ResolveNamedFactoryRequest,
	) (factorydefinitions.ResolveNamedFactoryResult, error) {
		gotRequest = request
		return want, nil
	}
	listEffective, _, resolveCurrentDir := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	request := factorydefinitions.ResolveNamedFactoryRequest{ProjectRoot: "/project", GlobalRoot: "/global", Name: "alpha"}
	got, err := service.ResolveNamedFactory(context.Background(), request)
	if err != nil {
		t.Fatalf("ResolveNamedFactory: unexpected error: %v", err)
	}
	if gotRequest != request {
		t.Fatalf("ResolveNamedFactory forwarded request = %+v, want %+v", gotRequest, request)
	}
	if got != want {
		t.Fatalf("ResolveNamedFactory result = %+v, want %+v", got, want)
	}
}

// TestCatalogPathsServiceResolveNamedFactoryHonorsCancelledContext proves
// ResolveNamedFactory preserves the pre-cancelled-context behavior of the
// ACP adapter it replaced: an already-cancelled context short-circuits
// before the named-path collaborator runs, returning context.Canceled
// instead of performing filesystem resolution.
func TestCatalogPathsServiceResolveNamedFactoryHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	called := false
	resolveNamedFactory := func(
		context.Context,
		factorydefinitions.ResolveNamedFactoryRequest,
	) (factorydefinitions.ResolveNamedFactoryResult, error) {
		called = true
		return factorydefinitions.ResolveNamedFactoryResult{}, nil
	}
	listEffective, _, resolveCurrentDir := validCatalogPathsCollaborators()

	logger, calls := newCaptureLogger()
	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := service.ResolveNamedFactory(ctx, factorydefinitions.ResolveNamedFactoryRequest{ProjectRoot: "/project", Name: "alpha"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveNamedFactory error = %v, want errors.Is context.Canceled", err)
	}
	if called {
		t.Fatal("ResolveNamedFactory invoked the collaborator despite a cancelled context")
	}
	if got != (factorydefinitions.ResolveNamedFactoryResult{}) {
		t.Fatalf("ResolveNamedFactory returned a non-empty result on cancellation: %+v", got)
	}

	if len(*calls) != 2 {
		t.Fatalf("ResolveNamedFactory logged %d calls, want 2 (start, failure): %+v", len(*calls), *calls)
	}
	failure := (*calls)[1]
	if failure.level != "warn" {
		t.Fatalf("failure log level = %q, want warn", failure.level)
	}
	if !hasKV(failure.kv, "reason", "context_canceled") {
		t.Fatalf("failure log missing reason=context_canceled: %+v", failure.kv)
	}
}

func TestCatalogPathsServiceResolveNamedFactoryLogsSuccessAndFailure(t *testing.T) {
	t.Parallel()

	listEffective, _, resolveCurrentDir := validCatalogPathsCollaborators()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resolveNamedFactory := func(
			context.Context,
			factorydefinitions.ResolveNamedFactoryRequest,
		) (factorydefinitions.ResolveNamedFactoryResult, error) {
			return factorydefinitions.ResolveNamedFactoryResult{
				Resolution: factorydefinitions.NamedFactoryResolution{
					Name:       "alpha",
					FactoryDir: "/project/alpha",
					Source:     factorydefinitions.NamedFactoryResolutionSourceProjectLocal,
				},
			}, nil
		}
		logger, calls := newCaptureLogger()
		service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
		if err != nil {
			t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
		}
		if _, err := service.ResolveNamedFactory(context.Background(), factorydefinitions.ResolveNamedFactoryRequest{Name: "alpha"}); err != nil {
			t.Fatalf("ResolveNamedFactory: unexpected error: %v", err)
		}
		if len(*calls) != 2 || (*calls)[1].level != "info" {
			t.Fatalf("ResolveNamedFactory success logs = %+v, want [info, info]", *calls)
		}
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		resolveNamedFactory := func(
			context.Context,
			factorydefinitions.ResolveNamedFactoryRequest,
		) (factorydefinitions.ResolveNamedFactoryResult, error) {
			return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
		}
		logger, calls := newCaptureLogger()
		service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
		if err != nil {
			t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
		}
		if _, err := service.ResolveNamedFactory(context.Background(), factorydefinitions.ResolveNamedFactoryRequest{Name: "missing"}); !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
			t.Fatalf("ResolveNamedFactory error = %v, want errors.Is ErrNamedFactoryNotFound", err)
		}
		if len(*calls) != 2 {
			t.Fatalf("ResolveNamedFactory logged %d calls, want 2 (start, failure): %+v", len(*calls), *calls)
		}
		if !hasKV((*calls)[1].kv, "reason", "named_factory_not_found") {
			t.Fatalf("failure log missing reason=named_factory_not_found: %+v", (*calls)[1].kv)
		}
	})
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationForwardsToCollaborator(t *testing.T) {
	t.Parallel()

	var gotRootDir string
	resolveCurrentDir := func(rootDir string) (string, error) {
		gotRootDir = rootDir
		return "/project/current", nil
	}
	listEffective, resolveNamedFactory, _ := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	got, err := service.ResolveCurrentFactoryLocation(context.Background(), factorydefinitions.ResolveCurrentFactoryLocationRequest{
		RootDir: "/project",
	})
	if err != nil {
		t.Fatalf("ResolveCurrentFactoryLocation: unexpected error: %v", err)
	}
	if gotRootDir != "/project" {
		t.Fatalf("ResolveCurrentFactoryLocation forwarded RootDir = %q, want %q", gotRootDir, "/project")
	}
	if got.FactoryDir != "/project/current" {
		t.Fatalf("ResolveCurrentFactoryLocation FactoryDir = %q, want %q", got.FactoryDir, "/project/current")
	}
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	called := false
	resolveCurrentDir := func(string) (string, error) {
		called = true
		return "", nil
	}
	listEffective, resolveNamedFactory, _ := validCatalogPathsCollaborators()

	logger, calls := newCaptureLogger()
	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.ResolveCurrentFactoryLocation(ctx, factorydefinitions.ResolveCurrentFactoryLocationRequest{RootDir: "/project"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveCurrentFactoryLocation error = %v, want errors.Is context.Canceled", err)
	}
	if called {
		t.Fatal("ResolveCurrentFactoryLocation invoked the collaborator despite a cancelled context")
	}
	if len(*calls) != 2 {
		t.Fatalf("ResolveCurrentFactoryLocation logged %d calls, want 2 (start, failure): %+v", len(*calls), *calls)
	}
	if !hasKV((*calls)[1].kv, "reason", "context_canceled") {
		t.Fatalf("failure log missing reason=context_canceled: %+v", (*calls)[1].kv)
	}
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationPropagatesTypedError(t *testing.T) {
	t.Parallel()

	resolveCurrentDir := func(string) (string, error) {
		return "", factorydefinitions.ErrFactoryLayoutNotFound
	}
	listEffective, resolveNamedFactory, _ := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	_, err = service.ResolveCurrentFactoryLocation(context.Background(), factorydefinitions.ResolveCurrentFactoryLocationRequest{RootDir: "/project"})
	if !errors.Is(err, factorydefinitions.ErrFactoryLayoutNotFound) {
		t.Fatalf("ResolveCurrentFactoryLocation error = %v, want errors.Is ErrFactoryLayoutNotFound", err)
	}
}
