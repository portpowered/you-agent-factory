package runtime_metrics_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var runtimeMetricsProcessState struct {
	once    sync.Once
	process support.ApplicationProcess
	err     error
}

type retainedMetricsServerKey struct{}

func runtimeMetricsProcess(t testing.TB) support.ApplicationProcess {
	t.Helper()
	runtimeMetricsProcessState.once.Do(func() {
		runtimeMetricsProcessState.process, runtimeMetricsProcessState.err = support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
			ProviderCommandRunner: support.NewStaticSuccessCommandRunner("runtime metrics COMPLETE"),
			APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
				server, ok := ctx.Value(retainedMetricsServerKey{}).(*support.ProcessAPIServer)
				if !ok {
					return fmt.Errorf("metrics scenario server is required")
				}
				return server.Start(ctx, request)
			},
		})
	})
	if runtimeMetricsProcessState.err != nil {
		t.Fatalf("build runtime metrics CLI process: %v", runtimeMetricsProcessState.err)
	}
	return runtimeMetricsProcessState.process
}

func TestMain(m *testing.M) {
	code := m.Run()
	if err := closeRuntimeMetricsProcess(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func FunctionalMonolithCleanup(t *testing.T) {
	t.Helper()
	if err := closeRuntimeMetricsProcess(); err != nil {
		t.Error(err)
	}
}

func closeRuntimeMetricsProcess() error {
	if runtimeMetricsProcessState.process == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:testsleep // bounded process teardown; public tests own completion signals
	defer cancel()
	return runtimeMetricsProcessState.process.Close(ctx)
}
