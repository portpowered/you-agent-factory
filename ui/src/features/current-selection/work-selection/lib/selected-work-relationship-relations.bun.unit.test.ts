import { describe, expect, it } from "bun:test";
import type { SelectedWorkRelationshipGraph } from "./selected-work-relationship-graph";
import { projectSelectedWorkRelationshipGraphToDashboardRelations } from "./selected-work-relationship-relations";

function readyGraph(): SelectedWorkRelationshipGraph {
  return {
    edges: [
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
        relationship: "DEPENDS_ON",
        requiredState: "approved",
        sourceWorkID: "work-blocked-story",
        targetWorkID: "work-active-story",
      },
      {
        relationship: "DEPENDS_ON",
        requiredState: "ready",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-dependency-story",
      },
    ],
    relations: [
      {
        source_work_id: "work-active-story",
        source_work_name: "Active Story",
        target_work_id: "work-parent-story",
        target_work_name: "Parent Story",
        type: "PARENT_CHILD",
      },
      {
        required_state: "approved",
        source_work_id: "work-blocked-story",
        source_work_name: "Blocked Story",
        target_work_id: "work-active-story",
        target_work_name: "Active Story",
        type: "DEPENDS_ON",
      },
    ],
    relatedWork: [
      {
        label: "Blocked Story",
        state: "blocked",
        traceID: "trace-blocked-story",
        workID: "work-blocked-story",
        workTypeID: "story",
      },
      {
        label: "Child Story",
        state: "queued",
        traceID: "trace-child-story",
        workID: "work-child-story",
        workTypeID: "task",
      },
      {
        label: "Dependency Story",
        state: "ready",
        traceID: "trace-dependency-story",
        workID: "work-dependency-story",
        workTypeID: "story",
      },
      {
        label: "Parent Story",
        state: "queued",
        traceID: "trace-parent-story",
        workID: "work-parent-story",
        workTypeID: "epic",
      },
    ],
    selectedWork: {
      label: "Active Story",
      state: "in_progress",
      traceID: "trace-active-story",
      workID: "work-active-story",
      workTypeID: "story",
    },
    status: "ready",
  };
}

describe("projectSelectedWorkRelationshipGraphToDashboardRelations", () => {
  it("projects ready relationship graphs from direct relations when available", () => {
    expect(
      projectSelectedWorkRelationshipGraphToDashboardRelations(readyGraph()),
    ).toEqual([
      {
        source_work_id: "work-active-story",
        source_work_name: "Active Story",
        target_work_id: "work-parent-story",
        target_work_name: "Parent Story",
        type: "PARENT_CHILD",
      },
      {
        required_state: "approved",
        source_work_id: "work-blocked-story",
        source_work_name: "Blocked Story",
        target_work_id: "work-active-story",
        target_work_name: "Active Story",
        type: "DEPENDS_ON",
      },
    ]);
  });

  it("returns no relations for loading, error, empty, or missing graphs", () => {
    expect(
      projectSelectedWorkRelationshipGraphToDashboardRelations(undefined),
    ).toBeUndefined();
    expect(
      projectSelectedWorkRelationshipGraphToDashboardRelations({
        status: "loading",
      }),
    ).toBeUndefined();
    expect(
      projectSelectedWorkRelationshipGraphToDashboardRelations({
        message: "unavailable",
        selectedWork: readyGraph().selectedWork,
        status: "error",
      }),
    ).toBeUndefined();
    expect(
      projectSelectedWorkRelationshipGraphToDashboardRelations({
        edges: [],
        relations: [],
        relatedWork: [],
        selectedWork: readyGraph().selectedWork,
        status: "empty",
      }),
    ).toBeUndefined();
  });

  it("projects repeated dependency edges when direct relations are unavailable", () => {
    const graph = readyGraph();
    const edgeOnlyGraph: SelectedWorkRelationshipGraph = {
      ...graph,
      relations: [],
    };

    expect(
      projectSelectedWorkRelationshipGraphToDashboardRelations(edgeOnlyGraph),
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          required_state: "ready",
          source_work_id: "work-active-story",
          target_work_id: "work-dependency-story",
          type: "DEPENDS_ON",
        }),
        expect.objectContaining({
          required_state: "approved",
          source_work_id: "work-blocked-story",
          target_work_id: "work-active-story",
          type: "DEPENDS_ON",
        }),
        expect.objectContaining({
          source_work_id: "work-active-story",
          target_work_id: "work-parent-story",
          type: "PARENT_CHILD",
        }),
      ]),
    );
  });
});
