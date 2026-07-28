package current

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestAPIGetAndSaveCurrentFactoryWithinOneSession proves that a Factory Session
// can read its Current Factory, save a valid updated definition through the
// public session API, and read back the saved customer-visible topology within
// the same session.
func TestAPIGetAndSaveCurrentFactoryWithinOneSession(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	current := getCurrentFactory(t, server.URL())
	if current.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("current factory name = %q, want alpha", current.Name)
	}
	assertFactoryWorkType(t, current, "alpha-task", "initial current factory")

	saved := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		functionalNamedFactoryBody("alpha", "story", advancedFactoryVersion(t, current.Version)),
	)
	assertFactoryWorkType(t, saved, "story", "save response")

	reloaded := getCurrentFactory(t, server.URL())
	if reloaded.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("reloaded current factory name = %q, want alpha", reloaded.Name)
	}
	assertFactoryWorkType(t, reloaded, "story", "subsequent get within session")
}

// TestAPISaveCurrentFactoryValidatesBeforePersistence proves that an invalid Current
// Factory save is rejected through the public session API before persistence, returns
// a structured validation error, and leaves the prior Current Factory unchanged on
// subsequent readback within the same session.
func TestAPISaveCurrentFactoryValidatesBeforePersistence(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for save")
	}
	assertFactoryWorkType(t, current, "alpha-task", "initial current factory")

	advanced := advancedFactoryVersion(t, current.Version)
	body := `{
		"name":"alpha",
		"version":{"physical":"` + advanced.Physical.UTC().Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(advanced.Logical.Int64(), 10) + `"},
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
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("error family = %q, want BAD_REQUEST", errResp.Family)
	}
	if errResp.Targets == nil || len(*errResp.Targets) == 0 {
		t.Fatalf("error targets = %#v, want topology validation targets", errResp.Targets)
	}
	if !hasValidationTargetCode(*errResp.Targets, factoryValidationCodeDanglingPlaceReference) {
		t.Fatalf("error targets = %#v, want dangling place reference target", errResp.Targets)
	}

	reloaded := getCurrentFactory(t, server.URL())
	if reloaded.Version == nil || *reloaded.Version != *current.Version {
		t.Fatalf("reloaded version = %#v, want unchanged %#v", reloaded.Version, current.Version)
	}
	assertFactoryWorkType(t, reloaded, "alpha-task", "current factory after rejected save")
}
