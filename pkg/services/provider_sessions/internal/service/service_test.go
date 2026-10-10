package service_test

import (
	"context"
	"errors"
	"testing"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	internalservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestCapturedReaderRequiredAtConstruction(t *testing.T) {
	t.Parallel()
	service, err := internalservice.New(nil)
	if service != nil || err == nil || err.Error() != "provider-session captured activity reader is required" {
		t.Fatalf("construction = %v, %v", service, err)
	}
}
func TestCapturedProviderNativeOnlyNotFound(t *testing.T) {
	t.Parallel()
	service, err := internalservice.New(emptyCapturedReader{})
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"codex", "cursor"} {
		_, err := service.Details(provider, "session_id", "native-only")
		var lookup *providersessions.LookupError
		if !errors.Is(err, providersessions.ErrSessionNotFound) || !errors.As(err, &lookup) || lookup.Root != "" || string(lookup.Provider) != provider {
			t.Fatalf("lookup = %v", err)
		}
	}
}

type emptyCapturedReader struct{}

func (emptyCapturedReader) ListPreparedWorkerSessionCaptures(context.Context, recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	return recordings.WorkerCapturedCatalogPage{}, nil
}

func (emptyCapturedReader) ListWorkerSessionCaptures(context.Context, recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	return recordings.WorkerCapturedCatalogPage{}, nil
}
func (emptyCapturedReader) ReadWorkerCapturedActivity(context.Context, recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	panic("unexpected activity read")
}
func (emptyCapturedReader) LookupWorkerSessionCapture(context.Context, string) (recordings.WorkerSessionCatalogEntry, error) {
	panic("unexpected lookup")
}

func TestCapturedProviderValidationBeforeLookup(t *testing.T) {
	t.Parallel()
	service, err := internalservice.New(emptyCapturedReader{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tuple := range [][3]string{{"unknown", "session_id", "valid"}, {" cursor ", "session_id", "valid"}, {"cursor", "wrong", "valid"}, {"cursor", "session_id", ""}, {"cursor", "session_id", "../private"}, {"cursor", "session_id", " valid "}} {
		if _, err := service.Details(tuple[0], tuple[1], tuple[2]); err == nil {
			t.Fatal("invalid tuple accepted")
		}
	}
}
