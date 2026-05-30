package validation

import (
	"encoding/json"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// CrossPathInvalidFactoryJSON is the shared invalid factory used to prove canonical
// validation targets are equivalent across explicit validation, config validation,
// factory save rejection, and POST /factory-validations.
const CrossPathInvalidFactoryJSON = `{
	"name":"alpha",
	"workTypes":[{"name":"story","states":[
		{"name":"queued","type":"INITIAL"},
		{"name":"queued-dup","type":"PROCESSING"}
	]}],
	"workers":[{"name":"worker-a"},{"name":"worker-a"}],
	"workstations":[{
		"name":"process",
		"worker":"missing-worker",
		"inputs":[{"workType":"story","state":"queued"}],
		"outputs":[{"workType":"story","state":"missing-state"}]
	}]
}`

// CrossPathValidAlphaFactoryJSON is a persistable alpha factory compatible with
// CrossPathInvalidFactoryJSON work-type names for save-rejection equivalence tests.
const CrossPathValidAlphaFactoryJSON = `{
	"name":"alpha",
	"workTypes":[{"name":"story","states":[
		{"name":"queued","type":"INITIAL"},
		{"name":"done","type":"TERMINAL"},
		{"name":"failed","type":"FAILED"}
	]}],
	"workers":[{"name":"worker-a"}],
	"workstations":[{
		"name":"process",
		"worker":"worker-a",
		"inputs":[{"workType":"story","state":"queued"}],
		"outputs":[{"workType":"story","state":"done"}],
		"onFailure":[{"workType":"story","state":"failed"}]
	}]
}`

// DecodeCrossPathInvalidFactory decodes the shared invalid factory fixture.
func DecodeCrossPathInvalidFactory() (factoryapi.Factory, error) {
	var factory factoryapi.Factory
	if err := json.Unmarshal([]byte(CrossPathInvalidFactoryJSON), &factory); err != nil {
		return factoryapi.Factory{}, err
	}
	return factory, nil
}
