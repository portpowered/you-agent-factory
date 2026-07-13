package root_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/wire"
	"go.uber.org/zap"
)

func TestStartReturnsConcreteGraphFailureBeforeInitializerActivation(t *testing.T) {
	t.Parallel()

	var apiStarts int
	application, err := root.Start(context.Background(), root.Inputs{
		Mode: initializer.ModeAPI,
		Graph: wire.Inputs{
			Config: &runtimehost.Config{
				Dir:                             filepath.Join(t.TempDir(), "missing"),
				Logger:                          zap.NewNop(),
				Clock:                           rootClock{},
				RuntimeFileLoggingPolicy:        runtimehost.RuntimeFileLoggingPolicyDisabled,
				RuntimeMetricsPolicy:            runtimehost.RuntimeMetricsPolicyDisabled,
				DurableSessionPersistencePolicy: factorysessionexecution.PersistencePolicyDisabled,
				APIServerStarter: func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
					apiStarts++
					return nil
				},
			},
			MCPInput:  strings.NewReader(""),
			MCPOutput: &bytes.Buffer{},
		},
	})
	if application != nil || err == nil {
		t.Fatalf("Start() = (%v, %v), want construction failure", application, err)
	}
	if apiStarts != 0 {
		t.Fatalf("API starts = %d, want zero", apiStarts)
	}
	if !strings.Contains(err.Error(), "construct runtime core") {
		t.Fatalf("Start() error = %v, want concrete construction phase", err)
	}
}

func TestStartPreservesCanceledConstructionContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	application, err := root.Start(ctx, root.Inputs{})
	if application != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() = (%v, %v), want nil application wrapping context.Canceled", application, err)
	}
}

func TestStartReturnsApplicationForConstructedGraph(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, factoryfixtures.MinimalFactoryConfig())
	application, err := root.Start(context.Background(), root.Inputs{
		Mode: initializer.ModeMCP,
		Graph: wire.Inputs{
			Config: &runtimehost.Config{
				Dir:                                     factoryDir,
				Logger:                                  zap.NewNop(),
				Clock:                                   rootClock{},
				RuntimeFileLoggingPolicy:                runtimehost.RuntimeFileLoggingPolicyDisabled,
				RuntimeMetricsPolicy:                    runtimehost.RuntimeMetricsPolicyDisabled,
				DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
				SkipBuiltInRunnerPrerequisiteValidation: true,
			},
			MCPInput:  strings.NewReader(""),
			MCPOutput: &bytes.Buffer{},
		},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if application == nil || application.Graph() == nil {
		t.Fatal("Start() returned no application graph")
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

type rootClock struct{}

func (rootClock) Now() time.Time { return time.Unix(0, 0).UTC() }
