package runtimebuild_test

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"go.uber.org/zap"
)

func TestRuntimeBuildConstructionRejectsMissingOwnedDependencies(t *testing.T) {
	t.Parallel()

	build := func(context.Context, runtimebuild.SessionBuildSpec) (any, error) {
		return struct{}{}, nil
	}
	tests := []struct {
		name   string
		clock  factory.Clock
		logger *zap.Logger
		build  runtimebuild.BundleBuilder
		want   string
	}{
		{name: "clock", logger: zap.NewNop(), build: build, want: "clock is required"},
		{name: "logger", clock: factory.RealClock{}, build: build, want: "logger is required"},
		{name: "builder", clock: factory.RealClock{}, logger: zap.NewNop(), want: "runtime builder is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := runtimebuild.New(runtimebuild.Config{}, test.clock, test.logger, test.build)
			if service != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() = (%v, %v), want nil service and error containing %q", service, err, test.want)
			}
		})
	}
}
