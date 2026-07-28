package automations_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const workRootImportPath = "github.com/portpowered/infinite-you/pkg/services/work"

// forbiddenWorkImportRoots are Work surfaces Automations production code must
// not depend on. Automations may import only the Work service root contract.
var forbiddenWorkImportRoots = []string{
	workRootImportPath + "/service",
	workRootImportPath + "/internal",
	"github.com/portpowered/infinite-you/pkg/work",
}

func forbiddenWorkImport(importPath string) bool {
	if importPath == workRootImportPath {
		return false
	}
	for _, forbidden := range forbiddenWorkImportRoots {
		if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
			return true
		}
	}
	workSubpackagePrefix := workRootImportPath + "/"
	if strings.HasPrefix(importPath, workSubpackagePrefix) {
		return true
	}
	legacyPrefix := "github.com/portpowered/infinite-you/pkg/work"
	return importPath == legacyPrefix || strings.HasPrefix(importPath, legacyPrefix+"/")
}

const automationsWorkRootAdmissionJSON = `{
  "requestId": "automations-work-root-admission",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "automations-work-root-admission",
      "workTypeName": "request",
      "state": "draft",
      "payload": {"source":"automations-work-root-admission"},
      "tags": {"source": "automations-work-root-admission"}
    }
  ]
}`

// TestAutomationsAdmitsWorkRootConstructedRequest proves Automations can admit
// Work Requests built exclusively through Work root construction helpers with
// assertable type, identity, tags, and payload fields at submitter handoff.
func TestAutomationsAdmitsWorkRootConstructedRequest(t *testing.T) {
	t.Parallel()

	parsed, err := work.ParseCanonicalWorkRequestJSON([]byte(automationsWorkRootAdmissionJSON))
	if err != nil {
		t.Fatalf("ParseCanonicalWorkRequestJSON: %v", err)
	}
	normalized, err := work.NormalizeWorkRequest(parsed, work.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"request": true},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	request := work.WorkRequestFromSubmitRequests(normalized)

	var submitCalls int
	var admitted work.WorkRequest
	submitter := automations.WorkRequestSubmitter(func(_ context.Context, req work.WorkRequest) error {
		submitCalls++
		admitted = req
		return nil
	})

	if err := submitter(context.Background(), request); err != nil {
		t.Fatalf("WorkRequestSubmitter handoff: %v", err)
	}
	if submitCalls != 1 {
		t.Fatalf("submitter calls = %d, want 1", submitCalls)
	}

	if admitted.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("admitted type = %q, want %q", admitted.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if admitted.RequestID != "automations-work-root-admission" {
		t.Fatalf("admitted request ID = %q, want %q", admitted.RequestID, "automations-work-root-admission")
	}
	if len(admitted.Works) != 1 {
		t.Fatalf("admitted works len = %d, want 1", len(admitted.Works))
	}

	got := admitted.Works[0]
	if got.WorkTypeID != "request" {
		t.Fatalf("admitted work type = %q, want %q", got.WorkTypeID, "request")
	}
	if got.Name != "automations-work-root-admission" {
		t.Fatalf("admitted work name = %q, want %q", got.Name, "automations-work-root-admission")
	}
	if got.State != "draft" {
		t.Fatalf("admitted work state = %q, want %q", got.State, "draft")
	}
	payload, ok := got.Payload.([]byte)
	if !ok {
		t.Fatalf("admitted payload type = %T, want []byte", got.Payload)
	}
	var payloadFields map[string]string
	if err := json.Unmarshal(payload, &payloadFields); err != nil {
		t.Fatalf("unmarshal admitted payload: %v", err)
	}
	if payloadFields["source"] != "automations-work-root-admission" {
		t.Fatalf("admitted payload source = %q, want %q", payloadFields["source"], "automations-work-root-admission")
	}
	if got.Tags["source"] != "automations-work-root-admission" {
		t.Fatalf("admitted tags = %#v, want source tag", got.Tags)
	}
}
