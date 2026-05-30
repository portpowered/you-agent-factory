import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import type { FactoryValidationResult } from "../../../api/factory-validation";
import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import { connectFactoryGraphNodes } from "../../factory-graph-editor/public";
import { baseFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft.test-helpers";
import { buildDraftAppliedFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-apply";
import { FACTORY_VALIDATION_DEBOUNCE_MS } from "../../factory-graph-editor/hooks/use-factory-validation";
import { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";

const validationFixtures = vi.hoisted(() => {
  const repeaterWithoutRejectRoute: FactoryValidationResult = {
    targets: [
      {
        code: "factory.workstation.missingRejectionRoute",
        message: "Workstation repeater must define a reject route.",
        severity: "error",
        subject: {
          id: "repeater",
          location: "ON_REJECTION",
          type: "WORKSTATION",
        },
      },
    ],
  };
  const validFactory: FactoryValidationResult = {
    targets: [],
  };
  const disconnectedFailureRoute: FactoryValidationResult = {
    targets: [
      {
        code: "factory.workstation.missingFailureRoute",
        message: "Workstation draft must define a failure route.",
        severity: "error",
        subject: {
          id: "draft",
          location: "ON_FAILURE",
          type: "WORKSTATION",
        },
      },
    ],
  };

  return {
    disconnectedFailureRoute,
    repeaterWithoutRejectRoute,
    validFactory,
  };
});

const editableDocument: CanonicalFactoryDefinition & {
  version: { logical: string; physical: string };
} = {
  ...baseFactoryDefinition,
  version: {
    logical: "4",
    physical: "2026-05-25T00:00:00Z",
  },
};

function createEmptyDraft() {
  return {
    additions: {
      resources: [],
      workers: [],
      workStates: [],
      workTypes: [],
      workstations: [],
    },
    edgeChanges: {
      additions: [],
      removals: [],
    },
    removals: {
      resources: [],
      workers: [],
      workStates: [],
      workTypes: [],
      workstations: [],
    },
  };
}

const hookState = vi.hoisted(() => ({
  addEntityController: {
    reset: vi.fn(),
  },
  connectionController: {
    blockedRemovalReason: null,
    connectionNotice: null as string | null,
    handleConnectionAnchorClick: vi.fn(),
    handleEditorConnect: vi.fn(),
    pendingConnectionSource: null,
    setBlockedRemovalReason: vi.fn(),
    setConnectionNotice: vi.fn(),
  },
  currentFactoryQuery: {
    data: null as CanonicalFactoryDefinition | null,
    status: "success" as const,
  },
  draftState: {
    baseDocument: null as CanonicalFactoryDefinition | null,
    draft: {
      additions: {
        resources: [],
        workers: [],
        workStates: [],
        workTypes: [],
        workstations: [],
      },
      edgeChanges: {
        additions: [],
        removals: [],
      },
      removals: {
        resources: [],
        workers: [],
        workStates: [],
        workTypes: [],
        workstations: [],
      },
    },
    hasChanges: false,
    latestDocument: null as CanonicalFactoryDefinition | null,
    pendingFactoryDefinition: null as CanonicalFactoryDefinition | null,
    replaceDraft: vi.fn(),
    resetDraft: vi.fn(),
    validationErrors: [],
  },
  removalController: {
    handleConfirmRemoval: vi.fn(),
    handleEditorEdgeDelete: vi.fn(),
    handleEditorNodeDelete: vi.fn(),
    pendingRemovalIntent: null,
    setPendingRemovalEdgeId: vi.fn(),
    setPendingRemovalNodeId: vi.fn(),
  },
  saveEditableDefinition: {
    error: null,
    mutateAsync: vi.fn(async () => undefined),
    reset: vi.fn(),
    status: "idle" as const,
  },
  unsupportedFromDefinition: undefined as string | undefined,
}));

function resetHookState() {
  hookState.currentFactoryQuery = {
    data: editableDocument,
    status: "success",
  };
  hookState.draftState = {
    baseDocument: editableDocument,
    draft: createEmptyDraft(),
    hasChanges: false,
    latestDocument: editableDocument,
    pendingFactoryDefinition: baseFactoryDefinition,
    replaceDraft: vi.fn(),
    resetDraft: vi.fn(),
    validationErrors: [],
  };
  hookState.unsupportedFromDefinition = undefined;
}

vi.mock("../../../api/factory-validation", async () => {
  const actual = await vi.importActual<
    typeof import("../../../api/factory-validation")
  >("../../../api/factory-validation");

  return {
    ...actual,
    validateFactoryDefinition: vi.fn(),
  };
});

vi.mock("../../current-factory-definition/public", () => ({
  useCurrentFactoryDocument: () => hookState.currentFactoryQuery,
  useSaveCurrentFactory: () => hookState.saveEditableDefinition,
}));

vi.mock("../../factory-graph-editor/hooks/use-editable-factory-graph", async () => {
  const actual = await vi.importActual<
    typeof import("../../factory-graph-editor/hooks/use-editable-factory-graph")
  >("../../factory-graph-editor/hooks/use-editable-factory-graph");

  return {
    ...actual,
    useEditableFactoryGraph: () => ({
      actions: {
        discard: hookState.draftState.resetDraft,
        save: vi.fn(async () => false),
      },
      draftState: hookState.draftState,
      saveState: {
        canSave: hookState.draftState.hasChanges,
        isStale: false,
      },
    }),
  };
});

vi.mock(
  "../../factory-graph-editor/lib/factory-graph-editor-additions",
  () => ({
    buildFactoryGraphAddEntityMenuActions: () => [],
  }),
);

vi.mock(
  "../../factory-graph-editor/lib/factory-graph-editor-save-summary",
  () => ({
    buildFactoryGraphSaveSummary: () => ({
      additions: [],
      removals: [],
    }),
  }),
);

vi.mock("../components/react-flow-current-activity-card-editor-chrome", () => ({
  useFactoryGraphAddEntityController: () => hookState.addEntityController,
}));

vi.mock("./factory-graph-editor-availability", () => ({
  findClassifierGraphEditorUnsupportedWorkstationName: () =>
    hookState.unsupportedFromDefinition,
}));

vi.mock("./react-flow-current-activity-card-editor-connections", () => ({
  useFactoryGraphConnectionController: () => hookState.connectionController,
}));

vi.mock("./react-flow-current-activity-card-editor-removals", () => ({
  useFactoryGraphRemovalController: () => hookState.removalController,
}));

import { validateFactoryDefinition } from "../../../api/factory-validation";

function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
}

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

function applyDraftMutation(
  draft: ReturnType<typeof createEmptyDraft>,
  pendingFactoryDefinition: CanonicalFactoryDefinition,
) {
  hookState.draftState = {
    ...hookState.draftState,
    draft,
    hasChanges: true,
    pendingFactoryDefinition,
  };
}

describe("useCurrentActivityGraphEditor live validation refresh", () => {
  beforeEach(() => {
    resetHookState();
    vi.mocked(validateFactoryDefinition).mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("revalidates the draft-applied factory after an add mutation without clearing pending edits", async () => {
    vi.mocked(validateFactoryDefinition)
      .mockResolvedValueOnce(validationFixtures.validFactory)
      .mockResolvedValueOnce(validationFixtures.repeaterWithoutRejectRoute);

    const queryClient = createQueryClient();
    const repeaterDraft = createEmptyDraft();
    repeaterDraft.additions.workstations.push({
      inputs: [],
      name: "repeater",
      outputs: [],
      type: "REPEATER_WORKSTATION",
      worker: "writer",
    });
    const pendingAfterAdd = buildDraftAppliedFactoryDefinition(
      baseFactoryDefinition,
      repeaterDraft,
    );

    const { rerender, result } = renderHook(
      () => useCurrentActivityGraphEditor(semanticWorkflowDashboardSnapshot),
      { wrapper: createWrapper(queryClient) },
    );

    act(() => {
      result.current.handleEditorModeToggle();
    });

    await waitFor(() => {
      expect(validateFactoryDefinition).toHaveBeenCalledTimes(1);
    });

    applyDraftMutation(repeaterDraft, pendingAfterAdd);
    rerender();
    await act(async () => {
      await new Promise<void>((resolve) => {
        window.setTimeout(resolve, FACTORY_VALIDATION_DEBOUNCE_MS + 50);
      });
    });

    await waitFor(
      () => {
        expect(validateFactoryDefinition).toHaveBeenCalledTimes(2);
        expect(result.current.structuralValidation.targets).toHaveLength(1);
      },
      { timeout: 1_000 },
    );

    expect(result.current.canInteractWithEditor).toBe(true);
    expect(result.current.draftState.pendingFactoryDefinition).toEqual(
      pendingAfterAdd,
    );
    expect(result.current.draftState.hasChanges).toBe(true);
  });

  it("revalidates after a connect mutation while editor interactions stay available", async () => {
    vi.mocked(validateFactoryDefinition)
      .mockResolvedValueOnce(validationFixtures.disconnectedFailureRoute)
      .mockResolvedValueOnce(validationFixtures.validFactory);

    const queryClient = createQueryClient();
    const disconnectedDraft = createEmptyDraft();
    const connectedDraft = connectFactoryGraphNodes({
      baseFactoryDefinition,
      draft: disconnectedDraft,
      sourceAnchorId: "workstation-on-failure-source",
      sourceNodeId: "workstation:draft",
      targetAnchorId: "work-state-input-target",
      targetNodeId: "work-state:story:done",
    });
    expect(connectedDraft.ok).toBe(true);
    const connectedDraftValue = connectedDraft.value;

    const pendingConnected = buildDraftAppliedFactoryDefinition(
      baseFactoryDefinition,
      connectedDraftValue,
    );

    hookState.draftState = {
      ...hookState.draftState,
      draft: disconnectedDraft,
      hasChanges: true,
      pendingFactoryDefinition: buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        disconnectedDraft,
      ),
    };

    const { rerender, result } = renderHook(
      () => useCurrentActivityGraphEditor(semanticWorkflowDashboardSnapshot),
      { wrapper: createWrapper(queryClient) },
    );

    act(() => {
      result.current.handleEditorModeToggle();
    });

    await waitFor(
      () => {
        expect(result.current.structuralValidation.targets).toHaveLength(1);
        expect(result.current.structuralValidation.targets[0]?.subject.location).toBe(
          "ON_FAILURE",
        );
      },
      { timeout: 1_000 },
    );

    applyDraftMutation(connectedDraftValue, pendingConnected);
    rerender();
    await act(async () => {
      await new Promise<void>((resolve) => {
        window.setTimeout(resolve, FACTORY_VALIDATION_DEBOUNCE_MS + 50);
      });
    });

    await waitFor(
      () => {
        expect(validateFactoryDefinition).toHaveBeenCalledTimes(2);
        expect(result.current.structuralValidation.targets).toEqual([]);
      },
      { timeout: 1_000 },
    );

    expect(result.current.canInteractWithEditor).toBe(true);
    expect(result.current.draftState.pendingFactoryDefinition).toEqual(
      pendingConnected,
    );
  });
});
