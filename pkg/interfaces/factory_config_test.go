package interfaces

import (
	"encoding/json"
	"testing"
)

func TestFactoryWorkstationConfigUnmarshalJSON_DecodesCanonicalRuntimeAndCronFields(t *testing.T) {
	var workstation FactoryWorkstationConfig
	if err := json.Unmarshal([]byte(`{
		"name":"nightly-review",
		"type":"MODEL_WORKSTATION",
		"worker":"planner",
		"cron":{
			"schedule":"*/5 * * * *",
			"triggerAtStart":true,
			"jitter":"2s",
			"expiryWindow":"30s"
		}
	}`), &workstation); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if workstation.Type != "MODEL_WORKSTATION" {
		t.Fatalf("expected canonical type to populate workstation runtime, got %+v", workstation)
	}
	if workstation.Cron == nil {
		t.Fatalf("expected canonical cron config to decode, got %+v", workstation)
	}
	if workstation.Cron.Schedule != "*/5 * * * *" || !workstation.Cron.TriggerAtStart || workstation.Cron.Jitter != "2s" || workstation.Cron.ExpiryWindow != "30s" {
		t.Fatalf("expected canonical cron config to decode intact, got %+v", workstation.Cron)
	}
}

func TestFactoryWorkstationConfigUnmarshalJSON_DecodesClassificationRoutes(t *testing.T) {
	var workstation FactoryWorkstationConfig
	if err := json.Unmarshal([]byte(`{
		"name":"classify-review",
		"type":"CLASSIFIER_WORKSTATION",
		"worker":"planner",
		"inputs":[{"workType":"task","state":"init"}],
		"classification_routes":[
			{"label":"approved","outputs":[{"workType":"task","state":"done"}]},
			{"label":"needs_review","outputs":[{"workType":"task","state":"failed"}]}
		]
	}`), &workstation); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if workstation.Type != WorkstationTypeClassify {
		t.Fatalf("expected classifier type to decode, got %+v", workstation)
	}
	if len(workstation.ClassificationRoutes) != 2 {
		t.Fatalf("expected two classification routes, got %+v", workstation.ClassificationRoutes)
	}
	if workstation.ClassificationRoutes[0].Label != "approved" || workstation.ClassificationRoutes[0].Outputs[0].StateName != "done" {
		t.Fatalf("expected first classification route to decode intact, got %+v", workstation.ClassificationRoutes[0])
	}
}

func TestCronConfigUnmarshalJSON_DecodesCanonicalFields(t *testing.T) {
	var cron CronConfig
	if err := json.Unmarshal([]byte(`{
		"schedule":"*/5 * * * *",
		"triggerAtStart":true,
		"jitter":"1s",
		"expiryWindow":"20s"
	}`), &cron); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if cron.Schedule != "*/5 * * * *" || !cron.TriggerAtStart || cron.Jitter != "1s" || cron.ExpiryWindow != "20s" {
		t.Fatalf("expected canonical cron fields to decode intact, got %+v", cron)
	}
}

func TestCronConfigUnmarshalJSON_IgnoresRetiredAliases(t *testing.T) {
	var cron CronConfig
	if err := json.Unmarshal([]byte(`{
		"schedule":"*/5 * * * *",
		"trigger_at_start":true,
		"expiry_window":"20s"
	}`), &cron); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if cron.TriggerAtStart {
		t.Fatalf("expected retired trigger_at_start alias to be ignored, got %+v", cron)
	}
	if cron.ExpiryWindow != "" {
		t.Fatalf("expected retired expiry_window alias to be ignored, got %+v", cron)
	}
}
