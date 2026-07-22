package wire

import (
	"io/fs"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/fileeffects"
)

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	service, err := NewService(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "session result projection is required") {
		t.Fatalf("NewService() error = %v, want deterministic required dependency error", err)
	}
	if service != nil {
		t.Fatalf("NewService() = %#v, want nil service", service)
	}
}

func TestNewServiceIsInertAndRequiresRuntimeClockBinding(t *testing.T) {
	clock := &recordingClock{}
	directories := &recordingDirectoryInspection{}
	service, err := NewService(
		nil,
		resultProjector{},
		nil,
		nil,
		nil,
		func() string { return "response-event-id" },
		func() string { return "session-id" },
		func() (string, error) { return "home", nil },
		directories,
		namedPathResolver{},
		fileeffects.InvocationInputReader(func(string) ([]byte, error) { return nil, nil }),
		fileeffects.InitialWorkReader(func(string) ([]byte, error) { return nil, nil }),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	if clock.calls != 0 {
		t.Fatalf("construction read clock %d times, want no runtime activity", clock.calls)
	}
	if directories.calls != 0 {
		t.Fatalf("construction inspected filesystem %d times, want no runtime activity", directories.calls)
	}
	if assembly, bindErr := service.ForRuntime(factorysessions.RuntimeBinding{}); bindErr == nil || assembly != nil {
		t.Fatalf("ForRuntime() without clock = %#v, %v; want deterministic error", assembly, bindErr)
	}
	assembly, err := service.ForRuntime(factorysessions.RuntimeBinding{Clock: clock})
	if err != nil || assembly == nil {
		t.Fatalf("ForRuntime() = %#v, %v; want bound assembly", assembly, err)
	}
	if clock.calls != 0 {
		t.Fatalf("runtime binding read clock %d times, want no runtime activity", clock.calls)
	}
	if directories.calls != 0 {
		t.Fatalf("runtime binding inspected filesystem %d times, want no runtime activity", directories.calls)
	}
}

type resultProjector struct{}

func (resultProjector) ProjectSessionResults(factoryruntime.SessionResultInput) factoryruntime.SessionResultProjection {
	return factoryruntime.SessionResultProjection{}
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

var _ factorysessions.DirectoryInspection = (*recordingDirectoryInspection)(nil)
