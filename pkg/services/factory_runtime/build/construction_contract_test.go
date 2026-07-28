package runtimebuild_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/build"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"go.uber.org/zap"
)

func TestConstructionRejectsMissingOwnedDependencies(t *testing.T) {
	t.Parallel()

	build := func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
		return &factoryhost.Bundle{}, nil
	}
	tests := []struct {
		name   string
		clock  factoryruntime.Clock
		logger *zap.Logger
		build  runtimebuild.BundleBuilder
		want   string
	}{
		{name: "clock", logger: zap.NewNop(), build: build, want: "clock is required"},
		{name: "logger", clock: platformclock.Real{}, build: build, want: "logger is required"},
		{name: "builder", clock: platformclock.Real{}, logger: zap.NewNop(), want: "runtime builder is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := runtimebuild.New(
				"",
				"",
				false,
				"",
				"",
				nil,
				func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
					return nil, errors.New("unused test loader")
				},
				nil,
				nil,
				nil,
				nil,
				nil,
				test.clock,
				testRuntimeID,
				test.logger,
				test.build,
				nil,
			)
			if service != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() = (%v, %v), want nil service and error containing %q", service, err, test.want)
			}
		})
	}
}
