package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/internal/service"
)

func TestReadPreservesSessionResolverErrorsWithoutSnapshotFallback(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{factorysessions.ErrSessionNotFound, errors.New("internal failure"), errors.New("unavailable"), context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()
			svc := internalservice.New(stubSessionResolver{err: fmt.Errorf("selected session: %w", cause)}, nil, nil)
			_, listErr := svc.ListWork(context.Background(), "missing", work.ListOptions{})
			_, showErr := svc.GetWork(context.Background(), "missing", "unknown")
			for _, err := range []error{listErr, showErr} {
				if !errors.Is(err, cause) || errors.Is(err, work.ErrWorkNotFound) {
					t.Fatalf("read error = %v, want original resolver cause %v without Work absence", err, cause)
				}
			}
		})
	}
}

func TestReadCanceledContextDoesNotSelectSession(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := internalservice.New(stubSessionResolver{err: factorysessions.ErrSessionNotFound}, nil, nil)
	_, err := svc.ListWork(ctx, "missing", work.ListOptions{})
	if !errors.Is(err, context.Canceled) || errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("canceled read = %v, want cancellation before selection", err)
	}
}
