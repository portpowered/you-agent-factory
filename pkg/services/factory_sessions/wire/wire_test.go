package wire

import (
	"io/fs"
	"runtime"
	"testing"
	"time"

	events "github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/testing/eventsstub"
)

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	valid := validNewServiceInputs()
	tests := []struct {
		name   string
		mutate func(*newServiceInputs)
	}{
		{name: "session result projection", mutate: func(in *newServiceInputs) { in.sessionResultProjection = nil }},
		{name: "response event ID generator", mutate: func(in *newServiceInputs) { in.eventIDs = nil }},
		{name: "session ID generator", mutate: func(in *newServiceInputs) { in.sessionIDs = nil }},
		{name: "home directory resolver", mutate: func(in *newServiceInputs) { in.resolveHome = nil }},
		{name: "directory inspection", mutate: func(in *newServiceInputs) { in.directoryInspection = nil }},
		{name: "named path resolver", mutate: func(in *newServiceInputs) { in.namedPaths = nil }},
		{name: "invocation input reader", mutate: func(in *newServiceInputs) { in.invocationInputFiles = nil }},
		{name: "initial Work reader", mutate: func(in *newServiceInputs) { in.initialWorkFiles = nil }},
		{name: "symlink resolver", mutate: func(in *newServiceInputs) { in.resolveSymlinks = nil }},
		{name: "events root", mutate: func(in *newServiceInputs) { in.eventsService = nil }},
		{name: "clock", mutate: func(in *newServiceInputs) { in.clock = nil }},
		{name: "live-change coordinator", mutate: func(in *newServiceInputs) { in.liveChangeCoordinator = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs := valid
			test.mutate(&inputs)
			service, err := inputs.callNewService()
			if err == nil {
				t.Fatalf("NewService() error = nil, want missing %s dependency", test.name)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
		})
	}
}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root factorysessions.Service = service
	if root == nil {
		t.Fatal("constructed root is nil")
	}
	liveControl, ok := service.(factorysessions.LiveControlService)
	if !ok {
		t.Fatal("constructed root does not publish the live-control capability")
	}
	if liveControl == nil {
		t.Fatal("constructed live-control capability is nil")
	}
	if any(liveControl) != any(root) {
		t.Fatalf("LiveControlService = %T, want the same authoritative Service instance %T", liveControl, root)
	}
	deletion, ok := service.(factorysessions.LiveDeletionService)
	if !ok || deletion == nil {
		t.Fatal("constructed root does not publish the live deletion capability")
	}
	if any(deletion) != any(root) {
		t.Fatalf("LiveDeletionService = %T, want the same authoritative Service instance %T", deletion, root)
	}
}

func TestNewServiceRetainsOneRuntimeAssemblyOnThePublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	assembly, err := RuntimeAssemblyFromService(service)
	if err != nil {
		t.Fatalf("RuntimeAssemblyFromService() error = %v", err)
	}
	if any(assembly) != any(service) {
		t.Fatalf("runtime assembly = %T(%[1]v), want the same process root %T(%[2]v)", assembly, service)
	}
}

func TestNewRuntimeOpeningRejectsIncompleteGroupsAtCompositionBoundary(t *testing.T) {
	t.Parallel()

	factory, err := NewRuntimeOpening(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if factory != nil {
		t.Fatalf("NewRuntimeOpening() = %#v, want nil factory", factory)
	}
	if got, want := err.Error(), "Factory Sessions runtime-opening Provider Sessions owner ports are required"; got != want {
		t.Fatalf("NewRuntimeOpening() error = %q, want %q", got, want)
	}
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	directories := &recordingDirectoryInspection{}
	homeCalls := 0
	symlinkCalls := 0
	invocationInputCalls := 0
	initialWorkCalls := 0
	eventIDCalls := 0
	sessionIDCalls := 0
	inputs := validNewServiceInputs()
	inputs.directoryInspection = directories
	inputs.resolveHome = func() (string, error) {
		homeCalls++
		panic("home directory resolved during inert construction")
	}
	inputs.resolveSymlinks = func(path string) (string, error) {
		symlinkCalls++
		panic("symlink resolved during inert construction")
	}
	inputs.invocationInputFiles = fileeffects.InvocationInputReader(func(string) ([]byte, error) {
		invocationInputCalls++
		panic("invocation input read during inert construction")
	})
	inputs.initialWorkFiles = fileeffects.InitialWorkReader(func(string) ([]byte, error) {
		initialWorkCalls++
		panic("initial Work read during inert construction")
	})
	inputs.eventIDs = func() string {
		eventIDCalls++
		return "response-event-id"
	}
	inputs.sessionIDs = func() string {
		sessionIDCalls++
		return "session-id"
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	service, err := inputs.callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root factorysessions.Service = service
	if root == nil {
		t.Fatal("constructed root is nil")
	}
	if directories.calls != 0 {
		t.Fatalf("construction inspected filesystem %d times, want no runtime activity", directories.calls)
	}
	if homeCalls != 0 || symlinkCalls != 0 || invocationInputCalls != 0 || initialWorkCalls != 0 {
		t.Fatalf(
			"construction invoked effect stubs (home=%d symlinks=%d invocation input=%d initial Work=%d), want inert construction",
			homeCalls, symlinkCalls, invocationInputCalls, initialWorkCalls,
		)
	}
	if eventIDCalls != 0 || sessionIDCalls != 0 {
		t.Fatalf(
			"construction invoked generators (event IDs=%d session IDs=%d), want inert construction",
			eventIDCalls, sessionIDCalls,
		)
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf("goroutine leak after construction: baseline=%d current=%d delta=%d", baseline, runtime.NumGoroutine(), leaked)
	}
}

type resultProjector struct{}

func (resultProjector) ProjectSessionResults(factoryruntime.SessionResultInput) factoryruntime.SessionResultProjection {
	return factoryruntime.SessionResultProjection{}
}

type newServiceInputs struct {
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory
	sessionResultProjection      factoryruntime.SessionResultProjectionOperation
	interpolation                factorydefinitions.InvocationInterpolationService
	invocationWorkTypes          factorydefinitions.InvocationWorkTypeService
	ttsObservability             factorydefinitions.TTSObservabilityService
	eventIDs                     factorysessions.ResponseEventIDGenerator
	responseEventRetentionLimits *factorysessions.ResponseEventRetentionLimits
	sessionIDs                   factorysessions.SessionIDGenerator
	resolveHome                  factorysessions.HomeDirectoryResolver
	directoryInspection          DirectoryInspection
	namedPaths                   factorydefinitions.NamedPathResolver
	invocationInputFiles         fileeffects.InvocationInputReader
	initialWorkFiles             fileeffects.InitialWorkReader
	resolveSymlinks              factorysessions.LogicalTargetResolveSymlinks
	eventsService                events.Service
	clock                        factoryruntime.Clock
	liveChangeCoordinator        LiveChangeCoordinator
}

func validNewServiceInputs() newServiceInputs {
	eventsService := eventsstub.New()
	return newServiceInputs{
		sessionResultProjection: resultProjector{},
		eventIDs:                func() string { return "response-event-id" },
		sessionIDs:              func() string { return "session-id" },
		resolveHome:             func() (string, error) { return "home", nil },
		directoryInspection:     &recordingDirectoryInspection{},
		namedPaths:              namedPathResolver{},
		invocationInputFiles:    fileeffects.InvocationInputReader(func(string) ([]byte, error) { return nil, nil }),
		initialWorkFiles:        fileeffects.InitialWorkReader(func(string) ([]byte, error) { return nil, nil }),
		resolveSymlinks:         func(path string) (string, error) { return path, nil },
		eventsService:           eventsService,
		clock:                   &recordingClock{},
		liveChangeCoordinator:   NewLiveChangeCoordinator(),
	}
}

func (in newServiceInputs) callNewService() (factorysessions.Service, error) {
	return NewService(
		in.newJavaScriptCheckpointStore,
		in.sessionResultProjection,
		in.interpolation,
		in.invocationWorkTypes,
		in.ttsObservability,
		in.eventIDs,
		in.responseEventRetentionLimits,
		in.sessionIDs,
		in.resolveHome,
		in.directoryInspection,
		in.namedPaths,
		in.invocationInputFiles,
		in.initialWorkFiles,
		in.resolveSymlinks,
		in.eventsService,
		in.clock,
		in.liveChangeCoordinator,
		nil,
	)
}

type recordingClock struct{ calls int }

func (c *recordingClock) Now() time.Time {
	c.calls++
	return time.Time{}
}

type recordingDirectoryInspection struct{ calls int }

func (d *recordingDirectoryInspection) Stat(string) (fs.FileInfo, error) {
	d.calls++
	return nil, fs.ErrNotExist
}

func (d *recordingDirectoryInspection) ReadDir(string) ([]fs.DirEntry, error) {
	d.calls++
	return nil, nil
}

type namedPathResolver struct{}

func (namedPathResolver) ResolveCandidatePaths(string, string, string) (factorydefinitions.NamedFactoryCandidatePaths, error) {
	return factorydefinitions.NamedFactoryCandidatePaths{}, nil
}
func (namedPathResolver) ResolveExistingDir(string, string) (string, error) { return "", nil }
func (namedPathResolver) RequireDefinitionDir(string) error                 { return nil }
func (namedPathResolver) ResolveCurrentDir(string) (string, error)          { return "", nil }
func (namedPathResolver) ReadCurrentPointer(string) (string, error)         { return "", nil }
func (namedPathResolver) WriteCurrentPointer(string, string) error          { return nil }

var _ DirectoryInspection = (*recordingDirectoryInspection)(nil)
