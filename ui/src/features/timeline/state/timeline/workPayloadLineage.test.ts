import { describe, expect, it } from "vitest";
import type { FactoryWorkItem } from "../../../../api/events";
import {
  emptyWorkPayloadLineageProjection,
  recordConsumedInputSnapshot,
  recordDispatchOutputSnapshot,
  recordWorkRequestSnapshot,
  resolveConsumedInputSnapshot,
  resolveInitialSubmittedSnapshot,
  resolveOutputWorkSnapshot,
  resolveSelectedWorkSnapshot,
  type WorkPayloadLineageProjection,
} from "./workPayloadLineage";

function projectionWorkItem(
  id: string,
  displayName: string,
  traceID: string,
  placeID: string,
  text: string,
): FactoryWorkItem {
  return {
    id,
    work_type_id: "task",
    display_name: displayName,
    trace_id: traceID,
    place_id: placeID,
    content: [{ type: "text", text }],
  };
}

function assertLineageTextContent(item: FactoryWorkItem, want: string): void {
  expect(item.content).toEqual([{ type: "text", text: want }]);
}

function buildCanonicalLineageProjection(): WorkPayloadLineageProjection {
  const projection = emptyWorkPayloadLineageProjection();
  const initial = projectionWorkItem(
    "work-1",
    "Draft",
    "trace-1",
    "task:init",
    "draft-v1",
  );
  const continued = projectionWorkItem(
    "work-1",
    "Draft",
    "trace-1",
    "task:complete",
    "draft-v2",
  );
  const downstream = projectionWorkItem(
    "work-2",
    "Follow up",
    "trace-2",
    "task:complete",
    "follow-up-v1",
  );
  const laterSelected = projectionWorkItem(
    "work-1",
    "Draft",
    "trace-1",
    "task:complete",
    "draft-v3",
  );

  recordWorkRequestSnapshot(projection, 1, "request/work-1-v1", initial);
  recordConsumedInputSnapshot(projection, "dispatch-1", initial);
  recordDispatchOutputSnapshot(
    projection,
    3,
    "dispatch-1",
    [initial],
    continued,
    0,
  );
  recordDispatchOutputSnapshot(
    projection,
    3,
    "dispatch-1",
    [initial],
    downstream,
    1,
  );
  recordWorkRequestSnapshot(projection, 4, "request/work-1-v3", laterSelected);

  return projection;
}

function buildDownstreamLineageProjection(): WorkPayloadLineageProjection {
  const projection = emptyWorkPayloadLineageProjection();
  const initial = projectionWorkItem(
    "work-root",
    "Root",
    "trace-root",
    "task:init",
    "root-v1",
  );
  const downstream = projectionWorkItem(
    "work-child",
    "Child",
    "trace-child",
    "task:review",
    "child-v1",
  );
  const laterSelected = projectionWorkItem(
    "work-child",
    "Child",
    "trace-child",
    "task:done",
    "child-v2",
  );

  recordWorkRequestSnapshot(projection, 1, "request/root-v1", initial);
  recordConsumedInputSnapshot(projection, "dispatch-create-child", initial);
  recordDispatchOutputSnapshot(
    projection,
    3,
    "dispatch-create-child",
    [initial],
    downstream,
    0,
  );
  recordConsumedInputSnapshot(projection, "dispatch-consume-child", downstream);
  recordWorkRequestSnapshot(projection, 5, "request/child-v2", laterSelected);

  return projection;
}

describe("WorkPayloadLineageProjection work-request snapshots", () => {
  it("records initial work-request snapshots and resolves them", () => {
    const projection = emptyWorkPayloadLineageProjection();
    const item = projectionWorkItem(
      "work-1",
      "Draft",
      "trace-1",
      "task:init",
      "draft-v1",
    );

    recordWorkRequestSnapshot(projection, 1, "request/work-1-v1", item);

    const initial = resolveInitialSubmittedSnapshot(projection, "work-1");
    expect(initial.status).toBe("RESOLVED");
    expect(initial.snapshot?.source_kind).toBe("WORK_REQUEST");
    expect(initial.snapshot?.continuity).toBe("INITIAL_SUBMISSION");
    assertLineageTextContent(
      initial.snapshot?.work_item ?? { id: "", work_type_id: "" },
      "draft-v1",
    );
  });

  it("continues same work ID across later work-request snapshots", () => {
    const projection = emptyWorkPayloadLineageProjection();
    const initial = projectionWorkItem(
      "work-1",
      "Draft",
      "trace-1",
      "task:init",
      "draft-v1",
    );
    const continued = projectionWorkItem(
      "work-1",
      "Draft",
      "trace-1",
      "task:complete",
      "draft-v3",
    );

    recordWorkRequestSnapshot(projection, 1, "request/work-1-v1", initial);
    recordWorkRequestSnapshot(projection, 4, "request/work-1-v3", continued);

    const selected = resolveSelectedWorkSnapshot(projection, "work-1");
    expect(selected.status).toBe("RESOLVED");
    expect(selected.snapshot?.continuity).toBe("SAME_WORK_ID_CONTINUATION");
    expect(selected.snapshot?.logical_work_id).toBe("work-1");
    assertLineageTextContent(
      selected.snapshot?.work_item ?? { id: "", work_type_id: "" },
      "draft-v3",
    );
    expect(selected.snapshot?.parent_work_ids).toEqual(["work-1"]);
  });
});

describe("WorkPayloadLineageProjection consumed-input pinning", () => {
  it("pins consumed-input snapshots at dispatch time", () => {
    const projection = buildCanonicalLineageProjection();

    const consumed = resolveConsumedInputSnapshot(
      projection,
      "dispatch-1",
      "work-1",
    );
    expect(consumed.status).toBe("RESOLVED");
    assertLineageTextContent(
      consumed.snapshot?.work_item ?? { id: "", work_type_id: "" },
      "draft-v1",
    );

    recordWorkRequestSnapshot(
      projection,
      5,
      "request/work-1-v4",
      projectionWorkItem(
        "work-1",
        "Draft",
        "trace-1",
        "task:complete",
        "draft-v4",
      ),
    );

    const consumedAfterResubmit = resolveConsumedInputSnapshot(
      projection,
      "dispatch-1",
      "work-1",
    );
    assertLineageTextContent(
      consumedAfterResubmit.snapshot?.work_item ?? { id: "", work_type_id: "" },
      "draft-v1",
    );

    const selected = resolveSelectedWorkSnapshot(projection, "work-1");
    assertLineageTextContent(
      selected.snapshot?.work_item ?? { id: "", work_type_id: "" },
      "draft-v4",
    );
  });

  it("marks unavailable consumed input with Go-aligned reason text", () => {
    const projection = emptyWorkPayloadLineageProjection();
    recordConsumedInputSnapshot(
      projection,
      "dispatch-missing",
      projectionWorkItem(
        "work-missing",
        "Missing",
        "trace-missing",
        "task:init",
        "missing-v1",
      ),
    );

    const consumed = resolveConsumedInputSnapshot(
      projection,
      "dispatch-missing",
      "work-missing",
    );
    expect(consumed.status).toBe("UNAVAILABLE");
    expect(consumed.reason).toBe(
      "no lineage snapshot was recorded before this dispatch consumed the work item",
    );
  });
});

describe("WorkPayloadLineageProjection dispatch output snapshots", () => {
  it("records dispatch output continuity for same-work and downstream outputs", () => {
    const projection = buildCanonicalLineageProjection();

    const sameWorkOutput = resolveOutputWorkSnapshot(
      projection,
      "dispatch-1",
      "work-1",
    );
    expect(sameWorkOutput.status).toBe("RESOLVED");
    assertLineageTextContent(
      sameWorkOutput.snapshot?.work_item ?? { id: "", work_type_id: "" },
      "draft-v2",
    );
    expect(sameWorkOutput.snapshot?.continuity).toBe(
      "SAME_WORK_ID_CONTINUATION",
    );
    expect(sameWorkOutput.snapshot?.logical_work_id).toBe("work-1");

    const downstreamOutput = resolveOutputWorkSnapshot(
      projection,
      "dispatch-1",
      "work-2",
    );
    expect(downstreamOutput.status).toBe("RESOLVED");
    assertLineageTextContent(
      downstreamOutput.snapshot?.work_item ?? { id: "", work_type_id: "" },
      "follow-up-v1",
    );
    expect(downstreamOutput.snapshot?.continuity).toBe("NEW_DOWNSTREAM_WORK");
    expect(downstreamOutput.snapshot?.parent_work_ids).toEqual(["work-1"]);
    expect(downstreamOutput.snapshot?.logical_work_id).toBe("work-2");
  });

  it("resolves selected latest snapshots separately from consumed dispatch-time snapshots", () => {
    const projection = buildCanonicalLineageProjection();

    const selected = resolveSelectedWorkSnapshot(projection, "work-1");
    expect(selected.status).toBe("RESOLVED");
    expect(selected.snapshot?.source_kind).toBe("WORK_REQUEST");
    assertLineageTextContent(
      selected.snapshot?.work_item ?? { id: "", work_type_id: "" },
      "draft-v3",
    );
  });

  it("resolves downstream output selection and chained consumption", () => {
    const projection = buildDownstreamLineageProjection();

    const selected = resolveSelectedWorkSnapshot(projection, "work-child");
    expect(selected.status).toBe("RESOLVED");
    assertLineageTextContent(
      selected.snapshot?.work_item ?? { id: "", work_type_id: "" },
      "child-v2",
    );
    expect(selected.snapshot?.logical_work_id).toBe("work-child");

    const consumed = resolveConsumedInputSnapshot(
      projection,
      "dispatch-consume-child",
      "work-child",
    );
    expect(consumed.status).toBe("RESOLVED");
    assertLineageTextContent(
      consumed.snapshot?.work_item ?? { id: "", work_type_id: "" },
      "child-v1",
    );
    expect(consumed.snapshot?.source_kind).toBe("DISPATCH_RESPONSE_OUTPUT");
    expect(consumed.snapshot?.continuity).toBe("NEW_DOWNSTREAM_WORK");
    expect(consumed.snapshot?.parent_work_ids).toEqual(["work-root"]);
  });
});

describe("WorkPayloadLineageProjection clone isolation", () => {
  it("clones returned snapshots so callers cannot mutate stored lineage state", () => {
    const projection = emptyWorkPayloadLineageProjection();
    const item = projectionWorkItem(
      "work-1",
      "Draft",
      "trace-1",
      "task:init",
      "draft-v1",
    );
    recordWorkRequestSnapshot(projection, 1, "request/work-1-v1", item);

    const selected = resolveSelectedWorkSnapshot(projection, "work-1");
    selected.snapshot?.work_item.content?.push({
      type: "text",
      text: "mutated",
    });

    const selectedAgain = resolveSelectedWorkSnapshot(projection, "work-1");
    expect(selectedAgain.snapshot?.work_item.content).toEqual([
      { type: "text", text: "draft-v1" },
    ]);
  });
});
