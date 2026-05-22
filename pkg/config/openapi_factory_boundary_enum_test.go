package config

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
	cases := generatedFactoryMisCasedWorkerEnumTestCases()
	cases = append(cases, generatedFactoryMisCasedWorkstationEnumTestCases()...)
	return cases
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
	if got := cfg.Workers[0].ModelLocality; got != interfaces.ModelLocalityLocal {
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
