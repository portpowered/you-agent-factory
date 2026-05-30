import type { Mock } from "vitest";
import { vi } from "vitest";

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryDocument,
} from "../api/current-factory-definition";
import type {
  EditableFactoryGraphViewModel,
  UseEditableFactoryGraphOptions,
} from "../features/factory-graph-editor/hooks/use-editable-factory-graph-types";
import {
  createEmptyFactoryGraphDraft,
  type FactoryGraphDraftDerivedState,
} from "../features/factory-graph-editor/lib/factory-graph-draft-types";
import {
  addFactoryGraphNode,
  connectFactoryGraphNodes,
  disconnectFactoryGraphEdge,
  removeFactoryGraphNode,
} from "../features/factory-graph-editor/lib/factory-graph-operations";

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
    replaceDraft: vi.fn(),
    resetDraft: vi.fn(),
    source: "current-factory",
    updateDraft: vi.fn(),
    validationErrors: [],
    ...overrides,
  };
}

function createMockEditableFactoryGraphActions(
  options: UseEditableFactoryGraphOptions,
  draftState: MockGraphEditorDraftState,
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
    discard: draftState.resetDraft,
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
    save: async () => {
      if (
        !options.saveFactoryDefinition ||
        !draftState.hasChanges ||
        !draftState.pendingFactoryDefinition ||
        !draftState.latestDocument ||
        activeWorkCount > 0
      ) {
        return false;
      }
      await options.saveFactoryDefinition({
        baseVersion: draftState.latestDocument.version,
        factoryDefinition: draftState.pendingFactoryDefinition,
      });
      draftState.replaceDraft(createEmptyFactoryGraphDraft());
      return true;
    },
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
      baseDefinition,
      activeWorkCount,
    ),
    blockedOperation: null,
    draftState,
    graphState: null,
    pendingState: {
      hasChanges: draftState.hasChanges,
      pendingFactoryDefinition: draftState.pendingFactoryDefinition,
    },
    projection: {
      edges: [],
      nodes: [],
    },
    saveState: {
      canSave:
        Boolean(options.saveFactoryDefinition) &&
        draftState.hasChanges &&
        draftState.pendingFactoryDefinition !== null &&
        draftState.latestDocument !== null &&
        activeWorkCount === 0 &&
        !isStale,
      isSaving: false,
      isStale,
      lastError: null,
      lastSuccess: false,
    },
    validationState: {
      errors: draftState.validationErrors,
      isValid: draftState.validationErrors.length === 0,
    },
  };
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
