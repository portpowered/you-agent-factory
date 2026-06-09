import { describe, expect, it } from "vitest";

import { singleNodeDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  baseFactoryDefinitionDocument,
  buildDivergentPlaneDashboardSnapshot,
  createMockGraphEditorDraftState,
  divergentDocumentPlaneFactoryDocument,
} from "../../../testing/graph-editor-harness";
import { sessionFactoryDocumentFromSnapshot } from "../../../testing/session-factory-mocks";
import { currentActivityCardFactoryDefinition } from "./current-activity-card-factory-definition";

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
  const editableDefinitionQueryData =
    overrides.editableDefinitionQuery &&
    "data" in overrides.editableDefinitionQuery
      ? overrides.editableDefinitionQuery.data
      : baseFactoryDefinitionDocument;

  return {
    draftState: overrides.draftState ?? createMockGraphEditorDraftState(),
    editableDefinitionQuery: {
      data: editableDefinitionQueryData,
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

  it("returns the saved factory document in observe mode while the query is pending when shared draft state already has it", () => {
    const savedDocument = {
      ...baseFactoryDefinitionDocument,
      layout: {
        nodes: [
          {
            id: "workstation:draft",
            position: { x: 540, y: 260 },
          },
        ],
        schemaVersion: 1 as const,
        viewport: { x: 14, y: 18, zoom: 1.2 },
      },
    };
    const snapshot = {
      ...structuredClone(singleNodeDashboardSnapshot),
      factory: {
        ...savedDocument,
        layout: undefined,
      },
    };

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          draftState: createMockGraphEditorDraftState({
            baseDocument: savedDocument,
            latestDocument: savedDocument,
          }),
          editableDefinitionQuery: { data: undefined, status: "pending" },
          editorMode: false,
        }),
        snapshot,
        "current",
      ),
    ).toEqual(savedDocument);
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

describe("currentActivityCardFactoryDefinition bundled docs", () => {
  it("keeps bundled docs from the saved document when observe mode prefers the timeline snapshot", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);
    const savedDocument = {
      ...sessionFactoryDocumentFromSnapshot(snapshot),
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Overview" },
            targetPath: "factory/docs/overview.md",
            type: "DOC",
          },
        ],
      },
    };

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableDefinitionQuery: {
            data: savedDocument,
            status: "success",
          },
          editorMode: false,
        }),
        snapshot,
        "fixed",
      ),
    ).toMatchObject({
      supportingFiles: savedDocument.supportingFiles,
      workstations: snapshot.factory?.workstations,
    });
  });

  it("keeps snapshot layout in observe mode when the saved document omits it", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);
    if (!snapshot.factory) {
      throw new Error("expected snapshot factory fixture");
    }

    snapshot.factory.layout = {
      nodes: [
        {
          id: "workstation:draft",
          position: { x: 480, y: 240 },
        },
      ],
      schemaVersion: 1,
      viewport: { x: 10, y: 20, zoom: 1.25 },
    };

    const savedDocument = {
      ...sessionFactoryDocumentFromSnapshot(snapshot),
    };
    delete savedDocument.layout;

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableDefinitionQuery: {
            data: savedDocument,
            status: "success",
          },
          editorMode: false,
        }),
        snapshot,
        "current",
      ),
    ).toMatchObject({
      layout: snapshot.factory.layout,
      workstations: savedDocument.workstations,
    });
  });
});
