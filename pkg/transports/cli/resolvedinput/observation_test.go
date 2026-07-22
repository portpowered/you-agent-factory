package resolvedinput_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
)

func TestResolveDetachesCollectionOnIngressAndEveryEgressBoundary(t *testing.T) {
	original := []string{"first", "second"}
	inputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{definition("input.labels", resolvedinput.ValueKindStringArray)},
		[]resolvedinput.Candidate{candidate("input.labels", resolvedinput.StringArrayValue(original))},
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	original[0] = "mutated ingress"

	accessed, err := inputs.StringArray("input.labels")
	if err != nil {
		t.Fatalf("StringArray() error = %v", err)
	}
	accessed[0] = "mutated accessor"
	lookedUp, ok := inputs.Lookup("input.labels")
	if !ok {
		t.Fatal("Lookup() found = false; want true")
	}
	lookedUpStrings := observedStrings(t, inputs, "input.labels")
	lookedUpStrings[0] = "mutated observation"

	got, err := inputs.StringArray("input.labels")
	if err != nil || !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("StringArray() after mutations = %#v, %v; want detached original", got, err)
	}
	if lookedUp.Kind() != resolvedinput.ValueKindStringArray {
		t.Fatalf("Lookup().Kind() = %q; want stringArray", lookedUp.Kind())
	}
	if gotObserved := observedStrings(t, inputs, "input.labels"); !reflect.DeepEqual(gotObserved, []string{"first", "second"}) {
		t.Fatalf("Observe() after mutation = %#v; want detached original", gotObserved)
	}
}

func TestSensitiveObservationsRedactScalarAndCollectionValues(t *testing.T) {
	const scalarSecret = "scalar-secret"
	collectionSecrets := []string{"first-secret", "second-secret"}
	inputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{
			{ID: "input.token", Kind: resolvedinput.ValueKindString, Sensitive: true, Precedence: []resolvedinput.Source{resolvedinput.SourceEnvironment}},
			{ID: "input.tokens", Kind: resolvedinput.ValueKindStringArray, Sensitive: true, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
		},
		[]resolvedinput.Candidate{
			{InputID: "input.token", Source: resolvedinput.SourceEnvironment, Value: resolvedinput.StringValue(scalarSecret)},
			{InputID: "input.tokens", Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringArrayValue(collectionSecrets)},
		},
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	observations := inputs.Observations()
	if len(observations) != 2 || observations[0].InputID != "input.token" || observations[1].InputID != "input.tokens" {
		t.Fatalf("Observations() = %#v; want stable-ID order", observations)
	}
	for _, observed := range observations {
		if observed.Value != resolvedinput.RedactedValue {
			t.Fatalf("observation %q value = %#v; want %q", observed.InputID, observed.Value, resolvedinput.RedactedValue)
		}
	}
	encoded, err := json.Marshal(observations)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	formatted := string(encoded)
	humanReadable := fmt.Sprint(observations)
	var decoded []resolvedinput.Observation
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, observed := range decoded {
		if observed.Value != resolvedinput.RedactedValue {
			t.Fatalf("serialized observation %q value = %#v; want redaction marker", observed.InputID, observed.Value)
		}
	}
	for _, secret := range append([]string{scalarSecret}, collectionSecrets...) {
		if strings.Contains(formatted, secret) || strings.Contains(humanReadable, secret) {
			t.Fatalf("observation output contains sensitive value %q: JSON=%s human=%v", secret, formatted, observations)
		}
	}
}

func TestNonSensitiveObservationExposesDetachedValueAndWinningState(t *testing.T) {
	inputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{{
			ID: "input.labels", Kind: resolvedinput.ValueKindStringArray,
			Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag, resolvedinput.SourceManifestDefault},
		}},
		[]resolvedinput.Candidate{{
			InputID: "input.labels", Source: resolvedinput.SourceManifestDefault,
			Value: resolvedinput.StringArrayValue([]string{"one", "two"}),
		}},
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	observed, ok := inputs.Observe("input.labels")
	if !ok {
		t.Fatal("Observe() found = false; want true")
	}
	want := resolvedinput.Observation{
		InputID: "input.labels", Kind: resolvedinput.ValueKindStringArray,
		Provenance: resolvedinput.SourceManifestDefault, Default: true,
		Value: []string{"one", "two"},
	}
	if !reflect.DeepEqual(observed, want) {
		t.Fatalf("Observe() = %#v; want %#v", observed, want)
	}
	observed.Value.([]string)[0] = "mutated"
	if got := observedStrings(t, inputs, "input.labels"); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("Observe() after egress mutation = %#v; want detached value", got)
	}
}

func observedStrings(t *testing.T, inputs resolvedinput.Inputs, inputID string) []string {
	t.Helper()
	observed, ok := inputs.Observe(inputID)
	if !ok {
		t.Fatalf("Observe(%q) found = false; want true", inputID)
	}
	values, ok := observed.Value.([]string)
	if !ok {
		t.Fatalf("Observe(%q).Value = %#v; want []string", inputID, observed.Value)
	}
	return values
}
