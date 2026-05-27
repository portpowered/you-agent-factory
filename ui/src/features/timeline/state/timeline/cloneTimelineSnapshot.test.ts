import { expect, it } from "vitest";

import type {
  DashboardTrace,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard";
import {
  cloneFailedWorkDetailsByWorkID,
  cloneInferenceAttemptsByDispatchID,
  cloneProviderSessionAttempts,
  cloneRelationsByWorkID,
  cloneTracesByWorkID,
  cloneWorkItemRef,
  cloneWorkRequestsByID,
  cloneWorkstationDispatchRequestsByID,
  uniqueSortedWorkRefs,
} from "./cloneTimelineSnapshot";

it("deduplicates and sorts work refs while ignoring empty work IDs", () => {
  expect(
    uniqueSortedWorkRefs([
      { work_id: "work-b", work_type_id: "task" },
      { work_id: "", work_type_id: "task" },
      { display_name: "latest", work_id: "work-a", work_type_id: "task" },
      { display_name: "replacement", work_id: "work-a", work_type_id: "task" },
    ]),
  ).toEqual([
    { display_name: "replacement", work_id: "work-a", work_type_id: "task" },
    { work_id: "work-b", work_type_id: "task" },
  ]);
});

it("clones trace, relation, work request, and failure payloads", () => {
  const trace: DashboardTrace = {
    dispatches: [
      {
        consumed_tokens: [
          {
            id: "token-1",
            place_id: "story:ready",
            tags: { priority: "high" },
          },
        ],
        input_items: [
          {
            previous_chaining_trace_ids: ["trace-0"],
            work_id: "work-input",
            work_type_id: "story",
          },
        ],
        output_items: [{ work_id: "work-output", work_type_id: "story" }],
        output_mutations: [
          {
            mutation_type: "TOKEN_CREATED",
            resulting_token: {
              id: "token-2",
              place_id: "story:ready",
              tags: { source: "result" },
            },
          },
        ],
        previous_chaining_trace_ids: ["trace-previous"],
        provider_session: { provider_session_id: "session-1" },
        request_id: "request-1",
        token_names: ["story-ready"],
        trace_ids: ["trace-1"],
        transition_id: "write-story",
        work_ids: ["work-input"],
        work_types: ["story"],
      },
    ],
    relations: [
      {
        child_id: "work-child",
        parent_id: "work-parent",
        relation: "created_by",
      },
    ],
    request_ids: ["request-1"],
    transition_ids: ["write-story"],
    work_ids: ["work-input"],
    work_items: [
      {
        previous_chaining_trace_ids: ["trace-0"],
        work_id: "work-input",
        work_type_id: "story",
      },
    ],
    workstation_sequence: ["write-story"],
  };

  const traces = cloneTracesByWorkID({ "work-input": trace });
  const relations = cloneRelationsByWorkID({
    "work-child": [{ child_id: "work-child", parent_id: "work-parent" }],
  });
  const requests = cloneWorkRequestsByID({
    "request-1": {
      request_id: "request-1",
      work_items: [
        {
          id: "work-input",
          tags: { owner: "factory" },
          work_type_id: "story",
        },
      ],
    },
  });
  const failures = cloneFailedWorkDetailsByWorkID({
    "work-input": {
      dispatch_id: "dispatch-1",
      failure_message: "failed",
      failure_reason: "script_error",
      transition_id: "write-story",
      work_item: {
        previous_chaining_trace_ids: ["trace-0"],
        work_id: "work-input",
        work_type_id: "story",
      },
      workstation_name: "Write story",
    },
  });

  const consumedTokenTags = trace.dispatches[0].consumed_tokens?.[0].tags;
  if (consumedTokenTags) {
    consumedTokenTags.priority = "low";
  }
  const mutationTokenTags =
    trace.dispatches[0].output_mutations?.[0].resulting_token?.tags;
  if (mutationTokenTags) {
    mutationTokenTags.source = "mutated";
  }

  expect(
    traces["work-input"].dispatches[0].consumed_tokens?.[0].tags?.priority,
  ).toBe("high");
  expect(
    traces["work-input"].dispatches[0].output_mutations?.[0].resulting_token
      ?.tags?.source,
  ).toBe("result");
  expect(relations["work-child"][0]).toEqual({
    child_id: "work-child",
    parent_id: "work-parent",
  });
  expect(requests["request-1"].work_items?.[0].tags).toEqual({
    owner: "factory",
  });
  expect(
    failures?.["work-input"].work_item.previous_chaining_trace_ids,
  ).toEqual(["trace-0"]);
});

it("clones nested workstation request views without sharing mutable arrays", () => {
  const request: DashboardWorkstationRequest = {
    counts: { dispatched_count: 1 },
    dispatch_id: "dispatch-1",
    dispatched_request_count: 1,
    errored_request_count: 0,
    inference_attempts: [{ request_id: "request-1" }],
    request_metadata: { source: "runtime" },
    request_view: {
      consumedTokens: [
        {
          id: "token-1",
          place_id: "story:ready",
          tags: { priority: "high" },
        },
      ],
      inputWorkItems: [
        {
          previous_chaining_trace_ids: ["trace-previous"],
          work_id: "work-input",
          work_type_id: "story",
        },
      ],
      inputWorkTypeIds: ["story"],
      traceIds: ["trace-1"],
    },
    responded_request_count: 1,
    response_metadata: { status: "accepted" },
    response_view: {
      outputMutations: [
        {
          mutation_type: "TOKEN_CREATED",
          resulting_token: {
            id: "token-2",
            place_id: "story:done",
            tags: { status: "done" },
          },
        },
      ],
      outputWorkItems: [{ work_id: "work-output", work_type_id: "story" }],
    },
    script_request: { args: ["--dry-run"] },
    script_response: { exit_code: 0 },
    trace_ids: ["trace-1"],
    transition_id: "write-story",
    work_items: [
      {
        previous_chaining_trace_ids: ["trace-previous"],
        work_id: "work-input",
        work_type_id: "story",
      },
    ],
    workstation_node_id: "write-story",
  };

  const cloned = cloneWorkstationDispatchRequestsByID({
    "dispatch-1": request,
    "dispatch-empty": {
      dispatch_id: "dispatch-empty",
      dispatched_request_count: 0,
      errored_request_count: 0,
      inference_attempts: [],
      responded_request_count: 0,
      transition_id: "idle",
      work_items: [],
      workstation_node_id: "idle",
    },
  });

  const requestTokenTags = request.request_view?.consumedTokens?.[0].tags;
  if (requestTokenTags) {
    requestTokenTags.priority = "low";
  }
  const responseTokenTags =
    request.response_view?.outputMutations?.[0].resulting_token?.tags;
  if (responseTokenTags) {
    responseTokenTags.status = "mutated";
  }
  request.script_request?.args?.push("--changed");

  expect(
    cloned["dispatch-1"].request_view?.consumedTokens?.[0].tags?.priority,
  ).toBe("high");
  expect(
    cloned["dispatch-1"].response_view?.outputMutations?.[0].resulting_token
      ?.tags?.status,
  ).toBe("done");
  expect(cloned["dispatch-1"].script_request?.args).toEqual(["--dry-run"]);
  expect(cloned["dispatch-empty"].request_view).toBeUndefined();
  expect(cloned["dispatch-empty"].response_view).toBeUndefined();
});

it("clones provider and inference diagnostics metadata", () => {
  const providerSessions = cloneProviderSessionAttempts([
    {
      diagnostics: {
        provider: {
          request_metadata: { prompt: "original" },
          response_metadata: { model: "example" },
        },
        rendered_prompt: { variables: { story: "PRD" } },
      },
      provider_session: { provider_session_id: "session-1" },
      work_items: [{ work_id: "work-1", work_type_id: "story" }],
    },
  ]);
  const attempts = cloneInferenceAttemptsByDispatchID({
    "dispatch-1": {
      "request-1": {
        diagnostics: {
          provider: {
            request_metadata: { prompt: "original" },
            response_metadata: { model: "example" },
          },
          rendered_prompt: { variables: { story: "PRD" } },
        },
        provider_session: { provider_session_id: "session-1" },
        request_id: "request-1",
      },
    },
  });

  expect(providerSessions[0].diagnostics?.provider?.request_metadata).toEqual({
    prompt: "original",
  });
  expect(providerSessions[0].diagnostics?.rendered_prompt?.variables).toEqual({
    story: "PRD",
  });
  expect(
    attempts?.["dispatch-1"]["request-1"].diagnostics?.provider
      ?.response_metadata,
  ).toEqual({ model: "example" });
});

it("returns undefined for empty optional clone maps", () => {
  expect(cloneFailedWorkDetailsByWorkID({})).toBeUndefined();
  expect(cloneInferenceAttemptsByDispatchID({})).toBeUndefined();
  expect(
    cloneWorkItemRef({
      previous_chaining_trace_ids: ["trace-1"],
      work_id: "work-1",
      work_type_id: "story",
    }).previous_chaining_trace_ids,
  ).toEqual(["trace-1"]);
});
