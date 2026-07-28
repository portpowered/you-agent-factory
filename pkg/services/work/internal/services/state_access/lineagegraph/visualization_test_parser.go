package lineagegraph

import (
	"encoding/json"
	"fmt"
	"strings"
)

func testVisualizationParser(data []byte) (BatchRequest, error) {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return BatchRequest{}, err
	}
	if strings.Contains(string(data), `"work_type_id"`) {
		return BatchRequest{}, fmt.Errorf("work_type_id is not supported")
	}
	var req struct {
		RequestID string `json:"requestId"`
		Type      string `json:"type"`
		Works     []struct {
			Name       string `json:"name"`
			WorkTypeID string `json:"workTypeName"`
		} `json:"works"`
		Relations []struct {
			Type           string `json:"type"`
			SourceWorkName string `json:"sourceWorkName"`
			TargetWorkName string `json:"targetWorkName"`
		} `json:"relations"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return BatchRequest{}, err
	}
	batch := BatchRequest{
		RequestID: req.RequestID,
		Type:      req.Type,
	}
	for _, work := range req.Works {
		batch.Works = append(batch.Works, BatchWork{
			Name:       work.Name,
			WorkTypeID: work.WorkTypeID,
		})
	}
	for _, rel := range req.Relations {
		batch.Relations = append(batch.Relations, BatchRelation{
			Type:           rel.Type,
			SourceWorkName: rel.SourceWorkName,
			TargetWorkName: rel.TargetWorkName,
		})
	}
	return batch, nil
}
