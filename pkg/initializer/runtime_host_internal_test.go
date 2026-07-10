package initializer

import (
	"context"
	"strings"
	"testing"
	"time"

	initializerdashboard "github.com/portpowered/infinite-you/pkg/initializer/dashboard"
)

func TestSessionRuntimeHostPropagatesDashboardStartupFailureAndCancelsRuntime(t *testing.T) {
	t.Parallel()

	sidecar, err := initializerdashboard.NewDashboardSidecar(initializerdashboard.DashboardSidecarConfig{
		Reader:   testDashboardReader{},
		Renderer: testDashboardRenderer{},
		Timing:   nilTickerTiming{},
	})
	if err != nil {
		t.Fatalf("NewDashboardSidecar: %v", err)
	}
	runtimeCanceled := make(chan struct{})
	host := &SessionRuntimeHost{dashboard: sidecar}
	err = host.runWithDashboard(context.Background(), func(ctx context.Context) error {
		<-ctx.Done()
		close(runtimeCanceled)
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "nil ticker") {
		t.Fatalf("runWithDashboard() error = %v, want dashboard startup failure", err)
	}
	<-runtimeCanceled
}

type testDashboardReader struct{}

func (testDashboardReader) ReadDashboard(context.Context, time.Time) (initializerdashboard.DashboardRenderInput, error) {
	return initializerdashboard.DashboardRenderInput{}, nil
}

type testDashboardRenderer struct{}

func (testDashboardRenderer) RenderDashboard(initializerdashboard.DashboardRenderInput) {}

type nilTickerTiming struct{}

func (nilTickerTiming) Now() time.Time { return time.Time{} }

func (nilTickerTiming) NewTicker(time.Duration) initializerdashboard.DashboardTicker { return nil }
