package globalconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestMessagingConfigurationDefaultsAndExplicitValues(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, document string
		enabled        bool
		hop, retention int
	}{
		{"absent", `{}`, true, 3, 30},
		{"empty", `{"messaging":{}}`, true, 3, 30},
		{"disabled and zero hop", `{"messaging":{"enabled":false,"retentionDays":1,"limits":{"maxHop":0}}}`, false, 0, 1},
		{"maximum expiry", `{"messaging":{"limits":{"defaultExpirySeconds":604800}}}`, true, 3, 30},
		{"last alias disables", `{"messaging":{"enabled":true},"MESSAGING":{"enabled":false}}`, false, 3, 30},
		{"last alias enables", `{"MESSAGING":{"enabled":false},"messaging":{"enabled":true}}`, true, 3, 30},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			config, diagnostics, err := DecodeWithDiagnostics([]byte(test.document))
			if err != nil || len(diagnostics.IgnoredJSONPaths) != 0 {
				t.Fatalf("decode: %v, diagnostics: %v", err, diagnostics)
			}
			effective, err := config.Messaging.Effective()
			if err != nil || effective.Enabled != test.enabled || effective.MaxHop != test.hop || effective.RetentionDays != test.retention {
				t.Fatalf("effective: %+v, %v", effective, err)
			}
			encoded, err := Encode(config)
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := Decode(encoded)
			if err != nil || !reflect.DeepEqual(config.Messaging, recovered.Messaging) {
				t.Fatalf("round trip changed authored presence: %v", err)
			}
		})
	}
}

func TestInvalidMessagingIsIsolatedAndPreservedDuringUnrelatedEdit(t *testing.T) {
	t.Parallel()
	for _, section := range []string{
		`null`, `false`, `[]`, `"private-invalid-value"`,
		`{"enabled":"private-invalid-value"}`, `{"limits":null}`, `{"enabled":null}`,
		`{"policy":{"scopeLabelKeys":null}}`, `{"policy":{"allowCrossScope":true}}`,
		`{"limits":{"maxBodyBytes":8193}}`, `{"limits":{"maxHop":-1}}`,
		`{"limits":{"maxHop":4}}`, `{"limits":{"sendsPerChainPerHour":0}}`,
		`{"limits":{"messagesPerThread":0}}`, `{"limits":{"defaultExpirySeconds":59}}`,
		`{"limits":{"defaultExpirySeconds":604801}}`, `{"retentionDays":0}`,
		`{"retentionDays":9223372036854775807}`, `{"policy":{"scopeLabelKeys":[""]}}`,
		`{"unknown":"private-invalid-value"}`,
	} {
		t.Run(section, func(t *testing.T) {
			t.Parallel()
			original := []byte(`{"defaults":{"workerModel":"old"},"messaging":` + section + `}`)
			config, err := Decode(original)
			if err != nil || config.Defaults.WorkerModel != "old" {
				t.Fatalf("Messaging blocked unrelated configuration: %v", err)
			}
			if _, err := config.Messaging.Effective(); !errors.Is(err, operatorsettings.ErrMessagingSettingsInvalid) || bytes.Contains([]byte(err.Error()), []byte("private-invalid-value")) {
				t.Fatalf("expected safe Messaging quarantine, got %v", err)
			}
			config.Defaults.WorkerModel = "new"
			canonical, err := Encode(config)
			if err != nil {
				t.Fatal(err)
			}
			preserved, err := PreserveUnknownFields(original, canonical)
			if err != nil {
				t.Fatal(err)
			}
			var before, after map[string]any
			if err := json.Unmarshal(original, &before); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(preserved, &after); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before["messaging"], after["messaging"]) {
				t.Fatal("unrelated edit changed invalid Messaging settings")
			}
			recovered, err := Decode(preserved)
			if err != nil || recovered.Defaults.WorkerModel != "new" {
				t.Fatalf("edit lost: %v", err)
			}
			if _, err := recovered.Messaging.Effective(); !errors.Is(err, operatorsettings.ErrMessagingSettingsInvalid) {
				t.Fatalf("reconstruction lost quarantine: %v", err)
			}
		})
	}
}

func TestMessagingConfigurationCloneDetachesAllMutableValues(t *testing.T) {
	t.Parallel()
	config, err := Decode([]byte(`{"messaging":{"enabled":false,"retentionDays":4,"policy":{"scopeLabelKeys":["tag:project"],"allowCrossScope":false},"limits":{"sendsPerChainPerHour":2,"messagesPerThread":3,"maxBodyBytes":40,"maxHop":0,"defaultExpirySeconds":60}}}`))
	if err != nil {
		t.Fatal(err)
	}
	cloned := config.Clone()
	*cloned.Messaging.Enabled = true
	*cloned.Messaging.RetentionDays = 5
	(*cloned.Messaging.Policy.ScopeLabelKeys)[0] = "other"
	*cloned.Messaging.Policy.AllowCrossScope = true
	*cloned.Messaging.Limits.SendsPerChainPerHour = 4
	*cloned.Messaging.Limits.MessagesPerThread = 4
	*cloned.Messaging.Limits.MaxBodyBytes = 50
	*cloned.Messaging.Limits.MaxHop = 1
	*cloned.Messaging.Limits.DefaultExpirySeconds = 90
	effective, err := config.Messaging.Effective()
	if err != nil || effective.Enabled || effective.RetentionDays != 4 || effective.ScopeLabelKeys[0] != "tag:project" || effective.SendsPerChainPerHour != 2 || effective.MessagesPerThread != 3 || effective.MaxBodyBytes != 40 || effective.MaxHop != 0 || effective.DefaultExpirySeconds != 60 {
		t.Fatalf("clone mutated source: %+v, %v", effective, err)
	}
	invalid, err := Decode([]byte(`{"messaging":"bad"}`))
	if err != nil {
		t.Fatal(err)
	}
	copy := invalid.Clone()
	copy.Messaging.InvalidJSON[1] = 'x'
	if bytes.Equal(copy.Messaging.InvalidJSON, invalid.Messaging.InvalidJSON) {
		t.Fatal("invalid JSON aliases")
	}
}
