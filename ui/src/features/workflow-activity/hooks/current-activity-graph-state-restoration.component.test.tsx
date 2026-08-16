import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { useCallback, useEffect, useState } from "react";

import { singleNodeDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  createEditableFactoryGraphHookWrapper,
  setupEditableFactoryGraphSaveTestEnvironment,
} from "../../../testing/editable-factory-graph-hook-test-helpers";
import type { AgentBentoLayoutItem } from "../../bento/components/agent-bento";
import { useDashboardWidgetRemoval } from "../../bento/hooks/removal/useDashboardWidgetRemoval";
import type { DashboardCardStateSnapshot } from "../../dashboard/lib/dashboard-card-state";
import { useCurrentActivityGraphState } from "./use-current-activity-graph-state";
import type { WorkflowActivityBentoCardState } from "./workflow-activity-card-state";

const graphSnapshot = {
  ...singleNodeDashboardSnapshot,
  factory: {
    ...singleNodeDashboardSnapshot.factory,
    version: {
      logical: "1",
      physical: "2026-08-16T00:00:00Z",
    },
  },
};

describe("useCurrentActivityGraphState restoration boundary", () => {
  beforeEach(() => {
    setupEditableFactoryGraphSaveTestEnvironment();
  });

  it("restores a real unsaved topology, layout, and editor snapshot", () => {
    const wrapper =
      createEditableFactoryGraphHookWrapper().EditableFactoryGraphHookWrapper;
    const first = renderHook(
      () =>
        useCurrentActivityGraphState(graphSnapshot, "en", "session-graph-undo"),
      { wrapper },
    );

    act(() => {
      first.result.current.editorControls.toggleMode();
      first.result.current.editorControls.selectTool("connect");
      first.result.current.addControls.startAction("resource");
    });

    const addDraft = first.result.current.addControls.draft;
    if (addDraft?.kind !== "resource") {
      throw new Error("Expected the resource add draft to be open.");
    }

    act(() => {
      first.result.current.addControls.updateDraft({
        ...addDraft,
        name: "undo-cache",
      });
    });

    act(() => {
      first.result.current.addControls.submit();
      first.result.current.layoutControls.updateViewport({
        x: 48,
        y: 96,
        zoom: 1.25,
      });
      first.result.current.editorControls.selectTool("connect");
    });

    const capturedState = structuredClone(
      first.result.current.cardStateSnapshot,
    );
    expect(capturedState.topologyDraft.additions.resources).toEqual([
      { capacity: 1, name: "undo-cache" },
    ]);
    expect(capturedState.layout.viewport).toEqual({
      x: 48,
      y: 96,
      zoom: 1.25,
    });
    expect(capturedState.editorMode).toBe(true);

    first.unmount();

    const restored = renderHook(
      ({ state }: { state: WorkflowActivityBentoCardState }) =>
        useCurrentActivityGraphState(
          graphSnapshot,
          "en",
          "session-graph-undo",
          undefined,
          undefined,
          state,
        ),
      {
        initialProps: { state: capturedState },
        wrapper,
      },
    );

    expect(restored.result.current.cardStateSnapshot.topologyDraft).toEqual(
      capturedState.topologyDraft,
    );
    expect(restored.result.current.cardStateSnapshot.layout).toEqual(
      capturedState.layout,
    );
    expect(restored.result.current.editorControls.isEditing).toBe(true);
    expect(restored.result.current.editorControls.activeTool).toBe("connect");
    expect(restored.result.current.addControls.draft).toEqual(
      capturedState.addEntityDraft,
    );
  });
});

const GRAPH_CARD_ID = "work-graph::primary";
const GRAPH_LAYOUT_ITEM: AgentBentoLayoutItem = {
  h: 8,
  id: GRAPH_CARD_ID,
  w: 12,
  widgetType: "work-graph",
  x: 0,
  y: 0,
};

function GraphCardStateHarness({
  onCardStateChange,
  onDirtyStateChange,
  onRemove,
  restoredCardState,
}: {
  onCardStateChange: (state: DashboardCardStateSnapshot) => void;
  onDirtyStateChange: (isDirty: boolean) => void;
  onRemove: () => void;
  restoredCardState?: WorkflowActivityBentoCardState;
}) {
  const graphState = useCurrentActivityGraphState(
    graphSnapshot,
    "en",
    "session-graph-undo",
    undefined,
    undefined,
    restoredCardState,
  );
  const { cardStateSnapshot } = graphState;

  useEffect(() => {
    onCardStateChange({ value: cardStateSnapshot, widgetType: "work-graph" });
  }, [cardStateSnapshot, onCardStateChange]);

  useEffect(() => {
    onDirtyStateChange(
      graphState.status.hasSharedGraphChanges ||
        graphState.status.preferencesDirty,
    );
  }, [
    graphState.status.hasSharedGraphChanges,
    graphState.status.preferencesDirty,
    onDirtyStateChange,
  ]);

  const resourceDraft = graphState.addControls.draft;
  return (
    <section>
      <button onClick={graphState.editorControls.toggleMode} type="button">
        Enter graph editor
      </button>
      <button
        onClick={() => graphState.addControls.startAction("resource")}
        type="button"
      >
        Start resource draft
      </button>
      {resourceDraft?.kind === "resource" ? (
        <>
          <button
            onClick={() =>
              graphState.addControls.updateDraft({
                ...resourceDraft,
                name: "undo-cache",
              })
            }
            type="button"
          >
            Name resource draft
          </button>
          <button onClick={() => graphState.addControls.submit()} type="button">
            Commit resource draft
          </button>
        </>
      ) : null}
      <button
        onClick={() =>
          graphState.layoutControls.updateViewport({
            x: 48,
            y: 96,
            zoom: 1.25,
          })
        }
        type="button"
      >
        Move graph layout
      </button>
      <button
        onClick={() => graphState.editorControls.selectTool("connect")}
        type="button"
      >
        Select connect tool
      </button>
      <button onClick={onRemove} type="button">
        Remove graph card
      </button>
      <output data-testid="graph-topology">
        {cardStateSnapshot.topologyDraft.additions.resources
          .map((resource) => resource.name)
          .join(",")}
      </output>
      <output data-testid="graph-layout">
        {JSON.stringify(cardStateSnapshot.layout.viewport)}
      </output>
      <output data-testid="graph-editor-state">
        {`${cardStateSnapshot.editorMode}:${cardStateSnapshot.activeTool}`}
      </output>
    </section>
  );
}

function GraphRemovalHarness() {
  const [dashboardLayout, setDashboardLayout] = useState<
    AgentBentoLayoutItem[]
  >([GRAPH_LAYOUT_ITEM]);
  const [cardState, setCardState] = useState<DashboardCardStateSnapshot>();
  const [dirtyCardInstanceIDs, setDirtyCardInstanceIDs] = useState<
    ReadonlySet<string>
  >(new Set());
  const [restoredCardState, setRestoredCardState] =
    useState<WorkflowActivityBentoCardState>();

  const reportCardState = useCallback((state: DashboardCardStateSnapshot) => {
    setCardState(state);
  }, []);
  const reportDirtyState = useCallback((isDirty: boolean) => {
    setDirtyCardInstanceIDs((currentIDs) => {
      const nextIDs = new Set(currentIDs);
      if (isDirty) {
        nextIDs.add(GRAPH_CARD_ID);
      } else {
        nextIDs.delete(GRAPH_CARD_ID);
      }
      return nextIDs;
    });
  }, []);
  const removeDashboardWidget = useCallback((widgetInstanceID: string) => {
    setDashboardLayout((currentLayout) =>
      currentLayout.filter((item) => item.id !== widgetInstanceID),
    );
    return {
      diagnostics: [],
      instanceHighWaterMarks: {},
      persisted: true,
    };
  }, []);
  const persistDashboardLayout = useCallback(
    (nextLayout: AgentBentoLayoutItem[]) => {
      setDashboardLayout(nextLayout);
      return {
        diagnostics: [],
        instanceHighWaterMarks: {},
        persisted: true,
      };
    },
    [],
  );
  const restoreDashboardCardState = useCallback(
    (_widgetInstanceID: string, state: DashboardCardStateSnapshot) => {
      setRestoredCardState(state.value as WorkflowActivityBentoCardState);
    },
    [],
  );
  const getDashboardCardState = useCallback(() => cardState, [cardState]);
  const removal = useDashboardWidgetRemoval({
    dashboardLayout,
    dirtyCardInstanceIDs,
    getDashboardCardState,
    getWidgetTitle: () => "Factory graph",
    persistDashboardLayout,
    removeDashboardWidget,
    restoreDashboardCardState,
  });

  const graphCardIsPresent = dashboardLayout.some(
    (item) => item.id === GRAPH_CARD_ID,
  );
  return (
    <>
      {graphCardIsPresent ? (
        <GraphCardStateHarness
          onCardStateChange={reportCardState}
          onDirtyStateChange={reportDirtyState}
          onRemove={() => removal.requestRemoval(GRAPH_CARD_ID)}
          restoredCardState={restoredCardState}
        />
      ) : null}
      {removal.pendingRemoval ? (
        <button onClick={removal.confirmRemoval} type="button">
          Confirm graph removal
        </button>
      ) : null}
      {removal.undoState?.status === "available" ? (
        <button onClick={removal.undoRemoval} type="button">
          Undo graph removal
        </button>
      ) : null}
    </>
  );
}

describe("dashboard graph card removal restoration", () => {
  beforeEach(() => {
    setupEditableFactoryGraphSaveTestEnvironment();
  });

  it("removes and undoes through the production graph state boundary", async () => {
    const wrapper =
      createEditableFactoryGraphHookWrapper().EditableFactoryGraphHookWrapper;
    render(<GraphRemovalHarness />, { wrapper });

    fireEvent.click(screen.getByRole("button", { name: "Enter graph editor" }));
    fireEvent.click(
      screen.getByRole("button", { name: "Start resource draft" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Name resource draft" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Commit resource draft" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Move graph layout" }));
    fireEvent.click(
      screen.getByRole("button", { name: "Select connect tool" }),
    );

    await waitFor(() => {
      expect(screen.getByTestId("graph-topology").textContent).toContain(
        "undo-cache",
      );
      expect(screen.getByTestId("graph-layout").textContent).toContain(
        JSON.stringify({ x: 48, y: 96, zoom: 1.25 }),
      );
      expect(screen.getByTestId("graph-editor-state").textContent).toContain(
        "true:connect",
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "Remove graph card" }));
    fireEvent.click(
      screen.getByRole("button", { name: "Confirm graph removal" }),
    );
    expect(screen.queryByTestId("graph-topology")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Undo graph removal" }));

    await waitFor(() => {
      expect(screen.getByTestId("graph-topology").textContent).toContain(
        "undo-cache",
      );
      expect(screen.getByTestId("graph-layout").textContent).toContain(
        JSON.stringify({ x: 48, y: 96, zoom: 1.25 }),
      );
      expect(screen.getByTestId("graph-editor-state").textContent).toContain(
        "true:connect",
      );
    });
  });
});
