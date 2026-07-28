package relationships

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	parentChildLineageRequestID = "request-parent-child-lineage"
	parentChildLineageParentID  = "parent-work-lineage-id"
	parentChildLineageChildID   = "child-work-lineage-id"
	parentChildLineageParent    = "parent"
	parentChildLineageChild     = "child"
	parentChildLineageWorkType  = "task"
)

// TestParentChildLineageSurvivesDispatchAndReplay proves through public CLI
// batch submission, Work inspection, provider dispatch observations, and
// retained Factory Event history that PARENT_CHILD lineage on a child work item
// remains observable after the child dispatches and after event-history
// reconstruction from the same session.
func TestParentChildLineageSurvivesDispatchAndReplay(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
	)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	defer server.Stop(t)

	baseURL := server.URL()
	binaryPath := buildParentChildCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	batchJSON := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workId":%q,"workTypeName":%q,"payload":{"role":"parent"}},{"name":%q,"workId":%q,"workTypeName":%q,"payload":{"role":"child"}}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":%q,"targetWorkName":%q}]}`,
		parentChildLineageRequestID,
		parentChildLineageParent,
		parentChildLineageParentID,
		parentChildLineageWorkType,
		parentChildLineageChild,
		parentChildLineageChildID,
		parentChildLineageWorkType,
		parentChildLineageChild,
		parentChildLineageParent,
	)
	submitOut, err := runParentChildCLI(ctx, binaryPath, dir, baseURL,
		"--json",
		"submit", "batch",
		batchJSON,
	)
	if err != nil {
		t.Fatalf("you submit batch: %v\noutput:\n%s", err, submitOut)
	}
	assertParentChildBatchSubmitAcknowledgment(t, submitOut, parentChildLineageRequestID)

	support.WaitForTerminalStatus(t, baseURL, 15*time.Second)

	listed := support.ListDefaultSessionWork(t, baseURL)
	assertParentChildLineageInWorkListing(t, listed, parentChildLineageChildID, parentChildLineageParentID)

	events := server.GetFactoryEvents(t)
	assertParentChildLineageInFactoryEvents(
		t,
		events,
		parentChildLineageRequestID,
		parentChildLineageChild,
		parentChildLineageParent,
		parentChildLineageParentID,
	)

	reconstructed := support.GetFactoryEventsAt(t, baseURL)
	if len(reconstructed) != len(events) {
		t.Fatalf(
			"reconstructed Factory Event count = %d, want %d from first retained-history read",
			len(reconstructed),
			len(events),
		)
	}
	assertParentChildLineageInFactoryEvents(
		t,
		reconstructed,
		parentChildLineageRequestID,
		parentChildLineageChild,
		parentChildLineageParent,
		parentChildLineageParentID,
	)

	assertParentChildLineageOnChildDispatch(t, provider, parentChildLineageChildID, parentChildLineageParentID)

	shown, err := runParentChildWorkShowCLIJSON(
		t,
		ctx,
		binaryPath,
		dir,
		baseURL,
		parentChildLineageChildID,
	)
	if err != nil {
		t.Fatalf("you work show %s: %v", parentChildLineageChildID, err)
	}
	assertParentChildRelationOnWork(t, shown, parentChildLineageParentID)
}

func assertParentChildBatchSubmitAcknowledgment(t *testing.T, output []byte, requestID string) {
	t.Helper()

	text := string(output)
	for _, marker := range []string{
		`"requestId":` + jsonStringLiteral(requestID),
		`"traceId":`,
		`"workCount":2`,
		parentChildLineageParent,
		parentChildLineageChild,
		parentChildLineageWorkType,
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("submit batch output missing %q:\n%s", marker, text)
		}
	}
}

func assertParentChildLineageInWorkListing(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	childWorkID, parentWorkID string,
) {
	t.Helper()

	child, ok := findListedWorkByID(listed, childWorkID)
	if !ok {
		t.Fatalf("public work list missing child %q: %#v", childWorkID, listed.Results)
	}
	assertParentChildRelationOnWork(t, child, parentWorkID)
}

func assertParentChildRelationOnWork(t *testing.T, item factoryapi.Work, parentWorkID string) {
	t.Helper()

	if item.Relations == nil || len(*item.Relations) == 0 {
		t.Fatalf("work %q missing relations in public listing/show: %#v", support.StringPointerValue(item.WorkId), item)
	}
	for _, relation := range *item.Relations {
		if relation.Type != factoryapi.RelationTypeParentChild {
			continue
		}
		if relation.TargetWorkId != nil && *relation.TargetWorkId == parentWorkID {
			return
		}
	}
	t.Fatalf(
		"work %q missing PARENT_CHILD relation to parent %q: relations=%#v",
		support.StringPointerValue(item.WorkId),
		parentWorkID,
		*item.Relations,
	)
}

func assertParentChildLineageInFactoryEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	requestID, childWorkName, parentWorkName, parentWorkID string,
) {
	t.Helper()

	foundWorkRequest := false
	foundRelationshipChange := false

	for _, event := range events {
		if support.StringPointerValue(event.Context.RequestId) != requestID {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			foundWorkRequest = true
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil {
				t.Fatalf("decode WORK_REQUEST event: %v", err)
			}
			if payload.Relations == nil {
				t.Fatalf("WORK_REQUEST payload missing relations: %#v", payload)
			}
			if !factoryEventRelationsIncludeParentChild(payload.Relations, childWorkName, parentWorkName) {
				t.Fatalf(
					"WORK_REQUEST relations = %#v, want PARENT_CHILD from %q to %q",
					payload.Relations,
					childWorkName,
					parentWorkName,
				)
			}
		case factoryapi.FactoryEventTypeRelationshipChangeRequest:
			foundRelationshipChange = true
			payload, err := event.Payload.AsRelationshipChangeRequestEventPayload()
			if err != nil {
				t.Fatalf("decode RELATIONSHIP_CHANGE_REQUEST event: %v", err)
			}
			if payload.Relation.Type != factoryapi.RelationTypeParentChild {
				t.Fatalf("relationship change type = %q, want PARENT_CHILD", payload.Relation.Type)
			}
			if payload.Relation.SourceWorkName != childWorkName || payload.Relation.TargetWorkName != parentWorkName {
				t.Fatalf(
					"relationship change = %#v, want %q PARENT_CHILD %q",
					payload.Relation,
					childWorkName,
					parentWorkName,
				)
			}
			if support.StringPointerValue(payload.Relation.TargetWorkId) != parentWorkID {
				t.Fatalf(
					"relationship target work id = %q, want %q",
					support.StringPointerValue(payload.Relation.TargetWorkId),
					parentWorkID,
				)
			}
		}
	}

	if !foundWorkRequest {
		t.Fatalf("Factory Event history missing WORK_REQUEST for request %q", requestID)
	}
	if !foundRelationshipChange {
		t.Fatalf("Factory Event history missing RELATIONSHIP_CHANGE_REQUEST for request %q", requestID)
	}
}

func factoryEventRelationsIncludeParentChild(
	relations *[]factoryapi.Relation,
	childWorkName, parentWorkName string,
) bool {
	if relations == nil {
		return false
	}
	for _, relation := range *relations {
		if relation.Type == factoryapi.RelationTypeParentChild &&
			relation.SourceWorkName == childWorkName &&
			relation.TargetWorkName == parentWorkName {
			return true
		}
	}
	return false
}

func assertParentChildLineageOnChildDispatch(
	t *testing.T,
	provider *testutil.MockProvider,
	childWorkID, parentWorkID string,
) {
	t.Helper()

	for _, call := range provider.Calls() {
		token := firstParentChildDispatchToken(call.InputTokens)
		if token.Color.WorkID != childWorkID {
			continue
		}
		for _, relation := range token.Color.Relations {
			if relation.Type == work.RelationParentChild && relation.TargetWorkID == parentWorkID {
				return
			}
		}
		t.Fatalf(
			"child dispatch token relations = %#v, want PARENT_CHILD target %q",
			token.Color.Relations,
			parentWorkID,
		)
	}
	t.Fatalf("provider dispatch history missing child work %q", childWorkID)
}

func firstParentChildDispatchToken(rawTokens any) workerexecution.Token {
	switch tokens := rawTokens.(type) {
	case []any:
		if len(tokens) == 0 {
			return workerexecution.Token{}
		}
		token, ok := tokens[0].(workerexecution.Token)
		if !ok {
			return workerexecution.Token{}
		}
		return token
	case []workerexecution.Token:
		if len(tokens) == 0 {
			return workerexecution.Token{}
		}
		return tokens[0]
	default:
		return workerexecution.Token{}
	}
}

func findListedWorkByID(listed factoryapi.ListWorkResponse, workID string) (factoryapi.Work, bool) {
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) == workID {
			return item, true
		}
	}
	return factoryapi.Work{}, false
}

func buildParentChildCLIBinary(t *testing.T) string {
	t.Helper()

	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/factory")
	build.Dir = testutil.MustRepoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build you CLI: %v\n%s", err, string(output))
	}
	return binaryPath
}

func runParentChildCLI(
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	args ...string,
) ([]byte, error) {
	cmdArgs := append([]string{"--server", serverURL}, args...)
	cmd := exec.CommandContext(ctx, binaryPath, cmdArgs...)
	cmd.Dir = workingDir
	return cmd.CombinedOutput()
}

func runParentChildWorkShowCLIJSON(
	t *testing.T,
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	workID string,
) (factoryapi.Work, error) {
	t.Helper()

	output, err := runParentChildCLI(ctx, binaryPath, workingDir, serverURL,
		"--json",
		"work", "show", workID,
	)
	if err != nil {
		return factoryapi.Work{}, err
	}
	var shown factoryapi.Work
	if err := json.Unmarshal(bytesTrimSpace(output), &shown); err != nil {
		t.Fatalf("decode work show JSON: %v\noutput:\n%s", err, output)
	}
	return shown, nil
}

func jsonStringLiteral(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func bytesTrimSpace(value []byte) []byte {
	return []byte(strings.TrimSpace(string(value)))
}
