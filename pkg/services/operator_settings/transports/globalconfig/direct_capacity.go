package globalconfig

import (
	"encoding/json"
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func validateDirectCapacityJSON(raw []byte) error {
	var document struct {
		Defaults map[string]json.RawMessage `json:"defaults"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return err
	}
	value, present := document.Defaults["directSessionCapacity"]
	if !present {
		return nil
	}
	var capacity *int64
	if json.Unmarshal(value, &capacity) != nil || capacity == nil || *capacity < 1 {
		return fmt.Errorf("%w: defaults.directSessionCapacity must be a positive int64 integer", operatorsettings.ErrDocumentMalformed)
	}
	return nil
}
