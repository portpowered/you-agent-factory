import { describe, expect, it } from "bun:test";

import { buildSelectedWorkRelationshipGraph } from "./selected-work-relationship-graph";
import {
  selectedWorkItem,
  snapshotFixture,
} from "./selected-work-relationship-graph.fixture";
import { projectSelectedWorkRelationshipGraphToDashboardRelations } from "./selected-work-relationship-relations";

function repeatedDependsOnSnapshot() {
  const snapshot = snapshotFixture();
  snapshot.relationsByWorkID["work-active-story"] = [
    {
      source_work_id: "work-active-story",
      sourceWorkName: "Active Story",
      targetWorkId: "work-dependency-story",
      targetWorkName: "Dependency Story",
      type: "DEPENDS_ON",
      requiredState: "ready",
    },
    {
      source_work_id: "work-active-story",
      sourceWorkName: "Active Story",
      targetWorkId: "work-second-dependency-story",
      targetWorkName: "Second Dependency Story",
      type: "DEPENDS_ON",
      requiredState: "ready",
    },
  ];
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

describe("projectSelectedWorkRelationshipGraphToDashboardRelations repeated DEPENDS_ON", () => {
  it("projects every dependency relation instance from a ready selected-work graph", () => {
    const graph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem,
      snapshot: repeatedDependsOnSnapshot(),
    });

    expect(graph.status).toBe("ready");
    if (graph.status !== "ready") {
      throw new Error(`expected ready graph, got ${graph.status}`);
    }

    const relations =
      projectSelectedWorkRelationshipGraphToDashboardRelations(graph);

    expect(
      relations?.filter(
        (relation) =>
          relation.type === "DEPENDS_ON" &&
          relation.source_work_id === "work-active-story",
      ),
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          source_work_id: "work-active-story",
          target_work_id: "work-dependency-story",
          type: "DEPENDS_ON",
        }),
        expect.objectContaining({
          source_work_id: "work-active-story",
          target_work_id: "work-second-dependency-story",
          type: "DEPENDS_ON",
        }),
      ]),
    );
    expect(relations).toHaveLength(graph.relations.length);
  });
});
