package generated

import "encoding/json"

// UnmarshalJSON accepts the legacy string state form for submitted or replayed
// work while keeping read responses on the structured WorkState shape.
func (w *Work) UnmarshalJSON(data []byte) error {
	type workAlias Work
	var raw struct {
		workAlias
		State json.RawMessage `json:"state,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*w = Work(raw.workAlias)
	if len(raw.State) == 0 || string(raw.State) == "null" {
		return nil
	}
	var state WorkState
	if err := json.Unmarshal(raw.State, &state); err == nil {
		w.State = &state
		return nil
	}
	var stateName string
	if err := json.Unmarshal(raw.State, &stateName); err != nil {
		return err
	}
	w.State = &WorkState{Name: stateName, Type: WorkStateTypePROCESSING}
	return nil
}
