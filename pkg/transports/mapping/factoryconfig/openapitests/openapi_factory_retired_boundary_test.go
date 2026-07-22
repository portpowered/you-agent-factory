package openapitests

import (
	. "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"strings"
	"testing"
)

func TestGeneratedFactoryFromOpenAPIJSON_RejectsRetiredFanInFieldAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"retired-fan-in-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"join":{"waitFor":"story","waitState":"complete","require":"all"}
		}]
	}`)

	assertGeneratedFactoryBoundaryError(t, cfgJSON, "workstations[0].join is not supported")
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsRetiredExhaustionRulesFieldAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"retired-exhaustion-rules-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"failed","type":"FAILED"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"failed"}]
		}],
		"exhaustionRules": [{
			"name":"execute-story-loop-breaker",
			"watchWorkstation":"execute-story",
			"maxVisits":3,
			"source":{"workType":"story","state":"init"},
			"target":{"workType":"story","state":"failed"}
		}]
	}`)

	assertGeneratedFactoryBoundaryError(t, cfgJSON, "exhaustion_rules is retired")
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsRetiredCronIntervalFieldAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"retired-cron-interval-factory",
		"workTypes": [{"name":"task","states":[{"name":"ready","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"daily-refresh",
			"behavior":"CRON",
			"worker":"executor",
			"outputs":[{"workType":"task","state":"complete"}],
			"cron":{"interval":"5m"}
		}]
	}`)

	assertGeneratedFactoryBoundaryError(
		t,
		cfgJSON,
		"workstations[0].cron.interval is not supported; use cron.schedule",
	)
}

func assertGeneratedFactoryBoundaryError(t *testing.T, cfgJSON []byte, message string) {
	t.Helper()

	_, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected generated factory boundary decode to fail")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), message) {
		t.Fatalf("expected boundary message %q, got %v", message, err)
	}
}
