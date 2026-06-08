import { describe, expect, it } from "vitest";
import { baseFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft.test-helpers";
import { buildCurrentActivityGraphLayoutFromFactory } from "./current-activity-factory-graph-layout";
import {
  migrateWorkStateGraphLayoutPositions,
  workStateFactoryGraphNodeId,
} from "./migrate-work-state-graph-layout-positions";
import { currentActivityGraphKey } from "./react-flow-current-activity-card-keys";

describe("migrateWorkStateGraphLayoutPositions", () => {
  it("copies stored positions from the previous work-state node id to the renamed id", () => {
    const previousNodeId = workStateFactoryGraphNodeId("story", "queued");
    const nextNodeId = workStateFactoryGraphNodeId("story", "ready");
    const graphKey = `resource:gpu|${previousNodeId}|workstation:draft::edge-a`;

    const migrated = migrateWorkStateGraphLayoutPositions({
      nextStateName: "ready",
      positionsByGraphKey: {
        [graphKey]: {
          [previousNodeId]: { x: 120, y: 240 },
        },
      },
      previousStateName: "queued",
      workTypeName: "story",
    });

    const migratedGraphKey = graphKey.replaceAll(previousNodeId, nextNodeId);
    expect(migrated[migratedGraphKey]).toEqual({
      [nextNodeId]: { x: 120, y: 240 },
    });
    expect(migrated[graphKey]).toBeUndefined();
    expect(migrated[migratedGraphKey]?.[previousNodeId]).toBeUndefined();
  });

  it("rewrites graph keys that embed the previous work-state node id in edge ids", async () => {
    const renamedFactoryDefinition = {
      ...baseFactoryDefinition,
      workTypes: [
        {
          name: "story",
          states: [
            { name: "ready", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          ...baseFactoryDefinition.workstations?.[0],
          inputs: [{ state: "ready", workType: "story" }],
          name: "draft",
          outputs: [{ state: "done", workType: "story" }],
        },
      ],
    };
    const graphLayout = await buildCurrentActivityGraphLayoutFromFactory(
      renamedFactoryDefinition,
    );
    const graphKey = currentActivityGraphKey(graphLayout);
    const previousNodeId = workStateFactoryGraphNodeId("story", "queued");
    const nextNodeId = workStateFactoryGraphNodeId("story", "ready");

    const migrated = migrateWorkStateGraphLayoutPositions({
      nextStateName: "ready",
      positionsByGraphKey: {
        [graphKey.replaceAll(nextNodeId, previousNodeId)]: {
          [previousNodeId]: { x: 44, y: 88 },
        },
      },
      previousStateName: "queued",
      workTypeName: "story",
    });

    expect(migrated[graphKey]?.[nextNodeId]).toEqual({ x: 44, y: 88 });
    expect(
      migrated[graphKey.replaceAll(nextNodeId, previousNodeId)],
    ).toBeUndefined();
  });

  it("leaves unrelated graph keys unchanged when they do not reference the renamed node", () => {
    const unrelated = {
      "worker:writer::workstation:draft": {
        "worker:writer": { x: 1, y: 2 },
      },
    };

    expect(
      migrateWorkStateGraphLayoutPositions({
        nextStateName: "ready",
        positionsByGraphKey: unrelated,
        previousStateName: "queued",
        workTypeName: "story",
      }),
    ).toEqual(unrelated);
  });
});
