package factoryfixtures

import (
	"encoding/json"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// CrossPathInvalidFactoryJSON is the shared invalid factory used to prove canonical
// validation targets are equivalent across explicit validation, config validation,
// factory save rejection, and POST /factory-validations.
//
// ProfilePrePersist (editable save pre-check) and ProfileTopology (validate-only
// endpoint) intentionally diverge on this fixture: validate-only includes deferred
// outcome-route findings; save pre-check rejects with the blocking-load subset only.
// See validationentry cross_path_profile_gap_test.go.
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

// DecodeCrossPathValidAlphaFactory decodes the shared valid alpha factory fixture.
func DecodeCrossPathValidAlphaFactory() (factoryapi.Factory, error) {
	var factory factoryapi.Factory
	if err := json.Unmarshal([]byte(CrossPathValidAlphaFactoryJSON), &factory); err != nil {
		return factoryapi.Factory{}, err
	}
	return factory, nil
}

const routelessOutputRoutesFactoryBase = `{
	"name":"alpha",
	"workTypes":[{"name":"task","states":[
		{"name":"init","type":"INITIAL"},
		{"name":"in-review","type":"PROCESSING"},
		{"name":"complete","type":"TERMINAL"},
		{"name":"failed","type":"FAILED"}
	]}],
	"workers":[{"name":"worker-a"}]`

// RoutelessCronFactoryJSON is a CRON workstation with no effective output routes.
const RoutelessCronFactoryJSON = routelessOutputRoutesFactoryBase + `,
	"workstations":[{
		"name":"cron",
		"behavior":"CRON",
		"worker":"worker-a"
	}]
}`

// RoutelessLogicalMoveFactoryJSON is a LOGICAL_MOVE workstation with no output routes.
const RoutelessLogicalMoveFactoryJSON = routelessOutputRoutesFactoryBase + `,
	"workstations":[{
		"name":"router",
		"type":"LOGICAL_MOVE"
	}]
}`

// RoutelessLogicalMoveCronFactoryJSON is a scheduled LOGICAL_MOVE (CRON) without outputs or worker.
const RoutelessLogicalMoveCronFactoryJSON = routelessOutputRoutesFactoryBase + `,
	"workstations":[{
		"name":"trigger-monkey",
		"type":"LOGICAL_MOVE",
		"behavior":"CRON",
		"cron":{"schedule":"0 * * * *"}
	}]
}`

func decodeRoutelessOutputRoutesFactory(jsonBody string) (factoryapi.Factory, error) {
	var factory factoryapi.Factory
	if err := json.Unmarshal([]byte(jsonBody), &factory); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("unmarshal factory: %w", err)
	}
	return factory, nil
}

// DecodeRoutelessCronFactory decodes RoutelessCronFactoryJSON.
func DecodeRoutelessCronFactory() (factoryapi.Factory, error) {
	return decodeRoutelessOutputRoutesFactory(RoutelessCronFactoryJSON)
}

// DecodeRoutelessLogicalMoveFactory decodes RoutelessLogicalMoveFactoryJSON.
func DecodeRoutelessLogicalMoveFactory() (factoryapi.Factory, error) {
	return decodeRoutelessOutputRoutesFactory(RoutelessLogicalMoveFactoryJSON)
}

// DecodeRoutelessLogicalMoveCronFactory decodes RoutelessLogicalMoveCronFactoryJSON.
func DecodeRoutelessLogicalMoveCronFactory() (factoryapi.Factory, error) {
	return decodeRoutelessOutputRoutesFactory(RoutelessLogicalMoveCronFactoryJSON)
}
