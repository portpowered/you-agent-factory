import { describe, expect, it } from "vitest";

import { buildSelectedWorkRelationshipGraph } from "./selected-work-relationship-graph";
import {
  selectedWorkItem,
  snapshotFixture,
} from "./selected-work-relationship-graph.fixture";

function mixedRelationshipSnapshot() {
  const snapshot = snapshotFixture();
  snapshot.relationsByWorkID = {
    "work-active-story": [
      {
        request_id: "request-dep-1",
        source_work_id: "work-active-story",
        sourceWorkName: "Active Story",
        targetWorkId: "work-dependency-story",
        targetWorkName: "Dependency Story",
        type: "DEPENDS_ON",
        requiredState: "ready",
      },
      {
        request_id: "request-dep-2",
        source_work_id: "work-active-story",
        sourceWorkName: "Active Story",
        targetWorkId: "work-second-dependency-story",
        targetWorkName: "Second Dependency Story",
        type: "DEPENDS_ON",
        requiredState: "ready",
      },
      {
        request_id: "request-parent",
        source_work_id: "work-active-story",
        sourceWorkName: "Active Story",
        targetWorkId: "work-parent-story",
        targetWorkName: "Parent Story",
        type: "PARENT_CHILD",
      },
    ],
  };
  snapshot.runtime.active_executions_by_dispatch_id[
    "dispatch-active-story"
  ].work_items?.push({
    display_name: "Second Dependency Story",
    state: "ready",
    trace_id: "trace-second-dependency-story",
    work_id: "work-second-dependency-story",
    work_type_id: "story",
  });
  return snapshot;
}

describe("buildSelectedWorkRelationshipGraph mixed relationships", () => {
  it("keeps repeated dependency and parent relationships together deterministically", () => {
    const snapshot = mixedRelationshipSnapshot();
    const firstGraph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem,
      snapshot,
    });
    const secondGraph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem,
      snapshot,
    });

    expect(firstGraph).toEqual(secondGraph);
    if (firstGraph.status !== "ready") {
      throw new Error(`expected ready graph, got ${firstGraph.status}`);
    }

    expect(firstGraph.relations).toEqual([
      {
        request_id: "request-dep-1",
        required_state: "ready",
        source_work_id: "work-active-story",
        source_work_name: "Active Story",
        target_work_id: "work-dependency-story",
        target_work_name: "Dependency Story",
        type: "DEPENDS_ON",
      },
      {
        request_id: "request-dep-2",
        required_state: "ready",
        source_work_id: "work-active-story",
        source_work_name: "Active Story",
        target_work_id: "work-second-dependency-story",
        target_work_name: "Second Dependency Story",
        type: "DEPENDS_ON",
      },
      {
        request_id: "request-parent",
        source_work_id: "work-active-story",
        source_work_name: "Active Story",
        target_work_id: "work-parent-story",
        target_work_name: "Parent Story",
        type: "PARENT_CHILD",
      },
    ]);
    expect(firstGraph.edges).toEqual([
      {
        relationship: "DEPENDS_ON",
        requiredState: "ready",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-dependency-story",
      },
      {
        relationship: "DEPENDS_ON",
        requiredState: "ready",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-second-dependency-story",
      },
      {
        relationship: "PARENT",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-parent-story",
      },
    ]);
  });
});

describe("buildSelectedWorkRelationshipGraph shared endpoints", () => {
  it("preserves distinct relationship instances that share type and endpoints", () => {
    const snapshot = snapshotFixture();
    snapshot.relationsByWorkID = {
      "work-active-story": [
        {
          request_id: "request-dep-a",
          source_work_id: "work-active-story",
          sourceWorkName: "Active Story",
          targetWorkId: "work-dependency-story",
          targetWorkName: "Dependency Story",
          type: "DEPENDS_ON",
          requiredState: "ready",
        },
        {
          request_id: "request-dep-b",
          source_work_id: "work-active-story",
          sourceWorkName: "Active Story",
          targetWorkId: "work-dependency-story",
          targetWorkName: "Dependency Story",
          type: "DEPENDS_ON",
          requiredState: "ready",
        },
      ],
    };

    const graph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem,
      snapshot,
    });

    expect(graph.status).toBe("ready");
    if (graph.status !== "ready") {
      throw new Error(`expected ready graph, got ${graph.status}`);
    }

    expect(graph.relations).toHaveLength(2);
    expect(graph.relations.map((relation) => relation.request_id)).toEqual([
      "request-dep-a",
      "request-dep-b",
    ]);
    expect(graph.edges).toHaveLength(2);
  });
});
