import { describe, expect, it } from "vitest";
import {
  factoryDefinitionSavePayloadHasGraphLayoutFields,
  findGraphLayoutPropertyPaths,
} from "../document-save/factory-graph-save-layout-boundary";
import { baseFactoryDefinition } from "../draft/factory-graph-draft.test-helpers";
import { createEmptyFactoryGraphDraft } from "../draft/factory-graph-draft-types";
import { moveFactoryLayoutNode } from "../layout/factory-graph-layout-operations";
import { applyFactoryGraphPendingEdits } from "../operations/factory-graph-operations";
import {
  DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES,
  writeFactoryGraphEditorPreferencesForScope,
} from "./factory-graph-editor-preferences";

describe("factory graph editor preference save boundary", () => {
  it("does not export private editor preferences in portable save payloads", () => {
    const storage = {
      getItem: () => null,
      setItem: () => undefined,
      removeItem: () => undefined,
      clear: () => undefined,
      key: () => null,
      length: 0,
    };

    writeFactoryGraphEditorPreferencesForScope(
      "session-alpha",
      {
        hiddenNodeClasses: new Set(["work-state", "resource"]),
        visibilityPreset: "workflow",
      },
      storage,
    );

    const pendingLayout = moveFactoryLayoutNode(
      baseFactoryDefinition.layout ?? {
        nodes: [],
        schemaVersion: 1,
        viewport: { x: 0, y: 0, zoom: 1 },
      },
      "workstation:draft",
      { x: 120, y: 48 },
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

    expect(findGraphLayoutPropertyPaths(saveInput.value)).toEqual([]);
    expect(
      factoryDefinitionSavePayloadHasGraphLayoutFields(saveInput.value),
    ).toBe(false);
    expect(saveInput.value).not.toHaveProperty("editorPreferences");
    expect(saveInput.value).not.toEqual(
      expect.objectContaining({
        hiddenNodeClasses: [
          ...DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES.hiddenNodeClasses,
        ],
      }),
    );
  });
});
