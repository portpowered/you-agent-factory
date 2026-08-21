package mappingtests

import (
	"reflect"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

const generatedFactoryBoundaryErrorPrefix = "decode factory generated-schema boundary"

func stringPtr(value string) *string {
	return &value
}

func TestFactoryConfigMapper_ExpandAndFlattenPreservesLogicalRoundTripGuard(t *testing.T) {
	mapper := NewFactoryConfigMapper()
	raw := []byte(`{
		"name":"logical-round-trip-test",
		"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"failed","type":"FAILED"}]}],
		"workstations":[
			{"name":"process","inputs":[{"workType":"task","state":"init"}]},
			{"name":"review","inputs":[{"workType":"task","state":"init"}]},
			{"name":"review-loop-breaker","inputs":[{"workType":"task","state":"init"}],"guards":[{"type":"VISIT_COUNT","workstation":"review","maxVisits":12,"logicalRoundTrip":{"workstations":["process","review"],"maxRawVisits":24}}]}
		]
	}`)

	cfg, err := mapper.Expand(raw)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}
	guard := cfg.Workstations[2].Guards[0]
	if guard.LogicalRoundTrip == nil || !reflect.DeepEqual(guard.LogicalRoundTrip.Workstations, []string{"process", "review"}) || guard.LogicalRoundTrip.MaxRawVisits != 24 {
		t.Fatalf("expanded logical round-trip guard = %#v, want pair and raw ceiling", guard.LogicalRoundTrip)
	}

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}
	payload := mustDecodeFactoryPayload(t, flattened)
	workstations := payload["workstations"].([]any)
	guardPayload := workstations[2].(map[string]any)["guards"].([]any)[0].(map[string]any)
	logical, ok := guardPayload["logicalRoundTrip"].(map[string]any)
	if !ok {
		t.Fatalf("flattened logicalRoundTrip = %#v, want object", guardPayload["logicalRoundTrip"])
	}
	if !reflect.DeepEqual(logical["workstations"], []any{"process", "review"}) || logical["maxRawVisits"] != float64(24) {
		t.Fatalf("flattened logicalRoundTrip = %#v, want pair and raw ceiling", logical)
	}
}
