package cli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type testHTTPClock struct{}

func (testHTTPClock) Now() time.Time { return time.Unix(1, 0) }
func testHTTPProtocol(t *testing.T) clihttp.Protocol {
	t.Helper()
	protocol, err := clihttp.NewProtocol(&http.Client{}, testHTTPClock{})
	if err != nil {
		t.Fatalf("build test HTTP protocol: %v", err)
	}
	return protocol
}

type pullProgressWriter struct {
	mu       sync.Mutex
	output   bytes.Buffer
	progress chan struct{}
	once     sync.Once
}

func (writer *pullProgressWriter) Write(payload []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if bytes.Contains(payload, []byte("models pull progress")) {
		writer.once.Do(func() { close(writer.progress) })
	}
	return writer.output.Write(payload)
}

func (writer *pullProgressWriter) String() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.output.String()
}

func TestModelsPullEmitsElapsedProgressOnDiagnosticsWhileWaiting(t *testing.T) {
	t.Parallel()

	pullStarted := make(chan struct{})
	releasePull := make(chan struct{})
	protocol, err := clihttp.NewProtocol(modelsPullDoer(func(*http.Request) (*http.Response, error) {
		close(pullStarted)
		<-releasePull
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"modelName":"voice","providerLocality":"LOCAL","outcome":"PULLED","managedRuntimePull":{"identity":"voice","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY"}}`,
			)),
		}, nil
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("pull protocol: %v", err)
	}
	progress := &pullProgressWriter{progress: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, pullErr := pullModel(pullOptions{
			Context: context.Background(), ModelName: "voice", Server: "http://factory.test",
			Verbose: true, Diagnostics: progress, HTTP: protocol,
			ProgressInterval: 5 * time.Millisecond,
		})
		done <- pullErr
	}()
	select {
	case <-pullStarted:
	case <-time.After(time.Second):
		t.Fatal("pull protocol was not invoked")
	}
	select {
	case <-progress.progress:
	case <-time.After(time.Second):
		t.Fatal("pull emitted no elapsed progress while waiting")
	}
	close(releasePull)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("pull error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pull did not stop progress after terminal response")
	}
	diagnostics := progress.String()
	if !strings.Contains(diagnostics, `models pull progress modelName="voice"`) ||
		!strings.Contains(diagnostics, "elapsed=") {
		t.Fatalf("progress diagnostics = %q, want model and elapsed time", diagnostics)
	}
}
