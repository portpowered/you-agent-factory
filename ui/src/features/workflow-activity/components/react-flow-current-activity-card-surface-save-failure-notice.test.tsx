import { fireEvent, render, screen } from "@testing-library/react";

import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { projectFactoryValidationTargets } from "../../factory-graph-editor/lib/projection/factory-validation-graph-projection";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";

vi.mock(
  "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
  () => ({
    FactoryGraphEditorNotice: ({
      children,
      dismissLabel,
      onDismiss,
      title,
      tone,
    }: {
      children: React.ReactNode;
      dismissLabel?: string;
      onDismiss?: () => void;
      title: string;
      tone: string;
    }) => (
      <section
        data-testid={`notice-${tone}`}
        role={tone === "danger" ? "alert" : "status"}
      >
        <h3>{title}</h3>
        <div>{children}</div>
        {onDismiss && dismissLabel ? (
          <button aria-label={dismissLabel} onClick={onDismiss} type="button">
            {dismissLabel}
          </button>
        ) : null}
      </section>
    ),
  }),
);

vi.mock("./react-flow-current-activity-card-viewport", () => ({
  CurrentActivityGraphViewport: () => <div data-testid="graph-viewport" />,
}));

function createEditorStub(overrides: Record<string, unknown> = {}) {
  return {
    activeTool: null,
    addMenuActions: [],
    addMenuOpen: false,
    blockedRemovalReason: null,
    canInteractWithEditor: true,
    canSaveDraft: true,
    connectionNotice: null,
    currentFactoryDefinition: null,
    dirtyStateSummary: {
      layoutDirty: false,
      preferencesDirty: false,
      topologyDirty: true,
    },
    draftState: { hasChanges: true, pendingFactoryDefinition: null },
    handleDiscardPendingChanges: vi.fn(),
    handleEditorConnect: vi.fn(),
    handleEditorEdgeDelete: vi.fn(),
    handleEditorNodeDelete: vi.fn(),
    hiddenNodeClasses: new Set(),
    hideShowMenuOpen: false,
    layoutDraftState: {
      canRedoLayout: false,
      canUndoLayout: false,
      layoutDirty: false,
    },
    moveLayoutNode: vi.fn(),
    moveLayoutNodesByDelta: vi.fn(),
    redoLayout: vi.fn(),
    resetLayout: vi.fn(),
    resetPreferences: vi.fn(),
    setActiveTool: vi.fn(),
    setAddMenuOpen: vi.fn(),
    setHideShowMenuOpen: vi.fn(),
    setIsConfirmingSave: vi.fn(),
    setVisibilityPreset: vi.fn(),
    toggleHiddenNodeClass: vi.fn(),
    undoLayout: vi.fn(),
    updateLayoutViewport: vi.fn(),
    visibilityPreset: "all",
    editorMode: true,
    structuralValidation: {
      projection: projectFactoryValidationTargets([]),
      targets: [],
    },
    hasActiveWork: false,
    isStaleDraft: false,
    saveAttemptRevision: 0,
    saveEditableDefinition: {
      error: null,
      isPending: false,
    },
    ...overrides,
  };
}

function createGraphStub() {
  return {
    edges: [],
    graphKey: "graph-key",
    handleNodesChange: vi.fn(),
    initialFitViewKey: "full-graph",
    initialFitViewOptions: { padding: 0.18 },
    nodes: [],
    setStoredNodePosition: vi.fn(),
  };
}

describe("CurrentActivityGraphSurface save failure notice", () => {
  it("dismisses the save failure notice without clearing draft changes and re-shows on a later failure revision", () => {
    const saveError = new CurrentFactoryDefinitionError(
      "The factory definition is invalid.",
      {
        code: "INVALID_FACTORY_DEFINITION",
        status: 400,
        targets: [],
      },
    );

    const { rerender } = render(
      <CurrentActivityGraphSurface
        editor={
          createEditorStub({
            saveAttemptRevision: 1,
            saveEditableDefinition: {
              error: saveError,
              isPending: false,
            },
          }) as never
        }
        graph={createGraphStub() as never}
        imports={{} as never}
        selection={null}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.getByText("Topology save failed")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByText("Topology save failed")).toBeNull();

    rerender(
      <CurrentActivityGraphSurface
        editor={
          createEditorStub({
            saveAttemptRevision: 1,
            saveEditableDefinition: {
              error: saveError,
              isPending: false,
            },
          }) as never
        }
        graph={createGraphStub() as never}
        imports={{} as never}
        selection={null}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );
    expect(screen.queryByText("Topology save failed")).toBeNull();

    rerender(
      <CurrentActivityGraphSurface
        editor={
          createEditorStub({
            saveAttemptRevision: 2,
            saveEditableDefinition: {
              error: saveError,
              isPending: false,
            },
          }) as never
        }
        graph={createGraphStub() as never}
        imports={{} as never}
        selection={null}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByText("Topology save failed")).toBeTruthy();
  });
});
