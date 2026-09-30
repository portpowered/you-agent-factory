package service

import (
	"context"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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

type startNamedDefinitions struct {
	factorydefinitions.Service
	request factorydefinitions.ResolveNamedFactoryRequest
	calls   int
}

func (s *startNamedDefinitions) ResolveNamedFactory(_ context.Context, request factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error) {
	s.request = request
	s.calls++
	return factorydefinitions.ResolveNamedFactoryResult{Resolution: factorydefinitions.NamedFactoryResolution{
		FactoryDir: filepath.Join(request.ProjectRoot, request.Name),
	}}, nil
}

func TestResolveStartFolderUsesRequestProjectForPackagedFactory(t *testing.T) {
	definitions := &startNamedDefinitions{}
	root := &Root{factoryDefinitions: definitions}
	project := t.TempDir()
	home := t.TempDir()
	request := factorysessions.SessionStartRequest{
		FolderPath: project,
		Source:     factorysessions.Source{Kind: factoryruntime.WorkflowSourceKindFactoryID, FactoryID: "@you/subagent"},
		Args:       map[string]any{"workingRoot": project},
	}
	got, err := root.resolveStartFolder(context.Background(), request, factorysessions.SessionRuntimeSelection{SystemConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	projectFactories := filepath.Join(project, "factory")
	globalFactories := filepath.Join(home, ".you-agent-factory", "factories")
	if definitions.calls != 1 || definitions.request.ProjectRoot != projectFactories ||
		definitions.request.GlobalRoot != globalFactories ||
		definitions.request.Name != "@you/subagent" {
		t.Fatalf("named Factory resolution = %+v, calls = %d", definitions.request, definitions.calls)
	}
	if want := filepath.Join(projectFactories, "@you/subagent"); got != want {
		t.Fatalf("Factory directory = %q, want %q", got, want)
	}
}

func TestResolveStartFolderPreservesAlreadyResolvedFactory(t *testing.T) {
	definitions := &startNamedDefinitions{}
	root := &Root{factoryDefinitions: definitions}
	project := t.TempDir()
	factoryDir := filepath.Join(project, "factory", "@you", "subagent")
	request := factorysessions.SessionStartRequest{
		FolderPath: factoryDir,
		Source:     factorysessions.Source{Kind: factoryruntime.WorkflowSourceKindFactoryID, FactoryID: "@you/subagent"},
		Args:       map[string]any{"workingRoot": project},
	}
	got, err := root.resolveStartFolder(context.Background(), request, factorysessions.SessionRuntimeSelection{})
	if err != nil || got != factoryDir || definitions.calls != 0 {
		t.Fatalf("resolved Factory directory = %q, calls = %d, error = %v", got, definitions.calls, err)
	}
}

func TestActivationOnlyStartAllocatesDistinctSessionIdentities(t *testing.T) {
	generated := 0
	root := &Root{generateSessionID: func() string {
		generated++
		return "chat-session-" + string(rune('0'+generated))
	}}
	request := factorysessions.SessionStartRequest{ActivationOnly: true}
	first, err := root.sessionIDForStart(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := root.sessionIDForStart(request)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == factorysessions.DefaultSessionID || second == factorysessions.DefaultSessionID {
		t.Fatalf("activation IDs = %q, %q; want distinct non-default IDs", first, second)
	}
	request.SessionID = first
	reused, err := root.sessionIDForStart(request)
	if err != nil || reused != first || generated != 2 {
		t.Fatalf("explicit activation ID = %q, generated = %d, error = %v", reused, generated, err)
	}
}
