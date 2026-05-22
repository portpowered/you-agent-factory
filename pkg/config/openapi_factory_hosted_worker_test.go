package config

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestGeneratedFactoryFromOpenAPIJSON_DecodesHostedLinearWorker(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"hosted-linear-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"queued","type":"PROCESSING"}]}],
		"workers": [{
			"name":"linear-poller",
			"type":"HOSTED_WORKER",
			"provider":"LINEAR",
			"auth":{"secretRef":"secrets/linear-api-key"},
			"linear":{
				"pollInterval":"45s",
				"teamIds":["team-a"],
				"stateIds":["state-b"],
				"mapping":{"workType":"story","state":"init"},
				"claim":{"assigneeField":"assignee.email"}
			}
		}],
		"workstations": [{
			"name":"poll-linear",
			"behavior":"POLLER",
			"worker":"linear-poller",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"queued"}]
		}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	assertGeneratedHostedLinearWorker(t, generated)

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	assertRuntimeHostedLinearWorker(t, cfg)
}

func assertGeneratedHostedLinearWorker(t *testing.T, generated factoryapi.Factory) {
	t.Helper()

	worker := (*generated.Workers)[0]
	if worker.Provider == nil || *worker.Provider != "LINEAR" {
		t.Fatalf("expected generated worker provider LINEAR, got %#v", worker.Provider)
	}
	if worker.Auth == nil || worker.Auth.SecretRef == nil || *worker.Auth.SecretRef != "secrets/linear-api-key" {
		t.Fatalf("expected generated worker auth.secretRef, got %#v", worker.Auth)
	}
	if worker.Linear == nil || worker.Linear.Mapping.WorkType == nil || *worker.Linear.Mapping.WorkType != "story" {
		t.Fatalf("expected generated worker linear mapping, got %#v", worker.Linear)
	}
}

func assertRuntimeHostedLinearWorker(t *testing.T, cfg interfaces.FactoryConfig) {
	t.Helper()

	runtimeWorker := cfg.Workers[0]
	if runtimeWorker.Type != interfaces.WorkerTypeHosted || runtimeWorker.Provider != interfaces.HostedWorkerProviderLinear {
		t.Fatalf("runtime hosted worker = %#v", runtimeWorker)
	}
	if runtimeWorker.Auth == nil || runtimeWorker.Auth.SecretRef != "secrets/linear-api-key" {
		t.Fatalf("runtime hosted auth = %#v", runtimeWorker.Auth)
	}
	if runtimeWorker.Linear == nil || runtimeWorker.Linear.Mapping.WorkType != "story" || runtimeWorker.Linear.Mapping.State != "init" {
		t.Fatalf("runtime hosted linear config = %#v", runtimeWorker.Linear)
	}
}

func TestFactoryConfigFromOpenAPIJSON_RejectsHostedLinearWorkerMissingMappingWithoutPanic(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"invalid-hosted-linear-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"queued","type":"PROCESSING"}]}],
		"workers": [{
			"name":"linear-poller",
			"type":"HOSTED_WORKER",
			"provider":"LINEAR",
			"auth":{"secretRef":"secrets/linear-api-key"},
			"linear":{}
		}],
		"workstations": [{
			"name":"poll-linear",
			"behavior":"POLLER",
			"worker":"linear-poller",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"queued"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	findings := NewConfigValidator().Validate(cfg).Errors()
	assertFindingMatch(t, findings, "hosted-worker-linear-mapping-work-type", "workers[0](linear-poller).linear.mapping.workType", "mapping.workType")
	assertFindingMatch(t, findings, "hosted-worker-linear-mapping-state", "workers[0](linear-poller).linear.mapping.state", "mapping.state")
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsHostedWorkerOAuthAuthFields(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"invalid-hosted-auth-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"queued","type":"PROCESSING"}]}],
		"workers": [{
			"name":"linear-poller",
			"type":"HOSTED_WORKER",
			"provider":"LINEAR",
			"auth":{"clientId":"abc","secretRef":"secrets/linear-api-key"},
			"linear":{"mapping":{"workType":"story","state":"init"}}
		}],
		"workstations": [{
			"name":"poll-linear",
			"behavior":"POLLER",
			"worker":"linear-poller",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"queued"}]
		}]
	}`)

	_, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected unsupported hosted auth shape to fail")
	}
	if !strings.Contains(err.Error(), "workers[0].auth.clientId") {
		t.Fatalf("expected hosted auth field path in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "v1 hosted workers do not support OAuth") {
		t.Fatalf("expected hosted auth guidance, got %v", err)
	}
}
