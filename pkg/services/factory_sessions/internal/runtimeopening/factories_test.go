package runtimeopening

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestWorkFactoryProjectsMaterializationServiceFromContentMaterializer(t *testing.T) {
	t.Parallel()

	var factory WorkFactory = func(resolver work.RuntimeResolver) work.Service {
		if resolver != nil {
			t.Fatalf("WorkFactory resolver = %#v, want nil for materialization projection", resolver)
		}
		return work.MaterializationService(materializerStub{})
	}

	service := factory(nil)
	if service == nil {
		t.Fatal("WorkFactory() = nil, want materialization service")
	}
	path, cleanup, err := service.MaterializeContentURL(t.Context(), "file:///fixtures/runtimeopening.png")
	if err != nil || path != "/tmp/runtimeopening.png" || cleanup == nil {
		t.Fatalf("MaterializeContentURL = (%q, %v, %v)", path, cleanup, err)
	}
	cleanup()
}

type materializerStub struct{}

func (materializerStub) MaterializeContentURL(_ context.Context, rawURL string) (string, work.ContentCleanup, error) {
	return "/tmp/runtimeopening.png", func() {}, nil
}
