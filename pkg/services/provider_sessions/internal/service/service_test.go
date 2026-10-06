package service_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	internalservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestCapturedReaderRequiredAtOwnerConstruction(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, missing := range []string{"filesystem", "captured activity reader"} {
		t.Run(missing, func(t *testing.T) {
			var files providersessionsinternal.FileSystem = platformfilesystem.Local{}
			var captured recordings.WorkerCapturedActivityReader = emptyCapturedReader{}
			switch missing {
			case "filesystem":
				files = nil
			case "captured activity reader":
				captured = nil
			}
			constructors := []func() (providersessions.Service, error){
				func() (providersessions.Service, error) {
					return internalservice.NewForRoots(files, filepath.WalkDir, filepath.EvalSymlinks, sql.Open, root, captured)
				},
				func() (providersessions.Service, error) {
					return internalservice.New(files, func() (string, error) { t.Fatal("home lookup before required-effect rejection"); return "", nil }, filepath.WalkDir, filepath.EvalSymlinks, sql.Open, providersessionsinternal.OperatingSystem(runtime.GOOS), captured)
				},
			}
			for _, construct := range constructors {
				service, err := construct()
				if service != nil || err == nil || err.Error() != "provider-session "+missing+" is required" {
					t.Fatalf("construction = %#v, %v; want nil service and exact missing-effect error", service, err)
				}
			}
		})
	}
}

func TestHomeFailurePreservesCauseAndNilOwnerService(t *testing.T) {
	t.Parallel()
	cause := errors.New("controlled home failure")
	service, err := internalservice.New(platformfilesystem.Local{}, func() (string, error) { return "", cause }, filepath.WalkDir, filepath.EvalSymlinks, sql.Open, providersessionsinternal.OperatingSystem(runtime.GOOS), emptyCapturedReader{})
	if service != nil || !errors.Is(err, cause) || err.Error() != "home directory: controlled home failure" {
		t.Fatalf("New = %#v, %v; want nil service and wrapped home cause", service, err)
	}
}

type emptyCapturedReader struct{}

func (emptyCapturedReader) ListWorkerSessionCaptures(context.Context, recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	return recordings.WorkerCapturedCatalogPage{}, nil
}
func (emptyCapturedReader) ReadWorkerCapturedActivity(context.Context, recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	panic("unexpected activity read")
}
func (emptyCapturedReader) LookupWorkerSessionCapture(context.Context, string) (recordings.WorkerSessionCatalogEntry, error) {
	panic("unexpected lookup")
}
