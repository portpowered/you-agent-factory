package internal_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal"
)

func TestInvocationInputPreservesContextErrorsAndTypesOtherFailures(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	expired, expire := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer expire()
	readFailure := errors.New("controlled input read failure")
	for _, test := range []struct {
		name    string
		ctx     context.Context
		request work.InvocationInputPreparationRequest
		want    error
		invalid bool
	}{
		{name: "cancellation", ctx: canceled, want: context.Canceled},
		{name: "deadline", ctx: expired, want: context.DeadlineExceeded},
		{name: "validation", ctx: t.Context(), request: work.InvocationInputPreparationRequest{Arguments: []string{""}}, want: work.ErrInvalidInvocationInput, invalid: true},
		{name: "reader failure", ctx: t.Context(), request: work.InvocationInputPreparationRequest{FilePath: stringPtrForInput("input.txt")}, want: readFailure, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := internalservice.NewService(nil, func(string) ([]byte, error) { return nil, readFailure },
				func(string) (fs.FileInfo, error) {
					return fstest.MapFS{"input.txt": &fstest.MapFile{Data: []byte("input")}}.Stat("input.txt")
				}, nil, nil)
			_, err := service.PrepareInvocationInput(test.ctx, test.request)
			if !errors.Is(err, test.want) {
				t.Fatalf("preparation error = %v, want identity %v", err, test.want)
			}
			if errors.Is(err, work.ErrInvalidInvocationInput) != test.invalid {
				t.Fatalf("preparation error = %v, invalid-input identity want %t", err, test.invalid)
			}
		})
	}
}

func stringPtrForInput(value string) *string { return &value }
