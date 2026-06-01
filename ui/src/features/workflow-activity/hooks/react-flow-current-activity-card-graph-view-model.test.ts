import { describe, expect, it } from "vitest";

import { singleNodeDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  baseFactoryDefinitionDocument,
  buildDivergentPlaneDashboardSnapshot,
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
  it("returns the timeline snapshot in observe mode while the scoped factory document is pending", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableDefinitionQuery: { status: "pending" },
          editorMode: false,
        }),
        snapshot,
        "current",
      ),
    ).toEqual(snapshot.factory);
  });

  it("returns the saved factory document in observe mode once the scoped factory document succeeds", () => {
    const snapshot = buildDivergentPlaneDashboardSnapshot();

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
        "current",
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
        "current",
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
        "current",
      ),
    ).toEqual(baseFactoryDefinitionDocument);
  });

  it("keeps the pending draft definition in editor mode when a topology save updates the saved document", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);
    const pendingDraft = {
      ...baseFactoryDefinitionDocument,
      workers: [
        ...(baseFactoryDefinitionDocument.workers ?? []),
        {
          model: "gpt-5-mini",
          name: "reviewer",
          type: "MODEL_WORKER" as const,
        },
      ],
    };
    const savedAfterTopologyChange = {
      ...baseFactoryDefinitionDocument,
      version: {
        logical: "10",
        physical: "2026-06-01T12:00:00Z",
      },
      workstations: [
        {
          ...(baseFactoryDefinitionDocument.workstations?.[0] ?? {
            inputs: [],
            name: "draft",
            outputs: [],
            type: "MODEL_WORKSTATION" as const,
          }),
          worker: "reviewer",
        },
      ],
    };
    const draftState = createMockGraphEditorDraftState({
      baseDocument: baseFactoryDefinitionDocument,
      hasChanges: true,
      latestDocument: savedAfterTopologyChange,
      pendingFactoryDefinition: pendingDraft,
    });

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          draftState,
          editableDefinitionQuery: {
            data: savedAfterTopologyChange,
            status: "success",
          },
          editorMode: true,
        }),
        snapshot,
        "current",
      ),
    ).toEqual(pendingDraft);
  });
});
