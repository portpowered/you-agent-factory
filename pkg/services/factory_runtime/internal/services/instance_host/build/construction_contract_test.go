package runtimebuild_test

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"go.uber.org/zap"
	"testing"
)

// Preparation consumes explicit identities and candidates without requiring
// unused loading or generation collaborators from the retired build bridge.
func TestPrepareSpecExplicitSelectionsSkipUnusedCollaborators(t *testing.T) {
	t.Parallel()
	logger := zap.NewNop()
	clock := &platformclock.Real{}
	candidate := &runtimeBuildLoadedSource{config: &factorydefinitions.FactoryConfig{}}
	owner := runtimebuild.New(nil, nil, nil, logger)
	selections := runtimebuild.SessionBuildSpec{Clock: clock, BaseLogger: logger}
	spec, err := owner.PrepareSpec(t.Context(), runtimebuild.BuildDefaults{},
		runtimebuild.SessionBuildValues{SessionID: "session", RuntimeInstanceID: "runtime", LoadedFactoryCfg: candidate}, selections)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Clock != clock || spec.BaseLogger != logger || spec.LoadedFactoryCfg != candidate || spec.RuntimeInstanceID != "runtime" || spec.SessionID != "session" {
		t.Fatalf("selected candidate = %#v", spec)
	}
	if selections.RuntimeInstanceID != "" || selections.LoadedFactoryCfg != nil {
		t.Fatal("preparation mutated caller selections")
	}
}
