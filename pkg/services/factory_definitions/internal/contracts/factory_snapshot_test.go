package factorycontracts

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestFactorySnapshotPreservesUnknownFieldsAndCloneIsolation(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"name": "factory-a",
		"futureField": map[string]any{
			"enabled": true,
		},
	}
	snapshot, err := NewFactorySnapshot(source)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	clone := snapshot.Clone()
	(*snapshot)[0] = '['

	var decoded map[string]any
	if err := clone.Decode(&decoded); err != nil {
		t.Fatalf("Decode clone: %v", err)
	}
	if decoded["name"] != "factory-a" {
		t.Fatalf("name = %#v, want factory-a", decoded["name"])
	}
	future, ok := decoded["futureField"].(map[string]any)
	if !ok || future["enabled"] != true {
		t.Fatalf("futureField = %#v, want preserved unknown object", decoded["futureField"])
	}
}

func TestFactoryEventEnvelopeDetachesPayloadAndPreservesUnknownFields(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"workId": "work-1",
		"future": map[string]any{"enabled": true},
	}
	boundary := struct {
		Context       FactoryEventContext       `json:"context"`
		ID            string                    `json:"id"`
		Payload       map[string]any            `json:"payload"`
		SchemaVersion FactoryEventSchemaVersion `json:"schemaVersion"`
		Type          FactoryEventType          `json:"type"`
	}{
		Context: FactoryEventContext{
			EventTime: time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC),
			Sequence:  4,
			Tick:      3,
		},
		ID:            "event-4",
		Payload:       payload,
		SchemaVersion: FactoryEventSchemaVersionV1,
		Type:          FactoryEventTypeWorkStateChange,
	}

	event, err := NewFactoryEvent(boundary)
	if err != nil {
		t.Fatalf("NewFactoryEvent() error = %v", err)
	}
	payload["workId"] = "mutated"
	delete(payload, "future")

	var decoded map[string]any
	if err := event.DecodePayload(&decoded); err != nil {
		t.Fatalf("DecodePayload() error = %v", err)
	}
	if decoded["workId"] != "work-1" {
		t.Fatalf("detached workId = %#v, want work-1", decoded["workId"])
	}
	if _, ok := decoded["future"]; !ok {
		t.Fatalf("decoded payload = %#v, want unknown future field preserved", decoded)
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var roundTrip map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if string(roundTrip["id"]) != `"event-4"` || string(roundTrip["schemaVersion"]) != `"agent-factory.event.v1"` {
		t.Fatalf("round-trip envelope = %s", encoded)
	}
}

func TestFactorySnapshotMarshalsAsFactoryObject(t *testing.T) {
	t.Parallel()

	snapshot, err := NewFactorySnapshot(map[string]any{"name": "factory-a"})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	encoded, err := json.Marshal(struct {
		Factory *FactorySnapshot `json:"factory"`
	}{Factory: snapshot})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got, want := string(encoded), `{"factory":{"name":"factory-a"}}`; got != want {
		t.Fatalf("Marshal = %s, want %s", got, want)
	}
}

func TestFactorySnapshotRejectsNonObjectJSON(t *testing.T) {
	t.Parallel()

	if _, err := NewFactorySnapshot([]string{"not", "a", "factory"}); err == nil {
		t.Fatal("NewFactorySnapshot(non-object) error = nil, want actionable validation error")
	}
	var snapshot FactorySnapshot
	if err := json.Unmarshal([]byte(`null`), &snapshot); err == nil {
		t.Fatal("UnmarshalJSON(null) error = nil, want Factory object validation error")
	}
}

func TestFactorySnapshotWithNamePreservesUnknownFieldsAndDetaches(t *testing.T) {
	t.Parallel()

	snapshot := FactorySnapshot(`{"name":"old","future":{"enabled":true}}`)
	updated, err := snapshot.WithName("alpha")
	if err != nil {
		t.Fatalf("WithName: %v", err)
	}
	var got struct {
		Name   string          `json:"name"`
		Future json.RawMessage `json:"future"`
	}
	if err := updated.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Name != "alpha" {
		t.Fatalf("name = %q, want alpha", got.Name)
	}
	if string(got.Future) != `{"enabled":true}` {
		t.Fatalf("future = %s", got.Future)
	}
	if string(snapshot) != `{"name":"old","future":{"enabled":true}}` {
		t.Fatalf("source snapshot mutated: %s", snapshot)
	}
}

func TestFactorySnapshotWithNameRejectsNilSnapshot(t *testing.T) {
	t.Parallel()

	var snapshot *FactorySnapshot
	if _, err := snapshot.WithName("alpha"); err == nil {
		t.Fatal("WithName: expected error")
	}
}

func TestCloneFactoryConfigPreservesWebhookDeclarations(t *testing.T) {
	policy := FactoryWebhookDeliveryPolicyConfig{
		RequestTimeout:    stringPointer("15s"),
		MaxAttempts:       intPointer(3),
		InitialBackoff:    stringPointer("2s"),
		BackoffMultiplier: floatPointer(1.5),
		MaxBackoff:        stringPointer("10s"),
	}
	source := &FactoryConfig{Webhooks: []FactoryWebhookConfig{{
		Name:             "monitor",
		Enabled:          true,
		URL:              "https://hooks.example.test/factory",
		SigningSecretRef: "secrets/factory-monitor",
		Filter: FactoryWebhookFilterConfig{
			EventTypes:       []string{FactoryWebhookEventTypeWorkStateChange},
			DispatchStatuses: []string{FactoryWebhookDispatchStatusFailed},
		},
		DeliveryPolicy: &policy,
	}}}

	clone, err := CloneFactoryConfig(source)
	if err != nil {
		t.Fatalf("CloneFactoryConfig: %v", err)
	}
	clone.Webhooks[0].Filter.EventTypes[0] = FactoryWebhookEventTypeDispatchResponse
	*clone.Webhooks[0].DeliveryPolicy.MaxAttempts = 4
	if source.Webhooks[0].Filter.EventTypes[0] != FactoryWebhookEventTypeWorkStateChange || *source.Webhooks[0].DeliveryPolicy.MaxAttempts != 3 {
		t.Fatalf("clone mutated source webhook declaration: source=%#v clone=%#v", source.Webhooks, clone.Webhooks)
	}
}

func TestResolveFactoryWebhookDeliveryPolicyAppliesDefaults(t *testing.T) {
	effective, err := ResolveFactoryWebhookDeliveryPolicy(nil)
	if err != nil {
		t.Fatalf("ResolveFactoryWebhookDeliveryPolicy: %v", err)
	}
	if effective.RequestTimeout != DefaultFactoryWebhookRequestTimeout ||
		effective.MaxAttempts != DefaultFactoryWebhookMaxAttempts ||
		effective.InitialBackoff != DefaultFactoryWebhookInitialBackoff ||
		effective.BackoffMultiplier != DefaultFactoryWebhookBackoffMultiplier ||
		effective.MaxBackoff != DefaultFactoryWebhookMaxBackoff {
		t.Fatalf("effective defaults = %#v", effective)
	}
}

func TestResolveFactoryWebhookDeliveryPolicyParsesAuthoredValues(t *testing.T) {
	maxAttempts := 3
	multiplier := 1.5
	effective, err := ResolveFactoryWebhookDeliveryPolicy(&FactoryWebhookDeliveryPolicyConfig{
		RequestTimeout:    stringPointer("15s"),
		MaxAttempts:       &maxAttempts,
		InitialBackoff:    stringPointer("2s"),
		BackoffMultiplier: &multiplier,
		MaxBackoff:        stringPointer("10s"),
	})
	if err != nil {
		t.Fatalf("ResolveFactoryWebhookDeliveryPolicy: %v", err)
	}
	if effective.RequestTimeout != 15*time.Second ||
		effective.MaxAttempts != maxAttempts ||
		effective.InitialBackoff != 2*time.Second ||
		effective.BackoffMultiplier != multiplier ||
		effective.MaxBackoff != 10*time.Second {
		t.Fatalf("effective authored values = %#v", effective)
	}

	defaults, err := ResolveFactoryWebhookDeliveryPolicy(&FactoryWebhookDeliveryPolicyConfig{})
	if err != nil {
		t.Fatalf("ResolveFactoryWebhookDeliveryPolicy(empty): %v", err)
	}
	if defaults.RequestTimeout != DefaultFactoryWebhookRequestTimeout ||
		defaults.MaxAttempts != DefaultFactoryWebhookMaxAttempts ||
		defaults.InitialBackoff != DefaultFactoryWebhookInitialBackoff ||
		defaults.BackoffMultiplier != DefaultFactoryWebhookBackoffMultiplier ||
		defaults.MaxBackoff != DefaultFactoryWebhookMaxBackoff {
		t.Fatalf("effective empty-policy defaults = %#v", defaults)
	}
}

func TestResolveFactoryWebhookDeliveryPolicyRejectsInvalidValues(t *testing.T) {
	maxAttempts := 0
	tooSmallMultiplier := 0.5
	nanMultiplier := math.NaN()
	infiniteMultiplier := math.Inf(1)
	cases := []struct {
		name   string
		config FactoryWebhookDeliveryPolicyConfig
	}{
		{name: "invalid request timeout", config: FactoryWebhookDeliveryPolicyConfig{RequestTimeout: stringPointer("not-a-duration")}},
		{name: "nonpositive request timeout", config: FactoryWebhookDeliveryPolicyConfig{RequestTimeout: stringPointer("0s")}},
		{name: "nonpositive max attempts", config: FactoryWebhookDeliveryPolicyConfig{MaxAttempts: &maxAttempts}},
		{name: "invalid initial backoff", config: FactoryWebhookDeliveryPolicyConfig{InitialBackoff: stringPointer("-1s")}},
		{name: "small multiplier", config: FactoryWebhookDeliveryPolicyConfig{BackoffMultiplier: &tooSmallMultiplier}},
		{name: "nan multiplier", config: FactoryWebhookDeliveryPolicyConfig{BackoffMultiplier: &nanMultiplier}},
		{name: "infinite multiplier", config: FactoryWebhookDeliveryPolicyConfig{BackoffMultiplier: &infiniteMultiplier}},
		{name: "invalid max backoff", config: FactoryWebhookDeliveryPolicyConfig{MaxBackoff: stringPointer("not-a-duration")}},
		{name: "max backoff before initial backoff", config: FactoryWebhookDeliveryPolicyConfig{
			InitialBackoff: stringPointer("5s"),
			MaxBackoff:     stringPointer("1s"),
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ResolveFactoryWebhookDeliveryPolicy(&test.config); err == nil {
				t.Fatal("ResolveFactoryWebhookDeliveryPolicy succeeded, want validation error")
			}
		})
	}
}

func stringPointer(value string) *string  { return &value }
func intPointer(value int) *int           { return &value }
func floatPointer(value float64) *float64 { return &value }
