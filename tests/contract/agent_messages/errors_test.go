package agentmessages_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/infinite-you/internal/testutil"
	factoryclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// This contract witness protects published error representations, not HTTP
// routing or admission policy. Public functional scenarios own those outcomes.
func TestMessageErrorsPreservePublishedClientContract(t *testing.T) {
	t.Parallel()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	spec, err := loader.LoadFromFile(testutil.MustRepoPath(t, "api/openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	schema := spec.Components.Schemas["ErrorResponse"].Value
	for _, code := range []factoryapi.ErrorResponseCode{
		factoryapi.ErrorResponseCodeMESSAGEINVALIDREQUEST,
		factoryapi.ErrorResponseCodeMESSAGENOTPERMITTED,
		factoryapi.ErrorResponseCodeMESSAGERECIPIENTNOTFOUND,
		factoryapi.ErrorResponseCodeMESSAGENOTFOUND,
		factoryapi.ErrorResponseCodeMESSAGEREQUESTCONFLICT,
		factoryapi.ErrorResponseCodeMESSAGEINTERRUPTUNSUPPORTED,
		factoryapi.ErrorResponseCodeMESSAGELIMITEXCEEDED,
		factoryapi.ErrorResponseCodeMESSAGECURSORINVALID,
		factoryapi.ErrorResponseCodeMESSAGINGDISABLED,
		factoryapi.ErrorResponseCodeMESSAGESTOREUNAVAILABLE,
		factoryapi.ErrorResponseCodeMESSAGESTORECORRUPT,
	} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			response := factoryapi.ErrorResponse{
				Code: code, Family: factoryapi.ErrorFamilyBadRequest, Message: "Message operation failed.",
			}
			if code == factoryapi.ErrorResponseCodeMESSAGELIMITEXCEEDED {
				response.Details = map[string]any{"dimension": "sender", "retryAfterSeconds": 60}
			}
			decoded := messageErrorClientRoundTrip(t, schema, response)
			if string(decoded.Code) != string(code) || decoded.Message != response.Message || string(decoded.Family) != string(response.Family) {
				t.Fatalf("generated client changed error identity: %#v", decoded)
			}
			if code == factoryapi.ErrorResponseCodeMESSAGELIMITEXCEEDED {
				details, ok := decoded.Details.(map[string]any)
				if !ok || details["dimension"] != "sender" || details["retryAfterSeconds"] != float64(60) {
					t.Fatalf("generated client lost rate-limit details: %#v", decoded.Details)
				}
			}
		})
	}
	if err := schema.VisitJSON(map[string]any{"message": "invalid", "family": "BAD_REQUEST", "code": "MESSAGE_MISSPELLED"}); err == nil {
		t.Fatal("public schema accepted an unknown message error code")
	}
}

func messageErrorClientRoundTrip(t testing.TB, schema *openapi3.Schema, response factoryapi.ErrorResponse) factoryclient.ErrorResponse {
	t.Helper()
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.VisitJSON(document); err != nil {
		t.Fatalf("message error violates published schema: %v", err)
	}
	var decoded factoryclient.ErrorResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestMessageAddressAmbiguityPreservesCandidateDetails(t *testing.T) {
	t.Parallel()
	workID := "work-1"
	response := factoryapi.ErrorResponse{
		Code: factoryapi.ErrorResponseCodeWORKERSESSIONAMBIGUOUS, Family: factoryapi.ErrorFamilyConflict,
		Message: "Worker Session ID is ambiguous. Select a Factory Session with --session.",
		Details: factoryapi.WorkerSessionAddressDetails{
			Candidates: []factoryapi.WorkerSessionAddressCandidate{
				{FactorySessionId: "factory-1", WorkerSessionId: "worker-1", WorkId: &workID, State: factoryapi.WorkerSessionAddressCandidateState("RUNNING")},
				{FactorySessionId: "factory-2", WorkerSessionId: "worker-1", State: factoryapi.WorkerSessionAddressCandidateState("COMPLETED")},
			},
		},
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var decoded factoryclient.ErrorResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Code != factoryclient.ErrorResponseCodeWORKERSESSIONAMBIGUOUS || decoded.Family != factoryclient.ErrorFamilyConflict {
		t.Fatal("message schema extension changed the existing ambiguity identity")
	}
	details, err := json.Marshal(decoded.Details)
	if err != nil {
		t.Fatal(err)
	}
	var candidates factoryclient.WorkerSessionAddressDetails
	if err := json.Unmarshal(details, &candidates); err != nil {
		t.Fatal(err)
	}
	want := factoryclient.WorkerSessionAddressDetails{Candidates: []factoryclient.WorkerSessionAddressCandidate{
		{FactorySessionId: "factory-1", WorkerSessionId: "worker-1", WorkId: &workID, State: factoryclient.WorkerSessionAddressCandidateState("RUNNING")},
		{FactorySessionId: "factory-2", WorkerSessionId: "worker-1", State: factoryclient.WorkerSessionAddressCandidateState("COMPLETED")},
	}}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("ambiguity no longer identifies the exact selectable owners: %#v", candidates)
	}
}
