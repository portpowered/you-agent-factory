package internal

import (
	"context"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
)

func TestSidecarOpeningPreservesModeAndPeerOwnership(t *testing.T) {
	t.Parallel()
	automation := &sidecarOpeningAutomation{active: make(map[string]bool)}
	clock := platformclock.NewDeterministic(time.Unix(0, 0), time.Second)
	opening := NewSidecarOpening(automation, clock)
	peer := &factoryhost.Bundle{FactorySessionID: "peer", RuntimeInstanceID: "peer-runtime", RuntimeCfg: sidecarOpeningConfig{}}
	candidate := &factoryhost.Bundle{FactorySessionID: "candidate", RuntimeInstanceID: "candidate-runtime", RuntimeCfg: sidecarOpeningConfig{}}
	for _, scope := range []struct {
		bundle  *factoryhost.Bundle
		service bool
	}{{peer, true}, {candidate, false}} {
		if err := opening.Scope(scope.service).Preseed(context.Background(), scope.bundle); err != nil {
			t.Fatal(err)
		}
		if got := automation.requests[len(automation.requests)-1]; got.RuntimeID != scope.bundle.RuntimeInstanceID || got.FactorySessionID != scope.bundle.FactorySessionID || got.Inputs.StartSchedulers != scope.service {
			t.Fatalf("activation selection = %#v", got)
		}
	}
	opening.Scope(false).Stop(&factoryhost.Handle{Bundle: candidate})
	if automation.active["candidate-runtime"] || !automation.active["peer-runtime"] {
		t.Fatalf("candidate stop changed peer ownership: %#v", automation.active)
	}
	if err := opening.Scope(false).Preseed(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if !automation.active["candidate-runtime"] || !automation.active["peer-runtime"] {
		t.Fatalf("candidate retry lost runtime ownership: %#v", automation.active)
	}
}

type sidecarOpeningConfig struct{ factory.LoadedConfig }

func (sidecarOpeningConfig) FactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{}
}
func (sidecarOpeningConfig) RuntimeBaseDir() string { return "execution" }

type sidecarOpeningAutomation struct {
	automations.Service
	requests []automations.RuntimeActivationRequest
	active   map[string]bool
}

func (a *sidecarOpeningAutomation) ActivateRuntime(_ context.Context, request automations.RuntimeActivationRequest) (automations.RuntimeActivationResult, error) {
	a.requests = append(a.requests, request)
	a.active[request.RuntimeID] = true
	return automations.RuntimeActivationResult{}, nil
}

func (a *sidecarOpeningAutomation) DeactivateRuntime(_ context.Context, request automations.RuntimeDeactivationRequest) (automations.RuntimeDeactivationResult, error) {
	delete(a.active, request.RuntimeID)
	return automations.RuntimeDeactivationResult{}, nil
}

func (*sidecarOpeningAutomation) StartRuntime(context.Context, string) error { return nil }
