import { describe, expect, it } from "vitest";

import type { FactoryEvent, FactoryWorkItem } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { reconstructFactoryReplayState } from "./buildSnapshot";
import {
  resolveConsumedInputSnapshot,
  resolveInitialSubmittedSnapshot,
  resolveOutputWorkSnapshot,
  resolveSelectedWorkSnapshot,
} from "./workPayloadLineage";

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
      workers: [
        {
          model: "gpt-5.4",
          modelProvider: "openai",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [
        {
          name: "task",
          states: [
            { name: "init", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "complete", type: "TERMINAL" },
            { name: "failed", type: "FAILED" },
          ],
        },
      ],
      workstations: [
        {
          id: "t-review",
          inputs: [{ state: "init", workType: "task" }],
          name: "Review",
          onFailure: [{ state: "failed", workType: "task" }],
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

function assertLineageTextContent(
  snapshot: { work_item: FactoryWorkItem } | undefined,
  want: string,
): void {
  expect(snapshot).toBeDefined();
  if (!snapshot) {
    return;
  }
  expect(snapshot.work_item.content).toEqual([{ type: "text", text: want }]);
}

describe("reconstructWorldState payload lineage recording", () => {
  it("records work-request, consumed-input, and dispatch-output snapshots during replay", () => {
    const initial = lineageWorkItem("work-1", "Draft", "trace-1", "draft-v1");
    const continued = lineageWorkItem("work-1", "Draft", "trace-1", "draft-v2");
    const downstream = lineageWorkItem(
      "work-2",
      "Follow up",
      "trace-2",
      "follow-up-v1",
    );
    const laterSelected = lineageWorkItem(
      "work-1",
      "Draft",
      "trace-1",
      "draft-v3",
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
      lineageWorkRequestEvent(4, "request/work-1-v3", laterSelected),
    ];

    const state = reconstructFactoryReplayState(events, 4);
    const lineage = state.payloadLineage;

    expect(Object.keys(lineage.snapshots_by_id)).toHaveLength(4);

    const initialResolution = resolveInitialSubmittedSnapshot(
      lineage,
      "work-1",
    );
    expect(initialResolution.status).toBe("RESOLVED");
    assertLineageTextContent(initialResolution.snapshot, "draft-v1");

    const consumed = resolveConsumedInputSnapshot(
      lineage,
      "dispatch-1",
      "work-1",
    );
    expect(consumed.status).toBe("RESOLVED");
    assertLineageTextContent(consumed.snapshot, "draft-v1");

    const selected = resolveSelectedWorkSnapshot(lineage, "work-1");
    expect(selected.status).toBe("RESOLVED");
    assertLineageTextContent(selected.snapshot, "draft-v3");

    const sameWorkOutput = resolveOutputWorkSnapshot(
      lineage,
      "dispatch-1",
      "work-1",
    );
    expect(sameWorkOutput.status).toBe("RESOLVED");
    expect(sameWorkOutput.snapshot?.continuity).toBe(
      "SAME_WORK_ID_CONTINUATION",
    );
    assertLineageTextContent(sameWorkOutput.snapshot, "draft-v2");

    const downstreamOutput = resolveOutputWorkSnapshot(
      lineage,
      "dispatch-1",
      "work-2",
    );
    expect(downstreamOutput.status).toBe("RESOLVED");
    expect(downstreamOutput.snapshot?.continuity).toBe("NEW_DOWNSTREAM_WORK");
    assertLineageTextContent(downstreamOutput.snapshot, "follow-up-v1");
  });
});

describe("reconstructWorldState payload lineage resolution", () => {
  it("marks consumed-input lineage unavailable when no work-request snapshot exists", () => {
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
    const consumed = resolveConsumedInputSnapshot(
      state.payloadLineage,
      "dispatch-missing",
      "work-missing",
    );

    expect(consumed.status).toBe("UNAVAILABLE");
    expect(consumed.reason).toBe(
      "no lineage snapshot was recorded before this dispatch consumed the work item",
    );
  });

  it("preserves consumed-input pin when the same work ID is resubmitted later", () => {
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
    const lineage = state.payloadLineage;

    const consumed = resolveConsumedInputSnapshot(
      lineage,
      "dispatch-consume-child",
      "work-child",
    );
    expect(consumed.status).toBe("RESOLVED");
    assertLineageTextContent(consumed.snapshot, "child-v1");

    const selected = resolveSelectedWorkSnapshot(lineage, "work-child");
    expect(selected.status).toBe("RESOLVED");
    assertLineageTextContent(selected.snapshot, "child-v2");
  });
});
