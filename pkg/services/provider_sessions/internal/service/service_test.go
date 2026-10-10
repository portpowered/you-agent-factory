package service_test

import (
	"context"
	"errors"
	"testing"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	internalservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestCapturedReaderConstructionIsInert(t *testing.T) {
	t.Parallel()
	reader := &constructionCapturedReader{}
	service, err := internalservice.New(reader)
	if service == nil || err != nil {
		t.Fatalf("construction = %v, %v", service, err)
	}
	reader.ready = true
	_, err = service.Details("codex", "session_id", "captured-session")
	if !errors.Is(err, providersessions.ErrSessionNotFound) || reader.calls != 1 {
		t.Fatalf("Details = %v, reader calls = %d", err, reader.calls)
	}
}

type constructionCapturedReader struct {
	emptyCapturedReader
	ready bool
	calls int
}

func (r *constructionCapturedReader) ListWorkerSessionCaptures(context.Context, recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	if !r.ready {
		panic("construction must not read captured activity")
	}
	r.calls++
	return recordings.WorkerCapturedCatalogPage{}, nil
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
	reader := &constructionCapturedReader{ready: true}
	service, err := internalservice.New(reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		provider, kind, id string
		want               error
	}{
		{"unknown", "session_id", "valid", providersessions.ErrUnsupportedProvider},
		{" cursor ", "session_id", "valid", providersessions.ErrUnsupportedProvider},
		{"cursor", "wrong", "valid", providersessions.ErrUnsupportedKind},
		{"cursor", "session_id", "", providersessions.ErrInvalidIdentifier},
		{"cursor", "session_id", "../private", providersessions.ErrInvalidIdentifier},
		{"cursor", "session_id", " valid ", providersessions.ErrInvalidIdentifier},
	} {
		if _, err := service.Details(test.provider, test.kind, test.id); !errors.Is(err, test.want) {
			t.Fatalf("Details(%q, %q, %q) = %v, want %v", test.provider, test.kind, test.id, err, test.want)
		}
	}
	if reader.calls != 0 {
		t.Fatalf("invalid identity read captured storage %d times", reader.calls)
	}
}
