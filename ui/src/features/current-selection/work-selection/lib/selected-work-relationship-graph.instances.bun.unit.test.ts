import { describe, expect, it } from "bun:test";

import { buildSelectedWorkRelationshipGraph } from "./selected-work-relationship-graph";
import {
  loadLocalAgentCliRuntimeBatch,
  localAgentCliRuntimeBatchSnapshot,
  localAgentCliRuntimeLoopbackWorkItem,
  selectedWorkItem,
  snapshotFixture,
} from "./selected-work-relationship-graph.fixture";

describe("buildSelectedWorkRelationshipGraph repeated DEPENDS_ON", () => {
  it("preserves every distinct DEPENDS_ON relationship from the selected work item", () => {
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

    const graph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem,
      snapshot,
    });

    expect(graph.status).toBe("ready");
    if (graph.status !== "ready") {
      throw new Error(`expected ready graph, got ${graph.status}`);
    }

    expect(
      graph.relations.filter((relation) => relation.type === "DEPENDS_ON"),
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
    expect(
      graph.edges.filter((edge) => edge.relationship === "DEPENDS_ON"),
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          sourceWorkID: "work-active-story",
          targetWorkID: "work-dependency-story",
        }),
        expect.objectContaining({
          sourceWorkID: "work-active-story",
          targetWorkID: "work-second-dependency-story",
        }),
      ]),
    );
    expect(graph.relatedWork.map((node) => node.workID)).toEqual(
      expect.arrayContaining([
        "work-dependency-story",
        "work-second-dependency-story",
      ]),
    );
  });
});

describe("factory-batch-local-agent-cli-runtime relationship graph", () => {
  it("preserves every DEPENDS_ON relation from the loopback work item in the smoke test fixture", () => {
    const batch = loadLocalAgentCliRuntimeBatch();
    const loopbackDependsOn = batch.relations.filter(
      (relation) =>
        relation.type === "DEPENDS_ON" &&
        relation.sourceWorkName === "local-agent-cli-runtime-loopback",
    );
    expect(loopbackDependsOn).toHaveLength(5);

    const graph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem: localAgentCliRuntimeLoopbackWorkItem(),
      snapshot: localAgentCliRuntimeBatchSnapshot(),
    });

    expect(graph.status).toBe("ready");
    if (graph.status !== "ready") {
      throw new Error(`expected ready graph, got ${graph.status}`);
    }

    const dependencyTargets = loopbackDependsOn.map(
      (relation) => `work-${relation.targetWorkName}`,
    );
    expect(
      graph.relations.filter(
        (relation) =>
          relation.type === "DEPENDS_ON" &&
          relation.source_work_id === "work-local-agent-cli-runtime-loopback",
      ),
    ).toHaveLength(5);
    expect(graph.relatedWork.map((node) => node.workID)).toEqual(
      expect.arrayContaining(dependencyTargets),
    );
    expect(
      graph.edges.filter(
        (edge) =>
          edge.relationship === "DEPENDS_ON" &&
          edge.sourceWorkID === "work-local-agent-cli-runtime-loopback",
      ),
    ).toHaveLength(5);
  });
});
