package service

import (
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDurableStartInheritsOnlyMatchingCurrentFactoryMockWorkers(t *testing.T) {
	configured := workers.NewEmptyMockWorkersConfig()
	current := &livesession.LiveSession{
		SessionState: livesession.SessionState{FactoryDir: "/current"},
		Handle:       &runtimebinding.SessionState{},
	}
	runtimebinding.SessionStateFrom(current).SetMockWorkers(configured)
	request := factorysessions.SessionStartRequest{FolderPath: "/current"}
	matched := inheritCurrentMockWorkers(request, current)
	if matched.RuntimeSelection == nil || matched.RuntimeSelection.Workers.MockWorkers == nil {
		t.Fatal("matching Current Factory lost mock worker selection")
	}
	matched.RuntimeSelection.Workers.MockWorkers.MockWorkers = append(
		matched.RuntimeSelection.Workers.MockWorkers.MockWorkers,
		workers.MockWorkerConfig{ID: "changed"},
	)
	if len(configured.MockWorkers) != 0 {
		t.Fatal("durable request mutated the live Factory's mock configuration")
	}
	other := inheritCurrentMockWorkers(factorysessions.SessionStartRequest{FolderPath: "/other"}, current)
	if other.RuntimeSelection != nil {
		t.Fatal("another Factory inherited Current Factory mock workers")
	}
}
