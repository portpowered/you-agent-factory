package factorysessionsse

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type factorySessionSSEBehaviorSuiteCase struct {
	suiteName      string
	testFile       string
	testNames      []string
	proseFragments []string
}

var factorySessionSSEBehaviorSuiteCases = []factorySessionSSEBehaviorSuiteCase{
	{
		suiteName: "fixture harness",
		testFile:  "fixtures_test.go",
		testNames: []string{
			"TestFactorySessionSSEFixture_RetainedHistoryIsStableAndOrdered",
			"TestFactorySessionSSEHarness_ReadsRetainedThenLiveEventsWithinTimeout",
			"TestFactorySessionSSEHarness_FailsClosedWhenTimeoutElapses",
			"TestFactorySessionSSEHarness_DecodesPublicFactoryEventRecords",
		},
		proseFragments: []string{
			"ascending tick",
			"text/event-stream",
			"retained history",
		},
	},
	{
		suiteName: "initial stream",
		testFile:  "initial_stream_test.go",
		testNames: []string{
			"TestFactorySessionSSEInitialStream_NoReconnectCursorReturnsEventStream",
			"TestFactorySessionSSEInitialStream_DeliversRetainedHistoryAsValidFactoryEventsInOrder",
			"TestFactorySessionSSEInitialStream_ContinuesWithLiveEventWithoutRetainedReplay",
			"TestFactorySessionSSEInitialStream_WritesSessionIdentityHandshakeHeaders",
		},
		proseFragments: []string{
			"ascending tick",
			"text/event-stream",
			"live factoryevent",
			"x-factory-session-backend-scope-id",
			"x-factory-session-stream-generation-id",
		},
	},
	{
		suiteName: "valid reconnect",
		testFile:  "reconnect_test.go",
		testNames: []string{
			"TestFactorySessionSSEReconnect_AfterEventIDSkipsAcknowledgedRetainedHistory",
			"TestFactorySessionSSEReconnect_AfterSequenceSkipsAcknowledgedRetainedHistory",
			"TestFactorySessionSSEReconnect_SecondConnectFromSameCursorYieldsDeterministicSuffix",
			"TestFactorySessionSSEReconnect_KeepsEventStreamFramingAndTargetSession",
		},
		proseFragments: []string{
			"after_event_id",
			"after_sequence",
			"sessionsequence",
			"reconnect",
		},
	},
	{
		suiteName: "invalid and expired cursor",
		testFile:  "invalid_cursor_test.go",
		testNames: []string{
			"TestFactorySessionSSEInvalidCursor_UnknownAfterEventIDReturnsTypedErrorNotFullHistory",
			"TestFactorySessionSSEInvalidCursor_UnknownAfterSequenceReturnsTypedErrorNotFullHistory",
			"TestFactorySessionSSEInvalidCursor_JSONProbeClassifiesStaleCursorWithOmitGuidance",
			"TestFactorySessionSSEInvalidCursor_JSONProbeValidCursorReturnsStreamReady",
		},
		proseFragments: []string{
			"replay bound",
			"400 on",
			"cursor_stale",
			"omitaftereventid",
			"omitaftersequence",
		},
	},
	{
		suiteName: "idle keepalive",
		testFile:  "keepalive_test.go",
		testNames: []string{
			"TestFactorySessionSSEKeepalive_UsesConnectionKeepAliveHeader",
			"TestFactorySessionSSEKeepalive_IdleStreamEmitsKeepaliveWithinBoundedTimeout",
			"TestFactorySessionSSEKeepalive_IdleKeepaliveDoesNotSerializeFactoryEventKinds",
			"TestFactorySessionSSEKeepalive_DeliversLiveEventAfterKeepaliveObserved",
		},
		proseFragments: []string{
			"keepalive",
			"connection keep-alive",
			"idle",
		},
	},
	{
		suiteName: "unknown session",
		testFile:  "unknown_session_test.go",
		testNames: []string{
			"TestFactorySessionSSEUnknownSession_ReturnsTypedNotFoundWithinBoundedTimeout",
			"TestFactorySessionSSEUnknownSession_NeverFallsBackToDefaultOrOtherSession",
			"TestFactorySessionSSEUnknownSession_JSONProbeClassifiesUnknownSessionDistinctFromCursorStale",
		},
		proseFragments: []string{
			"not_found",
			"unknown_session",
			"never falls back to the default session",
		},
	},
}

func TestFactorySessionSSEBehaviorSuite_AlignsWithAuthoredOperationMetadata(t *testing.T) {
	description := loadFactorySessionSSEOperationDescription(t)
	descriptionLower := strings.ToLower(description)

	if docID := loadFactorySessionSSEOperationDocID(t); docID != "agent-factory/api/factory-session-events" {
		t.Fatalf("x-doc-id = %q, want agent-factory/api/factory-session-events", docID)
	}

	for _, suiteCase := range factorySessionSSEBehaviorSuiteCases {
		t.Run(suiteCase.suiteName, func(t *testing.T) {
			testSource := readFactorySessionSSETestFile(t, suiteCase.testFile)
			for _, testName := range suiteCase.testNames {
				if !strings.Contains(testSource, "func "+testName) {
					t.Fatalf("test file %s missing %s", suiteCase.testFile, testName)
				}
			}
			for _, fragment := range suiteCase.proseFragments {
				if !strings.Contains(descriptionLower, strings.ToLower(fragment)) {
					t.Fatalf(
						"authored getEventsBySessionId description missing prose fragment %q required by %s",
						fragment,
						suiteCase.testFile,
					)
				}
			}
		})
	}
}

func loadFactorySessionSSEOperationDescription(t *testing.T) string {
	t.Helper()

	operation := loadFactorySessionSSEOperation(t)
	description, _ := operation["description"].(string)
	if strings.TrimSpace(description) == "" {
		t.Fatal("paths./factory-sessions/{session_id}/events.get.description is empty")
	}
	return description
}

func loadFactorySessionSSEOperationDocID(t *testing.T) string {
	t.Helper()

	operation := loadFactorySessionSSEOperation(t)
	docID, _ := operation["x-doc-id"].(string)
	return docID
}

func loadFactorySessionSSEOperation(t *testing.T) map[string]any {
	t.Helper()

	doc := loadBundledFactorySessionSSEOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("openapi paths object is missing")
	}
	eventsPath, ok := paths["/factory-sessions/{session_id}/events"].(map[string]any)
	if !ok {
		t.Fatal("openapi path /factory-sessions/{session_id}/events is missing")
	}
	operation, ok := eventsPath["get"].(map[string]any)
	if !ok {
		t.Fatal("openapi operation getEventsBySessionId is missing")
	}
	return operation
}

func loadBundledFactorySessionSSEOpenAPIDocument(t *testing.T) map[string]any {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve servertests directory")
	}
	openAPIPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "..", "..", "api", "openapi.yaml")
	data, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}
	return doc
}

func readFactorySessionSSETestFile(t *testing.T, name string) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve servertests directory")
	}
	testPath := filepath.Join(filepath.Dir(filename), name)
	data, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
