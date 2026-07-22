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

func TestNewRuntimeAssemblyRejectsMissingRequiredDependencies(t *testing.T) {
	assembly, err := NewRuntimeAssembly(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "session result projection is required") {
		t.Fatalf("NewRuntimeAssembly() error = %v, want deterministic required dependency error", err)
	}
	if assembly != nil {
		t.Fatalf("NewRuntimeAssembly() = %#v, want nil assembly", assembly)
	}
}

func TestNewRuntimeAssemblyIsInert(t *testing.T) {
	clock := &recordingClock{}
	directories := &recordingDirectoryInspection{}
	assembly, err := NewRuntimeAssembly(
		nil,
		resultProjector{},
		nil,
		nil,
		nil,
		clock,
		func() string { return "response-event-id" },
		func() string { return "session-id" },
		func() (string, error) { return "home", nil },
		directories,
		namedPathResolver{},
		fileeffects.InvocationInputReader(func(string) ([]byte, error) { return nil, nil }),
		fileeffects.InitialWorkReader(func(string) ([]byte, error) { return nil, nil }),
	)
	if err != nil {
		t.Fatalf("NewRuntimeAssembly() error = %v", err)
	}
	if assembly == nil {
		t.Fatal("NewRuntimeAssembly() returned nil assembly")
	}
	if clock.calls != 0 {
		t.Fatalf("construction read clock %d times, want no runtime activity", clock.calls)
	}
	if directories.calls != 0 {
		t.Fatalf("construction inspected filesystem %d times, want no runtime activity", directories.calls)
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
