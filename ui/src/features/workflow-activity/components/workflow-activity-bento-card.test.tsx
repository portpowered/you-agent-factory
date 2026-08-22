import { fireEvent, render, screen } from "@testing-library/react";
import * as React from "react";

const { mockGraphState, mockViewModel } = vi.hoisted(() => ({
  mockGraphState: {
    cardStateSnapshot: {
      activeTool: null,
      editorMode: true,
    },
  },
  mockViewModel: {
    editorControls: {
      canInteract: true,
      isEditing: true,
      toggleMode: vi.fn(),
      unavailableClassifierWorkstationName: undefined,
    },
    removalControls: {
      blockedReason: undefined,
      requestSelectionNodeRemoval: vi.fn(),
    },
    status: {
      dirtySummary: "Unsaved graph changes",
      hasSharedGraphChanges: true,
      isDefinitionLoading: false,
      loadErrorMessage: undefined,
      preferencesDirty: false,
    },
  },
}));

vi.mock("../../dashboard/session/dashboard-session-provider", () => ({
  useDashboardSession: () => ({ sessionID: "session-1" }),
}));

vi.mock("../hooks/use-current-activity-graph-state", () => ({
  useCurrentActivityGraphState: () => mockGraphState,
}));

vi.mock("../hooks/use-current-activity-graph-card-view-model", () => ({
  useCurrentActivityGraphCardViewModel: () => mockViewModel,
}));

vi.mock("../state/factory-graph-topology-editor-bridge", () => ({
  useFactoryGraphTopologyEditorBridge: () => vi.fn(),
}));

vi.mock("./bento-card/workflow-activity-bento-shell", () => ({
  WorkflowActivityBentoShell: ({
    children,
    headerAction,
  }: {
    children: React.ReactNode;
    headerAction: React.ReactNode;
  }) => (
    <section>
      {headerAction}
      {children}
    </section>
  ),
}));

vi.mock("./react-flow-current-activity-card-editor-chrome", () => ({
  CurrentActivityGraphHeaderActions: ({
    headerActions,
  }: {
    headerActions?: React.ReactNode;
  }) => <div data-testid="graph-header-actions">{headerActions}</div>,
}));

vi.mock("./react-flow-current-activity-card-view", () => ({
  ReactFlowCurrentActivityCardView: () => (
    <div data-testid="graph-surface">Graph surface</div>
  ),
}));

import { WorkflowActivityBentoCard } from "./workflow-activity-bento-card";

const baseProps = {
  importController: {} as never,
  now: 0,
  onDocAdded: vi.fn(),
  onNodeRemovedFromDraft: vi.fn(),
  onSelectDoc: vi.fn(),
  onSelectResource: vi.fn(),
  onSelectStateNode: vi.fn(),
  onSelectWorkID: vi.fn(),
  onSelectWorker: vi.fn(),
  onSelectWorkType: vi.fn(),
  onSelectWorkstation: vi.fn(),
  selection: null,
  snapshot: {} as never,
};

describe("WorkflowActivityBentoCard", () => {
  it("keeps dirty state and editor actions usable across parent rerenders", () => {
    const dirtyEvents: boolean[] = [];
    const onDiscard = vi.fn();
    const onSave = vi.fn();

    function ParentHarness() {
      const [, setRenderCount] = React.useState(0);

      return (
        <>
          <button
            onClick={() => setRenderCount((count) => count + 1)}
            type="button"
          >
            Rerender parent
          </button>
          <WorkflowActivityBentoCard
            {...baseProps}
            headerAction={
              <>
                <button onClick={onSave} type="button">
                  Save
                </button>
                <button onClick={onDiscard} type="button">
                  Discard
                </button>
              </>
            }
            onDirtyStateChange={(isDirty) => dirtyEvents.push(isDirty)}
          />
        </>
      );
    }

    render(<ParentHarness />);
    expect(dirtyEvents).toEqual([true]);

    fireEvent.click(screen.getByRole("button", { name: "Rerender parent" }));

    expect(dirtyEvents).toEqual([true]);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(onSave).toHaveBeenCalledOnce();
    expect(onDiscard).toHaveBeenCalledOnce();
    expect(screen.getByTestId("graph-surface")).toBeTruthy();
  });
});
