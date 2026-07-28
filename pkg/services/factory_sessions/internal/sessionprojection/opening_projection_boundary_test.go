package sessionprojection_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

const factoryRuntimeImportRoot = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

var openingProjectionLeaseImportRoots = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening/...",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection/...",
}

func TestOpeningProjectionLeaseImportsFactoryRuntimeOnlyThroughRoot(t *testing.T) {
	t.Parallel()

	for _, root := range openingProjectionLeaseImportRoots {
		for _, testMode := range []bool{false, true} {
			args := []string{"list", "-f", "{{.ImportPath}} {{join .Imports \" \"}} {{join .TestImports \" \"}}"}
			if testMode {
				args = append(args, "-test")
			}
			args = append(args, root)

			cmd := exec.Command("go", args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go list %s (test=%v): %v\n%s", root, testMode, err, output)
			}

			for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) < 1 {
					continue
				}
				pkgPath := fields[0]
				for _, imp := range fields[1:] {
					if imp == factoryRuntimeImportRoot {
						continue
					}
					if strings.HasPrefix(imp, factoryRuntimeImportRoot+"/") {
						t.Fatalf(
							"%s must import Factory Runtime only through %s; found direct import %s",
							pkgPath,
							factoryRuntimeImportRoot,
							imp,
						)
					}
				}
			}
		}
	}
}

func TestProjectionContextConstructsFromRootObservationAndSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 28, 3, 15, 0, 0, time.UTC)
	observation := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Health: factoryruntime.ObservationHealth{
			FactoryState:           "RUNNING",
			StreamGenerationID:     "stream-gen-boundary",
			LifecycleControlStatus: "PAUSED",
		},
	}
	snapshot := &factoryruntime.StateSnapshot{
		RuntimeStatus:          interfaces.RuntimeStatusIdle,
		FactoryState:           "RUNNING",
		LifecycleControlStatus: "PAUSED",
		StreamGenerationID:     "stream-gen-fallback",
	}
	session := &livesession.LiveSession{ID: "sess-opening-projection-boundary"}

	ctx, err := sessionprojection.BuildProjectionContext(sessionprojection.ProjectionBuildInput{
		Session:           session,
		Snapshot:          snapshot,
		Observation:       observation,
		BackendScopeID:    "backend-scope-boundary",
		LogicalSessionKey: "logical-key-boundary",
		Now:               now,
	})
	if err != nil {
		t.Fatalf("BuildProjectionContext: %v", err)
	}
	if ctx.Observation.Status != factoryruntime.ObservationStatusActive {
		t.Fatalf("observation status = %q, want ACTIVE", ctx.Observation.Status)
	}
	if ctx.Observation.Health.StreamGenerationID != "stream-gen-boundary" {
		t.Fatalf(
			"observation stream generation = %q, want stream-gen-boundary",
			ctx.Observation.Health.StreamGenerationID,
		)
	}
	if ctx.LifecycleControlStatus != "PAUSED" {
		t.Fatalf("lifecycle control status = %q, want PAUSED", ctx.LifecycleControlStatus)
	}

	projection := sessionprojection.ProjectRuntimeContract(ctx)
	if projection.Status != string(factoryruntime.ObservationStatusActive) {
		t.Fatalf("projected status = %q, want ACTIVE from root Observation", projection.Status)
	}
	if projection.StreamIdentity == nil {
		t.Fatal("stream identity = nil, want projection from root Observation.Health")
	}
	if projection.StreamIdentity.StreamGenerationID != "stream-gen-boundary" {
		t.Fatalf(
			"stream generation = %q, want stream-gen-boundary from Observation.Health",
			projection.StreamIdentity.StreamGenerationID,
		)
	}
	if projection.StreamIdentity.BackendScopeID != "backend-scope-boundary" {
		t.Fatalf("backend scope = %q, want backend-scope-boundary", projection.StreamIdentity.BackendScopeID)
	}
	if projection.LifecycleControlStatus == nil || *projection.LifecycleControlStatus != "PAUSED" {
		t.Fatalf("lifecycle control status = %#v, want PAUSED", projection.LifecycleControlStatus)
	}
}
