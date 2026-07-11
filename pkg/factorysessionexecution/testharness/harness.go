// Package testharness provides explicit, isolated durable execution composition
// for transport tests.
package testharness

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// Mode selects the durable execution collaborator used by a test.
type Mode string

const (
	ModeFake       Mode = "fake"
	ModeJavaScript Mode = "javascript-runtime"
)

// Config contains every dependency that may affect durable runtime behavior.
// JavaScript mode requires all fields except Provider, which is required only
// when ChildExecutorMode is live-provider.
type Config struct {
	Mode              Mode
	ProjectRoot       string
	Clock             factory.Clock
	Provider          workers.Provider
	Persistence       runtimepersist.Store
	ChildExecutorMode string
	FakeOptions       []factorysessionexecution.FakeServiceOption
}

// New creates one isolated durable execution service or returns a clear error
// for unsupported or incomplete composition.
func New(config Config) (factorysessionexecution.Service, error) {
	switch config.Mode {
	case ModeFake:
		if hasRuntimeDependencies(config) {
			return nil, fmt.Errorf("durable execution test harness: fake mode does not accept JavaScript runtime dependencies")
		}
		return factorysessionexecution.NewFakeService(config.FakeOptions...), nil
	case ModeJavaScript:
		if err := validateJavaScriptConfig(config); err != nil {
			return nil, err
		}
		return factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
			ProjectRoot:       strings.TrimSpace(config.ProjectRoot),
			Clock:             config.Clock,
			Provider:          config.Provider,
			Persistence:       config.Persistence,
			ChildExecutorMode: config.ChildExecutorMode,
		}), nil
	default:
		return nil, fmt.Errorf("durable execution test harness: unsupported mode %q", config.Mode)
	}
}

func hasRuntimeDependencies(config Config) bool {
	return strings.TrimSpace(config.ProjectRoot) != "" || config.Clock != nil || config.Provider != nil ||
		config.Persistence != nil || strings.TrimSpace(config.ChildExecutorMode) != ""
}

func validateJavaScriptConfig(config Config) error {
	if strings.TrimSpace(config.ProjectRoot) == "" {
		return fmt.Errorf("durable execution test harness: project root is required for JavaScript mode")
	}
	if config.Clock == nil {
		return fmt.Errorf("durable execution test harness: clock is required for JavaScript mode")
	}
	if config.Persistence == nil {
		return fmt.Errorf("durable execution test harness: persistence is required for JavaScript mode")
	}
	switch config.ChildExecutorMode {
	case factorysessionexecution.ChildExecutorModeFake:
		if config.Provider != nil {
			return fmt.Errorf("durable execution test harness: provider is only valid with live-provider child execution")
		}
	case factorysessionexecution.ChildExecutorModeLive:
		if config.Provider == nil {
			return fmt.Errorf("durable execution test harness: provider is required for live-provider child execution")
		}
	default:
		return fmt.Errorf("durable execution test harness: child executor mode must be %q or %q", factorysessionexecution.ChildExecutorModeFake, factorysessionexecution.ChildExecutorModeLive)
	}
	return nil
}
