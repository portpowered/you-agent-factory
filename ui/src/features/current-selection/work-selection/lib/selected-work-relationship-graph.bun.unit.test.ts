import { describe, expect, it } from "bun:test";

import { buildSelectedWorkRelationshipGraph } from "./selected-work-relationship-graph";
import {
  selectedWorkItem,
  snapshotFixture,
} from "./selected-work-relationship-graph.fixture";

describe("buildSelectedWorkRelationshipGraph", () => {
  it("builds the full connected relationship graph around the selected work item", () => {
    const graph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem,
      snapshot: snapshotFixture(),
    });

    expect(graph.status).toBe("ready");
    if (graph.status !== "ready") {
      throw new Error(`expected ready graph, got ${graph.status}`);
    }

    expect(graph.selectedWork).toMatchObject({
      label: "Active Story",
      state: "in_progress",
      traceID: "trace-active-story",
      workID: "work-active-story",
      workTypeID: "story",
    });
    expect(graph.relatedWork.map((node) => node.label)).toEqual([
      "Blocked Story",
      "Child Story",
      "Dependency Story",
      "Grandchild Story",
      "Parent Story",
    ]);
    expect(graph.relations).toEqual([
      {
        required_state: "ready",
        source_work_id: "work-active-story",
        source_work_name: "Active Story",
        target_work_id: "work-dependency-story",
        target_work_name: "Dependency Story",
        type: "DEPENDS_ON",
      },
      {
        required_state: "approved",
        source_work_id: "work-blocked-story",
        source_work_name: "Blocked Story",
        target_work_id: "work-active-story",
        target_work_name: "Active Story",
        type: "DEPENDS_ON",
      },
      {
        source_work_id: "work-active-story",
        source_work_name: "Active Story",
        target_work_id: "work-parent-story",
        target_work_name: "Parent Story",
        type: "PARENT_CHILD",
      },
      {
        source_work_id: "work-child-story",
        source_work_name: "Child Story",
        target_work_id: "work-active-story",
        target_work_name: "Active Story",
        type: "PARENT_CHILD",
      },
      {
        source_work_id: "work-child-story",
        source_work_name: "Child Story",
        target_work_id: "work-grandchild-story",
        target_work_name: "Grandchild Story",
        type: "PARENT_CHILD",
      },
    ]);
    expect(graph.edges).toEqual([
      {
        relationship: "CHILD",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-child-story",
      },
      {
        relationship: "DEPENDS_ON",
        requiredState: "ready",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-dependency-story",
      },
      {
        relationship: "PARENT",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-parent-story",
      },
      {
        relationship: "PARENT",
        sourceWorkID: "work-child-story",
        targetWorkID: "work-grandchild-story",
      },
      {
        relationship: "REQUIRED_BY",
        requiredState: "approved",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-blocked-story",
      },
    ]);
  });

  it("returns an explicit empty graph when no supported relationships exist", () => {
    const snapshot = snapshotFixture();
    snapshot.relationsByWorkID = {};

    const graph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem,
      snapshot,
    });

    expect(graph).toMatchObject({
      edges: [],
      relations: [],
      relatedWork: [],
      selectedWork: {
        label: "Active Story",
        workID: "work-active-story",
      },
      status: "empty",
    });
  });

  it("returns an explicit error state when relationship data is unavailable", () => {
    const { relationsByWorkID: _relationsByWorkID, ...snapshot } =
      snapshotFixture();

    const graph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem,
      snapshot,
    });

    expect(graph.status).toBe("error");
    if (graph.status !== "error") {
      throw new Error(`expected error graph, got ${graph.status}`);
    }
    expect(graph.message).toContain("unavailable");
  });
});
