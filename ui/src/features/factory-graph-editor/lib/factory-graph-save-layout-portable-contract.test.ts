import { describe, expect, it } from "vitest";

import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";
import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";
import { applyFactoryGraphPendingEdits } from "./factory-graph-operations";
import {
  factoryDefinitionSavePayloadHasGraphLayoutFields,
  findGraphLayoutPropertyPaths,
} from "./factory-graph-save-layout-boundary";

describe("factory graph portable layout contract", () => {
  it("allows the portable top-level layout contract", () => {
    expect(
      findGraphLayoutPropertyPaths({
        layout: {
          schemaVersion: 1,
          nodes: [
            {
              id: "workstation:draft",
              position: { x: 10, y: 20 },
            },
          ],
          viewport: { x: 0, y: 0, zoom: 1 },
        },
      }),
    ).toEqual([]);
  });

  it("preserves the portable top-level layout contract in save payloads", () => {
    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition: {
        ...baseFactoryDefinition,
        layout: {
          schemaVersion: 1,
          nodes: [
            {
              id: "workstation:draft",
              position: { x: 10, y: 20 },
            },
          ],
          viewport: { x: 0, y: 0, zoom: 1 },
        },
      },
      draft: createEmptyFactoryGraphDraft(),
    });
    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    expect(
      factoryDefinitionSavePayloadHasGraphLayoutFields(saveInput.value),
    ).toBe(false);
    expect(saveInput.value.layout).toEqual({
      schemaVersion: 1,
      nodes: [
        {
          id: "workstation:draft",
          position: { x: 10, y: 20 },
        },
      ],
      viewport: { x: 0, y: 0, zoom: 1 },
    });
  });
});
