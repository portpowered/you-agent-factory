package service

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
)

func TestFactoryService_SaveFactoryForSession_UpsertOnNonDefaultSessionDoesNotMutateDefault(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	harness.waitIdle(t, betaSessionID, "beta runtime")

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		betaSessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContractWithWorkType(t, "gamma", "gamma-task"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert gamma on beta session): %v", err)
	}
	if created.Name != factoryapi.FactoryName("gamma") {
		t.Fatalf("created factory name = %q, want gamma", created.Name)
	}

	betaCurrent, err := harness.svc.GetCurrentFactoryForSession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession(beta) after upsert: %v", err)
	}
	assertFactoryName(t, betaCurrent.Name, "gamma", "beta session current factory after upsert")
	assertFactoryWorkType(t, betaCurrent.WorkTypes, "gamma-task", "beta session work types after upsert")

	defaultCurrent, err := harness.svc.GetCurrentFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession(default) after beta upsert: %v", err)
	}
	assertFactoryName(t, defaultCurrent.Name, "alpha", "default session current factory after beta upsert")
	assertFactoryWorkType(t, defaultCurrent.WorkTypes, "task", "default session work types after beta upsert")

	assertCurrentFactoryPointer(t, harness.rootDir, "alpha", "global default pointer after beta session upsert")
	if _, err := config.ResolveNamedFactoryDir(harness.rootDir, "gamma"); err == nil {
		t.Fatal("expected gamma factory to persist only under the beta session root, not the service root")
	}
}

func TestFactoryService_SaveFactoryForSession_UpsertReplaceRequiresFreshVersion(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert beta): %v", err)
	}
	if created.Version == nil {
		t.Fatal("expected created factory version metadata")
	}

	stale := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	stale.Version = created.Version
	_, err = harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		stale,
	)
	if err == nil {
		t.Fatal("expected stale upsert replace to fail")
	}

	fresh := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	fresh.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
	replaced, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		fresh,
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert replace beta): %v", err)
	}
	assertFactoryWorkType(t, replaced.WorkTypes, "story", "replaced beta work types")
}
