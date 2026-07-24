// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: layout save scenarios share one factory fixture.
import { describe, expect, it } from "vitest";
import { factoryDefinitionSavePayloadHasGraphLayoutFields } from "../../document-save/factory-graph-save-layout-boundary";
import { baseFactoryDefinition } from "../../draft/factory-graph-draft.test-helpers";
import { createEmptyFactoryGraphDraft } from "../../draft/factory-graph-draft-types";
import { buildFactoryGraphSaveSummary } from "../../editor-runtime/factory-graph-editor-save-summary";
import {
  applyFactoryGraphPendingEdits,
  connectFactoryGraphNodes,
  disconnectFactoryGraphEdge,
} from "../../operations/factory-graph-operations";
import { setFactoryLayoutEdgeWaypoints } from "../factory-graph-layout-edge-waypoints";
import {
  createDefaultFactoryLayout,
  factoryLayoutFromDefinition,
  moveFactoryLayoutNode,
} from "../factory-graph-layout-operations";

const EDGE_ID = "workstation-output:workstation:draft->work-state:story:done";
const FAILURE_EDGE_ID =
  "workstation-on-failure:workstation:draft->work-state:story:done";

describe("factory graph layout save", () => {
  it("persists layout-only edits without topology draft changes", () => {
    const pendingLayout = moveFactoryLayoutNode(
      createDefaultFactoryLayout(),
      "workstation:draft",
      { x: 144, y: 288 },
    );
    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      pendingLayout,
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    expect(
      factoryDefinitionSavePayloadHasGraphLayoutFields(saveInput.value),
    ).toBe(false);
    expect(saveInput.value.layout).toEqual({
      nodes: [
        {
          id: "workstation:draft",
          position: { x: 144, y: 288 },
        },
      ],
      schemaVersion: 1,
    });
    for (const [index, workstation] of (
      baseFactoryDefinition.workstations ?? []
    ).entries()) {
      expect(saveInput.value.workstations?.[index]).toMatchObject(workstation);
    }
    for (const [index, workType] of (
      baseFactoryDefinition.workTypes ?? []
    ).entries()) {
      expect(saveInput.value.workTypes?.[index]).toMatchObject(workType);
    }
  });

  it("persists authored edge waypoints through the shared layout save pipeline", () => {
    const pendingLayout = setFactoryLayoutEdgeWaypoints(
      createDefaultFactoryLayout(),
      EDGE_ID,
      [
        { x: 200, y: 300 },
        { x: 260, y: 340 },
      ],
    );
    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      pendingLayout,
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    expect(saveInput.value.layout?.edges).toEqual([
      {
        id: EDGE_ID,
        waypoints: [
          { x: 200, y: 300 },
          { x: 260, y: 340 },
        ],
      },
    ]);
    for (const [index, workstation] of (
      baseFactoryDefinition.workstations ?? []
    ).entries()) {
      expect(saveInput.value.workstations?.[index]).toMatchObject(workstation);
    }
    for (const [index, workType] of (
      baseFactoryDefinition.workTypes ?? []
    ).entries()) {
      expect(saveInput.value.workTypes?.[index]).toMatchObject(workType);
    }

    const reloadedLayout = factoryLayoutFromDefinition(saveInput.value);
    expect(reloadedLayout.edges).toEqual(saveInput.value.layout?.edges);
  });

  it("identifies waypoint-only edits as layout-only save summaries", () => {
    const summary = buildFactoryGraphSaveSummary({
      draft: createEmptyFactoryGraphDraft(),
      hasLayoutChanges: true,
    });

    expect(summary.kind).toBe("layout-only");
    expect(summary.dirtyState.topologyDirty).toBe(false);
    expect(summary.dirtyState.layoutDirty).toBe(true);
  });

  it("prunes stale edge waypoint layout when a topology edge is removed before save", () => {
    const connected = connectFactoryGraphNodes({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      sourceAnchorId: "workstation-on-failure-source",
      sourceNodeId: "workstation:draft",
      targetAnchorId: "work-state-input-target",
      targetNodeId: "work-state:story:done",
    });
    expect(connected.ok).toBe(true);
    if (!connected.ok) {
      return;
    }

    const disconnected = disconnectFactoryGraphEdge({
      baseFactoryDefinition,
      draft: connected.value,
      edgeId: FAILURE_EDGE_ID,
    });
    expect(disconnected.ok).toBe(true);
    if (!disconnected.ok) {
      return;
    }

    const pendingLayout = setFactoryLayoutEdgeWaypoints(
      createDefaultFactoryLayout(),
      FAILURE_EDGE_ID,
      [{ x: 180, y: 220 }],
    );
    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition,
      draft: disconnected.value,
      pendingLayout,
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    expect(saveInput.value.layout?.edges).toBeUndefined();
  });

  it("persists visual groups through the shared layout save pipeline without topology changes", () => {
    const baseWithGroups = {
      ...baseFactoryDefinition,
      layout: {
        schemaVersion: 1,
        groups: [
          {
            bounds: { x: 360, y: 120, width: 520, height: 360 },
            color: "blue",
            id: "review-lane",
            label: "Review",
            locked: false,
            nodeIds: ["workstation:draft"],
            parentGroupId: null,
          },
        ],
      },
    };
    const pendingLayout = {
      ...createDefaultFactoryLayout(),
      groups: [
        {
          bounds: { x: 380, y: 140, width: 520, height: 360 },
          color: "green",
          id: "review-lane",
          label: "Review lane",
          locked: true,
          nodeIds: ["workstation:draft", "workstation:missing"],
          parentGroupId: null,
        },
        {
          bounds: { x: 40, y: 60, width: 120, height: 80 },
          id: "empty-lane",
          nodeIds: [],
        },
      ],
    };
    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition: baseWithGroups,
      draft: createEmptyFactoryGraphDraft(),
      pendingLayout,
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    expect(saveInput.value.layout?.groups).toEqual([
      {
        bounds: { x: 380, y: 140, width: 520, height: 360 },
        color: "green",
        id: "review-lane",
        label: "Review lane",
        locked: true,
        nodeIds: ["workstation:draft"],
        parentGroupId: null,
      },
      {
        bounds: { x: 40, y: 60, width: 120, height: 80 },
        id: "empty-lane",
        nodeIds: [],
      },
    ]);
    for (const [index, workstation] of (
      baseFactoryDefinition.workstations ?? []
    ).entries()) {
      expect(saveInput.value.workstations?.[index]).toMatchObject(workstation);
    }
    for (const [index, workType] of (
      baseFactoryDefinition.workTypes ?? []
    ).entries()) {
      expect(saveInput.value.workTypes?.[index]).toMatchObject(workType);
    }

    const reloadedLayout = factoryLayoutFromDefinition(saveInput.value);
    expect(reloadedLayout.groups).toEqual(saveInput.value.layout?.groups);
  });

  it("round-trips edited visual groups through save preparation and reload", () => {
    const pendingLayout = {
      ...createDefaultFactoryLayout(),
      groups: [
        {
          bounds: { x: 80, y: 60, width: 360, height: 240 },
          color: "warning",
          id: "planning-lane",
          label: "Planning",
          locked: false,
          nodeIds: ["workstation:draft"],
          parentGroupId: "parent-lane",
        },
      ],
    };
    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      pendingLayout,
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    const reloadedLayout = factoryLayoutFromDefinition(saveInput.value);
    expect(reloadedLayout.groups).toEqual([
      {
        bounds: { x: 80, y: 60, width: 360, height: 240 },
        color: "warning",
        id: "planning-lane",
        label: "Planning",
        locked: false,
        nodeIds: ["workstation:draft"],
        parentGroupId: "parent-lane",
      },
    ]);
    for (const [index, workType] of (
      baseFactoryDefinition.workTypes ?? []
    ).entries()) {
      expect(saveInput.value.workTypes?.[index]).toMatchObject(workType);
    }
    for (const [index, workstation] of (
      baseFactoryDefinition.workstations ?? []
    ).entries()) {
      expect(saveInput.value.workstations?.[index]).toMatchObject(workstation);
    }
  });
});
