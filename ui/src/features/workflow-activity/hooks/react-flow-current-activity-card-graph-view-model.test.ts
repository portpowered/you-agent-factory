import { describe, expect, it } from "vitest";

import { singleNodeDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  baseFactoryDefinitionDocument,
  createMockGraphEditorDraftState,
  divergentDocumentPlaneFactoryDocument,
} from "../../../testing/graph-editor-harness";
import { currentActivityCardFactoryDefinition } from "./react-flow-current-activity-card-graph-view-model";

function createEditorStub(
  overrides: {
    draftState?: ReturnType<typeof createMockGraphEditorDraftState>;
    editableDefinitionQuery?: {
      data?: typeof baseFactoryDefinitionDocument;
      status: "error" | "pending" | "success";
    };
    editorMode?: boolean;
  } = {},
) {
  return {
    draftState: overrides.draftState ?? createMockGraphEditorDraftState(),
    editableDefinitionQuery: {
      data:
        overrides.editableDefinitionQuery?.data ??
        baseFactoryDefinitionDocument,
      status: overrides.editableDefinitionQuery?.status ?? "success",
    },
    editorMode: overrides.editorMode ?? false,
  } as Parameters<typeof currentActivityCardFactoryDefinition>[0];
}

describe("currentActivityCardFactoryDefinition", () => {
  it("returns null in observe mode while the scoped factory document is pending", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableDefinitionQuery: { status: "pending" },
          editorMode: false,
        }),
        snapshot,
      ),
    ).toBeNull();
  });

  it("returns the saved factory document in observe mode once the scoped factory document succeeds", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableDefinitionQuery: {
            data: divergentDocumentPlaneFactoryDocument,
            status: "success",
          },
          editorMode: false,
        }),
        snapshot,
      ),
    ).toEqual(divergentDocumentPlaneFactoryDocument);
  });

  it("returns null in editor mode while the scoped factory document is pending", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableDefinitionQuery: { status: "pending" },
          editorMode: true,
        }),
        snapshot,
      ),
    ).toBeNull();
  });

  it("returns the pending document definition in editor mode after the document succeeds", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);
    const draftState = createMockGraphEditorDraftState({
      latestDocument: baseFactoryDefinitionDocument,
      pendingFactoryDefinition: baseFactoryDefinitionDocument,
    });

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          draftState,
          editableDefinitionQuery: { status: "success" },
          editorMode: true,
        }),
        snapshot,
      ),
    ).toEqual(baseFactoryDefinitionDocument);
  });
});
