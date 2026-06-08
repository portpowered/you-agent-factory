import { describe, expect, it } from "vitest";

import { baseFactoryDefinition } from "../../draft/factory-graph-draft.test-helpers";
import { createEmptyFactoryGraphDraft } from "../../draft/factory-graph-draft-types";
import { applyFactoryGraphPendingEdits } from "../../operations/factory-graph-operations";
import { factoryDefinitionSavePayloadHasGraphLayoutFields } from "../../document-save/factory-graph-save-layout-boundary";
import { setFactoryLayoutEdgeWaypoints } from "../factory-graph-layout-edge-waypoints";
import {
  createDefaultFactoryLayout,
  factoryLayoutFromDefinition,
  moveFactoryLayoutNode,
} from "../factory-graph-layout-operations";
import { buildFactoryGraphSaveSummary } from "../../editor-runtime/factory-graph-editor-save-summary";

const EDGE_ID =
  "workstation-output:workstation:draft->work-state:story:done";

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
    for (const [index, workType] of (baseFactoryDefinition.workTypes ?? []).entries()) {
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
    for (const [index, workType] of (baseFactoryDefinition.workTypes ?? []).entries()) {
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
});
