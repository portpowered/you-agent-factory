package apicontract_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/sessionpersistence"
)

type syncPreflightRecoveryFixtureCatalog struct {
	Scenarios                []syncPreflightRecoveryScenario      `json:"scenarios"`
	IdentityScopeComparisons []syncPreflightIdentityScopeScenario `json:"identityScopeComparisons"`
}

type syncPreflightRecoveryScenario struct {
	ID       string                       `json:"id"`
	Tags     syncPreflightRecoveryTags    `json:"tags"`
	Response map[string]any               `json:"response"`
}

type syncPreflightRecoveryTags struct {
	ReasonCode         string `json:"reasonCode"`
	CheckpointReusable string `json:"checkpointReusable,omitempty"`
	CursorValid        string `json:"cursorValid,omitempty"`
}

type syncPreflightIdentityScopeScenario struct {
	ID                 string         `json:"id"`
	Previous           map[string]any `json:"previous"`
	Current            map[string]any `json:"current"`
	WantClassification string         `json:"wantClassification"`
}

func TestOpenAPIContract_SyncPreflightRecoveryFixturesValidateAndRoundTrip(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	catalog := loadSyncPreflightRecoveryFixtureCatalog(t)

	seenReasonCodes := map[string]struct{}{}
	for _, scenario := range catalog.Scenarios {
		t.Run(scenario.ID, func(t *testing.T) {
			assertSyncPreflightRecoveryScenarioFixture(t, doc, scenario)
			seenReasonCodes[scenario.Tags.ReasonCode] = struct{}{}
		})
	}

	for _, reasonCode := range []string{"ok", "cursor_stale", "session_not_found", "logical_session_remap"} {
		if _, ok := seenReasonCodes[reasonCode]; !ok {
			t.Fatalf("sync preflight recovery fixture coverage for %s = missing, want scenario", reasonCode)
		}
	}
}

func TestOpenAPIContract_SyncPreflightIdentityScopeComparisonsDistinguishBackendAndStreamChanges(t *testing.T) {
	catalog := loadSyncPreflightRecoveryFixtureCatalog(t)

	for _, scenario := range catalog.IdentityScopeComparisons {
		t.Run(scenario.ID, func(t *testing.T) {
			previous := identityScopeFromFixtureMap(scenario.Previous)
			current := identityScopeFromFixtureMap(scenario.Current)

			reason, ok := sessionpersistence.ClassifyIdentityMismatch(previous, current)
			if !ok {
				t.Fatal("ClassifyIdentityMismatch = false, want mismatch")
			}
			if string(reason) != scenario.WantClassification {
				t.Fatalf("classification = %q, want %q", reason, scenario.WantClassification)
			}

			if previous.BackendScopeID == current.BackendScopeID &&
				scenario.WantClassification == string(sessionpersistence.ReasonBackendScopeChanged) {
				t.Fatal("backend scope classification requires backendScopeId change")
			}
			if previous.StreamGenerationID == current.StreamGenerationID &&
				scenario.WantClassification == string(sessionpersistence.ReasonStreamGenerationChanged) {
				t.Fatal("stream generation classification requires streamGenerationId change")
			}
			if previous.BackendScopeID != current.BackendScopeID &&
				scenario.WantClassification == string(sessionpersistence.ReasonStreamGenerationChanged) &&
				previous.FactorySessionID == current.FactorySessionID {
				if previous.BackendScopeID == current.BackendScopeID {
					t.Fatal("stream-only classification should not change backendScopeId")
				}
			}
		})
	}
}

func assertSyncPreflightRecoveryScenarioFixture(
	t *testing.T,
	doc *openapi3.T,
	scenario syncPreflightRecoveryScenario,
) {
	t.Helper()

	assertOpenAPIFixtureValidates(t, doc, "FactorySessionSyncPreflightResponse", scenario.Response)
	assertGeneratedFixtureRoundTrip(t, scenario.Response, "FactorySessionSyncPreflightResponse", func(raw []byte) {
		var value factoryapi.FactorySessionSyncPreflightResponse
		decodeRoundTripJSON(t, raw, &value, scenario.ID+" sync preflight response")
		assertSyncPreflightRecoveryOutcome(t, scenario, value)
	})
}

func assertSyncPreflightRecoveryOutcome(
	t *testing.T,
	scenario syncPreflightRecoveryScenario,
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	t.Helper()

	if string(response.ReasonCode) != scenario.Tags.ReasonCode {
		t.Fatalf("%s reasonCode = %q, want %q", scenario.ID, response.ReasonCode, scenario.Tags.ReasonCode)
	}

	switch response.ReasonCode {
	case factoryapi.Ok:
		if !response.CheckpointReusable {
			t.Fatalf("%s checkpointReusable = false, want true", scenario.ID)
		}
		if !response.ReconnectCursor.ValidForStreamGeneration {
			t.Fatalf("%s reconnect cursor validForStreamGeneration = false, want true", scenario.ID)
		}
		if response.BackendScopeId == nil || response.FactorySessionId == nil || response.StreamGenerationId == nil {
			t.Fatalf("%s identity fields = %#v, want full identity set", scenario.ID, response)
		}
	case factoryapi.CursorStale:
		if response.CheckpointReusable {
			t.Fatalf("%s checkpointReusable = true, want false", scenario.ID)
		}
		if response.ReconnectCursor.ValidForStreamGeneration {
			t.Fatalf("%s reconnect cursor validForStreamGeneration = true, want false", scenario.ID)
		}
		if response.BackendScopeId == nil || response.FactorySessionId == nil || response.StreamGenerationId == nil {
			t.Fatalf("%s identity fields = %#v, want full identity set for stale cursor", scenario.ID, response)
		}
	case factoryapi.SessionNotFound:
		if response.CheckpointReusable {
			t.Fatalf("%s checkpointReusable = true, want false", scenario.ID)
		}
		if response.BackendScopeId != nil || response.FactorySessionId != nil || response.StreamGenerationId != nil {
			t.Fatalf("%s identity fields = %#v, want nil for missing session", scenario.ID, response)
		}
	case factoryapi.LogicalSessionRemap:
		if response.CheckpointReusable {
			t.Fatalf("%s checkpointReusable = true, want false", scenario.ID)
		}
		if response.BackendScopeId == nil || response.FactorySessionId == nil || response.StreamGenerationId == nil {
			t.Fatalf("%s identity fields = %#v, want full identity set for remap", scenario.ID, response)
		}
	default:
		t.Fatalf("%s reasonCode = %q, want supported recovery outcome", scenario.ID, response.ReasonCode)
	}

	diagnostic, ok := sessionpersistence.InvalidationFromSyncPreflight(response)
	switch response.ReasonCode {
	case factoryapi.Ok:
		if ok {
			t.Fatalf("%s invalidation diagnostic = %#v, want none for ok", scenario.ID, diagnostic)
		}
	default:
		if !ok {
			t.Fatalf("%s invalidation diagnostic missing for %q", scenario.ID, response.ReasonCode)
		}
	}
}

func identityScopeFromFixtureMap(payload map[string]any) sessionpersistence.IdentityScope {
	return sessionpersistence.IdentityScope{
		BackendScopeID:      stringFixtureValue(payload, "backendScopeId"),
		LogicalSessionKeyID: stringFixtureValue(payload, "logicalSessionKeyId"),
		FactorySessionID:    stringFixtureValue(payload, "factorySessionId"),
		StreamGenerationID:  stringFixtureValue(payload, "streamGenerationId"),
	}
}

func stringFixtureValue(payload map[string]any, key string) string {
	value, ok := payload[key].(string)
	if !ok {
		return ""
	}
	return value
}

func loadSyncPreflightRecoveryFixtureCatalog(t *testing.T) syncPreflightRecoveryFixtureCatalog {
	t.Helper()

	fixtureBytes, err := os.ReadFile("../testdata/sync-preflight-recovery-contract-fixtures.json")
	if err != nil {
		t.Fatalf("read sync preflight recovery contract fixtures: %v", err)
	}

	var catalog syncPreflightRecoveryFixtureCatalog
	if err := json.Unmarshal(fixtureBytes, &catalog); err != nil {
		t.Fatalf("decode sync preflight recovery contract fixtures: %v", err)
	}
	if len(catalog.Scenarios) == 0 {
		t.Fatal("sync preflight recovery contract fixtures contain no scenarios")
	}
	if len(catalog.IdentityScopeComparisons) == 0 {
		t.Fatal("sync preflight recovery contract fixtures contain no identity scope comparisons")
	}
	return catalog
}
