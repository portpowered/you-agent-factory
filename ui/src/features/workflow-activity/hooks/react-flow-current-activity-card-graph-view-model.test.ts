import { act, renderHook } from "@testing-library/react";
import type { NodeChange } from "@xyflow/react";
import { describe, expect, it } from "vitest";

import { singleNodeDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  baseFactoryDefinition,
  baseFactoryDefinitionDocument,
  buildDivergentPlaneDashboardSnapshot,
  createMockGraphEditorDraftState,
  divergentDocumentPlaneFactoryDocument,
} from "../../../testing/graph-editor-harness";
import { sessionFactoryDocumentFromSnapshot } from "../../../testing/session-factory-mocks";
import type { GraphLayout } from "../../flowchart/lib/layout";
import { currentActivityCardFactoryDefinition } from "./current-activity-card-factory-definition";
import { useCurrentActivityGraphViewModel } from "./react-flow-current-activity-card-graph-view-model";

function createEditorStub(
  overrides: {
    draftState?: ReturnType<typeof createMockGraphEditorDraftState>;
    editableFactoryDocument?: typeof baseFactoryDefinitionDocument;
    editableFactoryDocumentStatus?: "error" | "pending" | "success";
    editorMode?: boolean;
  } = {},
) {
  const editableFactoryDocument =
    "editableFactoryDocument" in overrides
      ? overrides.editableFactoryDocument
      : baseFactoryDefinitionDocument;

  return {
    draftState: overrides.draftState ?? createMockGraphEditorDraftState(),
    editableFactoryDocument,
    editableFactoryDocumentStatus:
      overrides.editableFactoryDocumentStatus ?? "success",
    editorMode: overrides.editorMode ?? false,
  } as Parameters<typeof currentActivityCardFactoryDefinition>[0];
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: this suite keeps the factory selection contract cases together.
describe("currentActivityCardFactoryDefinition", () => {
  it("returns the timeline snapshot in observe mode while the scoped factory document is pending", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableFactoryDocumentStatus: "pending",
          editorMode: false,
        }),
        snapshot,
      ),
    ).toEqual(snapshot.factory);
  });

  it("returns the event-computed snapshot factory in observe mode while shared draft state has a saved document", () => {
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
          editableFactoryDocument: undefined,
          editableFactoryDocumentStatus: "pending",
          editorMode: false,
        }),
        snapshot,
      ),
    ).toEqual(snapshot.factory);
  });

  it("returns the event-computed snapshot factory in observe mode once the scoped factory document succeeds", () => {
    const snapshot = buildDivergentPlaneDashboardSnapshot();

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableFactoryDocument: divergentDocumentPlaneFactoryDocument,
          editableFactoryDocumentStatus: "success",
          editorMode: false,
        }),
        snapshot,
      ),
    ).toEqual(snapshot.factory);
  });

  it("returns null in editor mode while the scoped factory document is pending", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableFactoryDocumentStatus: "pending",
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
          editableFactoryDocumentStatus: "success",
          editorMode: true,
        }),
        snapshot,
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
          editableFactoryDocument: savedAfterTopologyChange,
          editableFactoryDocumentStatus: "success",
          editorMode: true,
        }),
        snapshot,
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
          editableFactoryDocument: savedDocument,
          editableFactoryDocumentStatus: "success",
          editorMode: false,
        }),
        snapshot,
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
          editableFactoryDocument: savedDocument,
          editableFactoryDocumentStatus: "success",
          editorMode: false,
        }),
        snapshot,
      ),
    ).toMatchObject({
      layout: snapshot.factory.layout,
      workstations: savedDocument.workstations,
    });
  });
});

describe("useCurrentActivityGraphViewModel node state", () => {
  it("keeps node positions from the graph projection instead of React Flow presentation changes", () => {
    const graphLayout: GraphLayout = {
      edges: [],
      height: 360,
      nodes: [
        {
          column: 0,
          height: 120,
          nodeId: "work-state:story:queued",
          nodeKind: "state_position",
          place: {
            kind: "work_state",
            place_id: "story:queued",
            state_value: "queued",
            type_id: "story",
          },
          row: 0,
          width: 140,
          x: 120,
          y: 80,
        },
        {
          column: 1,
          height: 160,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 220,
          workstationNodeId: "review",
          x: 360,
          y: 120,
        },
      ],
      width: 600,
    };
    const editor = {
      activeTool: null,
      canInteractWithEditor: true,
      editorMode: true,
      graphProjection: {
        canonicalLayoutViewport: null,
        displayFactoryDefinition: baseFactoryDefinition,
        graphLayout,
        pendingAdditionEdgeIds: new Set<string>(),
        positionedGraphLayout: graphLayout,
        visibleGraphEdges: [],
      },
      handleConnectionAnchorClick: vi.fn(),
      pendingConnectionSource: null,
      validationTargets: [],
    };
    const snapshot = {
      ...structuredClone(singleNodeDashboardSnapshot),
      factory: baseFactoryDefinition,
      runtime: {
        ...singleNodeDashboardSnapshot.runtime,
        in_flight_dispatch_count: 0,
      },
    };
    const noop = vi.fn();
    const { result } = renderHook(() =>
      useCurrentActivityGraphViewModel({
        editor: editor as Parameters<
          typeof useCurrentActivityGraphViewModel
        >[0]["editor"],
        now: 0,
        onSelectDoc: noop,
        onSelectResource: noop,
        onSelectStateNode: noop,
        onSelectWorkID: noop,
        onSelectWorker: noop,
        onSelectWorkType: noop,
        onSelectWorkstation: noop,
        selection: null,
        snapshot,
      }),
    );
    const workStateNode = () =>
      result.current.nodes.find(
        (node) => node.id === "work-state:story:queued",
      );

    expect(workStateNode()?.position).toEqual({ x: 120, y: 80 });

    act(() => {
      result.current.handleNodesChange([
        {
          id: "work-state:story:queued",
          position: { x: 999, y: 999 },
          type: "position",
        },
        {
          id: "work-state:story:queued",
          selected: true,
          type: "select",
        },
      ] satisfies NodeChange[]);
    });

    expect(workStateNode()).toMatchObject({
      position: { x: 120, y: 80 },
      selected: true,
    });
  });
});
