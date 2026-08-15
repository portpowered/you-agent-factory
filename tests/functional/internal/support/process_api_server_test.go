package support

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
)

func TestProcessAPIServerWaitForURLReportsNeverInvokedStarter(t *testing.T) {
	server := NewProcessAPIServer()

	startedAt := time.Now()
	_, err := server.WaitForBaseURL(processAPIServerReadyTimeout)
	if err == nil {
		t.Fatal("WaitForBaseURL error = nil, want never-invoked diagnostic")
	}
	if !strings.Contains(err.Error(), "process API server starter was never invoked") {
		t.Fatalf("WaitForBaseURL error = %q, want never-invoked diagnostic", err)
	}
	if !strings.Contains(err.Error(), "--with-server was probably omitted") {
		t.Fatalf("WaitForBaseURL error = %q, want --with-server guidance", err)
	}
	if elapsed := time.Since(startedAt); elapsed >= processAPIServerReadyTimeout {
		t.Fatalf("WaitForBaseURL elapsed = %s, want fast failure before readiness ceiling", elapsed)
	}
}

func TestProcessAPIServerWaitForURLClassifiesInvokedButNotReady(t *testing.T) {
	server := NewProcessAPIServer()
	ctx, cancel := context.WithCancel(t.Context())
	releaseBound := make(chan struct{})
	startDone := make(chan error, 1)
	go func() {
		startDone <- server.Start(ctx, platformhttpserver.StartRequest{
			Handler: http.NotFoundHandler(),
			OnBound: func(platformhttpserver.Binding) {
				<-releaseBound
			},
		})
	}()
	defer func() {
		close(releaseBound)
		cancel()
		if err := <-startDone; err != nil {
			t.Errorf("ProcessAPIServer.Start() error = %v, want nil", err)
		}
	}()

	select {
	case <-server.startedSignal:
	case <-time.After(time.Second):
		t.Fatal("ProcessAPIServer.Start() did not enter the starter")
	}

	_, err := server.WaitForBaseURL(50 * time.Millisecond)
	if err == nil {
		t.Fatal("WaitForBaseURL error = nil, want invoked-but-not-ready timeout")
	}
	if !strings.Contains(err.Error(), "after starter was invoked") {
		t.Fatalf("WaitForBaseURL error = %q, want invoked-but-not-ready diagnostic", err)
	}
	if strings.Contains(err.Error(), "--with-server") {
		t.Fatalf("WaitForBaseURL error = %q, want no missing-activation diagnosis", err)
	}
}

func TestProcessAPIServerWaitForURLReturnsDynamicURLAfterStart(t *testing.T) {
	server := NewProcessAPIServer()
	ctx, cancel := context.WithCancel(t.Context())
	startDone := make(chan error, 1)
	go func() {
		startDone <- server.Start(ctx, platformhttpserver.StartRequest{
			Handler: http.NotFoundHandler(),
		})
	}()

	baseURL, err := server.WaitForBaseURL(time.Second)
	if err != nil {
		t.Fatalf("WaitForBaseURL() error = %v, want dynamic URL", err)
	}
	if !strings.HasPrefix(baseURL, "http://") {
		t.Fatalf("WaitForBaseURL() = %q, want httptest HTTP URL", baseURL)
	}

	cancel()
	if err := <-startDone; err != nil {
		t.Fatalf("ProcessAPIServer.Start() error = %v, want nil", err)
	}
}
