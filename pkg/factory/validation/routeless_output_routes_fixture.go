package validation

import (
	"encoding/json"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

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
