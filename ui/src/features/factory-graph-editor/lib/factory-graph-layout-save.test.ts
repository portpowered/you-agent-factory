import { describe, expect, it } from "vitest";

import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";
import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";
import { applyFactoryGraphPendingEdits } from "./factory-graph-operations";
import { factoryDefinitionSavePayloadHasGraphLayoutFields } from "./factory-graph-save-layout-boundary";
import {
  createDefaultFactoryLayout,
  moveFactoryLayoutNode,
} from "./factory-graph-layout-operations";

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
    expect(saveInput.value.workstations).toEqual(
      baseFactoryDefinition.workstations,
    );
    expect(saveInput.value.workTypes).toEqual(baseFactoryDefinition.workTypes);
  });
});
