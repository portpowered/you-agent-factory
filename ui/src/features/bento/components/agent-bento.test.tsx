import {
  act,
  createEvent,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach } from "vitest";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { NoSelectionDetailCard } from "../../current-selection/base/components/detail/no-selection-detail-card";
import { InlineAddWidgetCard } from "../../dashboard-add-card/components/inline-add-widget-card";
import { WorkTotalsCard } from "../../work-totals/components/work-totals-card";
import {
  AgentBentoCard,
  AgentBentoLayout,
  type AgentBentoLayoutItem,
  toCompactGridLayout,
} from "./agent-bento";

const defaultLayout: AgentBentoLayoutItem[] = [
  { h: 2, id: "activity", widgetType: "activity", w: 6, x: 0, y: 0 },
  { h: 2, id: "trace", widgetType: "trace", w: 6, x: 6, y: 0 },
];

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

function renderBentoBoard(onLayoutChange = vi.fn()) {
  render(
    <AgentBentoLayout
      cards={[
        {
          id: "activity",
          widgetType: "activity",
          children: (
            <AgentBentoCard title="Current activity">
              <p>Active workstation graph goes here.</p>
            </AgentBentoCard>
          ),
        },
        {
          id: "trace",
          widgetType: "trace",
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

function expectNoVerticalScrollContainer(element: HTMLElement) {
  expect(element.className).not.toMatch(/overflow-y-(auto|scroll)/);
  expect(window.getComputedStyle(element).overflowY).not.toMatch(
    /^(auto|scroll)$/,
  );
}

function createTouch(identifier: number, clientX: number, clientY: number) {
  return { clientX, clientY, identifier };
}

function installMeasuredBoardWidth(initialWidth: number) {
  const previousResizeObserver = globalThis.ResizeObserver;
  let measuredWidth = initialWidth;
  const observers: ControlledResizeObserver[] = [];

  class ControlledResizeObserver {
    private readonly callback: ResizeObserverCallback;
    private target: Element | null = null;

    public constructor(callback: ResizeObserverCallback) {
      this.callback = callback;
      observers.push(this);
    }

    public disconnect(): void {
      this.target = null;
    }

    public observe(target: Element): void {
      this.target = target;
      this.emit();
    }

    public unobserve(): void {
      this.target = null;
    }

    public emit(): void {
      if (!this.target) {
        return;
      }

      this.callback(
        [
          {
            contentRect: { width: measuredWidth } as DOMRectReadOnly,
            target: this.target,
          } as ResizeObserverEntry,
        ],
        this as unknown as ResizeObserver,
      );
    }
  }

  globalThis.ResizeObserver =
    ControlledResizeObserver as unknown as typeof ResizeObserver;

  return {
    restore() {
      globalThis.ResizeObserver = previousResizeObserver;
      observers.length = 0;
    },
    setWidth(width: number) {
      measuredWidth = width;
      for (const observer of observers) {
        observer.emit();
      }
    },
  };
}

function renderCompactBoard(onLayoutChange = vi.fn()) {
  return render(
    <AgentBentoLayout
      cards={[
        {
          id: "activity",
          widgetType: "activity",
          children: (
            <AgentBentoCard
              headerAction={<button type="button">Open activity</button>}
              title="Current activity"
            >
              <p>Active workstation graph goes here.</p>
            </AgentBentoCard>
          ),
        },
        {
          id: "trace",
          widgetType: "trace",
          children: (
            <AgentBentoCard title="Trace grid">
              <p>Trace dispatches stay visible.</p>
            </AgentBentoCard>
          ),
        },
        {
          id: "terminal",
          widgetType: "terminal",
          children: (
            <AgentBentoCard title="Terminal work">
              <p>Completed work remains available.</p>
            </AgentBentoCard>
          ),
        },
      ]}
      initialWidth={768}
      layout={[
        { h: 2, id: "activity", widgetType: "activity", w: 4, x: 0, y: 5 },
        { h: 3, id: "trace", widgetType: "trace", w: 3, x: 6, y: 0 },
        {
          h: 1,
          id: "terminal",
          widgetType: "terminal",
          w: 6,
          x: 0,
          y: 0,
        },
      ]}
      onLayoutChange={onLayoutChange}
    />,
  );
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

    const activityCard = screen.getByRole("article", {
      name: "Current activity",
    });
    const activityHeader = activityCard.querySelector("header");
    const activityTitle = within(activityCard).getByRole("heading", {
      name: "Current activity",
    });
    const activityBody = screen
      .getByText("Active workstation graph goes here.")
      .closest(".af-body-text");

    expect(
      screen.getByRole("region", { name: "you-agent-factory bento board" }),
    ).toBeTruthy();
    expect(activityCard).toBeTruthy();
    expect(screen.getByRole("article", { name: "Trace grid" })).toBeTruthy();
    expect(
      screen.getByText("Active workstation graph goes here."),
    ).toBeTruthy();
    expect(screen.getByText("Trace dispatches stay visible.")).toBeTruthy();
    expect(activityCard.dataset.dashboardPanelShell).toBe("grid-card");
    expect(activityCard.className).toContain("border-outline");
    expect(activityCard.className).toContain("bg-surface-container-high");
    expect(activityCard.className).toContain("shadow-af-card");
    expect(getGridItem("Current activity").dataset.bentoCardId).toBe(
      "activity",
    );
    expect(getGridItem("Current activity").className).toContain(
      "overflow-visible",
    );
    expect(getGridItem("Trace grid").dataset.bentoCardId).toBe("trace");
    expect(activityTitle.className).toContain("af-section-heading");
    expect(activityBody?.className).toContain("af-body-text");
    expect(activityHeader?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(activityHeader?.className).toContain("cursor-grab");
  });

  it("lets the board and grid grow without creating a vertical scroll container", () => {
    renderBentoBoard();

    const board = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const grid = board.querySelector(".react-grid-layout");
    const activityItem = getGridItem("Current activity");

    if (!(grid instanceof HTMLElement)) {
      throw new Error(
        "expected bento board to render a react-grid-layout root",
      );
    }

    expect(board.className).toContain("overflow-x-clip");
    expect(board.className).not.toContain("overflow-x-hidden");
    expectNoVerticalScrollContainer(board);
    expectNoVerticalScrollContainer(grid);
    expectNoVerticalScrollContainer(activityItem);
    expect(grid.className).toContain("min-h-px");
    expect(grid.style.height).not.toBe("");
  });

  it("keeps bento elevation on the card while graph-bearing interiors stay flat", () => {
    render(
      <AgentBentoLayout
        cards={[
          {
            id: "activity",
            widgetType: "activity",
            children: (
              <AgentBentoCard title="Current activity">
                <section
                  aria-label="Current activity graph"
                  className="shadow-none"
                  data-current-activity-flow
                >
                  <div className="react-flow shadow-none" />
                </section>
              </AgentBentoCard>
            ),
          },
          {
            id: "trace",
            widgetType: "trace",
            children: (
              <AgentBentoCard title="Trace grid">
                <section
                  aria-label="Trace graph"
                  className="shadow-none"
                  data-dashboard-graph-frame
                >
                  <div className="react-flow shadow-none" />
                </section>
              </AgentBentoCard>
            ),
          },
          {
            id: "work-totals",
            widgetType: "work-totals",
            children: (
              <WorkTotalsCard
                completedCount={3}
                dispatchedCount={5}
                failedCount={1}
                inFlightDispatchCount={2}
              />
            ),
          },
        ]}
        initialWidth={1180}
        layout={[
          { h: 4, id: "activity", widgetType: "activity", w: 4, x: 0, y: 0 },
          { h: 4, id: "trace", widgetType: "trace", w: 4, x: 4, y: 0 },
          {
            h: 2,
            id: "work-totals",
            widgetType: "work-totals",
            w: 4,
            x: 8,
            y: 0,
          },
        ]}
      />,
    );

    for (const cardTitle of ["Current activity", "Trace grid", "Work totals"]) {
      const card = screen.getByRole("article", { name: cardTitle });
      const gridItem = getGridItem(cardTitle);

      expect(gridItem.className).toContain("overflow-visible");
      expect(card.dataset.dashboardPanelShell).toBe("grid-card");
      expect(card.className).toContain("shadow-af-card");
    }

    const activityGraph = screen.getByRole("region", {
      name: "Current activity graph",
    });
    const traceGraph = screen.getByRole("region", { name: "Trace graph" });

    for (const graphFrame of [activityGraph, traceGraph]) {
      expect(graphFrame.className).toContain("shadow-none");
      expect(graphFrame.className).not.toContain("shadow-af-card");
      expect(graphFrame.className).not.toContain("shadow-af-panel");
      const reactFlowCanvas = graphFrame.querySelector(".react-flow");
      expect(reactFlowCanvas?.className).toContain("shadow-none");
      expect(reactFlowCanvas?.className).not.toContain("shadow-af-card");
      expect(reactFlowCanvas?.className).not.toContain("shadow-af-panel");
    }
  });

  it("keeps movement enabled and updates grid position during pointer interaction", async () => {
    const { onLayoutChange } = renderBentoBoard();
    const activityItem = getGridItem("Current activity");
    const initialStyle = activityItem.getAttribute("style");
    const dragHandle = within(activityItem).getByRole("heading", {
      name: "Current activity",
    });

    fireEvent.mouseDown(dragHandle, {
      button: 0,
      buttons: 1,
      clientX: 120,
      clientY: 40,
    });

    await waitFor(() => {
      expect(activityItem.classList.contains("react-draggable-dragging")).toBe(
        true,
      );
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
    expect(
      screen.getByText("Active workstation graph goes here."),
    ).toBeTruthy();
  });

  it("moves a card with touch and commits through the canonical layout seam", async () => {
    const onLayoutChange = vi.fn();
    renderBentoBoard(onLayoutChange);

    const activityItem = getGridItem("Current activity");
    const dragHandle = within(activityItem).getByRole("heading", {
      name: "Current activity",
    });
    const start = createTouch(1, 120, 40);
    const end = createTouch(1, 220, 40);

    fireEvent.touchStart(dragHandle, {
      changedTouches: [start],
      touches: [start],
    });
    fireEvent.touchMove(document, {
      changedTouches: [end],
      touches: [end],
    });
    fireEvent.touchEnd(document, {
      changedTouches: [end],
      touches: [],
    });

    await waitFor(() => {
      expect(onLayoutChange).toHaveBeenCalledTimes(1);
    });
    expect(onLayoutChange).toHaveBeenLastCalledWith(
      expect.arrayContaining([
        expect.objectContaining({ id: "activity", x: 1, y: 0 }),
      ]),
    );
  });

  it("resizes a card with touch through the same canonical layout seam", async () => {
    const onLayoutChange = vi.fn();
    renderBentoBoard(onLayoutChange);

    const activityItem = getGridItem("Current activity");
    const resizeHandle = activityItem.querySelector(
      "[data-bento-touch-resize-handle='e']",
    );
    if (!(resizeHandle instanceof HTMLElement)) {
      throw new Error("expected the activity card east resize handle");
    }

    const start = createTouch(2, 520, 120);
    const end = createTouch(2, 620, 120);
    fireEvent.touchStart(resizeHandle, {
      changedTouches: [start],
      touches: [start],
    });
    fireEvent.touchMove(document, {
      changedTouches: [end],
      touches: [end],
    });
    fireEvent.touchEnd(document, {
      changedTouches: [end],
      touches: [],
    });

    await waitFor(() => {
      expect(onLayoutChange).toHaveBeenCalledTimes(1);
    });
    expect(onLayoutChange).toHaveBeenLastCalledWith(
      expect.arrayContaining([
        expect.objectContaining({ id: "activity", w: 7 }),
      ]),
    );
  });

  it("restores the stored layout when a touch gesture is cancelled", async () => {
    const onLayoutChange = vi.fn();
    renderBentoBoard(onLayoutChange);

    const board = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const activityItem = getGridItem("Current activity");
    const initialSignature = activityItem.dataset.layoutSignature;
    const dragHandle = within(activityItem).getByRole("heading", {
      name: "Current activity",
    });
    const start = createTouch(3, 120, 40);
    const moved = createTouch(3, 220, 40);

    fireEvent.touchStart(dragHandle, {
      changedTouches: [start],
      touches: [start],
    });
    fireEvent.touchMove(document, {
      changedTouches: [moved],
      touches: [moved],
    });
    fireEvent.touchCancel(document, {
      changedTouches: [moved],
      touches: [],
    });

    await waitFor(() => {
      expect(activityItem.dataset.layoutSignature).toBe(initialSignature);
    });
    expect(onLayoutChange).not.toHaveBeenCalled();
    expect(board.querySelector(".react-draggable-dragging")).toBeNull();
  });

  it("keeps dashboard controls tappable and ignores their touch gestures", () => {
    const onLayoutChange = vi.fn();
    const onClick = vi.fn();
    render(
      <AgentBentoLayout
        cards={[
          {
            id: "activity",
            widgetType: "activity",
            children: (
              <AgentBentoCard
                headerAction={
                  <button onClick={onClick} type="button">
                    Open activity
                  </button>
                }
                title="Current activity"
              >
                <p>Active workstation graph goes here.</p>
              </AgentBentoCard>
            ),
          },
        ]}
        initialWidth={960}
        layout={[
          {
            h: 2,
            id: "activity",
            widgetType: "activity",
            w: 6,
            x: 0,
            y: 0,
          },
        ]}
        onLayoutChange={onLayoutChange}
      />,
    );

    const button = screen.getByRole("button", { name: "Open activity" });
    const start = createTouch(4, 120, 40);
    const end = createTouch(4, 220, 40);
    fireEvent.touchStart(button, {
      changedTouches: [start],
      touches: [start],
    });
    fireEvent.touchMove(document, {
      changedTouches: [end],
      touches: [end],
    });
    fireEvent.touchEnd(document, {
      changedTouches: [end],
      touches: [],
    });
    fireEvent.click(button);

    expect(onLayoutChange).not.toHaveBeenCalled();
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("leaves card-body touch scrolling to the scroll viewport", () => {
    const onLayoutChange = vi.fn();
    render(
      <AgentBentoLayout
        cards={[
          {
            id: "activity",
            widgetType: "activity",
            children: (
              <AgentBentoCard
                bodyProps={{ "data-testid": "activity-body" }}
                title="Current activity"
              >
                <div>
                  {Array.from(
                    { length: 12 },
                    (_, index) => `Scrollable activity row ${index + 1}`,
                  ).map((row) => (
                    <p key={row}>{row}</p>
                  ))}
                </div>
              </AgentBentoCard>
            ),
          },
        ]}
        initialWidth={960}
        layout={[
          { h: 2, id: "activity", widgetType: "activity", w: 6, x: 0, y: 0 },
        ]}
        onLayoutChange={onLayoutChange}
      />,
    );

    const body = screen.getByTestId("activity-body");
    const start = createTouch(6, 120, 120);
    const end = createTouch(6, 120, 220);
    const touchStart = createEvent.touchStart(body, {
      changedTouches: [start],
      touches: [start],
    });
    const touchMove = createEvent.touchMove(document, {
      changedTouches: [end],
      touches: [end],
    });
    const touchEnd = createEvent.touchEnd(document, {
      changedTouches: [end],
      touches: [],
    });

    fireEvent(body, touchStart);
    fireEvent(document, touchMove);
    fireEvent(document, touchEnd);

    expect(touchStart.defaultPrevented).toBe(false);
    expect(touchMove.defaultPrevented).toBe(false);
    expect(touchEnd.defaultPrevented).toBe(false);
    expect(onLayoutChange).not.toHaveBeenCalled();
  });

  it("cancels touch-pointer gestures without persisting a preview", async () => {
    const onLayoutChange = vi.fn();
    renderBentoBoard(onLayoutChange);

    const activityItem = getGridItem("Current activity");
    const initialSignature = activityItem.dataset.layoutSignature;
    const dragHandle = within(activityItem).getByRole("heading", {
      name: "Current activity",
    });

    fireEvent.pointerDown(dragHandle, {
      clientX: 120,
      clientY: 40,
      pointerId: 5,
      pointerType: "touch",
    });
    fireEvent.pointerMove(document, {
      clientX: 220,
      clientY: 40,
      pointerId: 5,
      pointerType: "touch",
    });
    fireEvent.pointerCancel(document, {
      clientX: 220,
      clientY: 40,
      pointerId: 5,
      pointerType: "touch",
    });

    await waitFor(() => {
      expect(activityItem.dataset.layoutSignature).toBe(initialSignature);
    });
    expect(onLayoutChange).not.toHaveBeenCalled();
  });

  it("renders right, bottom, and bottom-right resize handles for grid cards", () => {
    renderBentoBoard();

    for (const cardTitle of ["Current activity", "Trace grid"]) {
      const gridItem = getGridItem(cardTitle);

      expect(gridItem.querySelector(".react-resizable-handle-e")).toBeTruthy();
      expect(gridItem.querySelector(".react-resizable-handle-s")).toBeTruthy();
      expect(gridItem.querySelector(".react-resizable-handle-se")).toBeTruthy();
      for (const handle of gridItem.querySelectorAll<HTMLElement>(
        "[data-bento-touch-resize-handle]",
      )) {
        expect(handle.style.width).toBe("44px");
        expect(handle.style.height).toBe("44px");
        expect(handle.style.touchAction).toBe("none");
      }
    }
  });

  it("stacks cards into a single full-width column in compact layouts", () => {
    const compactLayout = toCompactGridLayout([
      { h: 2, i: "activity", w: 6, x: 0, y: 0 },
      { h: 3, i: "trace", w: 6, x: 6, y: 0 },
      { h: 4, i: "terminal", w: 12, x: 0, y: 3 },
    ]);

    expect(compactLayout).toEqual([
      {
        h: 2,
        i: "activity",
        isDraggable: false,
        isResizable: false,
        maxW: 12,
        minW: 12,
        w: 12,
        x: 0,
        y: 0,
      },
      {
        h: 3,
        i: "trace",
        isDraggable: false,
        isResizable: false,
        maxW: 12,
        minW: 12,
        w: 12,
        x: 0,
        y: 2,
      },
      {
        h: 4,
        i: "terminal",
        isDraggable: false,
        isResizable: false,
        maxW: 12,
        minW: 12,
        w: 12,
        x: 0,
        y: 5,
      },
    ]);
  });

  it.each([320, 390, 640, 768])(
    "projects a %spx board into stable full-width order without horizontal overflow",
    async (boardWidth) => {
      const measuredBoard = installMeasuredBoardWidth(boardWidth);

      try {
        renderCompactBoard();

        const board = screen.getByRole("region", {
          name: "you-agent-factory bento board",
        });

        await waitFor(() => {
          const items = [
            ...board.querySelectorAll<HTMLElement>(".react-grid-item"),
          ];

          expect(items).toHaveLength(3);
          expect(items.map((item) => item.dataset.bentoCardId)).toEqual([
            "terminal",
            "trace",
            "activity",
          ]);

          for (const item of items) {
            expect(Number.parseFloat(item.style.width)).toBeCloseTo(boardWidth);
            expect(item.style.transform).toMatch(/translate\(0px,[-\d.]+px\)/);
            expect(item.classList.contains("react-draggable")).toBe(false);
            expect(item.classList.contains("react-resizable-hide")).toBe(true);
          }

          expect(board.scrollWidth).toBeLessThanOrEqual(board.clientWidth);
        });

        expect(
          within(
            screen.getByRole("article", { name: "Current activity" }),
          ).getByRole("button", { name: "Open activity" }),
        ).toBeTruthy();
        expect(
          screen.getByText("Completed work remains available."),
        ).toBeTruthy();
      } finally {
        measuredBoard.restore();
      }
    },
  );

  it("does not persist compact geometry and restores the canonical layout above the breakpoint", async () => {
    const measuredBoard = installMeasuredBoardWidth(768);
    const onLayoutChange = vi.fn();

    try {
      renderCompactBoard(onLayoutChange);

      const board = screen.getByRole("region", {
        name: "you-agent-factory bento board",
      });
      const activityItem = board.querySelector<HTMLElement>(
        '[data-bento-card-id="activity"]',
      );
      if (!activityItem) {
        throw new Error("expected the activity card to render");
      }

      const canonicalSignature = activityItem.dataset.layoutSignature;
      await waitFor(() => {
        expect(Number.parseFloat(activityItem.style.width)).toBeCloseTo(768);
      });

      const compactWidth = activityItem.style.width;
      expect(onLayoutChange).not.toHaveBeenCalled();

      await act(async () => {
        measuredBoard.setWidth(1024);
        await Promise.resolve();
      });

      await waitFor(() => {
        expect(activityItem.style.width).not.toBe(compactWidth);
      });

      expect(Number.parseFloat(activityItem.style.width)).toBeLessThan(768);
      expect(activityItem.dataset.layoutSignature).toBe(canonicalSignature);
      expect(onLayoutChange).not.toHaveBeenCalled();
      expect(board.querySelector(".react-resizable-handle-e")).toBeTruthy();
    } finally {
      measuredBoard.restore();
    }
  });

  it("renders a localized accessible name for the movable board", () => {
    render(
      <AgentBentoLayout
        cards={[
          {
            id: "activity",
            widgetType: "activity",
            children: (
              <AgentBentoCard title="Current activity">
                <p>Active workstation graph goes here.</p>
              </AgentBentoCard>
            ),
          },
        ]}
        initialWidth={960}
        layout={[
          { h: 2, id: "activity", widgetType: "activity", w: 6, x: 0, y: 0 },
        ]}
        locale="zh-CN"
      />,
    );

    expect(
      screen.getByRole("region", { name: "you-agent-factory Bento 看板" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("region", { name: "you-agent-factory bento board" }),
    ).toBeNull();
  });

  it.each([
    ["right", ".react-resizable-handle-e", { clientX: 80, clientY: 120 }],
    ["bottom", ".react-resizable-handle-s", { clientX: 260, clientY: 220 }],
  ])(
    "persists layout changes from the %s resize handle",
    async (_label, selector, endPoint) => {
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
    },
  );

  it("lets the inline add-widget card move through the shared grid handle and keeps a single add-widget instance", async () => {
    const onLayoutChange = vi.fn();

    render(
      <AgentBentoLayout
        cards={[
          {
            id: "activity",
            widgetType: "activity",
            children: (
              <AgentBentoCard title="Current activity">
                <p>Active workstation graph goes here.</p>
              </AgentBentoCard>
            ),
          },
          {
            id: "add-widget::inline-add",
            widgetType: "add-widget",
            children: <InlineAddWidgetCard />,
          },
        ]}
        initialWidth={960}
        layout={[
          { h: 2, id: "activity", widgetType: "activity", w: 6, x: 0, y: 0 },
          {
            h: 4,
            id: "add-widget::inline-add",
            widgetType: "add-widget",
            w: 4,
            x: 6,
            y: 0,
          },
        ]}
        onLayoutChange={onLayoutChange}
      />,
    );

    const addWidgetItem = getGridItem("Add widget");
    const initialStyle = addWidgetItem.getAttribute("style");
    const dragHandle = within(addWidgetItem).getByRole("heading", {
      name: "Add widget",
    });

    fireEvent.mouseDown(dragHandle, {
      button: 0,
      buttons: 1,
      clientX: 640,
      clientY: 48,
    });

    await waitFor(() => {
      expect(addWidgetItem.classList.contains("react-draggable-dragging")).toBe(
        true,
      );
    });

    fireEvent.mouseMove(document, {
      buttons: 1,
      clientX: 360,
      clientY: 196,
    });
    fireEvent.mouseUp(document, {
      button: 0,
      clientX: 360,
      clientY: 196,
    });

    await waitFor(() => {
      expect(addWidgetItem.getAttribute("style")).not.toBe(initialStyle);
      expect(onLayoutChange).toHaveBeenCalledWith(
        expect.arrayContaining([
          expect.objectContaining({
            id: "add-widget::inline-add",
            widgetType: "add-widget",
          }),
        ]),
      );
    });

    const latestLayout = onLayoutChange.mock.calls.at(-1)?.[0] as
      | AgentBentoLayoutItem[]
      | undefined;

    expect(
      latestLayout?.filter((item) => item.widgetType === "add-widget"),
    ).toHaveLength(1);
    expect(
      within(addWidgetItem).getByRole("combobox", { name: "Browse widgets" })
        .textContent,
    ).toContain("No widgets available");
  });

  it("renders real dashboard feature cards through the shared bento seam", () => {
    render(
      <AgentBentoLayout
        cards={[
          {
            id: "work-totals",
            widgetType: "work-totals",
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
            widgetType: "current-selection",
            children: <NoSelectionDetailCard />,
          },
        ]}
        initialWidth={1180}
        layout={[
          {
            h: 2,
            id: "work-totals",
            widgetType: "work-totals",
            w: 4,
            x: 0,
            y: 0,
          },
          {
            h: 4,
            id: "current-selection",
            widgetType: "current-selection",
            w: 8,
            x: 4,
            y: 0,
          },
        ]}
      />,
    );

    const board = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const workTotals = screen.getByRole("article", { name: "Work totals" });
    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });

    expect(board).toBeTruthy();
    expect(workTotals.dataset.dashboardPanelShell).toBe("grid-card");
    expect(currentSelection.dataset.dashboardPanelShell).toBe("grid-card");
    expect(workTotals.className).toContain("border-outline");
    expect(within(workTotals).getByLabelText("work totals")).toBeTruthy();
    expect(within(workTotals).getByText("In progress")).toBeTruthy();
    expect(within(workTotals).getByText("Completed")).toBeTruthy();
    expect(currentSelection.className).toContain("bg-surface-container-high");
    expect(
      within(currentSelection).getByRole("button", { name: "Undo selection" }),
    ).toHaveProperty("disabled", true);
    expect(
      within(currentSelection).getByRole("button", { name: "Redo selection" }),
    ).toHaveProperty("disabled", true);
    expect(
      within(currentSelection).getByText(
        "Select a workstation, work item, or state node to inspect live details.",
      ),
    ).toBeTruthy();
    expect(getGridItem("Work totals").dataset.bentoCardId).toBe("work-totals");
    expect(getGridItem("Current selection").dataset.bentoCardId).toBe(
      "current-selection",
    );
  });
});

describe("AgentBentoCard", () => {
  it("uses the shared ScrollArea primitive by default for card bodies", () => {
    render(
      <AgentBentoCard
        bodyProps={{ "data-testid": "submit-work-body" }}
        title="Submit work"
      >
        <p>Long form body</p>
      </AgentBentoCard>,
    );

    const card = screen.getByRole("article", { name: "Submit work" });
    const viewport = card.querySelector("[data-radix-scroll-area-viewport]");

    expect(viewport).toBeTruthy();
    expect(card.className).toContain("h-full");
    expect(card.className).toContain("overflow-hidden");
    expect(viewport?.className).toContain("af-body-text");
    expect(viewport?.className).toContain("h-full");
    expect(screen.getByTestId("submit-work-body")).toBe(viewport);
  });

  it("supports explicit localized body scrolling through the shared ScrollArea primitive", () => {
    render(
      <AgentBentoCard
        bodyProps={{ "data-testid": "submit-work-scroll-body" }}
        bodyScroll
        title="Submit work"
      >
        <p>Long form body</p>
      </AgentBentoCard>,
    );

    const card = screen.getByRole("article", { name: "Submit work" });
    const viewport = card.querySelector("[data-radix-scroll-area-viewport]");

    expect(card.className).toContain("h-full");
    expect(card.className).toContain("overflow-hidden");
    expect(viewport).toBeTruthy();
    expect(viewport?.hasAttribute("data-radix-scroll-area-viewport")).toBe(
      true,
    );
    expect(screen.getByTestId("submit-work-scroll-body")).toBe(viewport);
  });

  it("renders the canonical shared header with an h3 title and header drag handle", () => {
    render(
      <AgentBentoCard title="Work totals">
        <p>Totals body</p>
      </AgentBentoCard>,
    );

    const card = screen.getByRole("article", { name: "Work totals" });
    const header = card.querySelector("header");

    expect(header).toBeTruthy();
    expect(
      within(card).getByRole("heading", { level: 3, name: "Work totals" }),
    ).toBeTruthy();
    expect(header?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(header?.className).toContain("cursor-grab");
    expect(card.querySelectorAll("header")).toHaveLength(1);
  });

  it("renders optional header actions in the shared tools region without a move control", () => {
    render(
      <AgentBentoCard
        headerAction={<button type="button">Remove card</button>}
        title="Current selection"
      >
        <p>Selection body</p>
      </AgentBentoCard>,
    );

    const card = screen.getByRole("article", { name: "Current selection" });
    const header = card.querySelector("header");
    const toolsRegion = header?.lastElementChild;
    const removeButton = within(card).getByRole("button", {
      name: "Remove card",
    });

    expect(removeButton).toBeTruthy();
    expect(header?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(toolsRegion?.contains(removeButton)).toBe(true);
  });

  it("does not start grid drag when a header action button receives pointer down", async () => {
    const onLayoutChange = vi.fn();

    render(
      <AgentBentoLayout
        cards={[
          {
            id: "activity",
            widgetType: "activity",
            children: (
              <AgentBentoCard
                headerAction={<button type="button">Remove card</button>}
                title="Current activity"
              >
                <p>Active workstation graph goes here.</p>
              </AgentBentoCard>
            ),
          },
        ]}
        initialWidth={960}
        layout={[
          { h: 2, id: "activity", widgetType: "activity", w: 6, x: 0, y: 0 },
        ]}
        onLayoutChange={onLayoutChange}
      />,
    );

    const card = screen.getByRole("article", { name: "Current activity" });
    const gridItem = card.closest(".react-grid-item");
    const removeButton = within(card).getByRole("button", {
      name: "Remove card",
    });

    fireEvent.mouseDown(removeButton, {
      button: 0,
      buttons: 1,
      clientX: 120,
      clientY: 40,
    });
    fireEvent.mouseMove(document, {
      buttons: 1,
      clientX: 340,
      clientY: 40,
    });
    fireEvent.mouseUp(document, {
      button: 0,
      clientX: 340,
      clientY: 40,
    });

    await waitFor(() => {
      expect(gridItem?.classList.contains("react-draggable-dragging")).toBe(
        false,
      );
    });
    expect(onLayoutChange).not.toHaveBeenCalled();
  });

  it("supports compact shared chrome for dense dashboard cards", () => {
    render(
      <AgentBentoCard
        bodyProps={{ "data-testid": "factory-graph-body" }}
        chromeDensity="compact"
        title="Factory graph"
      >
        <p>Graph content</p>
      </AgentBentoCard>,
    );

    const card = screen.getByRole("article", { name: "Factory graph" });
    const header = card.querySelector("header");
    const body = screen.getByTestId("factory-graph-body");

    expect(header?.className).toContain("min-h-11");
    expect(header?.className).toContain("px-3");
    expect(header?.className).toContain("cursor-grab");
    expect(body.className).toContain("px-3");
    expect(body.className).toContain("pt-3");
    expect(body.className).toContain("pb-4");
    expect(body.className).toContain("gap-2");
  });
});
