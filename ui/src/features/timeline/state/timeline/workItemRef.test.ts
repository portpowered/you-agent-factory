import { describe, expect, it } from "vitest";

import type { FactoryWorkItem } from "../../../../api/events";
import {
  selectedWorkItemRefForID,
  workItemRef,
  workItemRefWithConsumedPayload,
  workItemRefWithSelectedPayload,
} from "./workItemRef";
import {
  emptyWorkPayloadLineageProjection,
  recordConsumedInputSnapshot,
  recordWorkRequestSnapshot,
} from "./workPayloadLineage";

function workItem(id: string, text: string): FactoryWorkItem {
  return {
    id,
    display_name: id,
    trace_id: `trace-${id}`,
    work_type_id: "story",
    content: [{ type: "text", text }],
  };
}

describe("workItemRef lineage projection", () => {
  it("projects selected snapshots with content and lineage metadata", () => {
    const lineage = emptyWorkPayloadLineageProjection();
    const item = workItem("work-1", "draft-v1");
    recordWorkRequestSnapshot(lineage, 1, "request/work-1", item);

    const ref = workItemRefWithSelectedPayload(lineage, item);

    expect(ref).toEqual(
      expect.objectContaining({
        content: [{ type: "text", text: "draft-v1" }],
        lineage_continuity: "INITIAL_SUBMISSION",
        lineage_logical_work_id: "work-1",
        lineage_source_kind: "WORK_REQUEST",
        payload_status: "RESOLVED",
        work_id: "work-1",
      }),
    );
  });

  it("marks unavailable selected refs with Go-aligned reason text", () => {
    const lineage = emptyWorkPayloadLineageProjection();
    const item = workItem("work-missing", "ignored");

    const ref = selectedWorkItemRefForID(lineage, item.id, item);

    expect(ref.payload_status).toBe("UNAVAILABLE");
    expect(ref.payload_unavailable_reason).toBe(
      "no lineage snapshot is available for this work item",
    );
    expect(ref.content).toBeUndefined();
    expect(workItemRef(item)).not.toHaveProperty("payload_status");
  });

  it("pins consumed-input refs at dispatch time", () => {
    const lineage = emptyWorkPayloadLineageProjection();
    const initial = workItem("work-1", "draft-v1");
    const resubmit = workItem("work-1", "draft-v2");
    recordWorkRequestSnapshot(lineage, 1, "request/work-1-v1", initial);
    recordConsumedInputSnapshot(lineage, "dispatch-1", initial);
    recordWorkRequestSnapshot(lineage, 2, "request/work-1-v2", resubmit);

    const consumed = workItemRefWithConsumedPayload(
      lineage,
      "dispatch-1",
      resubmit,
    );
    const selected = workItemRefWithSelectedPayload(lineage, resubmit);

    expect(consumed.content).toEqual([{ type: "text", text: "draft-v1" }]);
    expect(selected.content).toEqual([{ type: "text", text: "draft-v2" }]);
  });
});
