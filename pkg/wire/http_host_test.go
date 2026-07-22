package wire

import (
	"context"
	"net/http"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestProvideAPIServerStarterHonorsRootEdgeOverride(t *testing.T) {
	t.Parallel()

	called := false
	override := platformhttpserver.Starter(func(_ context.Context, request platformhttpserver.StartRequest) error {
		called = true
		if request.Port != 8123 || !request.AutoPort || request.Handler == nil {
			t.Fatalf("override request = %+v", request)
		}
		return nil
	})
	starter, err := provideAPIServerStarter(serviceedges.Edges{APIServerStarter: override})
	if err != nil {
		t.Fatalf("provideAPIServerStarter: %v", err)
	}
	if err := starter(t.Context(), platformhttpserver.StartRequest{
		Handler: http.NotFoundHandler(), Port: 8123, AutoPort: true,
	}); err != nil {
		t.Fatalf("starter override: %v", err)
	}
	if !called {
		t.Fatal("root APIServerStarter override was not selected")
	}
}
