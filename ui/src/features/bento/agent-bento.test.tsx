import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";

import { NoSelectionDetailCard } from "../current-selection/no-selection-detail-card";
import { WorkTotalsCard } from "../work-totals/work-totals-card";
import {
  AgentBentoCard,
  AgentBentoLayout,
  type AgentBentoLayoutItem,
} from "./agent-bento";

const defaultLayout: AgentBentoLayoutItem[] = [
  { id: "activity", x: 0, y: 0, w: 6, h: 2 },
  { id: "trace", x: 6, y: 0, w: 6, h: 2 },
];

function renderBentoBoard(onLayoutChange = vi.fn()) {
  render(
    <AgentBentoLayout
      cards={[
        {
          id: "activity",
          children: (
            <AgentBentoCard title="Current activity">
              <p>Active workstation graph goes here.</p>
            </AgentBentoCard>
          ),
        },
        {
          id: "trace",
          children: (
            <AgentBentoCard title="Trace grid">
              <p>Trace dispatches stay visible.</p>
            </AgentBentoCard>
          ),
        },
      ]}
      initialWidth={960}
      layout={defaultLayout}
      onLayoutChange={onLayoutChange}
    />,
  );

  return { onLayoutChange };
}

function getGridItem(cardTitle: string): HTMLElement {
  const card = screen.getByRole("article", { name: cardTitle });
  const gridItem = card.closest(".react-grid-item");
  if (!(gridItem instanceof HTMLElement)) {
    throw new Error(`expected ${cardTitle} to render inside a grid item`);
  }

  return gridItem;
}

describe("AgentBentoLayout", () => {
  beforeEach(() => {
    Object.defineProperty(HTMLElement.prototype, "offsetParent", {
      configurable: true,
      get() {
        return this.parentElement ?? document.body;
      },
    });
  });

  it("renders card IDs, titles, and body content inside the movable board", () => {
    renderBentoBoard();

    const moveButton = screen.getByRole("button", { name: "Move Current activity" });
    const activityCard = screen.getByRole("article", { name: "Current activity" });
    const activityTitle = within(activityCard).getByRole("heading", { name: "Current activity" });
    const activityBody = screen.getByText("Active workstation graph goes here.").parentElement;

    expect(screen.getByRole("region", { name: "Agent Factory bento board" })).toBeTruthy();
    expect(activityCard).toBeTruthy();
    expect(screen.getByRole("article", { name: "Trace grid" })).toBeTruthy();
    expect(screen.getByText("Active workstation graph goes here.")).toBeTruthy();
    expect(screen.getByText("Trace dispatches stay visible.")).toBeTruthy();
    expect(getGridItem("Current activity").dataset.bentoCardId).toBe("activity");
    expect(getGridItem("Trace grid").dataset.bentoCardId).toBe("trace");
    expect(activityTitle.className).toContain("af-dashboard-section-heading");
    expect(activityBody?.className).toContain("af-dashboard-body-text");
    expect(moveButton.textContent?.trim()).toBe("");
    expect(moveButton.querySelector("svg")).toBeTruthy();
  });

  it("keeps movement enabled and updates grid position during pointer interaction", async () => {
    const { onLayoutChange } = renderBentoBoard();
    const activityItem = getGridItem("Current activity");
    const initialStyle = activityItem.getAttribute("style");
    const dragHandle = within(activityItem).getByRole("button", {
      name: "Move Current activity",
    });

    fireEvent.mouseDown(dragHandle, {
      button: 0,
      buttons: 1,
      clientX: 120,
      clientY: 40,
    });

    await waitFor(() => {
      expect(activityItem.classList.contains("react-draggable-dragging")).toBe(true);
    });

    fireEvent.mouseMove(document, {
      buttons: 1,
      clientX: 340,
      clientY: 40,
    });

    await waitFor(() => {
      expect(activityItem.getAttribute("style")).not.toBe(initialStyle);
    });

    fireEvent.mouseUp(document, {
      button: 0,
      clientX: 340,
      clientY: 40,
    });

    await waitFor(() => {
      expect(onLayoutChange).toHaveBeenCalled();
    });
    expect(screen.getByText("Active workstation graph goes here.")).toBeTruthy();
  });

  it("renders right, bottom, and bottom-right resize handles for grid cards", () => {
    renderBentoBoard();

    for (const cardTitle of ["Current activity", "Trace grid"]) {
      const gridItem = getGridItem(cardTitle);

      expect(gridItem.querySelector(".react-resizable-handle-e")).toBeTruthy();
      expect(gridItem.querySelector(".react-resizable-handle-s")).toBeTruthy();
      expect(gridItem.querySelector(".react-resizable-handle-se")).toBeTruthy();
    }
  });

  it.each([
    ["right", ".react-resizable-handle-e", { clientX: 80, clientY: 120 }],
    ["bottom", ".react-resizable-handle-s", { clientX: 260, clientY: 220 }],
  ])("persists layout changes from the %s resize handle", async (_label, selector, endPoint) => {
    const { onLayoutChange } = renderBentoBoard();
    const activityItem = getGridItem("Current activity");
    const initialStyle = activityItem.getAttribute("style");
    const resizeHandle = activityItem.querySelector(selector);

    if (!(resizeHandle instanceof HTMLElement)) {
      throw new Error(`expected ${selector} resize handle`);
    }

    fireEvent.mouseDown(resizeHandle, {
      button: 0,
      buttons: 1,
      clientX: 240,
      clientY: 120,
    });
    fireEvent.mouseMove(document, {
      buttons: 1,
      ...endPoint,
    });
    fireEvent.mouseUp(document, {
      button: 0,
      ...endPoint,
    });

    await waitFor(() => {
      expect(activityItem.getAttribute("style")).not.toBe(initialStyle);
      expect(onLayoutChange).toHaveBeenCalled();
    });
  });

  it("renders real dashboard feature cards through the shared bento seam", () => {
    render(
      <AgentBentoLayout
        cards={[
          {
            id: "work-totals",
            children: (
              <WorkTotalsCard
                completedCount={3}
                dispatchedCount={5}
                failedCount={1}
                inFlightDispatchCount={2}
              />
            ),
          },
          {
            id: "current-selection",
            children: <NoSelectionDetailCard />,
          },
        ]}
        initialWidth={1180}
        layout={[
          { id: "work-totals", x: 0, y: 0, w: 4, h: 2 },
          { id: "current-selection", x: 4, y: 0, w: 8, h: 4 },
        ]}
      />,
    );

    const board = screen.getByRole("region", { name: "Agent Factory bento board" });
    const workTotals = screen.getByRole("article", { name: "Work totals" });
    const currentSelection = screen.getByRole("article", { name: "Current selection" });

    expect(board).toBeTruthy();
    expect(within(workTotals).getByLabelText("work totals")).toBeTruthy();
    expect(within(workTotals).getByText("In progress")).toBeTruthy();
    expect(within(workTotals).getByText("Completed")).toBeTruthy();
    expect(currentSelection.className).toContain("rounded-2xl");
    expect(within(currentSelection).getByRole("button", { name: "Undo selection" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(within(currentSelection).getByRole("button", { name: "Redo selection" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(
      within(currentSelection).getByText(
        "Select a workstation, work item, or state node to inspect live details.",
      ),
    ).toBeTruthy();
    expect(getGridItem("Work totals").dataset.bentoCardId).toBe("work-totals");
    expect(getGridItem("Current selection").dataset.bentoCardId).toBe("current-selection");
  });
});

