package root_composition_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	routingActivationRouterWorkstation = "router"

	relationshipActivationRequiredState = "complete"
	relationshipActivationStartWS       = "start"
	relationshipActivationFinishWS      = "finish"

	relationshipActivationPrerequisiteID = "fun-work-prerequisite-a"
	relationshipActivationDependentID    = "fun-work-dependent-b"

	parentChildActivationRequestID = "fun-work-parent-child-request"
	parentChildActivationParentID  = "fun-work-parent-id"
	parentChildActivationChildID   = "fun-work-child-id"
	parentChildActivationParent    = "fun-work-parent"
	parentChildActivationChild     = "fun-work-child"
	parentChildActivationWorkType  = "task"
)

// TestWorkRoutingActivatesThroughRootBuildProcessAfterLifecycle proves logical-move
// routing advances Work to an observable public outcome after runtime lifecycle on a
// process constructed only through root.BuildProcess with edges.Edges effect
// replacement. Detailed classifier and logical-move coverage remains under
// tests/functional/work/routing; this test closes the explicit public-process
// activation gap.
func TestWorkRoutingActivatesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "logical_move_dir"))
	configureRoutingActivationLogicalMoveWorkstation(t, dir, routingActivationRouterWorkstation)
	testutil.WriteSeedFile(t, dir, "task", []byte("fun-work-logical-move-payload"))

	provider := testutil.NewMockProvider()
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	if provider.CallCount() != 0 {
		t.Fatalf(
			"provider call count = %d after logical-move routing, want 0 workerless routing",
			provider.CallCount(),
		)
	}
	doneLocation := support.WorkCustomerLocation("task", "done")
	if got := support.CountWorkAtCustomerState(listed, doneLocation); got != 1 {
		t.Fatalf("%s work count = %d, want 1 after logical move; listed=%#v", doneLocation, got, listed)
	}
	initLocation := support.WorkCustomerLocation("task", "init")
	if got := support.CountWorkAtCustomerState(listed, initLocation); got != 0 {
		t.Fatalf("%s work count = %d, want 0 after logical move; listed=%#v", initLocation, got, listed)
	}

	dispatches := support.ObserveDispatchEvents(t, events)
	logicalMoveDispatches := filterRoutingActivationWorkstationDispatches(
		dispatches,
		routingActivationRouterWorkstation,
	)
	if len(logicalMoveDispatches) == 0 {
		t.Fatalf("no dispatch events at logical-move workstation %q", routingActivationRouterWorkstation)
	}
	for _, dispatch := range logicalMoveDispatches {
		if dispatch.Response == nil {
			continue
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf(
				"logical-move dispatch %q outcome = %s, want ACCEPTED",
				dispatch.DispatchID,
				dispatch.Response.Outcome,
			)
		}
		if dispatch.Response.ProviderFailure != nil {
			t.Fatalf(
				"logical-move dispatch %q providerFailure = %#v, want no provider invocation",
				dispatch.DispatchID,
				dispatch.Response.ProviderFailure,
			)
		}
	}
}

// TestWorkRelationshipsActivateThroughRootBuildProcessAfterLifecycle proves
// DEPENDS_ON and PARENT_CHILD relationship outcomes are observable through public
// Work surfaces after runtime lifecycle on processes built only via root.BuildProcess
// and edges.Edges. Detailed relationship coverage remains under
// tests/functional/work/relationships; this test closes the explicit public-process
// activation gap.
func TestWorkRelationshipsActivateThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("depends_on", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			WorkID:     relationshipActivationPrerequisiteID,
			Payload:    []byte("fun-work prerequisite"),
		})
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			WorkID:     relationshipActivationDependentID,
			Payload:    []byte("fun-work dependent"),
			Relations: []work.Relation{
				{
					Type:          work.RelationDependsOn,
					TargetWorkID:  relationshipActivationPrerequisiteID,
					RequiredState: relationshipActivationRequiredState,
				},
			},
		})

		runner := testutil.NewProviderCommandRunner(
			relationshipActivationProviderSuccess(),
			relationshipActivationProviderSuccess(),
			relationshipActivationProviderSuccess(),
			relationshipActivationProviderSuccess(),
		)

		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
			15*time.Second,
		)

		completeLocation := support.WorkCustomerLocation("task", relationshipActivationRequiredState)
		if !support.HasWorkAtCustomerState(listed, relationshipActivationPrerequisiteID, completeLocation) {
			t.Fatalf(
				"prerequisite %q not at %q in public listing: %#v",
				relationshipActivationPrerequisiteID,
				relationshipActivationRequiredState,
				listed,
			)
		}
		if !support.HasWorkAtCustomerState(listed, relationshipActivationDependentID, completeLocation) {
			t.Fatalf(
				"dependent %q not at %q in public listing: %#v",
				relationshipActivationDependentID,
				relationshipActivationRequiredState,
				listed,
			)
		}
		if runner.CallCount() != 4 {
			t.Fatalf(
				"provider command runner calls = %d, want 4 starter and finisher invocations",
				runner.CallCount(),
			)
		}

		prerequisiteCompleteSequence, dependentStartSequence := relationshipActivationDependsOnDispatchOrdering(
			t,
			events,
			relationshipActivationPrerequisiteID,
			relationshipActivationDependentID,
		)
		if dependentStartSequence <= prerequisiteCompleteSequence {
			t.Fatalf(
				"dependent %q dispatch sequence = %d, want after prerequisite %q complete sequence %d",
				relationshipActivationDependentID,
				dependentStartSequence,
				relationshipActivationPrerequisiteID,
				prerequisiteCompleteSequence,
			)
		}

		if session.Runtime.Progress.Categories.Terminal != 2 || session.Runtime.Progress.Categories.Failed != 0 {
			t.Fatalf(
				"session progress categories = %+v, want two terminal and zero failed",
				session.Runtime.Progress.Categories,
			)
		}
	})

	t.Run("parent_child", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "COMPLETE"},
			workerexecution.InferenceResponse{Content: "COMPLETE"},
			workerexecution.InferenceResponse{Content: "COMPLETE"},
			workerexecution.InferenceResponse{Content: "COMPLETE"},
		)
		edges := serviceedges.Edges{ProviderOverride: provider}

		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                dir,
			WaitForServiceModeRuntime: true,
			Edges:                     edges,
		})
		t.Cleanup(func() { server.Stop(t) })

		baseURL := server.URL()
		process := support.BuildProcess(t, edges)

		batchOutput := executeRelationshipActivationBatchSubmitCLI(
			t,
			process,
			baseURL,
			relationshipActivationParentChildBatchJSON(),
		)
		decodeRelationshipActivationBatchSubmitJSON(t, batchOutput, parentChildActivationRequestID)

		support.WaitForTerminalStatus(t, baseURL, 15*time.Second)

		listed := support.ListDefaultSessionWork(t, baseURL)
		assertRelationshipActivationParentChildInListing(
			t,
			listed,
			parentChildActivationChildID,
			parentChildActivationParentID,
		)

		events := server.GetFactoryEvents(t)
		assertRelationshipActivationParentChildInFactoryEvents(
			t,
			events,
			parentChildActivationRequestID,
			parentChildActivationChild,
			parentChildActivationParent,
			parentChildActivationParentID,
		)

		assertRelationshipActivationParentChildOnChildDispatch(
			t,
			provider,
			parentChildActivationChildID,
			parentChildActivationParentID,
		)
	})
}

func configureRoutingActivationLogicalMoveWorkstation(t *testing.T, dir, workstationName string) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode factory config: %v", err)
	}
	for _, raw := range config["workstations"].([]any) {
		workstation := raw.(map[string]any)
		if workstation["name"] == workstationName {
			workstation["type"] = "LOGICAL_MOVE"
			workstation["worker"] = ""
		}
	}
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("encode factory config: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory config: %v", err)
	}

	workstationConfigPath := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.WriteFile(workstationConfigPath, []byte("---\ntype: LOGICAL_MOVE\n---\n"), 0o644); err != nil {
		t.Fatalf("write logical workstation config: %v", err)
	}
}

func filterRoutingActivationWorkstationDispatches(
	dispatches []support.DispatchEventObservation,
	workstation string,
) []support.DispatchEventObservation {
	filtered := make([]support.DispatchEventObservation, 0, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != workstation {
			continue
		}
		filtered = append(filtered, dispatch)
	}
	return filtered
}

func relationshipActivationProviderSuccess() platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("COMPLETE")}
}

func relationshipActivationDependsOnDispatchOrdering(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	prerequisiteWorkID, dependentWorkID string,
) (prerequisiteCompleteSequence, dependentStartSequence int) {
	t.Helper()

	prerequisiteCompleteSequence = -1
	dependentStartSequence = -1

	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				continue
			}
			if payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.TransitionId != relationshipActivationFinishWS {
				continue
			}
			if !relationshipActivationDispatchEventIncludesWork(event.Context.WorkIds, prerequisiteWorkID) {
				continue
			}
			prerequisiteCompleteSequence = event.Context.Sequence
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				continue
			}
			if payload.TransitionId != relationshipActivationStartWS {
				continue
			}
			if !relationshipActivationDispatchRequestIncludesWork(payload, dependentWorkID) {
				continue
			}
			if prerequisiteCompleteSequence < 0 {
				t.Fatalf(
					"dependent %q received %q dispatch before prerequisite %q reached %q",
					dependentWorkID,
					relationshipActivationStartWS,
					prerequisiteWorkID,
					relationshipActivationRequiredState,
				)
			}
			if dependentStartSequence < 0 {
				dependentStartSequence = event.Context.Sequence
			}
		}
	}

	if prerequisiteCompleteSequence < 0 {
		t.Fatalf(
			"prerequisite %q never reached %q through public dispatch",
			prerequisiteWorkID,
			relationshipActivationRequiredState,
		)
	}
	if dependentStartSequence < 0 {
		t.Fatalf(
			"dependent %q never received public %q dispatch",
			dependentWorkID,
			relationshipActivationStartWS,
		)
	}
	return prerequisiteCompleteSequence, dependentStartSequence
}

func relationshipActivationDispatchRequestIncludesWork(
	payload factoryapi.DispatchRequestEventPayload,
	workID string,
) bool {
	for _, input := range payload.Inputs {
		if input.WorkId == workID {
			return true
		}
	}
	return false
}

func relationshipActivationDispatchEventIncludesWork(workIDs *[]string, workID string) bool {
	if workIDs == nil {
		return false
	}
	for _, candidate := range *workIDs {
		if candidate == workID {
			return true
		}
	}
	return false
}

func executeRelationshipActivationBatchSubmitCLI(
	t *testing.T,
	process support.Process,
	serverURL string,
	batchJSON string,
) string {
	t.Helper()

	home := t.TempDir()
	args := []string{
		"you", "--server", serverURL, "--json",
		"submit", "batch", batchJSON,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = relationshipActivationHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(batch submit) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs.Stdout()
}

type relationshipActivationBatchSubmitJSON struct {
	RequestID string `json:"requestId"`
	TraceID   string `json:"traceId"`
	WorkCount int    `json:"workCount"`
}

func decodeRelationshipActivationBatchSubmitJSON(
	t *testing.T,
	output string,
	requestID string,
) relationshipActivationBatchSubmitJSON {
	t.Helper()

	var submitted relationshipActivationBatchSubmitJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &submitted); err != nil {
		t.Fatalf("decode batch submit JSON: %v\noutput:\n%s", err, output)
	}
	if submitted.RequestID != requestID {
		t.Fatalf("batch submit requestId = %q, want %q", submitted.RequestID, requestID)
	}
	if strings.TrimSpace(submitted.TraceID) == "" || submitted.WorkCount != 2 {
		t.Fatalf("batch submit response missing trace or work count: %#v", submitted)
	}
	return submitted
}

func relationshipActivationParentChildBatchJSON() string {
	return `{
		"requestId": "` + parentChildActivationRequestID + `",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{
				"name": "` + parentChildActivationParent + `",
				"workId": "` + parentChildActivationParentID + `",
				"workTypeName": "` + parentChildActivationWorkType + `",
				"payload": {"role": "parent"}
			},
			{
				"name": "` + parentChildActivationChild + `",
				"workId": "` + parentChildActivationChildID + `",
				"workTypeName": "` + parentChildActivationWorkType + `",
				"payload": {"role": "child"}
			}
		],
		"relations": [
			{
				"type": "PARENT_CHILD",
				"sourceWorkName": "` + parentChildActivationChild + `",
				"targetWorkName": "` + parentChildActivationParent + `"
			}
		]
	}`
}

func relationshipActivationHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}

func assertRelationshipActivationParentChildInListing(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	childWorkID, parentWorkID string,
) {
	t.Helper()

	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != childWorkID {
			continue
		}
		assertRelationshipActivationParentChildRelationOnWork(t, item, parentWorkID)
		return
	}
	t.Fatalf("public work list missing child %q: %#v", childWorkID, listed.Results)
}

func assertRelationshipActivationParentChildRelationOnWork(
	t *testing.T,
	item factoryapi.Work,
	parentWorkID string,
) {
	t.Helper()

	if item.Relations == nil || len(*item.Relations) == 0 {
		t.Fatalf(
			"work %q missing relations in public listing: %#v",
			support.StringPointerValue(item.WorkId),
			item,
		)
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

func assertRelationshipActivationParentChildInFactoryEvents(
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
			if !relationshipActivationFactoryEventRelationsIncludeParentChild(
				payload.Relations,
				childWorkName,
				parentWorkName,
			) {
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

func relationshipActivationFactoryEventRelationsIncludeParentChild(
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

func assertRelationshipActivationParentChildOnChildDispatch(
	t *testing.T,
	provider *testutil.MockProvider,
	childWorkID, parentWorkID string,
) {
	t.Helper()

	for _, call := range provider.Calls() {
		token := relationshipActivationFirstDispatchToken(call.InputTokens)
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

func relationshipActivationFirstDispatchToken(rawTokens any) workerexecution.Token {
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
