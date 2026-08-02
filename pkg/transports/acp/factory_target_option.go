package acp

import (
	"errors"

	acpsdk "github.com/coder/acp-go-sdk"
)

// FactoryTargetOptionID is the stable ACP session configuration option id
// used to represent Factory target selection.
const FactoryTargetOptionID acpsdk.SessionConfigId = "target"

// FactoryTargetOptionName is the human-readable label shown for the Factory
// target selector.
const FactoryTargetOptionName = "Factory"

// FactoryTargetOptionCategory is the UX-only semantic category advertised
// for the Factory target selector. It does not make the option an ACP model
// resource; a Factory is an orchestration definition, not a You Model.
const FactoryTargetOptionCategory acpsdk.SessionConfigOptionCategory = acpsdk.SessionConfigOptionCategoryModel

// FactoryTargetOptionType is the required discriminant of the ACP
// select-option union.
const FactoryTargetOptionType = "select"

// ErrFactoryTargetOptionEmpty reports that a Factory target option was built
// with no selectable choices.
var ErrFactoryTargetOptionEmpty = errors.New("acp: factory target option requires at least one choice")

// ErrFactoryTargetOptionCurrentValueMissing reports that a Factory target
// option's current value is not one of its own choices.
var ErrFactoryTargetOptionCurrentValueMissing = errors.New("acp: factory target option current value must be one of its choices")

// ErrFactoryTargetOptionNotSelect reports that a decoded ACP session
// configuration option was not the select-option variant this V0 boundary
// requires for Factory target selection.
var ErrFactoryTargetOptionNotSelect = errors.New("acp: factory target option requires the select-option union variant")

// FactoryTargetChoice is one selectable Factory target value.
type FactoryTargetChoice struct {
	Value acpsdk.SessionConfigValueId
	Name  string
}

// FactoryTargetOption is the domain-level representation of Factory target
// selection. It is a session configuration option, never an ACP model
// resource.
type FactoryTargetOption struct {
	CurrentValue acpsdk.SessionConfigValueId
	Choices      []FactoryTargetChoice
}

// ToSessionConfigOption serializes the Factory target option as the ACP
// select-option discriminated union (type=select) with a stable id, name,
// current value, and allowed values.
func (option FactoryTargetOption) ToSessionConfigOption() (acpsdk.SessionConfigOption, error) {
	if len(option.Choices) == 0 {
		return acpsdk.SessionConfigOption{}, ErrFactoryTargetOptionEmpty
	}
	found := false
	ungrouped := make(acpsdk.SessionConfigSelectOptionsUngrouped, 0, len(option.Choices))
	for _, choice := range option.Choices {
		if choice.Value == option.CurrentValue {
			found = true
		}
		ungrouped = append(ungrouped, acpsdk.SessionConfigSelectOption{
			Value: choice.Value,
			Name:  choice.Name,
		})
	}
	if !found {
		return acpsdk.SessionConfigOption{}, ErrFactoryTargetOptionCurrentValueMissing
	}
	category := FactoryTargetOptionCategory
	return acpsdk.SessionConfigOption{
		Select: &acpsdk.SessionConfigOptionSelect{
			Id:           FactoryTargetOptionID,
			Name:         FactoryTargetOptionName,
			Category:     &category,
			CurrentValue: option.CurrentValue,
			Options: acpsdk.SessionConfigSelectOptions{
				Ungrouped: &ungrouped,
			},
			Type: FactoryTargetOptionType,
		},
	}, nil
}

// FactoryTargetOptionFromSessionConfigOption decodes an ACP session
// configuration option back into the domain-level Factory target
// representation, rejecting anything that is not the select-option variant
// with a non-empty choice set containing the current value.
func FactoryTargetOptionFromSessionConfigOption(wire acpsdk.SessionConfigOption) (FactoryTargetOption, error) {
	if wire.Select == nil || wire.Select.Type != FactoryTargetOptionType {
		return FactoryTargetOption{}, ErrFactoryTargetOptionNotSelect
	}
	selectOption := wire.Select
	var choices []FactoryTargetChoice
	if selectOption.Options.Ungrouped != nil {
		for _, choice := range *selectOption.Options.Ungrouped {
			choices = append(choices, FactoryTargetChoice{Value: choice.Value, Name: choice.Name})
		}
	}
	if len(choices) == 0 {
		return FactoryTargetOption{}, ErrFactoryTargetOptionEmpty
	}
	found := false
	for _, choice := range choices {
		if choice.Value == selectOption.CurrentValue {
			found = true
			break
		}
	}
	if !found {
		return FactoryTargetOption{}, ErrFactoryTargetOptionCurrentValueMissing
	}
	return FactoryTargetOption{
		CurrentValue: selectOption.CurrentValue,
		Choices:      choices,
	}, nil
}
