package climanifest_test

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

func TestCanonicalInputValueDecodingPreservesAuthoredTypes(t *testing.T) {
	payload := []byte(`{
		"id":"example.flag.labels",
		"long":"label",
		"shorthand":"l",
		"aliases":["tag"],
		"scope":"local",
		"valueType":"stringArray",
		"required":false,
		"repeatable":true,
		"normalization":"trim",
		"completion":"dynamic",
		"visibility":"visible",
		"lifecycle":{},
		"kind":"named",
		"minCardinality":0,
		"maxCardinality":-1,
		"defaultValue":{"stringArray":[]},
		"acceptedSources":["cli","manifest-default"],
		"handlerBindingId":"example.binding.labels"
	}`)

	var input climanifest.Flag
	if err := json.Unmarshal(payload, &input); err != nil {
		t.Fatalf("decode canonical input: %v", err)
	}
	if input.DefaultValue == nil || input.DefaultValue.StringArray == nil || len(*input.DefaultValue.StringArray) != 0 {
		t.Fatalf("defaultValue = %#v, want an explicitly authored empty string array", input.DefaultValue)
	}
	if input.HandlerBindingID != "example.binding.labels" {
		t.Fatalf("handlerBindingId = %q, want example.binding.labels", input.HandlerBindingID)
	}
}

func TestCanonicalInputValueDecodingPreservesAbsentDefault(t *testing.T) {
	payload := []byte(`{
		"id":"example.flag.count",
		"long":"count",
		"aliases":[],
		"scope":"local",
		"valueType":"int",
		"required":true,
		"minCardinality":1,
		"maxCardinality":1,
		"repeatable":false,
		"acceptedSources":["cli"],
		"handlerBindingId":"example.binding.count"
	}`)

	var input climanifest.Flag
	if err := json.Unmarshal(payload, &input); err != nil {
		t.Fatalf("decode canonical input without default: %v", err)
	}
	if input.DefaultValue != nil {
		t.Fatalf("defaultValue = %#v, want absent default", input.DefaultValue)
	}
}

func TestCommand_InputLookups(t *testing.T) {
	command := climanifest.Command{
		ID: "you.session.show",
		Arguments: map[string]climanifest.Argument{
			"session-id": {
				Position: 0,
				Name:     "session-id",
			},
		},
		Flags: map[string]climanifest.Flag{
			"json": {Long: "json"},
		},
	}

	arg, ok := command.ArgumentAt(0)
	if !ok || arg.Name != "session-id" {
		t.Fatalf("ArgumentAt(0) = %+v, %t; want session-id, true", arg, ok)
	}
	if _, ok := command.ArgumentAt(1); ok {
		t.Fatal("ArgumentAt(1) ok = true, want false")
	}

	flag, ok := command.FlagByLong("json")
	if !ok || flag.Long != "json" {
		t.Fatalf("FlagByLong(json) = %+v, %t; want json, true", flag, ok)
	}
	if _, ok := command.FlagByLong("missing"); ok {
		t.Fatal("FlagByLong(missing) ok = true, want false")
	}

	if _, err := command.RequireArgumentAt(0); err != nil {
		t.Fatalf("RequireArgumentAt(0) error = %v", err)
	}
	if _, err := command.RequireArgumentAt(1); err == nil {
		t.Fatal("RequireArgumentAt(1) error = nil, want missing argument failure")
	}
	if _, err := command.RequireFlagByLong("json"); err != nil {
		t.Fatalf("RequireFlagByLong(json) error = %v", err)
	}
	if _, err := command.RequireFlagByLong("missing"); err == nil {
		t.Fatal("RequireFlagByLong(missing) error = nil, want missing flag failure")
	}
}

func TestCommand_InputLookups_NilMaps(t *testing.T) {
	command := climanifest.Command{ID: "you"}
	if _, ok := command.ArgumentAt(0); ok {
		t.Fatal("ArgumentAt on nil map ok = true, want false")
	}
	if _, ok := command.FlagByLong("json"); ok {
		t.Fatal("FlagByLong on nil map ok = true, want false")
	}
}
