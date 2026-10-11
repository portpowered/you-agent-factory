package internal

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"
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
	opening := NewSidecarOpening(automation, clock, nil, nil, nil)
	peer := &factoryhost.Bundle{FactorySessionID: "peer", RuntimeInstanceID: "peer-runtime", RuntimeCfg: sidecarOpeningConfig{}}
	candidate := &factoryhost.Bundle{FactorySessionID: "candidate", RuntimeInstanceID: "candidate-runtime", RuntimeCfg: sidecarOpeningConfig{}}
	for _, scope := range []struct {
		bundle  *factoryhost.Bundle
		service bool
	}{{peer, true}, {candidate, false}} {
		if err := opening.Scope(scope.service, false).Preseed(context.Background(), scope.bundle); err != nil {
			t.Fatal(err)
		}
		if got := automation.requests[len(automation.requests)-1]; got.RuntimeID != scope.bundle.RuntimeInstanceID || got.FactorySessionID != scope.bundle.FactorySessionID || got.Inputs.StartSchedulers != scope.service {
			t.Fatalf("activation selection = %#v", got)
		}
	}
	opening.Scope(false, false).Stop(&factoryhost.Handle{Bundle: candidate})
	if automation.active["candidate-runtime"] || !automation.active["peer-runtime"] {
		t.Fatalf("candidate stop changed peer ownership: %#v", automation.active)
	}
	if err := opening.Scope(false, false).Preseed(context.Background(), candidate); err != nil {
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
	requests   []automations.RuntimeActivationRequest
	active     map[string]bool
	starts     int
	stops      int
	onActivate func(automations.RuntimeActivationRequest) error
}

func (a *sidecarOpeningAutomation) ActivateRuntime(_ context.Context, request automations.RuntimeActivationRequest) (automations.RuntimeActivationResult, error) {
	a.requests = append(a.requests, request)
	if a.onActivate != nil {
		if err := a.onActivate(request); err != nil {
			return automations.RuntimeActivationResult{}, err
		}
	}
	a.active[request.RuntimeID] = true
	return automations.RuntimeActivationResult{}, nil
}

func TestCompletedLifecycleSidecarUsesSelectedInputEffectsAndPreservesPeerOnRetry(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{"peer/input": {Data: []byte("peer payload")}}
	var walked []string
	walker := func(path string, visit fs.WalkDirFunc) error {
		walked = append(walked, path)
		return visit(path, nil, nil)
	}
	var ids int
	newID := func() string { ids++; return "selected-request" }
	cause := fs.ErrNotExist
	var payloads []string
	automation := &sidecarOpeningAutomation{active: make(map[string]bool)}
	automation.onActivate = func(request automations.RuntimeActivationRequest) error {
		return assertSelectedSidecarInputEffects(t, request, &payloads)
	}
	opening := NewSidecarOpening(automation, platformclock.Real{}, files, walker, newID)
	peer := &factoryhost.Bundle{Dir: "peer", FactorySessionID: "peer", RuntimeInstanceID: "peer-runtime", RuntimeCfg: sidecarOpeningConfig{}}
	candidate := &factoryhost.Bundle{Dir: "candidate", FactorySessionID: "candidate", RuntimeInstanceID: "candidate-runtime", RuntimeCfg: sidecarOpeningConfig{}}
	if len(automation.requests) != 0 || ids != 0 || len(walked) != 0 {
		t.Fatal("construction ran input effects")
	}
	if err := opening.Scope(true, false).Preseed(t.Context(), peer); err != nil {
		t.Fatal(err)
	}
	if err := opening.Scope(false, false).Preseed(t.Context(), candidate); !errors.Is(err, cause) {
		t.Fatalf("preseed cause = %v", err)
	}
	if !automation.active["peer-runtime"] || automation.active["candidate-runtime"] {
		t.Fatalf("failed activation changed scopes: %v", automation.active)
	}
	files["candidate/input"] = &fstest.MapFile{Data: []byte("candidate payload")}
	if err := opening.Scope(false, false).Preseed(t.Context(), candidate); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(payloads, []string{"peer payload", "candidate payload"}) || !reflect.DeepEqual(walked, []string{"peer", "candidate"}) || ids != 2 {
		t.Fatalf("selected effects: payloads=%v walked=%v ids=%d", payloads, walked, ids)
	}
	opening.Scope(false, false).Stop(&factoryhost.Handle{Bundle: candidate})
	if !automation.active["peer-runtime"] || automation.active["candidate-runtime"] {
		t.Fatalf("stop changed peer: %v", automation.active)
	}
}

func assertSelectedSidecarInputEffects(t *testing.T, request automations.RuntimeActivationRequest, payloads *[]string) error {
	t.Helper()
	input := request.Inputs.Filesystem
	data, err := input.Files.ReadFile(request.Snapshot.FactoryDir + "/input")
	if err != nil {
		return err
	}
	*payloads = append(*payloads, string(data))
	if err := input.WalkDirectory(request.Snapshot.FactoryDir, func(path string, _ fs.DirEntry, err error) error {
		if path != request.Snapshot.FactoryDir {
			t.Errorf("walked %q", path)
		}
		return err
	}); err != nil {
		return err
	}
	if got := input.WorkRequestIDs(); got != "selected-request" {
		t.Errorf("request ID = %q", got)
	}
	return nil
}

func (a *sidecarOpeningAutomation) DeactivateRuntime(_ context.Context, request automations.RuntimeDeactivationRequest) (automations.RuntimeDeactivationResult, error) {
	a.stops++
	delete(a.active, request.RuntimeID)
	return automations.RuntimeDeactivationResult{}, nil
}

func (a *sidecarOpeningAutomation) StartRuntime(context.Context, string) error {
	a.starts++
	return nil
}

func TestHistoricalSidecarDoesNotAdmitInputsOrStopLivePeer(t *testing.T) {
	t.Parallel()
	automation := &sidecarOpeningAutomation{active: make(map[string]bool)}
	opening := NewSidecarOpening(automation, platformclock.NewDeterministic(time.Unix(0, 0), time.Second), nil, nil, nil)
	peer := &factoryhost.Bundle{FactorySessionID: "peer", RuntimeInstanceID: "peer-runtime", RuntimeCfg: sidecarOpeningConfig{}}
	if err := opening.Scope(true, false).Preseed(t.Context(), peer); err != nil {
		t.Fatal(err)
	}
	for _, serviceMode := range []bool{false, true} {
		historical := opening.Scope(serviceMode, true)
		bundle := &factoryhost.Bundle{FactorySessionID: "history", RuntimeInstanceID: "history-runtime"}
		handle := &factoryhost.Handle{Bundle: bundle}
		if err := historical.Preseed(t.Context(), bundle); err != nil {
			t.Fatal(err)
		}
		if err := historical.Start(t.Context(), handle); err != nil {
			t.Fatal(err)
		}
		if err := historical.Start(t.Context(), handle); err != nil {
			t.Fatal(err)
		}
		historical.Stop(handle)
		if handle.SidecarCancel != nil {
			t.Fatal("historical observation was not stopped")
		}
	}
	if len(automation.requests) != 1 || !automation.active["peer-runtime"] || automation.starts != 0 || automation.stops != 0 {
		t.Fatalf("historical sidecars changed live automation: requests=%v active=%v starts=%d stops=%d", automation.requests, automation.active, automation.starts, automation.stops)
	}
}
