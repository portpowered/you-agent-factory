package definitions

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCurrentFactoryPUT_ReturnsMultipleTopologyValidationTargets(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server, runner := startDefinitionsValidationServer(t, rootDir)
	defer server.Stop(t)

	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for save")
	}

	body := `{
		"name":"alpha",
		"version":{"physical":"` + current.Version.Physical.UTC().Add(time.Nanosecond).Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(current.Version.Logical.Int64()+1, 10) + `"},
		"workTypes":[{"name":"story","states":[
			{"name":"queued","type":"INITIAL"},
			{"name":"queued-dup","type":"PROCESSING"}
		]}],
		"workers":[
			{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"},
			{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}
		],
		"workstations":[{
			"name":"process",
			"behavior":"STANDARD",
			"type":"MODEL_WORKSTATION",
			"worker":"missing-worker",
			"inputs":[{"workType":"story","state":"queued"}],
			"outputs":[{"workType":"story","state":"missing-state"}]
		}]
	}`

	resp := saveCurrentFactoryDefinitionExpectStatus(t, server.URL(), body, http.StatusBadRequest)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode invalid current factory save response")
	if errResp.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("error code = %q, want INVALID_FACTORY", errResp.Code)
	}
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("error family = %q, want BAD_REQUEST", errResp.Family)
	}
	if errResp.Targets == nil || len(*errResp.Targets) < 2 {
		t.Fatalf("error targets = %#v, want multiple blocking validation targets", errResp.Targets)
	}
	if !hasValidationTargetCode(*errResp.Targets, validationCodeDuplicateIdentifier) ||
		!hasValidationTargetCode(*errResp.Targets, validationCodeDanglingWorkerReference) ||
		!hasValidationTargetCode(*errResp.Targets, validationCodeDanglingPlaceReference) {
		t.Fatalf("error targets = %#v, want duplicate worker, dangling worker, and dangling place targets", errResp.Targets)
	}
	assertValidationRunnerIdle(t, runner)
}

func TestCurrentFactoryPUT_ReturnsCanonicalTopologyValidationTargets(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server, runner := startDefinitionsValidationServer(t, rootDir)
	defer server.Stop(t)

	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for save")
	}

	body := `{
		"name":"alpha",
		"version":{"physical":"` + current.Version.Physical.UTC().Add(time.Nanosecond).Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(current.Version.Logical.Int64()+1, 10) + `"},
		"workTypes":[{"name":"story","states":[{"name":"queued","type":"INITIAL"}]}],
		"workers":[{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],
		"workstations":[{"name":"process","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"worker-a","inputs":[{"workType":"story","state":"queued"}],"outputs":[{"workType":"story","state":"missing-state"}]}]
	}`

	resp := saveCurrentFactoryDefinitionExpectStatus(t, server.URL(), body, http.StatusBadRequest)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode invalid current factory save response")
	if errResp.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("error code = %q, want INVALID_FACTORY", errResp.Code)
	}
	if errResp.Targets == nil || len(*errResp.Targets) == 0 {
		t.Fatalf("error targets = %#v, want canonical topology validation targets", errResp.Targets)
	}
	if !hasValidationTarget(
		*errResp.Targets,
		validationCodeDanglingPlaceReference,
		factoryapi.FactoryValidationSubjectTypeRoute,
		"process->story:missing-state",
		factoryapi.FactoryValidationSubjectLocationOutputs,
	) {
		t.Fatalf("error targets = %#v, want dangling output workstation target", errResp.Targets)
	}
	assertValidationRunnerIdle(t, runner)
}

func TestCurrentFactoryPUT_RejectsTypeCountCollisionBeforePersistingDefaultFactory(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server, runner := startDefinitionsValidationServer(t, rootDir)
	defer server.Stop(t)

	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for save")
	}
	body, err := json.Marshal(map[string]any{
		"name":             "UNDEFINED",
		"factoryDirectory": "factory",
		"sourceDirectory":  "factory",
		"version":          versionDocument(advancedFactoryVersion(t, current.Version)),
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "in-review", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":             "processor",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CLAUDE",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "claude-sonnet-4-20250514",
		}},
		"workstations": []map[string]any{{
			"name":     "process",
			"behavior": "REPEATER",
			"type":     "MODEL_WORKSTATION",
			"worker":   "processor",
			"inputs":   []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{
				{"workType": "task", "state": "in-review"},
				{"workType": "task", "state": "complete"},
			},
			"onContinue": []map[string]string{{"workType": "task", "state": "init"}},
			"onFailure":  []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal type-count collision save document: %v", err)
	}

	resp := saveCurrentFactoryDefinitionExpectStatus(t, server.URL(), string(body), http.StatusBadRequest)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode type-count collision save response")
	if errResp.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("error code = %q, want INVALID_FACTORY", errResp.Code)
	}
	if errResp.Targets == nil || !hasValidationTarget(
		*errResp.Targets,
		validationCodeConflictingWorkStateOutputs,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"process",
		factoryapi.FactoryValidationSubjectLocationOutputs,
	) {
		t.Fatalf("error targets = %#v, want conflicting output workstation target", errResp.Targets)
	}

	reloaded := getCurrentFactory(t, server.URL())
	if reloaded.Version == nil || *reloaded.Version != *current.Version {
		t.Fatalf("reloaded version = %#v, want unchanged %#v", reloaded.Version, current.Version)
	}
	assertFactoryWorkType(t, reloaded, "task", "reloaded factory after rejected type-count collision")
	assertValidationRunnerIdle(t, runner)
}

func TestFactoryTransformation_CreateNamedFactory_ReturnsBobOnFailureTarget(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server, runner := startDefinitionsValidationServer(t, rootDir)
	defer server.Stop(t)

	body := `{
		"name":"beta",
		"id":"beta",
		"workTypes":[{"name":"task","states":[
			{"name":"in-review","type":"PROCESSING"},
			{"name":"complete","type":"TERMINAL"}
		]}],
		"workers":[{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],
		"workstations":[{
			"name":"bob",
			"behavior":"REPEATER",
			"type":"MODEL_WORKSTATION",
			"worker":"worker-a",
			"inputs":[],
			"outputs":[{"workType":"task","state":"in-review"}],
			"onRejection":[{"workType":"task","state":"complete"}]
		}]
	}`

	resp := createNamedFactoryExpectStatus(t, server.URL(), body, http.StatusBadRequest)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode invalid named factory create response")
	if errResp.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("error code = %q, want INVALID_FACTORY", errResp.Code)
	}
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("error family = %q, want BAD_REQUEST", errResp.Family)
	}
	if errResp.Targets == nil || !hasValidationTarget(
		*errResp.Targets,
		validationCodeWorkstationMissingFailureRoute,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"bob",
		factoryapi.FactoryValidationSubjectLocationOnFailure,
	) {
		t.Fatalf("error targets = %#v, want bob ON_FAILURE target", errResp.Targets)
	}
	assertValidationRunnerIdle(t, runner)
}

func TestFactoryTransformation_CreateNamedFactory_ReturnsMultipleTopologyValidationTargets(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server, runner := startDefinitionsValidationServer(t, rootDir)
	defer server.Stop(t)

	body := `{
		"name":"beta",
		"id":"beta",
		"workTypes":[{"name":"story","states":[
			{"name":"queued","type":"INITIAL"},
			{"name":"queued-dup","type":"PROCESSING"}
		]}],
		"workers":[
			{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"},
			{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}
		],
		"workstations":[{
			"name":"process",
			"behavior":"STANDARD",
			"type":"MODEL_WORKSTATION",
			"worker":"missing-worker",
			"inputs":[{"workType":"story","state":"queued"}],
			"outputs":[{"workType":"story","state":"missing-state"}]
		}]
	}`

	resp := createNamedFactoryExpectStatus(t, server.URL(), body, http.StatusBadRequest)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode invalid named factory create response")
	if errResp.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("error code = %q, want INVALID_FACTORY", errResp.Code)
	}
	if errResp.Targets == nil || len(*errResp.Targets) < 2 {
		t.Fatalf("error targets = %#v, want multiple blocking validation targets", errResp.Targets)
	}
	if !hasValidationTargetCode(*errResp.Targets, validationCodeDuplicateIdentifier) ||
		!hasValidationTargetCode(*errResp.Targets, validationCodeDanglingWorkerReference) ||
		!hasValidationTargetCode(*errResp.Targets, validationCodeDanglingPlaceReference) {
		t.Fatalf("error targets = %#v, want duplicate worker, dangling worker, and dangling place targets", errResp.Targets)
	}
	assertValidationRunnerIdle(t, runner)
}
