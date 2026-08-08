package acp_test

import (
	"encoding/json"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp"
)

func sampleFactoryTargetOption() acp.FactoryTargetOption {
	return acp.FactoryTargetOption{
		CurrentValue: "factory:@you/factory-builder",
		Choices: []acp.FactoryTargetChoice{
			{Value: "factory:@you/factory-builder", Name: "Factory Builder"},
			{Value: "factory:@you/review", Name: "Review"},
		},
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestFactoryTargetOptionToSessionConfigOptionSerializesSelectUnion(t *testing.T) {
	option := sampleFactoryTargetOption()

	wire, err := option.ToSessionConfigOption()
	if err != nil {
		t.Fatalf("ToSessionConfigOption() unexpected error: %v", err)
	}
	if wire.Select == nil {
		t.Fatal("ToSessionConfigOption() Select is nil, want the select-option variant")
	}
	if wire.Boolean != nil {
		t.Fatal("ToSessionConfigOption() Boolean is set, want nil")
	}
	if wire.Select.Type != "select" {
		t.Fatalf("ToSessionConfigOption() Type = %q, want %q", wire.Select.Type, "select")
	}
	if wire.Select.Id != acp.FactoryTargetOptionID {
		t.Fatalf("ToSessionConfigOption() Id = %q, want %q", wire.Select.Id, acp.FactoryTargetOptionID)
	}
	if wire.Select.Name != acp.FactoryTargetOptionName {
		t.Fatalf("ToSessionConfigOption() Name = %q, want %q", wire.Select.Name, acp.FactoryTargetOptionName)
	}
	if wire.Select.CurrentValue != option.CurrentValue {
		t.Fatalf("ToSessionConfigOption() CurrentValue = %q, want %q", wire.Select.CurrentValue, option.CurrentValue)
	}
	if wire.Select.Options.Ungrouped == nil {
		t.Fatal("ToSessionConfigOption() Options.Ungrouped is nil")
	}
	if got, want := len(*wire.Select.Options.Ungrouped), len(option.Choices); got != want {
		t.Fatalf("ToSessionConfigOption() choice count = %d, want %d", got, want)
	}

	if err := wire.Validate(); err != nil {
		t.Fatalf("wire.Validate() unexpected error: %v", err)
	}

	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("json.Marshal(wire) unexpected error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(wire) unexpected error: %v", err)
	}
	if decoded["type"] != "select" {
		t.Fatalf("serialized type = %v, want %q", decoded["type"], "select")
	}
	if decoded["id"] != string(acp.FactoryTargetOptionID) {
		t.Fatalf("serialized id = %v, want %q", decoded["id"], acp.FactoryTargetOptionID)
	}
	if decoded["currentValue"] != string(option.CurrentValue) {
		t.Fatalf("serialized currentValue = %v, want %q", decoded["currentValue"], option.CurrentValue)
	}
	if _, present := decoded["options"]; !present {
		t.Fatal("serialized option is missing 'options'")
	}
}

func TestFactoryTargetOptionRoundTripsThroughSessionConfigOption(t *testing.T) {
	option := sampleFactoryTargetOption()

	wire, err := option.ToSessionConfigOption()
	if err != nil {
		t.Fatalf("ToSessionConfigOption() unexpected error: %v", err)
	}

	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("json.Marshal(wire) unexpected error: %v", err)
	}

	var decodedWire acpsdk.SessionConfigOption
	if err := json.Unmarshal(data, &decodedWire); err != nil {
		t.Fatalf("json.Unmarshal(wire) unexpected error: %v", err)
	}

	decoded, err := acp.FactoryTargetOptionFromSessionConfigOption(decodedWire)
	if err != nil {
		t.Fatalf("FactoryTargetOptionFromSessionConfigOption() unexpected error: %v", err)
	}
	if decoded.CurrentValue != option.CurrentValue {
		t.Fatalf("round-tripped CurrentValue = %q, want %q", decoded.CurrentValue, option.CurrentValue)
	}
	if len(decoded.Choices) != len(option.Choices) {
		t.Fatalf("round-tripped choice count = %d, want %d", len(decoded.Choices), len(option.Choices))
	}
	for i, choice := range option.Choices {
		if decoded.Choices[i] != choice {
			t.Fatalf("round-tripped choice[%d] = %+v, want %+v", i, decoded.Choices[i], choice)
		}
	}
}

func TestFactoryTargetOptionToSessionConfigOptionRejectsEmptyChoices(t *testing.T) {
	option := acp.FactoryTargetOption{CurrentValue: "factory:@you/review"}
	_, err := option.ToSessionConfigOption()
	if !errors.Is(err, acp.ErrFactoryTargetOptionEmpty) {
		t.Fatalf("ToSessionConfigOption() error = %v, want %v", err, acp.ErrFactoryTargetOptionEmpty)
	}
}

func TestFactoryTargetOptionToSessionConfigOptionRejectsMissingCurrentValue(t *testing.T) {
	option := acp.FactoryTargetOption{
		CurrentValue: "factory:@you/unknown",
		Choices: []acp.FactoryTargetChoice{
			{Value: "factory:@you/review", Name: "Review"},
		},
	}
	_, err := option.ToSessionConfigOption()
	if !errors.Is(err, acp.ErrFactoryTargetOptionCurrentValueMissing) {
		t.Fatalf("ToSessionConfigOption() error = %v, want %v", err, acp.ErrFactoryTargetOptionCurrentValueMissing)
	}
}

func TestFactoryTargetOptionFromSessionConfigOptionRejectsBooleanVariant(t *testing.T) {
	wire := acpsdk.SessionConfigOption{
		Boolean: &acpsdk.SessionConfigOptionBoolean{
			Id:           "target",
			Name:         "Factory",
			CurrentValue: true,
			Type:         "boolean",
		},
	}
	_, err := acp.FactoryTargetOptionFromSessionConfigOption(wire)
	if !errors.Is(err, acp.ErrFactoryTargetOptionNotSelect) {
		t.Fatalf("FactoryTargetOptionFromSessionConfigOption() error = %v, want %v", err, acp.ErrFactoryTargetOptionNotSelect)
	}
}

func TestFactoryTargetOptionFromSessionConfigOptionRejectsEmptyOptions(t *testing.T) {
	wire := acpsdk.SessionConfigOption{
		Select: &acpsdk.SessionConfigOptionSelect{
			Id:           "target",
			Name:         "Factory",
			CurrentValue: "factory:@you/review",
			Type:         "select",
		},
	}
	_, err := acp.FactoryTargetOptionFromSessionConfigOption(wire)
	if !errors.Is(err, acp.ErrFactoryTargetOptionEmpty) {
		t.Fatalf("FactoryTargetOptionFromSessionConfigOption() error = %v, want %v", err, acp.ErrFactoryTargetOptionEmpty)
	}
}

func TestFactoryTargetOptionNeverProducesModelCapabilityFields(t *testing.T) {
	option := sampleFactoryTargetOption()
	wire, err := option.ToSessionConfigOption()
	if err != nil {
		t.Fatalf("ToSessionConfigOption() unexpected error: %v", err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("json.Marshal(wire) unexpected error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(wire) unexpected error: %v", err)
	}
	for _, forbidden := range []string{"model", "modelId", "models"} {
		if _, present := decoded[forbidden]; present {
			t.Fatalf("serialized Factory target option unexpectedly contains model field %q", forbidden)
		}
	}
}
