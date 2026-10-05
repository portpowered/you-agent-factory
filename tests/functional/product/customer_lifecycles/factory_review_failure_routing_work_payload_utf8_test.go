package customer_lifecycles_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	utf8Section = "§"
	utf8EmDash  = "—"
)

// TestWorkPayload_NonASCIIRoundTripsByteExact submits synthetic payloads
// holding U+00A7 and U+2014 (as raw UTF-8 and as JSON unicode escapes, as a
// plain string and as an object) and requires the identical text from the Work
// read, the WORK_REQUEST event, and the rendered worker prompt.
func testFactoryreviewfailureroutingWorkPayload_NonASCIIRoundTripsByteExact(t *testing.T) {
	t.Parallel()
	text := "rule " + utf8Section + "4 " + utf8EmDash + " keep"
	rawText, _ := json.Marshal(text) // Go keeps non-ASCII as raw UTF-8
	escapedText := `"rule §4 — keep"`
	rawObject := `{"contract":"rule ` + utf8Section + `4 ` + utf8EmDash + ` keep"}`
	escapedObject := `{"contract":"rule §4 — keep"}`

	for name, tc := range map[string]struct {
		payloadJSON string
		want        string
	}{
		"string raw utf8": {string(rawText), string(rawText)},
		"string escaped":  {escapedText, string(rawText)},
		"object raw utf8": {rawObject, rawObject},
		"object escaped":  {escapedObject, rawObject},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			scenario, capture := openPayloadScenario(t)
			id := scenario.marker + "-idea"
			body := fmt.Sprintf(
				`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workId":%q,"workTypeName":"idea","traceId":%q,"currentChainingTraceId":%q,"payload":%s}]}`,
				scenario.marker+"-req", id, id, scenario.marker+"-trace", scenario.marker+"-trace", tc.payloadJSON,
			)
			scenario.putWorkRequest(t, scenario.marker+"-req", []byte(body), 1)

			prompt := capture.awaitContaining(t, "rule ")
			if !strings.Contains(prompt, text) {
				t.Fatalf("worker prompt does not contain %q byte-exact; prompt = %q", text, prompt)
			}

			read := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(scenario.fixture.baseURL, scenario.sessionID, "/work/"+id))
			assertPayloadJSONEquals(t, "Work read", read.Payload, tc.want)

			found := false
			for _, event := range support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID) {
				if event.Type != factoryapi.FactoryEventTypeWorkRequest {
					continue
				}
				payload, err := event.Payload.AsWorkRequestEventPayload()
				if err != nil || payload.Works == nil {
					continue
				}
				for _, item := range *payload.Works {
					if item.WorkId != nil && *item.WorkId == id {
						found = true
						assertPayloadJSONEquals(t, "WORK_REQUEST event", item.Payload, tc.want)
					}
				}
			}
			if !found {
				t.Fatalf("no WORK_REQUEST event recorded Work %q", id)
			}
		})
	}
}

func assertPayloadJSONEquals(t *testing.T, label string, payload any, want string) {
	t.Helper()
	got, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("%s: marshal payload: %v", label, err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("%s: decode got: %v", label, err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("%s: decode want: %v", label, err)
	}
	gotNorm, _ := json.Marshal(gotValue)
	wantNorm, _ := json.Marshal(wantValue)
	if string(gotNorm) != string(wantNorm) || strings.ContainsAny(string(got), "Ââ") {
		t.Fatalf("%s payload = %q, want %q", label, got, want)
	}
}
