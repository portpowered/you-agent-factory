package config

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func mustDecodeFactoryPayload(t *testing.T, flattened []byte) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(flattened, &payload); err != nil {
		t.Fatalf("unmarshal flattened payload: %v", err)
	}
	return payload
}

func assertNoRetiredExhaustionRulesPayload(t *testing.T, payload map[string]any) {
	t.Helper()

	if _, ok := payload["exhaustionRules"]; ok {
		t.Fatalf("expected canonical payload not to advertise exhaustionRules, got %#v", payload["exhaustionRules"])
	}
	if _, ok := payload["exhaustion_rules"]; ok {
		t.Fatalf("expected canonical payload not to advertise exhaustion_rules, got %#v", payload["exhaustion_rules"])
	}
}

func assertLoopBreakerPayload(t *testing.T, payload map[string]any, name string, watchedWorkstation string, maxVisits int) {
	t.Helper()

	workstations, ok := payload["workstations"].([]any)
	if !ok || len(workstations) != 1 {
		t.Fatalf("expected one guarded loop breaker workstation, got %#v", payload["workstations"])
	}

	loopBreaker := findPayloadWorkstationByName(workstations, name)
	if loopBreaker == nil {
		t.Fatalf("expected guarded loop breaker workstation %q in %#v", name, workstations)
	}
	if got := loopBreaker["type"]; got != interfaces.WorkstationTypeLogical {
		t.Fatalf("loop breaker type = %#v, want %q", got, interfaces.WorkstationTypeLogical)
	}
	guards, ok := loopBreaker["guards"].([]any)
	if !ok || len(guards) != 1 {
		t.Fatalf("expected one loop breaker guard, got %#v", loopBreaker["guards"])
	}
	guard := guards[0].(map[string]any)
	if got := guard["type"]; got != "VISIT_COUNT" {
		t.Fatalf("guard type = %#v, want %q", got, "VISIT_COUNT")
	}
	if got := guard["workstation"]; got != watchedWorkstation {
		t.Fatalf("guard workstation = %#v, want %s", got, watchedWorkstation)
	}
	if got := guard["maxVisits"]; got != float64(maxVisits) {
		t.Fatalf("guard maxVisits = %#v, want %d", got, maxVisits)
	}
}

func findPayloadWorkstationByName(workstations []any, name string) map[string]any {
	for _, item := range workstations {
		workstation, ok := item.(map[string]any)
		if ok && workstation["name"] == name {
			return workstation
		}
	}
	return nil
}

func assertExpandedLoopBreaker(t *testing.T, cfg *interfaces.FactoryConfig, name string, watchedWorkstation string, maxVisits int) {
	t.Helper()

	if len(cfg.Workstations) != 1 {
		t.Fatalf("expected 1 workstation after expand, got %#v", cfg.Workstations)
	}

	var loopBreaker *interfaces.FactoryWorkstationConfig
	for i := range cfg.Workstations {
		if cfg.Workstations[i].Name == name {
			loopBreaker = &cfg.Workstations[i]
			break
		}
	}
	if loopBreaker == nil {
		t.Fatalf("expected expanded loop breaker workstation %q in %#v", name, cfg.Workstations)
	}
	if loopBreaker.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("expanded loop breaker type = %q, want %q", loopBreaker.Type, interfaces.WorkstationTypeLogical)
	}
	if len(loopBreaker.Guards) != 1 {
		t.Fatalf("expected expanded loop breaker to retain one guard, got %#v", loopBreaker.Guards)
	}
	if loopBreaker.Guards[0].Workstation != watchedWorkstation || loopBreaker.Guards[0].MaxVisits != maxVisits {
		t.Fatalf("expanded loop breaker guard = %#v, want visit_count on %s max %d", loopBreaker.Guards[0], watchedWorkstation, maxVisits)
	}
}

func portableResourceManifestMapperFixture() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Name: "portable-resource-manifest-test",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			RequiredTools: []interfaces.RequiredToolConfig{{
				Name:        "python",
				Command:     "python3",
				Purpose:     "Runs portable helper scripts",
				VersionArgs: []string{"--version"},
			}},
			BundledFiles: []interfaces.BundledFileConfig{{
				Type:       "SCRIPT",
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "print('portable')\n",
				},
			}, {
				Type:       "ROOT_HELPER",
				TargetPath: "Makefile",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "test:\n\tgo test ./...\n",
				},
			}, {
				Type:       "DOC",
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "# Usage\n",
				},
			}},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
		}},
	}
}

func assertFlattenedPortableResourceManifestPayload(t *testing.T, payload map[string]any) {
	t.Helper()

	resourceManifest, ok := payload["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("expected canonical payload to include supportingFiles, got %#v", payload["supportingFiles"])
	}
	requiredTools, ok := resourceManifest["requiredTools"].([]any)
	if !ok || len(requiredTools) != 1 {
		t.Fatalf("expected one required tool, got %#v", resourceManifest["requiredTools"])
	}
	requiredTool := requiredTools[0].(map[string]any)
	if got := requiredTool["command"]; got != "python3" {
		t.Fatalf("required tool command = %#v, want %q", got, "python3")
	}
	if got := requiredTool["purpose"]; got != "Runs portable helper scripts" {
		t.Fatalf("required tool purpose = %#v", got)
	}
	versionArgs, ok := requiredTool["versionArgs"].([]any)
	if !ok || len(versionArgs) != 1 || versionArgs[0] != "--version" {
		t.Fatalf("required tool versionArgs = %#v", requiredTool["versionArgs"])
	}

	bundledFiles, ok := resourceManifest["bundledFiles"].([]any)
	if !ok || len(bundledFiles) != 3 {
		t.Fatalf("expected three bundled files, got %#v", resourceManifest["bundledFiles"])
	}
	assertBundledFilePayload(t, bundledFiles[0].(map[string]any), "ROOT_HELPER", "Makefile", "test:\n\tgo test ./...\n")
	assertBundledFilePayload(t, bundledFiles[1].(map[string]any), "DOC", "factory/docs/usage.md", "# Usage\n")
	assertBundledFilePayload(t, bundledFiles[2].(map[string]any), "SCRIPT", "factory/scripts/setup-workspace.py", "print('portable')\n")
}

func assertBundledFilePayload(t *testing.T, payload map[string]any, wantType, wantTargetPath string, wantInline string) {
	t.Helper()

	if got := payload["type"]; got != wantType {
		t.Fatalf("bundled file type = %#v, want %q", got, wantType)
	}
	if got := payload["targetPath"]; got != wantTargetPath {
		t.Fatalf("bundled file targetPath = %#v, want %q", got, wantTargetPath)
	}
	content, ok := payload["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected bundled file content object, got %#v", payload["content"])
	}
	if got := content["encoding"]; got != "utf-8" {
		t.Fatalf("bundled file encoding = %#v", got)
	}
	if got := content["inline"]; got != wantInline {
		t.Fatalf("bundled file inline = %#v, want %q", got, wantInline)
	}
}

func assertExpandedPortableResourceManifest(t *testing.T, expanded *interfaces.FactoryConfig) {
	t.Helper()

	if expanded.ResourceManifest == nil {
		t.Fatal("expected resourceManifest to round-trip")
	}
	if len(expanded.ResourceManifest.RequiredTools) != 1 {
		t.Fatalf("expected one required tool after expand, got %#v", expanded.ResourceManifest.RequiredTools)
	}
	if expanded.ResourceManifest.RequiredTools[0].Purpose != "Runs portable helper scripts" {
		t.Fatalf("required tool purpose after expand = %#v", expanded.ResourceManifest.RequiredTools[0])
	}
	if len(expanded.ResourceManifest.BundledFiles) != 3 {
		t.Fatalf("expected three bundled files after expand, got %#v", expanded.ResourceManifest.BundledFiles)
	}
	if expanded.ResourceManifest.BundledFiles[0].TargetPath != "Makefile" || expanded.ResourceManifest.BundledFiles[0].Content.Inline != "test:\n\tgo test ./...\n" {
		t.Fatalf("bundled root helper after expand = %#v", expanded.ResourceManifest.BundledFiles[0])
	}
	if expanded.ResourceManifest.BundledFiles[1].Content.Inline != "# Usage\n" {
		t.Fatalf("bundled doc inline after expand = %#v", expanded.ResourceManifest.BundledFiles[1])
	}
	if expanded.ResourceManifest.BundledFiles[2].Content.Inline != "print('portable')\n" {
		t.Fatalf("bundled script inline after expand = %#v", expanded.ResourceManifest.BundledFiles[2])
	}
}

func assertMissingKey(t *testing.T, payload map[string]any, key string) {
	t.Helper()
	if _, ok := payload[key]; ok {
		t.Fatalf("did not expect key %q in %#v", key, payload)
	}
}
