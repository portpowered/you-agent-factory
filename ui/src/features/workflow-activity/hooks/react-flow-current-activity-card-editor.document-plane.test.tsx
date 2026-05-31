import { act, renderHook } from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  buildDivergentPlaneDashboardSnapshot,
  buildDivergentSnapshotPlaneFactory,
  divergentDocumentPlaneFactoryDocument,
} from "../../../testing/graph-editor-harness";
import { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";

const fixtureState = vi.hoisted(() => ({
  currentFactoryQuery: {
    data: undefined as typeof divergentDocumentPlaneFactoryDocument | undefined,
    status: "success" as const,
  },
  draftState: {
    baseDocument: undefined as typeof divergentDocumentPlaneFactoryDocument | undefined,
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
    latestDocument: undefined as typeof divergentDocumentPlaneFactoryDocument | undefined,
    pendingFactoryDefinition: undefined as
      | typeof divergentDocumentPlaneFactoryDocument
      | undefined,
    replaceDraft: vi.fn(),
    resetDraft: vi.fn(),
    validationErrors: [] as string[],
  },
  saveEditableDefinition: {
    error: null,
    mutateAsync: vi.fn(async () => undefined),
    reset: vi.fn(),
    status: "idle" as const,
  },
}));

function resetDivergentDocumentFixtureState() {
  fixtureState.currentFactoryQuery = {
    data: divergentDocumentPlaneFactoryDocument,
    status: "success",
  };
  fixtureState.draftState = {
    ...fixtureState.draftState,
    baseDocument: divergentDocumentPlaneFactoryDocument,
    latestDocument: divergentDocumentPlaneFactoryDocument,
    pendingFactoryDefinition: divergentDocumentPlaneFactoryDocument,
  };
}

vi.mock("../../current-factory-definition/public", () => ({
  useCurrentFactoryDocument: () => fixtureState.currentFactoryQuery,
  useSaveCurrentFactory: () => fixtureState.saveEditableDefinition,
}));

vi.mock("../../factory-graph-editor/hooks/use-editable-factory-graph", () => ({
  useEditableFactoryGraph: () => ({
    actions: {
      discard: fixtureState.draftState.resetDraft,
      save: vi.fn(),
    },
    draftState: fixtureState.draftState,
    saveState: {
      canSave: false,
      isStale: false,
    },
  }),
}));

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
  useFactoryGraphAddEntityController: () => ({
    reset: vi.fn(),
  }),
}));

vi.mock("./react-flow-current-activity-card-editor-connections", () => ({
  useFactoryGraphConnectionController: () => ({
    blockedRemovalReason: null,
    connectionNotice: null,
    handleConnectionAnchorClick: vi.fn(),
    handleEditorConnect: vi.fn(),
    pendingConnectionSource: null,
    setBlockedRemovalReason: vi.fn(),
    setConnectionNotice: vi.fn(),
  }),
}));

vi.mock("./react-flow-current-activity-card-editor-removals", () => ({
  useFactoryGraphRemovalController: () => ({
    handleConfirmRemoval: vi.fn(),
    handleEditorEdgeDelete: vi.fn(),
    handleEditorNodeDelete: vi.fn(),
    pendingRemovalIntent: null,
    setPendingRemovalEdgeId: vi.fn(),
    setPendingRemovalNodeId: vi.fn(),
  }),
}));

vi.mock("../../factory-graph-editor/hooks/use-factory-validation", () => ({
  useFactoryValidation: () => ({
    data: { targets: [] },
    isError: false,
    isFetching: false,
    isLoading: false,
    projection: {
      handleErrorsByAnchorId: new Map(),
      nodeErrorsByNodeId: new Map(),
    },
    targets: [],
  }),
}));

vi.mock("./react-flow-current-activity-card-editor-value", () => ({
  buildCurrentActivityGraphEditorValue: (value: Record<string, unknown>) =>
    value,
}));

describe("useCurrentActivityGraphEditor document plane", () => {
  beforeEach(() => {
    resetDivergentDocumentFixtureState();
  });

  it("blocks editor entry from the document definition when it contains a classifier workstation", () => {
    const documentWithClassifier = {
      ...divergentDocumentPlaneFactoryDocument,
      workstations: [
        {
          ...divergentDocumentPlaneFactoryDocument.workstations?.[0],
          classificationRoutes: [
            {
              label: "approved",
              output: { state: "done", workType: "story" },
            },
          ],
          type: "CLASSIFIER_WORKSTATION" as const,
        },
      ],
    };
    fixtureState.draftState = {
      ...fixtureState.draftState,
      baseDocument: documentWithClassifier,
      latestDocument: documentWithClassifier,
      pendingFactoryDefinition: documentWithClassifier,
    };
    fixtureState.currentFactoryQuery = {
      data: documentWithClassifier,
      status: "success",
    };

    const snapshot = buildDivergentPlaneDashboardSnapshot();

    const { result } = renderHook(() =>
      useCurrentActivityGraphEditor(snapshot, "en", "session-alpha"),
    );

    act(() => {
      result.current.handleEditorModeToggle();
    });

    expect(
      result.current.editorUnavailableClassifierWorkstationName,
    ).toBe("Document Only");
    expect(result.current.canInteractWithEditor).toBe(false);
  });

  it("allows editor entry when only the snapshot plane has a classifier workstation", () => {
    const snapshot = buildDivergentPlaneDashboardSnapshot();
    const snapshotFactory = structuredClone(buildDivergentSnapshotPlaneFactory());
    const classifierWorkstation = snapshotFactory.workstations?.find(
      (workstation) => workstation.name === "Snapshot Only",
    );
    if (classifierWorkstation) {
      classifierWorkstation.type = "CLASSIFIER_WORKSTATION";
      classifierWorkstation.classificationRoutes = [
        {
          label: "approved",
          output: { state: "done", workType: "story" },
        },
      ];
    }
    snapshot.factory = snapshotFactory;

    const { result } = renderHook(() =>
      useCurrentActivityGraphEditor(snapshot, "en", "session-alpha"),
    );

    act(() => {
      result.current.handleEditorModeToggle();
    });

    expect(
      result.current.editorUnavailableClassifierWorkstationName,
    ).toBeUndefined();
    expect(result.current.canInteractWithEditor).toBe(true);
  });

  it("evaluates observe-mode classifier availability from the snapshot factory while the document is still loading", () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    const reviewWorkstation = snapshot.factory?.workstations?.find(
      (workstation) => workstation.name === "Review",
    );
    if (reviewWorkstation) {
      reviewWorkstation.type = "CLASSIFIER_WORKSTATION";
      reviewWorkstation.classificationRoutes = [
        {
          label: "approved",
          output: { state: "complete", workType: "story" },
        },
      ];
    }
    fixtureState.currentFactoryQuery = {
      data: undefined,
      status: "pending",
    };

    const { result } = renderHook(() =>
      useCurrentActivityGraphEditor(snapshot, "en", "session-alpha"),
    );

    expect(result.current.editorMode).toBe(false);
    expect(
      result.current.editorUnavailableClassifierWorkstationName,
    ).toBe("Review");
  });
});
