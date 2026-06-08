// biome-ignore lint/nursery/noExcessiveLinesPerFile: shared graph-editor fixtures and editable-graph test helpers stay colocated so hook and card tests import one stable harness seam.
import type { Mock } from "vitest";
import { vi } from "vitest";

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryDocument,
} from "../api/current-factory-definition";
import type { DashboardSnapshot } from "../api/dashboard/types";
import { singleNodeDashboardSnapshot } from "../components/dashboard/test-fixtures";
import { useFactoryDocumentSave } from "../features/current-factory-definition/hooks/useFactoryDocumentSave";
import type { FactoryDocumentSaveState } from "../features/current-selection/base/hooks/factory-document-save-types";
import type {
  EditableFactoryGraphViewModel,
  UseEditableFactoryGraphOptions,
} from "../features/factory-graph-editor/hooks/use-editable-factory-graph-types";
import { currentFactoryDocument } from "../features/factory-graph-editor/lib/draft/factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "../features/factory-graph-editor/lib/draft/factory-graph-draft-graph";
import {
  createEmptyFactoryGraphDraft,
  type FactoryGraphDraftDerivedState,
} from "../features/factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  addFactoryLayoutEdgeWaypoint,
  moveFactoryLayoutEdgeWaypoint,
} from "../features/factory-graph-editor/lib/layout/factory-graph-layout-edge-waypoints";
import {
  createDefaultFactoryLayout,
  moveFactoryLayoutNode,
} from "../features/factory-graph-editor/lib/layout/factory-graph-layout-operations";
import {
  addFactoryGraphNode,
  applyFactoryGraphPendingEdits,
  connectFactoryGraphNodes,
  disconnectFactoryGraphEdge,
  removeFactoryGraphNode,
} from "../features/factory-graph-editor/lib/operations/factory-graph-operations";

export const baseFactoryDefinition: CanonicalFactoryDefinition = {
  name: "Current Factory",
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workstations: [
    {
      body: "Review the work item.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "review",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
        {
          name: "done",
          type: "TERMINAL",
        },
      ],
    },
  ],
};

export const baseFactoryDefinitionDocument: CurrentFactoryDocument = {
  ...baseFactoryDefinition,
  version: {
    logical: "8",
    physical: "2026-05-18T15:32:00Z",
  },
};

export const workerDenseFactoryDefinitionDocument: CurrentFactoryDocument = {
  ...baseFactoryDefinition,
  resources: [
    {
      capacity: 2,
      name: "gpu",
    },
  ],
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
    {
      model: "gpt-5",
      name: "reviewer",
      type: "MODEL_WORKER",
    },
    {
      model: "gpt-5",
      name: "stalled",
      type: "MODEL_WORKER",
    },
  ],
  workstations: [
    {
      body: "Draft the story.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "draft",
      outputs: [
        {
          state: "review",
          workType: "story",
        },
      ],
      resources: [{ capacity: 1, name: "gpu" }],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
    {
      body: "Review the draft.",
      inputs: [
        {
          state: "review",
          workType: "story",
        },
      ],
      name: "review",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "reviewer",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
        {
          name: "review",
          type: "PROCESSING",
        },
        {
          name: "done",
          type: "TERMINAL",
        },
      ],
    },
  ],
  version: {
    logical: "9",
    physical: "2026-05-19T01:12:00Z",
  },
};

export type MockGraphEditorDraftState = FactoryGraphDraftDerivedState & {
  documentSave: FactoryDocumentSaveState;
  replaceDraft: Mock<(draft: FactoryGraphDraftDerivedState["draft"]) => void>;
  resetDraft: Mock<() => void>;
  updateDraft: Mock<
    (
      updater: (
        draft: FactoryGraphDraftDerivedState["draft"],
      ) => FactoryGraphDraftDerivedState["draft"],
    ) => void
  >;
};

const staleDraftDocumentSaveState: FactoryDocumentSaveState = {
  message:
    "The factory definition changed while you were editing. Refresh or discard your draft before saving.",
  status: "warning",
};

export function buildMockGraphSavePayload(
  draftState: Pick<MockGraphEditorDraftState, "draft" | "latestDocument">,
): {
  baseVersion: CurrentFactoryDocument["version"];
  factory: CanonicalFactoryDefinition;
} {
  const latestDocument =
    draftState.latestDocument ?? baseFactoryDefinitionDocument;
  const saveInput = applyFactoryGraphPendingEdits({
    baseFactoryDefinition: latestDocument,
    draft: draftState.draft,
  });

  if (!saveInput.ok) {
    throw new Error("Expected mock graph draft to produce a save payload.");
  }

  return {
    baseVersion: latestDocument.version,
    factory: saveInput.value,
  };
}

function resolveMockGraphDocumentSaveState(
  draftState: MockGraphEditorDraftState,
  isStale: boolean,
): FactoryDocumentSaveState {
  if (draftState.documentSave.status === "confirming") {
    return { status: "confirming" };
  }

  if (isStale) {
    return staleDraftDocumentSaveState;
  }

  return draftState.documentSave;
}

export interface MockEditableFactoryGraphHooks {
  useEditableFactoryGraph: Mock<
    (options: UseEditableFactoryGraphOptions) => EditableFactoryGraphViewModel
  >;
  useFactoryGraphDraftState: Mock<
    (options: UseEditableFactoryGraphOptions) => MockGraphEditorDraftState
  >;
}

export function createMockGraphEditorDraftState(
  overrides: Partial<MockGraphEditorDraftState> = {},
): MockGraphEditorDraftState {
  return {
    baseDocument: baseFactoryDefinitionDocument,
    draft: createEmptyFactoryGraphDraft(),
    graph: {
      edges: [],
      nodes: [],
    },
    hasChanges: false,
    latestDocument: baseFactoryDefinitionDocument,
    pendingFactoryDefinition: baseFactoryDefinition,
    adoptSavedFactoryDocument: vi.fn(),
    replaceDraft: vi.fn(),
    resetDraft: vi.fn(),
    source: "current-factory",
    updateDraft: vi.fn(),
    validationErrors: [],
    documentSave: overrides.documentSave ?? { status: "idle" },
    ...overrides,
  };
}

/** Draft-state double for `useEditableFactoryGraph` hook tests with mutable replace/reset. */
export function createHookTestGraphEditorDraftState(
  overrides: Partial<MockGraphEditorDraftState> = {},
): MockGraphEditorDraftState {
  const document =
    overrides.latestDocument ??
    overrides.baseDocument ??
    currentFactoryDocument;
  const draft = overrides.draft ?? createEmptyFactoryGraphDraft();
  const state = createMockGraphEditorDraftState({
    baseDocument: document,
    draft,
    graph: buildFactoryGraphTopologyFromDefinition(document),
    hasChanges: overrides.hasChanges ?? false,
    latestDocument: document,
    pendingFactoryDefinition: overrides.pendingFactoryDefinition ?? document,
    source: overrides.source ?? "current-factory",
    updateDraft: overrides.updateDraft ?? vi.fn(),
    validationErrors: overrides.validationErrors ?? [],
    ...overrides,
  });

  if (overrides.replaceDraft === undefined) {
    state.replaceDraft = vi.fn((nextDraft) => {
      state.draft = nextDraft;
      state.hasChanges = true;
    });
  }

  if (overrides.resetDraft === undefined) {
    state.resetDraft = vi.fn(() => {
      state.draft = createEmptyFactoryGraphDraft();
      state.hasChanges = false;
    });
  }

  return state;
}

function createMockLayoutDraftState() {
  const baseLayout = createDefaultFactoryLayout();
  const state = {
    adoptSavedLayout: vi.fn((layout: typeof baseLayout) => {
      state.baseLayout = structuredClone(layout);
      state.layout = structuredClone(layout);
      state.hasChanges = false;
      state.layoutDirty = false;
    }),
    baseLayout,
    canRedoLayout: false,
    canUndoLayout: false,
    hasChanges: false,
    layout: structuredClone(baseLayout),
    layoutDirty: false,
    addEdgeWaypoint: vi.fn(
      (
        edgeId: string,
        position: { x: number; y: number },
        insertIndex?: number,
      ) => {
        state.layout = addFactoryLayoutEdgeWaypoint(
          state.layout,
          edgeId,
          position,
          insertIndex,
        );
        state.hasChanges = true;
        state.layoutDirty = true;
      },
    ),
    moveEdgeWaypoint: vi.fn(
      (
        edgeId: string,
        waypointIndex: number,
        position: { x: number; y: number },
      ) => {
        state.layout = moveFactoryLayoutEdgeWaypoint(
          state.layout,
          edgeId,
          waypointIndex,
          position,
        );
        state.hasChanges = true;
        state.layoutDirty = true;
      },
    ),
    moveNode: vi.fn((nodeId: string, position: { x: number; y: number }) => {
      state.layout = moveFactoryLayoutNode(state.layout, nodeId, position);
      state.hasChanges = true;
      state.layoutDirty = true;
    }),
    moveNodesByDelta: vi.fn(),
    pruneLayoutHistoryForNodeIds: vi.fn(),
    redoLayout: vi.fn(),
    replaceLayout: vi.fn(),
    resetLayout: vi.fn(() => {
      state.layout = structuredClone(state.baseLayout);
      state.hasChanges = false;
      state.layoutDirty = false;
    }),
    undoLayout: vi.fn(),
    updateViewport: vi.fn(),
  };

  return state;
}

function createMockEditableFactoryGraphActions(
  options: UseEditableFactoryGraphOptions,
  draftState: MockGraphEditorDraftState,
  layoutDraftState: ReturnType<typeof createMockLayoutDraftState>,
  baseDefinition: CurrentFactoryDocument | null,
  activeWorkCount: number,
): EditableFactoryGraphViewModel["actions"] {
  return {
    addNode: (node) => {
      const result = baseDefinition
        ? addFactoryGraphNode({
            baseFactoryDefinition: baseDefinition,
            draft: draftState.draft,
            node,
          })
        : {
            message: "Load the current factory before editing graph nodes.",
            ok: false as const,
            reason: "INVALID_FIELD" as const,
          };
      if (result.ok) {
        draftState.replaceDraft(result.value);
      }
      return result;
    },
    connectNodes: (connection) => {
      const result = baseDefinition
        ? connectFactoryGraphNodes({
            baseFactoryDefinition: baseDefinition,
            draft: draftState.draft,
            ...connection,
          })
        : {
            message: "Load the current factory before connecting graph nodes.",
            ok: false as const,
            reason: "INVALID_CONNECTION" as const,
          };
      if (result.ok) {
        draftState.replaceDraft(result.value);
      }
      return result;
    },
    discard: () => {
      draftState.resetDraft();
      layoutDraftState.resetLayout();
    },
    disconnectEdge: (edgeId) => {
      const result = baseDefinition
        ? disconnectFactoryGraphEdge({
            baseFactoryDefinition: baseDefinition,
            draft: draftState.draft,
            edgeId,
          })
        : {
            message:
              "Load the current factory before disconnecting graph edges.",
            ok: false as const,
            reason: "UNKNOWN_EDGE" as const,
          };
      if (result.ok) {
        draftState.replaceDraft(result.value);
      }
      return result;
    },
    removeNode: (nodeId) => {
      const result = baseDefinition
        ? removeFactoryGraphNode({
            baseFactoryDefinition: baseDefinition,
            draft: draftState.draft,
            nodeId,
          })
        : {
            message: "Load the current factory before removing graph nodes.",
            ok: false as const,
            reason: "NODE_NOT_FOUND" as const,
          };
      if (result.ok) {
        draftState.replaceDraft(result.value);
      }
      return result;
    },
    addEdgeWaypoint: layoutDraftState.addEdgeWaypoint,
    moveEdgeWaypoint: layoutDraftState.moveEdgeWaypoint,
    moveLayoutNode: layoutDraftState.moveNode,
    moveLayoutNodesByDelta: layoutDraftState.moveNodesByDelta,
    resetLayout: layoutDraftState.resetLayout,
    redoLayout: layoutDraftState.redoLayout,
    undoLayout: layoutDraftState.undoLayout,
    save: async () => {
      const hasPendingEdits =
        draftState.hasChanges || layoutDraftState.hasChanges;
      if (
        options.factoryDocumentScopeKey == null ||
        !hasPendingEdits ||
        !draftState.pendingFactoryDefinition ||
        !draftState.latestDocument ||
        activeWorkCount > 0
      ) {
        return false;
      }

      const saveInput = applyFactoryGraphPendingEdits({
        baseFactoryDefinition: draftState.latestDocument,
        draft: draftState.draft,
        pendingLayout: layoutDraftState.layout,
      });
      if (!saveInput.ok) {
        return false;
      }

      const { saveAsync } = vi.mocked(useFactoryDocumentSave)();
      await saveAsync({
        baseVersion: draftState.latestDocument.version,
        factory: saveInput.value,
      });
      draftState.replaceDraft(createEmptyFactoryGraphDraft());
      layoutDraftState.adoptSavedLayout(
        saveInput.value.layout ?? createDefaultFactoryLayout(),
      );
      return true;
    },
    updateLayoutViewport: layoutDraftState.updateViewport,
    updateNodeField: () => ({
      message: "Field editing is not exercised by this component test.",
      ok: false,
      reason: "INVALID_FIELD",
    }),
  };
}

export function createMockEditableFactoryGraph(
  options: UseEditableFactoryGraphOptions,
  draftState: MockGraphEditorDraftState,
): EditableFactoryGraphViewModel {
  const baseDefinition =
    draftState.latestDocument ?? draftState.baseDocument ?? null;
  const layoutDraftState = createMockLayoutDraftState();
  const activeWorkCount = options.activeWorkCount ?? 0;
  const isStale =
    draftState.hasChanges &&
    draftState.baseDocument?.version !== undefined &&
    draftState.latestDocument?.version !== undefined &&
    (draftState.baseDocument.version.logical !==
      draftState.latestDocument.version.logical ||
      draftState.baseDocument.version.physical !==
        draftState.latestDocument.version.physical);

  return {
    actions: createMockEditableFactoryGraphActions(
      options,
      draftState,
      layoutDraftState,
      baseDefinition,
      activeWorkCount,
    ),
    blockedOperation: null,
    draftState,
    graphState: null,
    layoutDraftState,
    pendingState: {
      canRedoLayout: layoutDraftState.canRedoLayout,
      canUndoLayout: layoutDraftState.canUndoLayout,
      dirtyState: {
        layoutDirty: layoutDraftState.layoutDirty,
        preferencesDirty: false,
        topologyDirty: draftState.hasChanges,
      },
      hasChanges: draftState.hasChanges || layoutDraftState.hasChanges,
      hasLayoutChanges: layoutDraftState.hasChanges,
      hasPortableDocumentChanges:
        draftState.hasChanges || layoutDraftState.hasChanges,
      hasPreferenceChanges: false,
      hasTopologyChanges: draftState.hasChanges,
      layoutDirty: layoutDraftState.layoutDirty,
      pendingFactoryDefinition: draftState.pendingFactoryDefinition,
      preferencesDirty: false,
      topologyDirty: draftState.hasChanges,
    },
    projection: {
      edges: [],
      nodes: [],
    },
    documentSaveControls: {
      beginConfirmation: vi.fn(() => {
        draftState.documentSave = { status: "confirming" };
      }),
      cancelConfirmation: vi.fn(() => {
        draftState.documentSave = { status: "idle" };
      }),
      clearSaveFeedback: vi.fn(() => {
        draftState.documentSave = { status: "idle" };
      }),
    },
    saveMutation: {
      error: null,
      isPending: false,
      reset: vi.fn(),
    },
    saveState: {
      get canSave() {
        return (
          options.factoryDocumentScopeKey != null &&
          (draftState.hasChanges || layoutDraftState.hasChanges) &&
          draftState.pendingFactoryDefinition !== null &&
          draftState.latestDocument !== null &&
          activeWorkCount === 0 &&
          !isStale
        );
      },
      get documentSave() {
        return resolveMockGraphDocumentSaveState(draftState, isStale);
      },
      isStale,
    },
    validationState: {
      errors: draftState.validationErrors,
      isValid: draftState.validationErrors.length === 0,
    },
  };
}

const divergentPlaneStoryWorkType = {
  name: "story",
  states: [
    {
      name: "queued",
      type: "INITIAL" as const,
    },
    {
      name: "done",
      type: "TERMINAL" as const,
    },
  ],
};

const divergentDocumentOnlyWorkstation = {
  body: "From the persisted document plane.",
  inputs: [
    {
      state: "queued",
      workType: "story",
    },
  ],
  id: "document-only",
  name: "Document Only",
  outputs: [
    {
      state: "done",
      workType: "story",
    },
  ],
  type: "MODEL_WORKSTATION" as const,
  worker: "writer",
};

const divergentSnapshotOnlyWorkstation = {
  body: "Only present on the live snapshot plane.",
  inputs: [
    {
      state: "queued",
      workType: "story",
    },
  ],
  id: "snapshot-only",
  name: "Snapshot Only",
  outputs: [
    {
      state: "done",
      workType: "story",
    },
  ],
  type: "MODEL_WORKSTATION" as const,
  worker: "writer",
};

/** Factory document with a workstation absent from a divergent snapshot-only node. */
export const divergentDocumentPlaneFactoryDocument: CurrentFactoryDocument = {
  ...baseFactoryDefinition,
  name: "Document Factory",
  version: {
    logical: "7",
    physical: "2026-05-31T12:00:00Z",
  },
  workTypes: [divergentPlaneStoryWorkType],
  workstations: [divergentDocumentOnlyWorkstation],
};

/** Snapshot factory that adds a workstation missing from the document plane. */
export function buildDivergentSnapshotPlaneFactory(): CanonicalFactoryDefinition {
  return {
    ...divergentDocumentPlaneFactoryDocument,
    name: "Snapshot Factory",
    workstations: [
      ...(divergentDocumentPlaneFactoryDocument.workstations ?? []),
      divergentSnapshotOnlyWorkstation,
    ],
  };
}

export function buildDivergentPlaneDashboardSnapshot(): DashboardSnapshot {
  const snapshot = structuredClone(singleNodeDashboardSnapshot);
  snapshot.factory = buildDivergentSnapshotPlaneFactory();
  snapshot.topology = {
    edges: [],
    workstation_node_ids: [],
    workstation_nodes_by_id: {},
  };

  return snapshot;
}

export function wireMockEditableFactoryGraph(
  hooks: MockEditableFactoryGraphHooks,
  draftState: MockGraphEditorDraftState = createMockGraphEditorDraftState(),
): MockGraphEditorDraftState {
  hooks.useFactoryGraphDraftState.mockReturnValue(draftState);
  hooks.useEditableFactoryGraph.mockImplementation((options) =>
    createMockEditableFactoryGraph(
      options,
      hooks.useFactoryGraphDraftState(options) as MockGraphEditorDraftState,
    ),
  );
  return draftState;
}
