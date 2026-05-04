package bootstrap_portability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type agentFactoryExportImportFixture struct {
	Name                string
	WorkType            string
	TerminalState       string
	CanonicalPayload    []byte
	AuthoredDir         string
	ExpectedActivePlace string
	ExpectedProject     string
}

type agentFactoryExportImportFixtureOptions struct {
	workType      string
	terminalState string
}

type agentFactoryExportImportFixtureOption func(*agentFactoryExportImportFixtureOptions)

func newAgentFactoryExportImportFixture(
	t *testing.T,
	name string,
	opts ...agentFactoryExportImportFixtureOption,
) agentFactoryExportImportFixture {
	t.Helper()

	options := agentFactoryExportImportFixtureOptions{
		workType:      "task",
		terminalState: "complete",
	}
	for _, opt := range opts {
		opt(&options)
	}

	authoredDir := t.TempDir()
	writeAgentFactoryExportImportAuthoredLayout(t, authoredDir, name, options.workType, options.terminalState)

	canonicalPayload, err := factoryconfig.FlattenFactoryConfig(authoredDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(%s): %v", name, err)
	}
	if _, err := factoryconfig.LoadRuntimeConfig(authoredDir, nil); err != nil {
		t.Fatalf("LoadRuntimeConfig(authored fixture %s): %v", name, err)
	}

	return agentFactoryExportImportFixture{
		Name:                name,
		WorkType:            options.workType,
		TerminalState:       options.terminalState,
		CanonicalPayload:    canonicalPayload,
		AuthoredDir:         authoredDir,
		ExpectedActivePlace: options.workType + ":" + options.terminalState,
		ExpectedProject:     name,
	}
}

func writeAgentFactoryExportImportAuthoredLayout(
	t *testing.T,
	authoredDir, project, workType, terminalState string,
) {
	t.Helper()

	payload, err := json.MarshalIndent(map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{{
			"name": workType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": terminalState, "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "worker-a",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": workType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": workType, "state": terminalState}},
			"onFailure": []map[string]string{{"workType": workType, "state": "failed"}},
		}},
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal export/import authored layout: %v", err)
	}

	writeAgentFactoryExportImportFile(t, filepath.Join(authoredDir, interfaces.FactoryConfigFile), append(payload, '\n'))
	writeAgentFactoryExportImportFile(
		t,
		filepath.Join(authoredDir, "workers", "worker-a", "AGENTS.md"),
		[]byte(strings.ReplaceAll(`---
type: MODEL_WORKER
modelProvider: claude
executorProvider: script_wrap
model: claude-sonnet-4-20250514
---

You are worker `+project+`.
`, "+project+", project)),
	)
	writeAgentFactoryExportImportFile(
		t,
		filepath.Join(authoredDir, "workstations", "process", "AGENTS.md"),
		[]byte(strings.ReplaceAll(`---
type: MODEL_WORKSTATION
---

Do the `+project+` work.
`, "+project+", project)),
	)
}

func writeAgentFactoryExportImportFile(t *testing.T, path string, contents []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestAgentFactoryExportImportFixture_AuthoredLayoutInterpolatesProjectSpecificPromptContent(t *testing.T) {
	fixture := newAgentFactoryExportImportFixture(t, "acme-export")

	workerPromptPath := filepath.Join(fixture.AuthoredDir, "workers", "worker-a", "AGENTS.md")
	workerPrompt, err := os.ReadFile(workerPromptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", workerPromptPath, err)
	}
	if strings.Contains(string(workerPrompt), "+project+") {
		t.Fatalf("worker prompt kept literal +project+ placeholder: %s", workerPrompt)
	}
	if !strings.Contains(string(workerPrompt), "acme-export") {
		t.Fatalf("worker prompt = %q, want project-specific content", string(workerPrompt))
	}

	workstationPromptPath := filepath.Join(fixture.AuthoredDir, "workstations", "process", "AGENTS.md")
	workstationPrompt, err := os.ReadFile(workstationPromptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", workstationPromptPath, err)
	}
	if strings.Contains(string(workstationPrompt), "+project+") {
		t.Fatalf("workstation prompt kept literal +project+ placeholder: %s", workstationPrompt)
	}
	if !strings.Contains(string(workstationPrompt), "acme-export") {
		t.Fatalf("workstation prompt = %q, want project-specific content", string(workstationPrompt))
	}
}

func TestAgentFactoryExportImportFixture_FlattenedPayloadKeepsCanonicalArrayRoutes(t *testing.T) {
	fixture := newAgentFactoryExportImportFixture(t, "array-routes-export")

	assertAgentFactoryExportImportCanonicalRouteArraysJSON(t, fixture.CanonicalPayload)

	generated, err := factoryconfig.GeneratedFactoryFromOpenAPIJSON(fixture.CanonicalPayload)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(%s): %v", fixture.Name, err)
	}
	assertAgentFactoryExportImportGeneratedRouteArrays(t, generated)
}

func assertAgentFactoryExportImportCanonicalRouteArraysJSON(t *testing.T, data []byte) {
	t.Helper()

	var payload struct {
		Workstations []map[string]any `json:"workstations"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal canonical agent export/import payload: %v", err)
	}
	if len(payload.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one process workstation", payload.Workstations)
	}

	routes, ok := payload.Workstations[0]["onFailure"].([]any)
	if !ok {
		t.Fatalf("workstation onFailure = %#v, want JSON array", payload.Workstations[0]["onFailure"])
	}
	if len(routes) != 1 {
		t.Fatalf("workstation onFailure len = %d, want 1", len(routes))
	}
}

func assertAgentFactoryExportImportGeneratedRouteArrays(t *testing.T, factory factoryapi.Factory) {
	t.Helper()

	if factory.Workstations == nil || len(*factory.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v, want one process workstation", factory.Workstations)
	}

	workstation := (*factory.Workstations)[0]
	if workstation.OnFailure == nil || len(*workstation.OnFailure) != 1 {
		t.Fatalf("generated process workstation onFailure = %#v, want one array route", workstation.OnFailure)
	}
}
