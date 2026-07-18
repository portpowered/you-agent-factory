package openapitests

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func TestFactoryConfigFromOpenAPIJSON_RejectsUnsafePortableLayoutAnnotationContent(t *testing.T) {
	const maxEmbeddedImageBytes = 2 * 1024 * 1024
	maximumImageData := base64.StdEncoding.EncodeToString(make([]byte, maxEmbeddedImageBytes))
	overlargeImageData := base64.StdEncoding.EncodeToString(make([]byte, maxEmbeddedImageBytes+1))

	tests := []struct {
		name        string
		annotations string
		wantPath    string
	}{
		{"overlong note title", layoutNoteAnnotation("note", strings.Repeat("t", 161), "safe"), "layout.annotations[0].note.title"},
		{"overlong note body", layoutNoteAnnotation("note", "", strings.Repeat("b", 4001)), "layout.annotations[0].note.body"},
		{"blank alternative text", layoutImageAnnotation("image", "", "EMBEDDED", "image/png", "AQID"), "layout.annotations[0].image.alternativeText"},
		{"overlong alternative text", layoutImageAnnotation("image", strings.Repeat("a", 501), "EMBEDDED", "image/png", "AQID"), "layout.annotations[0].image.alternativeText"},
		{"invalid base64", layoutImageAnnotation("image", "Example", "EMBEDDED", "image/png", "AQI"), "layout.annotations[0].image.source.data"},
		{"unsupported svg media type", layoutImageAnnotation("image", "Example", "EMBEDDED", "image/svg+xml", "AQID"), "layout.annotations[0].image.source.mediaType"},
		{"unsupported remote source", layoutImageAnnotation("image", "Example", "REMOTE", "image/png", "AQID"), "layout.annotations[0].image.source.kind"},
		{"individual image exceeds byte limit", layoutImageAnnotation("image", "Example", "EMBEDDED", "image/png", overlargeImageData), "layout.annotations[0].image.source.data"},
		{
			"factory image budget exceeded",
			strings.Join([]string{
				layoutImageAnnotation("one", "One", "EMBEDDED", "image/png", maximumImageData),
				layoutImageAnnotation("two", "Two", "EMBEDDED", "image/jpeg", maximumImageData),
				layoutImageAnnotation("three", "Three", "EMBEDDED", "image/webp", maximumImageData),
				layoutImageAnnotation("four", "Four", "EMBEDDED", "image/png", maximumImageData),
				layoutImageAnnotation("five", "Five", "EMBEDDED", "image/png", maximumImageData),
			}, ","),
			"layout.annotations[4].image.source.data",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := FactoryConfigFromOpenAPIJSON(layoutFactoryJSON(test.annotations))
			if err == nil {
				t.Fatal("expected unsafe layout annotation to be rejected")
			}
			if !strings.Contains(err.Error(), test.wantPath) {
				t.Fatalf("error = %v, want path %q", err, test.wantPath)
			}
		})
	}
}

func TestFactoryConfigFromOpenAPIJSON_PreservesLiteralLayoutAnnotationText(t *testing.T) {
	literalText := "# Guidance\n[not a link](javascript:alert(1))\n<img src=example>"
	annotations := layoutNoteAnnotation("literal-note", "", literalText) + "," +
		layoutImageAnnotation("literal-image", "<img alt=literal>", "EMBEDDED", "image/png", "AQID")

	cfg, err := FactoryConfigFromOpenAPIJSON(layoutFactoryJSON(annotations))
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if got := cfg.Layout.Annotations[0].Note.Body; got != literalText {
		t.Fatalf("literal note body = %q, want %q", got, literalText)
	}
	if got := cfg.Layout.Annotations[1].Image.AlternativeText; got != "<img alt=literal>" {
		t.Fatalf("literal alternative text = %q", got)
	}
}

func layoutFactoryJSON(annotations string) []byte {
	return []byte(`{
		"name":"layout-factory",
		"layout":{"schemaVersion":1,"annotations":[` + annotations + `],"viewport":{"x":0,"y":0,"zoom":1}},
		"workTypes":[{"name":"task","states":[{"name":"ready","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],
		"workers":[{"name":"writer","type":"MODEL_WORKER"}],
		"workstations":[{"name":"review","worker":"writer","inputs":[{"workType":"task","state":"ready"}],"outputs":[{"workType":"task","state":"done"}]}]
	}`)
}

func layoutNoteAnnotation(id, title, body string) string {
	return `{"id":"` + id + `","kind":"NOTE","position":{"x":10,"y":20},"note":{"title":` + marshalLayoutAnnotationString(title) + `,"body":` + marshalLayoutAnnotationString(body) + `,"tone":"INFO"}}`
}

func layoutImageAnnotation(id, alternativeText, kind, mediaType, data string) string {
	return `{"id":"` + id + `","kind":"IMAGE","position":{"x":10,"y":20},"image":{"source":{"kind":` + marshalLayoutAnnotationString(kind) + `,"mediaType":` + marshalLayoutAnnotationString(mediaType) + `,"data":` + marshalLayoutAnnotationString(data) + `},"alternativeText":` + marshalLayoutAnnotationString(alternativeText) + `}}`
}

func marshalLayoutAnnotationString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsMisCasedEnumValuesAtBoundary(t *testing.T) {
	for _, tc := range generatedFactoryMisCasedEnumTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedFactoryRejectsMisCasedEnumValue(t, tc.fieldPath, tc.value, tc.payload)
		})
	}
}

type generatedFactoryMisCasedEnumTestCase struct {
	name      string
	fieldPath string
	value     string
	payload   string
}

func assertGeneratedFactoryRejectsMisCasedEnumValue(t *testing.T, fieldPath, value, payload string) {
	t.Helper()

	_, err := GeneratedFactoryFromOpenAPIJSON([]byte(payload))
	if err == nil {
		t.Fatal("expected mis-cased enum value to fail at generated boundary")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), fieldPath) {
		t.Fatalf("expected field path %q in error, got %v", fieldPath, err)
	}
	if !strings.Contains(err.Error(), `unsupported value "`+value+`"`) {
		t.Fatalf("expected unsupported value %q in error, got %v", value, err)
	}
}

func generatedFactoryMisCasedEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	cases := generatedFactoryMisCasedWorkTypeEnumTestCases()
	cases = append(cases, generatedFactoryMisCasedWorkerEnumTestCases()...)
	cases = append(cases, generatedFactoryMisCasedWorkstationEnumTestCases()...)
	cases = append(cases, generatedFactoryMisCasedOrchestratorEnumTestCases()...)
	cases = append(cases, generatedFactoryMisCasedResourceEnumTestCases()...)
	return cases
}

func generatedFactoryMisCasedWorkTypeEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	return []generatedFactoryMisCasedEnumTestCase{
		{
			name:      "work type handling behavior",
			fieldPath: "workTypes[0].handlingBehavior[0]",
			value:     "default",
			payload: `{
				"name":"work-type-handling-factory",
				"workTypes": [{
					"name":"story",
					"handlingBehavior":["default"],
					"states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]
				}],
				"workers": [{"name":"executor"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
	}
}

func generatedFactoryMisCasedWorkerEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	return []generatedFactoryMisCasedEnumTestCase{
		{
			name:      "worker type",
			fieldPath: "workers[0].type",
			value:     "model_worker",
			payload: `{
				"name":"worker-type-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"model_worker"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "worker model provider",
			fieldPath: "workers[0].modelProvider",
			value:     "Claude",
			payload: `{
				"name":"worker-model-provider-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER","modelProvider":"Claude"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "worker model locality",
			fieldPath: "workers[0].modelLocality",
			value:     "local",
			payload: `{
				"name":"worker-model-locality-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER","modelLocality":"local"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "worker operation name",
			fieldPath: "workers[0].operations[0].name",
			value:     "tts",
			payload: `{
				"name":"worker-operation-name-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER","operations":[{"name":"tts","outputs":[{"name":"audio","contentTypes":["AUDIO"]}]}]}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "worker operation content type",
			fieldPath: "workers[0].operations[0].outputs[0].contentTypes[0]",
			value:     "audio",
			payload: `{
				"name":"worker-operation-content-type-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER","operations":[{"name":"TTS","outputs":[{"name":"audio","contentTypes":["audio"]}]}]}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
	}
}

func generatedFactoryMisCasedOrchestratorEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	return []generatedFactoryMisCasedEnumTestCase{
		{
			name:      "orchestrator kind",
			fieldPath: "orchestrator.kind",
			value:     "javascript",
			payload: `{
				"name":"miscased-orchestrator-factory",
				"orchestrator":{
					"kind":"javascript",
					"javascript":{"sourceRef":"factory/workflows/review.js"}
				}
			}`,
		},
	}
}

func generatedFactoryMisCasedResourceEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	return []generatedFactoryMisCasedEnumTestCase{
		{
			name:      "resource type",
			fieldPath: "resources[0].type",
			value:     "model",
			payload: `{
				"name":"miscased-resource-factory",
				"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"resources":[{"name":"slot","type":"model","capacity":1}],
				"workers":[{"name":"executor"}],
				"workstations":[{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
	}
}

func generatedFactoryMisCasedWorkstationEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	return []generatedFactoryMisCasedEnumTestCase{
		{
			name:      "workstation behavior",
			fieldPath: "workstations[0].behavior",
			value:     "cron",
			payload: `{
				"name":"workstation-behavior-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"behavior":"cron",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "workstation type",
			fieldPath: "workstations[0].type",
			value:     "logical_move",
			payload: `{
				"name":"workstation-type-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"logical_move",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "workstation operation",
			fieldPath: "workstations[0].operation",
			value:     "tts",
			payload: `{
				"name":"workstation-operation-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"MODEL_INVOKE",
					"operation":"tts",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "workstation outcome format",
			fieldPath: "workstations[0].outcomeFormat",
			value:     "Decision-Envelope",
			payload: `{
				"name":"workstation-outcome-format-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER"}],
				"workstations": [{
					"name":"review-story",
					"worker":"executor",
					"type":"MODEL_WORKSTATION",
					"outcomeFormat":"Decision-Envelope",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_ParsesCanonicalUppercaseSharedEnumsAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"uppercase-enums-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"executor",
			"type":"MODEL_WORKER",
			"modelProvider":"CODEX",
			"modelLocality":"LOCAL",
			"operations":[{
				"name":"TTS",
				"inputs":[{"name":"text","contentTypes":["TEXT"],"required":true}],
				"outputs":[{"name":"audio","contentTypes":["AUDIO"]}]
			}],
			"executorProvider":"SCRIPT_WRAP"
		}],
		"workstations": [{
			"name":"execute-story",
			"behavior":"STANDARD",
			"worker":"executor",
			"type":"MODEL_INVOKE",
			"operation":"TTS",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	assertGeneratedCanonicalUppercaseWorkerEnums(t, generated)

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	assertRuntimeCanonicalUppercaseWorkerEnums(t, cfg)
}

func assertGeneratedCanonicalUppercaseWorkerEnums(t *testing.T, generated factoryapi.Factory) {
	t.Helper()
	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("expected one generated worker, got %#v", generated.Workers)
	}
	worker := (*generated.Workers)[0]
	if worker.ModelProvider == nil || *worker.ModelProvider != factoryapi.WorkerModelProviderCodex {
		t.Fatalf("expected generated worker modelProvider CODEX, got %#v", worker.ModelProvider)
	}
	if worker.ModelLocality == nil || *worker.ModelLocality != factoryapi.WorkerModelLocalityLocal {
		t.Fatalf("expected generated worker modelLocality LOCAL, got %#v", worker.ModelLocality)
	}
	if worker.ExecutorProvider == nil || *worker.ExecutorProvider != factoryapi.WorkerProviderScriptWrap {
		t.Fatalf("expected generated worker executorProvider SCRIPT_WRAP, got %#v", worker.ExecutorProvider)
	}
	if worker.Operations == nil || len(*worker.Operations) != 1 {
		t.Fatalf("expected one generated worker operation, got %#v", worker.Operations)
	}
}

func assertRuntimeCanonicalUppercaseWorkerEnums(t *testing.T, cfg interfaces.FactoryConfig) {
	t.Helper()
	if got := cfg.Workers[0].ModelProvider; got != "codex" {
		t.Fatalf("expected runtime worker modelProvider codex, got %q", got)
	}
	if got := cfg.Workers[0].ModelLocality; got != workerconfig.ModelLocalityLocal {
		t.Fatalf("expected runtime worker modelLocality LOCAL, got %q", got)
	}
	if got := cfg.Workers[0].ExecutorProvider; got != "script_wrap" {
		t.Fatalf("expected runtime worker executorProvider script_wrap, got %q", got)
	}
	if len(cfg.Workers[0].Operations) != 1 || cfg.Workers[0].Operations[0].Name != "TTS" {
		t.Fatalf("expected runtime worker TTS operation, got %#v", cfg.Workers[0].Operations)
	}
	if got := cfg.Workstations[0].Type; got != interfaces.WorkstationTypeInvoke {
		t.Fatalf("expected runtime workstation type MODEL_INVOKE, got %q", got)
	}
	if got := cfg.Workstations[0].Operation; got != "TTS" {
		t.Fatalf("expected runtime workstation operation TTS, got %q", got)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsUnsupportedExecutorProviderAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"unsupported-executor-provider-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"executor",
			"type":"MODEL_WORKER",
			"executorProvider":"custom-executor"
		}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	_, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected unsupported executorProvider to fail at generated boundary")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workers[0].executorProvider") {
		t.Fatalf("expected executorProvider field path in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `unsupported value "custom-executor"`) {
		t.Fatalf("expected unsupported executorProvider value in error, got %v", err)
	}
}
