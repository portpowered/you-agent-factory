import { describe, expect, it } from "vitest";

import type { DashboardWorkItemRef } from "../../../../api/dashboard";
import type { FactoryEvent, FactoryWorkItem } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { reconstructFactoryReplayState } from "./buildSnapshot";
import dispatchWithoutInputContentFixture from "./fixtures/payload-lineage/dispatch-without-input-content.json";
import workRequestWithContentFixture from "./fixtures/payload-lineage/work-request-with-content.json";
import { projectRuntime } from "./projectRuntime";
import {
  consumedWorkItemRefsForDispatch,
  selectedWorkItemRefForID,
} from "./workItemRef";

interface PayloadLineageFixtureExpectedRef {
  work_id: string;
  payload_status: string;
  content?: DashboardWorkItemRef["content"];
  payload_unavailable_reason?: string;
  lineage_continuity?: string;
  lineage_source_kind?: string;
}

interface PayloadLineageGoldenFixture {
  provenance: {
    sessionId: string;
    capturedAt: string;
    description: string;
  };
  selectedTick: number;
  events: FactoryEvent[];
  expected: {
    selectedByWorkId?: Record<string, PayloadLineageFixtureExpectedRef>;
    placeOccupancyByWorkId?: Record<string, PayloadLineageFixtureExpectedRef>;
    consumedByDispatchId?: Record<
      string,
      Record<string, PayloadLineageFixtureExpectedRef>
    >;
  };
}

const eventTime = "2026-06-01T12:00:00.000Z";

function factoryEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    context: {
      eventTime,
      sequence: tick,
      tick,
      ...context,
    },
    id,
    payload,
    type,
  };
}

const initialStructureRequest = factoryEvent(
  "event-structure",
  0,
  FACTORY_EVENT_TYPES.initialStructureRequest,
  {
    factory: {
      workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
      workTypes: [
        {
          name: "task",
          states: [
            { name: "init", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "complete", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          id: "t-review",
          inputs: [{ state: "init", workType: "task" }],
          name: "Review",
          outputs: [{ state: "review", workType: "task" }],
          worker: "reviewer",
        },
        {
          id: "t-follow-up",
          inputs: [{ state: "review", workType: "task" }],
          name: "Follow Up",
          outputs: [{ state: "complete", workType: "task" }],
          worker: "reviewer",
        },
      ],
    },
  },
);

function lineageWorkItem(
  id: string,
  displayName: string,
  traceID: string,
  text: string,
): FactoryWorkItem {
  return {
    id,
    display_name: displayName,
    trace_id: traceID,
    work_type_id: "task",
    content: [{ type: "text", text }],
  };
}

function lineageWorkRequestEvent(
  tick: number,
  requestID: string,
  item: FactoryWorkItem,
): FactoryEvent {
  return factoryEvent(
    `event-work-${item.id}-${tick}`,
    tick,
    FACTORY_EVENT_TYPES.workRequest,
    {
      source: "external-submit",
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          content: item.content,
          name: item.display_name,
          traceId: item.trace_id,
          workId: item.id,
          workTypeName: item.work_type_id,
        },
      ],
    },
    {
      requestId: requestID,
      traceIds: item.trace_id ? [item.trace_id] : undefined,
      workIds: [item.id],
    },
  );
}

function lineageDispatchRequestEvent(
  tick: number,
  dispatchID: string,
  transitionID: string,
  item: FactoryWorkItem,
): FactoryEvent {
  return factoryEvent(
    `event-dispatch-request-${dispatchID}`,
    tick,
    FACTORY_EVENT_TYPES.dispatchRequest,
    {
      inputs: [
        {
          content: item.content,
          name: item.display_name,
          traceId: item.trace_id,
          workId: item.id,
          workTypeName: item.work_type_id,
        },
      ],
      resources: [],
      transitionId: transitionID,
    },
    {
      dispatchId: dispatchID,
      traceIds: item.trace_id ? [item.trace_id] : undefined,
      workIds: [item.id],
    },
  );
}

function lineageDispatchResponseEvent(
  tick: number,
  dispatchID: string,
  transitionID: string,
  traceIDs: string[],
  workIDs: string[],
  outputs: FactoryWorkItem[],
): FactoryEvent {
  return factoryEvent(
    `event-dispatch-response-${dispatchID}`,
    tick,
    FACTORY_EVENT_TYPES.dispatchResponse,
    {
      outcome: "ACCEPTED",
      outputWork: outputs.map((item) => ({
        content: item.content,
        name: item.display_name,
        traceId: item.trace_id,
        workId: item.id,
        workTypeName: item.work_type_id,
      })),
      transitionId: transitionID,
    },
    {
      dispatchId: dispatchID,
      traceIds: traceIDs,
      workIds: workIDs,
    },
  );
}

function expectRefMatchesExpected(
  actual: DashboardWorkItemRef | undefined,
  expected: PayloadLineageFixtureExpectedRef,
): void {
  expect(actual).toEqual(expect.objectContaining(expected));
}

function replayGoldenFixture(fixture: PayloadLineageGoldenFixture) {
  const state = reconstructFactoryReplayState(
    fixture.events,
    fixture.selectedTick,
  );
  const runtime = projectRuntime(state);
  return { runtime, state };
}

function findPlaceOccupancyRef(
  runtime: ReturnType<typeof projectRuntime>,
  state: ReturnType<typeof reconstructFactoryReplayState>,
  workID: string,
): DashboardWorkItemRef | undefined {
  const placeID = state.workItemsByID[workID]?.place_id;
  if (!placeID) {
    return undefined;
  }
  return runtime.current_work_items_by_place_id?.[placeID]?.find(
    (ref) => ref.work_id === workID,
  );
}

describe("replayWorldState payload lineage golden fixtures", () => {
  it("replays work-request-with-content and projects selected refs", () => {
    const fixture =
      workRequestWithContentFixture as PayloadLineageGoldenFixture;
    const { runtime, state } = replayGoldenFixture(fixture);

    for (const [workID, expected] of Object.entries(
      fixture.expected.selectedByWorkId ?? {},
    )) {
      const selected = selectedWorkItemRefForID(
        state.payloadLineage,
        workID,
        state.workItemsByID[workID],
      );
      expectRefMatchesExpected(selected, expected);
    }

    for (const [workID, expected] of Object.entries(
      fixture.expected.placeOccupancyByWorkId ?? {},
    )) {
      const placeRef = findPlaceOccupancyRef(runtime, state, workID);
      expectRefMatchesExpected(placeRef, expected);
    }
  });

  it("replays dispatch-without-input-content and projects consumed refs", () => {
    const fixture =
      dispatchWithoutInputContentFixture as PayloadLineageGoldenFixture;
    const { state } = replayGoldenFixture(fixture);

    for (const [dispatchID, consumedByWorkID] of Object.entries(
      fixture.expected.consumedByDispatchId ?? {},
    )) {
      const dispatch = state.activeDispatches[dispatchID];
      if (!dispatch) {
        throw new Error(`expected active dispatch ${dispatchID}`);
      }

      const consumedRefs = consumedWorkItemRefsForDispatch(
        state.payloadLineage,
        dispatchID,
        dispatch.consumedTokens,
        state.workItemsByID,
      );

      for (const [workID, expected] of Object.entries(consumedByWorkID)) {
        const consumedRef = consumedRefs.find((ref) => ref.work_id === workID);
        expectRefMatchesExpected(consumedRef, expected);
      }
    }

    for (const [workID, expected] of Object.entries(
      fixture.expected.selectedByWorkId ?? {},
    )) {
      const selected = selectedWorkItemRefForID(
        state.payloadLineage,
        workID,
        state.workItemsByID[workID],
      );
      expectRefMatchesExpected(selected, expected);
    }
  });
});

describe("replayWorldState payload lineage synthetic replay submit-only", () => {
  it("projects submit-only content into place occupancy", () => {
    const item = lineageWorkItem(
      "work-submit-only",
      "Submit only",
      "trace-1",
      "submit-only-v1",
    );
    const events: FactoryEvent[] = [
      initialStructureRequest,
      lineageWorkRequestEvent(1, "request/submit-only", item),
    ];

    const state = reconstructFactoryReplayState(events, 1);
    const runtime = projectRuntime(state);
    const placeRef = findPlaceOccupancyRef(runtime, state, item.id);

    expect(placeRef).toEqual(
      expect.objectContaining({
        content: [{ type: "text", text: "submit-only-v1" }],
        payload_status: "RESOLVED",
        work_id: "work-submit-only",
      }),
    );
  });
});

describe("replayWorldState payload lineage synthetic replay consumed unavailable", () => {
  it("marks consumed input unavailable when dispatch consumes work without a prior snapshot", () => {
    const events: FactoryEvent[] = [
      initialStructureRequest,
      factoryEvent(
        "event-dispatch-missing",
        1,
        FACTORY_EVENT_TYPES.dispatchRequest,
        {
          inputs: [{ workId: "work-missing" }],
          resources: [],
          transitionId: "t-review",
        },
        {
          dispatchId: "dispatch-missing",
          traceIds: ["trace-missing"],
          workIds: ["work-missing"],
        },
      ),
    ];

    const state = reconstructFactoryReplayState(events, 1);
    const dispatch = state.activeDispatches["dispatch-missing"];
    const consumedRefs = consumedWorkItemRefsForDispatch(
      state.payloadLineage,
      "dispatch-missing",
      dispatch.consumedTokens,
      state.workItemsByID,
    );

    expect(consumedRefs).toEqual([
      expect.objectContaining({
        payload_status: "UNAVAILABLE",
        payload_unavailable_reason:
          "no lineage snapshot was recorded before this dispatch consumed the work item",
        work_id: "work-missing",
      }),
    ]);
  });
});

describe("replayWorldState payload lineage synthetic replay dispatch output", () => {
  it("projects dispatch output work content into output refs", () => {
    const initial = lineageWorkItem("work-1", "Draft", "trace-1", "draft-v1");
    const continued = lineageWorkItem("work-1", "Draft", "trace-1", "draft-v2");
    const downstream = lineageWorkItem(
      "work-2",
      "Follow up",
      "trace-2",
      "follow-up-v1",
    );

    const events: FactoryEvent[] = [
      initialStructureRequest,
      lineageWorkRequestEvent(1, "request/work-1-v1", initial),
      lineageDispatchRequestEvent(2, "dispatch-1", "t-review", initial),
      lineageDispatchResponseEvent(
        3,
        "dispatch-1",
        "t-review",
        ["trace-1", "trace-2"],
        ["work-1", "work-2"],
        [continued, downstream],
      ),
    ];

    const state = reconstructFactoryReplayState(events, 3);
    const runtime = projectRuntime(state);
    const request = runtime.workstation_requests_by_dispatch_id?.["dispatch-1"];

    expect(request?.response?.output_work_items).toEqual([
      expect.objectContaining({
        content: [{ type: "text", text: "draft-v2" }],
        lineage_continuity: "SAME_WORK_ID_CONTINUATION",
        payload_status: "RESOLVED",
        work_id: "work-1",
      }),
      expect.objectContaining({
        content: [{ type: "text", text: "follow-up-v1" }],
        lineage_continuity: "NEW_DOWNSTREAM_WORK",
        payload_status: "RESOLVED",
        work_id: "work-2",
      }),
    ]);
  });
});

describe("replayWorldState payload lineage synthetic replay consumed pin", () => {
  it("keeps consumed-input pin unchanged when the same work ID is resubmitted later", () => {
    const initial = lineageWorkItem(
      "work-child",
      "Child",
      "trace-child",
      "child-v1",
    );
    const laterSelected = lineageWorkItem(
      "work-child",
      "Child",
      "trace-child",
      "child-v2",
    );

    const events: FactoryEvent[] = [
      initialStructureRequest,
      lineageWorkRequestEvent(1, "request/child-v1", initial),
      lineageDispatchRequestEvent(
        2,
        "dispatch-consume-child",
        "t-follow-up",
        initial,
      ),
      lineageWorkRequestEvent(3, "request/child-v2", laterSelected),
    ];

    const state = reconstructFactoryReplayState(events, 3);
    const dispatch = state.activeDispatches["dispatch-consume-child"];
    const consumedRefs = consumedWorkItemRefsForDispatch(
      state.payloadLineage,
      "dispatch-consume-child",
      dispatch.consumedTokens,
      state.workItemsByID,
    );
    const selected = selectedWorkItemRefForID(
      state.payloadLineage,
      "work-child",
      state.workItemsByID["work-child"],
    );

    expect(consumedRefs).toEqual([
      expect.objectContaining({
        content: [{ type: "text", text: "child-v1" }],
        payload_status: "RESOLVED",
        work_id: "work-child",
      }),
    ]);
    expect(selected).toEqual(
      expect.objectContaining({
        content: [{ type: "text", text: "child-v2" }],
        payload_status: "RESOLVED",
        work_id: "work-child",
      }),
    );
  });
});
