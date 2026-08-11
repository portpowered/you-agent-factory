package providercatalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCapabilitySchemaPublishesCompleteVocabulary(t *testing.T) {
	t.Parallel()

	catalog, err := Build(repositoryFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(catalog.Files[CatalogSchemaPath], &schema); err != nil {
		t.Fatalf("decode generated catalog schema: %v", err)
	}
	defs := schema["$defs"].(map[string]any)
	assertEnum := func(name string, want []any) {
		t.Helper()
		definition := defs[name].(map[string]any)
		if got := definition["enum"]; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s enum = %#v, want %#v", name, got, want)
		}
	}
	assertEnum("ProviderCapabilitySupport", []any{"supported", "unsupported", "conditional", "unknown"})
	assertEnum("ProviderModalitySupport", []any{"supported", "unsupported", "conditional", "unknown"})
	assertEnum("ProviderToolSupport", []any{"supported", "unsupported", "conditional", "unknown"})
	assertEnum("ProviderModalityTransport", []any{"inline", "file_path", "acp_resource", "tool_mediated", "none"})
	assertEnum("ProviderToolAvailability", []any{"built_in", "optional", "operator_configured", "external", "unknown"})
	assertEnum("ProviderCapabilityEvidenceKind", []any{"primary_documentation", "protocol_probe", "conformance_fixture", "maintainer_assertion"})
	assertEnum("ProviderHarnessKind", []any{"native_cli", "acp"})
	assertEnum("ProviderModelCatalogPosture", []any{"exact", "runtime_discovered", "operator_selected", "unknown"})
	assertEnum("ProviderACPResourceDelivery", []any{"implemented", "unsupported", "conditional", "unknown"})

	if err := validateCatalogSemantics([]any{validCapabilityManifest()}); err != nil {
		t.Fatalf("valid capability manifest rejected: %v", err)
	}
}

func TestCapabilityValidationRejectsInvalidFacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(map[string]any)
		wantErr string
	}{
		{
			name: "duplicate facts",
			mutate: func(manifest map[string]any) {
				routes := manifest["harnessRoutes"].([]any)
				routes = append(routes, routes[0])
				manifest["harnessRoutes"] = routes
			},
			wantErr: `duplicate capability fact "harness/input/text"`,
		},
		{
			name: "dangling evidence reference",
			mutate: func(manifest map[string]any) {
				route := manifest["harnessRoutes"].([]any)[0].(map[string]any)
				route["evidenceRefs"] = []any{"missing"}
			},
			wantErr: `references dangling evidence "missing"`,
		},
		{
			name: "out of bounds evidence fact reference",
			mutate: func(manifest map[string]any) {
				evidence := manifest["evidence"].([]any)[0].(map[string]any)
				evidence["factRefs"] = []any{"model/not-a-model/output/image"}
			},
			wantErr: `references out-of-bounds fact "model/not-a-model/output/image"`,
		},
		{
			name: "unqualified conditional fact",
			mutate: func(manifest map[string]any) {
				route := manifest["harnessRoutes"].([]any)[0].(map[string]any)
				route["support"] = "conditional"
				delete(route, "condition")
			},
			wantErr: "conditional support requires a condition",
		},
		{
			name: "condition on non-conditional fact",
			mutate: func(manifest map[string]any) {
				route := manifest["harnessRoutes"].([]any)[0].(map[string]any)
				route["condition"] = "only when configured"
			},
			wantErr: "condition is only valid for conditional support",
		},
		{
			name: "non-unknown fact without evidence",
			mutate: func(manifest map[string]any) {
				route := manifest["harnessRoutes"].([]any)[0].(map[string]any)
				delete(route, "evidenceRefs")
			},
			wantErr: "non-unknown support requires evidenceRefs",
		},
		{
			name: "invalid support transport",
			mutate: func(manifest map[string]any) {
				route := manifest["harnessRoutes"].([]any)[0].(map[string]any)
				route["transport"] = "none"
			},
			wantErr: "supported support requires a non-none transport",
		},
		{
			name: "unknown support cannot claim transport",
			mutate: func(manifest map[string]any) {
				route := manifest["harnessRoutes"].([]any)[0].(map[string]any)
				route["support"] = "unknown"
				route["transport"] = "inline"
				delete(route, "evidenceRefs")
			},
			wantErr: "unknown support cannot claim transport",
		},
		{
			name: "invalid route modality",
			mutate: func(manifest map[string]any) {
				route := manifest["harnessRoutes"].([]any)[0].(map[string]any)
				route["modality"] = "document"
			},
			wantErr: `unknown modality "document"`,
		},
		{
			name: "ACP metadata on native harness",
			mutate: func(manifest map[string]any) {
				harness := manifest["harness"].(map[string]any)
				harness["kind"] = "native_cli"
			},
			wantErr: "acpSupport is only valid for an acp harness",
		},
		{
			name: "ACP resource route on native harness",
			mutate: func(manifest map[string]any) {
				harness := manifest["harness"].(map[string]any)
				harness["kind"] = "native_cli"
				delete(harness, "acpSupport")
			},
			wantErr: "acp_resource transport requires an acp harness",
		},
		{
			name: "ACP resource route without delivery evidence",
			mutate: func(manifest map[string]any) {
				acp := manifest["harness"].(map[string]any)["acpSupport"].(map[string]any)
				acp["resourceDelivery"] = "unknown"
			},
			wantErr: "acp_resource transport requires implemented or conditional resource delivery",
		},
		{
			name: "invalid model catalog posture",
			mutate: func(manifest map[string]any) {
				manifest["modelCatalogPosture"] = "inferred"
			},
			wantErr: `unknown model catalog posture "inferred"`,
		},
		{
			name: "direct and tool mediated output contradiction",
			mutate: func(manifest map[string]any) {
				model := manifest["models"].([]any)[0].(map[string]any)
				modalities := model["modalities"].([]any)
				modalities = append(modalities, map[string]any{
					"direction":    "output",
					"modality":     "image",
					"support":      "supported",
					"transport":    "inline",
					"evidenceRefs": []any{"docs"},
				})
				model["modalities"] = modalities
			},
			wantErr: "direct model output contradicts tool-mediated output for image",
		},
		{
			name: "direct model tool-mediated output",
			mutate: func(manifest map[string]any) {
				model := manifest["models"].([]any)[0].(map[string]any)
				modality := model["modalities"].([]any)[0].(map[string]any)
				modality["direction"] = "output"
				modality["transport"] = "tool_mediated"
			},
			wantErr: "direct model output cannot use tool_mediated transport",
		},
		{
			name: "unknown tool availability cannot enable by default",
			mutate: func(manifest map[string]any) {
				tool := manifest["tools"].([]any)[0].(map[string]any)
				tool["availability"] = "unknown"
				tool["defaultEnabled"] = true
			},
			wantErr: "unknown availability requires null defaultEnabled",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			manifest := validCapabilityManifest()
			test.mutate(manifest)
			err := validateCatalogSemantics([]any{manifest})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateCatalogSemantics() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func validCapabilityManifest() map[string]any {
	manifest := semanticManifest("codex", nil)
	manifest["modelCatalogPosture"] = "exact"
	manifest["harness"] = map[string]any{
		"kind": "acp",
		"acpSupport": map[string]any{
			"support":          "supported",
			"protocolVersion":  "1.0",
			"resourceDelivery": "implemented",
			"evidenceRefs":     []any{"docs"},
		},
	}
	manifest["harnessRoutes"] = []any{
		testModalityFact("input", "text", "supported", "inline", "docs"),
		testModalityFact("input", "image", "supported", "file_path", "probe"),
		testModalityFact("input", "audio", "supported", "acp_resource", "fixture"),
		testModalityFact("output", "video", "supported", "tool_mediated", "maintainer"),
	}
	manifest["models"] = []any{
		map[string]any{
			"id":      "gpt-5.6",
			"efforts": []any{"low"},
			"modalities": []any{
				testModalityFact("input", "text", "supported", "inline", "docs"),
				testModalityFact("input", "image", "unsupported", "none", "probe"),
				testModalityFactWithCondition("output", "audio", "conditional", "file_path", "audio is enabled by account policy", "fixture"),
				map[string]any{"direction": "output", "modality": "video", "support": "unknown", "transport": "none"},
			},
		},
	}
	manifest["tools"] = []any{
		map[string]any{
			"name":           "image_generation",
			"support":        "supported",
			"description":    "Generate image output through the provider tool.",
			"evidenceRefs":   []any{"docs"},
			"availability":   "built_in",
			"defaultEnabled": true,
			"outputModalities": []any{
				map[string]any{
					"modality":     "image",
					"support":      "supported",
					"transport":    "tool_mediated",
					"evidenceRefs": []any{"docs"},
				},
			},
		},
	}
	manifest["evidence"] = []any{
		map[string]any{
			"id":         "docs",
			"kind":       "primary_documentation",
			"verifiedOn": "2026-08-10",
			"url":        "https://example.com/provider/docs",
			"factRefs":   []any{"harness/input/text", "model/gpt-5.6/input/text", "tool/image_generation"},
		},
		map[string]any{
			"id":         "probe",
			"kind":       "protocol_probe",
			"verifiedOn": "2026-08-10",
			"factRefs":   []any{"harness/input/image", "model/gpt-5.6/input/image"},
		},
		map[string]any{
			"id":         "fixture",
			"kind":       "conformance_fixture",
			"verifiedOn": "2026-08-10",
			"factRefs":   []any{"harness/input/audio", "model/gpt-5.6/output/audio"},
		},
		map[string]any{
			"id":         "maintainer",
			"kind":       "maintainer_assertion",
			"verifiedOn": "2026-08-10",
			"factRefs":   []any{"harness/output/video"},
		},
	}
	return manifest
}

func testModalityFact(direction, modality, support, transport, evidence string) map[string]any {
	return map[string]any{
		"direction":    direction,
		"modality":     modality,
		"support":      support,
		"transport":    transport,
		"evidenceRefs": []any{evidence},
	}
}

func testModalityFactWithCondition(direction, modality, support, transport, condition, evidence string) map[string]any {
	fact := testModalityFact(direction, modality, support, transport, evidence)
	fact["condition"] = condition
	return fact
}
