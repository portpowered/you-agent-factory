package runtime_metrics_test

import (
	"context"
	"errors"
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

var activeMetricsProcessState struct {
	once    sync.Once
	process support.ApplicationProcess
	routes  *activeMetricsProviderRoutes
	err     error
}

func activeMetricsProcess(t testing.TB) (support.ApplicationProcess, *activeMetricsProviderRoutes) {
	t.Helper()
	activeMetricsProcessState.once.Do(func() {
		activeMetricsProcessState.routes = &activeMetricsProviderRoutes{}
		activeMetricsProcessState.process, activeMetricsProcessState.err = support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
			ProviderCommandRunner: activeMetricsProcessState.routes,
			APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
				return ctx.Value(retainedMetricsServerKey{}).(*support.ProcessAPIServer).Start(ctx, request)
			},
		})
	})
	if activeMetricsProcessState.err != nil {
		t.Fatal(activeMetricsProcessState.err)
	}
	return activeMetricsProcessState.process, activeMetricsProcessState.routes
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:testsleep // bounded process teardown; public tests own completion signals
	defer cancel()
	var result error
	for _, process := range []support.ApplicationProcess{runtimeMetricsProcessState.process, activeMetricsProcessState.process} {
		if process != nil {
			result = errors.Join(result, process.Close(ctx))
		}
	}
	return result
}
