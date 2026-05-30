import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "bun:test";

vi.mock("../lib/trace-factory-graph-layout", () => ({
  buildTraceFactoryGraphLayoutPositions: async () => new Map(),
}));

vi.mock("@xyflow/react", async () => ({
  Background: () => <div data-testid="trace-card-flow-background" />,
  Controls: () => <div data-testid="trace-card-flow-controls" />,
  Handle: () => null,
  MarkerType: { ArrowClosed: "arrowclosed" },
  Position: { Left: "left", Right: "right" },
  ReactFlow: ({
    children,
    nodeTypes,
    nodes,
    onError,
  }: {
    children?: ReactNode;
    nodeTypes: Record<
      string,
      (props: { data: Record<string, unknown> }) => ReactNode
    >;
    nodes: Array<{ data: Record<string, unknown>; id: string; type: string }>;
    onError?: (id: string, message: string) => void;
  }) => (
    <div
      data-has-on-error={String(Boolean(onError))}
      data-node-ids={JSON.stringify(nodes.map((node) => node.id))}
      data-testid="trace-card-react-flow"
    >
      {nodes.map((node) => {
        const NodeView = nodeTypes[node.type];
        return (
          <div key={node.id}>
            <NodeView data={node.data} />
          </div>
        );
      })}
      {children}
    </div>
  ),
  applyNodeChanges: (
    _changes: Array<Record<string, unknown>>,
    nodes: Array<Record<string, unknown>>,
  ) => nodes,
}));

import type { DashboardTrace } from "../../../api/dashboard/types";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../components/ui/dashboard-typography";
import { expectNoVerticalScrollContainer } from "../lib/trace-grid-card-scroll-test-helpers";
import { TraceGridBentoCard } from "./trace-grid-card";

const populatedTrace: DashboardTrace = {
  dispatches: [
    {
      input_items: [
        {
          display_name: "Active Story",
          current_chaining_trace_id: "trace-active-story-chain",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      current_chaining_trace_id: "trace-review-chain",
      dispatch_id: "dispatch-review-active",
      duration_millis: 1000,
      end_time: "2026-04-08T12:00:01Z",
      outcome: "ACCEPTED",
      output_items: [
        {
          display_name: "Reviewed Story",
          current_chaining_trace_id: "trace-review-chain",
          work_id: "work-reviewed-story",
          work_type_id: "story",
        },
      ],
      start_time: "2026-04-08T12:00:00Z",
      transition_id: "plan",
      workstation_name: "Plan",
    },
    {
      input_items: [
        {
          display_name: "Reviewed Story",
          current_chaining_trace_id: "trace-review-chain",
          work_id: "work-reviewed-story",
          work_type_id: "story",
        },
      ],
      current_chaining_trace_id: "trace-implement-chain",
      dispatch_id: "dispatch-implement-active",
      duration_millis: 2000,
      end_time: "2026-04-08T12:00:04Z",
      outcome: "ACCEPTED",
      output_items: [
        {
          display_name: "Implemented Story",
          current_chaining_trace_id: "trace-implement-chain",
          work_id: "work-implemented-story",
          work_type_id: "story",
        },
      ],
      previous_chaining_trace_ids: ["trace-review-chain"],
      start_time: "2026-04-08T12:00:02Z",
      transition_id: "implement",
      workstation_name: "Implement",
    },
  ],
  work_items: [
    {
      display_name: "Active Story",
      work_id: "work-active-story",
      work_type_id: "story",
    },
    {
      display_name: "Reviewed Story",
      work_id: "work-reviewed-story",
      work_type_id: "story",
    },
    {
      display_name: "Implemented Story",
      work_id: "work-implemented-story",
      work_type_id: "story",
    },
  ],
  trace_id: "trace-active-story",
  relations: [
    {
      request_id: "request-story-batch",
      required_state: "DONE",
      source_work_id: "work-active-story",
      source_work_name: "Active Story",
      target_work_id: "work-reviewed-story",
      target_work_name: "Reviewed Story",
      type: "PARENT_CHILD",
    },
  ],
  transition_ids: ["plan", "implement"],
  work_ids: ["work-active-story"],
  workstation_sequence: ["Plan", "Implement"],
};

function useBrowserShimsForTraceGridTests() {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });
}

describe("TraceGridBentoCard ready state", () => {
  useBrowserShimsForTraceGridTests();

  it("renders populated trace data as a bento card table", async () => {
    const onSelectWorkID = vi.fn();
    const { rerender } = render(
      <TraceGridBentoCard
        onSelectWorkID={onSelectWorkID}
        state={{ status: "ready", trace: populatedTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    expect(
      within(card).queryByText(
        "Resolves from selected-tick factory event history.",
      ),
    ).toBeNull();
    expect(within(card).getByText("Trace dispatch grid")).toBeTruthy();
    expect(within(card).getByText("Dispatch flow")).toBeTruthy();
    expect(
      await within(card).findByRole("region", {
        name: "Dispatch relationship graph",
      }),
    ).toBeTruthy();
    const table = within(card).getByRole("table");
    expect(table.className).toContain(DASHBOARD_BODY_TEXT_CLASS);
    const caption = within(card).getByText("Trace dispatch grid");
    expect(caption.className).toContain(DASHBOARD_SUPPORTING_LABEL_CLASS);
    const inputHeader = within(card).getByRole("columnheader", {
      name: "Input items",
    });
    expect(inputHeader.className).toContain(DASHBOARD_SUPPORTING_LABEL_CLASS);
    expect(within(card).getAllByText("Plan").length).toBeGreaterThan(0);
    expect(within(card).getAllByText("Implement").length).toBeGreaterThan(0);
    const dispatchPill = within(card)
      .getAllByText("dispatch-review-active")
      .find((element) => element.tagName === "SPAN");
    if (!dispatchPill) {
      throw new Error(
        "Expected dispatch pill to render in the trace grid table.",
      );
    }
    expect(dispatchPill.className).toContain(DASHBOARD_SUPPORTING_CODE_CLASS);
    expect(dispatchPill.className).toContain("border-af-info-border");
    expect(dispatchPill.className).toContain("bg-af-info-surface");
    expect(dispatchPill.className).toContain("py-0.5");
    expect(within(card).getByText("Accepted · 1s")).toBeTruthy();
    expect(within(card).getByText("Accepted · 2s")).toBeTruthy();
    const tableScroller = card.querySelector("[data-trace-dispatch-table]");
    expect(tableScroller?.className).toContain("overflow-x-auto");
    expect(tableScroller?.className).toContain("overflow-y-clip");
    expect(tableScroller?.className).toContain("overscroll-x-contain");
    expectNoVerticalScrollContainer(tableScroller as Element, {
      requireOverflowYClip: true,
    });
    expect(table.className).toContain("min-w-[640px]");
    expect(
      await within(card).findByRole("region", { name: "Batch relation graph" }),
    ).toBeTruthy();
    expect(
      within(card).queryByRole("columnheader", { name: "Consumed tokens" }),
    ).toBeNull();
    expect(
      within(card).queryByRole("columnheader", { name: "Output mutations" }),
    ).toBeNull();
    expect(
      within(card).queryByRole("columnheader", { name: "Workstation run" }),
    ).toBeNull();
    const traceIDSection = within(card).getByText("Trace ID").closest("div");
    expect(traceIDSection?.querySelector("dd")?.className).toContain(
      "[overflow-wrap:anywhere]",
    );

    rerender(
      <TraceGridBentoCard
        onSelectWorkID={onSelectWorkID}
        state={{ status: "ready", trace: populatedTrace }}
      />,
    );

    expect(
      await within(card).findByRole("region", { name: "Batch relation graph" }),
    ).toBeTruthy();
  });
});

describe("TraceGridBentoCard work item selection", () => {
  useBrowserShimsForTraceGridTests();

  it("expands resolved work items and preserves selection callbacks", () => {
    const onSelectWorkID = vi.fn();
    render(
      <TraceGridBentoCard
        onSelectWorkID={onSelectWorkID}
        state={{ status: "ready", trace: populatedTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const workItemsSection = within(card).getByRole("region", {
      name: "3 work items",
    });
    const expandButton = within(workItemsSection).getByRole("button", {
      name: "Expand",
    });
    expect(expandButton.getAttribute("aria-expanded")).toBe("false");
    expect(
      within(workItemsSection).queryByText("(story):Implemented Story"),
    ).toBeNull();

    fireEvent.click(expandButton);

    expect(expandButton.className).toContain("px-2.5");
    expect(expandButton.className).toContain("py-2");
    expect(expandButton.getAttribute("aria-expanded")).toBe("true");
    expect(
      within(card).getAllByText("(story):Active Story").length,
    ).toBeGreaterThan(0);
    expect(
      within(card).getAllByText("(story):Reviewed Story").length,
    ).toBeGreaterThan(0);
    expect(
      within(card).getAllByText("(story):Implemented Story").length,
    ).toBeGreaterThan(0);
    const activeStoryButtons = within(card).getAllByRole("button", {
      name: "(story):Active Story",
    });
    expect(activeStoryButtons[0]?.className).toContain(
      "border-af-accent-border",
    );

    fireEvent.click(activeStoryButtons[0]);
    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story");
  });
});

class ControlledResizeObserver {
  public static instances: ControlledResizeObserver[] = [];

  public constructor(private readonly callback: ResizeObserverCallback) {
    ControlledResizeObserver.instances.push(this);
  }

  public disconnect(): void {}

  public observe(target: Element): void {
    this.resize(target, 0, 0);
  }

  public unobserve(): void {}

  public resize(target: Element, width: number, height: number): void {
    this.callback(
      [
        {
          borderBoxSize: [],
          contentBoxSize: [],
          contentRect: {
            bottom: height,
            height,
            left: 0,
            right: width,
            top: 0,
            width,
            x: 0,
            y: 0,
            toJSON: () => ({}),
          },
          devicePixelContentBoxSize: [],
          target,
        } as ResizeObserverEntry,
      ],
      this,
    );
  }
}

describe("TraceGridBentoCard graph sizing", () => {
  const originalResizeObserver = globalThis.ResizeObserver;

  beforeEach(() => {
    ControlledResizeObserver.instances = [];
    globalThis.ResizeObserver =
      ControlledResizeObserver as unknown as typeof ResizeObserver;
  });

  afterEach(() => {
    cleanup();
    globalThis.ResizeObserver = originalResizeObserver;
  });

  it("waits for measured graph dimensions before mounting React Flow", async () => {
    render(
      <TraceGridBentoCard state={{ status: "ready", trace: populatedTrace }} />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    expect(within(card).getByText("Dispatch flow")).toBeTruthy();
    expect(within(card).queryByTestId("trace-card-react-flow")).toBeNull();

    const viewports = card.querySelectorAll("[data-trace-graph-viewport]");
    expect(viewports).toHaveLength(2);
    const frames = card.querySelectorAll("[data-dashboard-graph-frame]");
    expect(frames).toHaveLength(2);
    expect(
      [...frames].every(
        (frame) =>
          frame.className.includes("resize") &&
          frame.className.includes("min-w-80") &&
          frame.className.includes("max-w-full"),
      ),
    ).toBe(true);
    expect(
      [...viewports].every(
        (viewport) =>
          viewport.getAttribute("style")?.includes("height: 100%") &&
          viewport.getAttribute("style")?.includes("width: 100%"),
      ),
    ).toBe(true);

    act(() => {
      ControlledResizeObserver.instances.forEach((observer, index) => {
        observer.resize(viewports[index] ?? viewports[0], 800, 320);
      });
    });

    await waitFor(() => {
      expect(within(card).getAllByTestId("trace-card-react-flow")).toHaveLength(
        2,
      );
    });
    expect(
      within(card)
        .getAllByTestId("trace-card-react-flow")
        .every((flow) => flow.getAttribute("data-has-on-error") === "true"),
    ).toBe(true);
  });
});

describe("TraceGridBentoCard state handling", () => {
  useBrowserShimsForTraceGridTests();

  it("renders explicit empty, loading, and error states", () => {
    const { container, rerender } = render(
      <TraceGridBentoCard
        state={{ status: "empty", workID: "work-missing" }}
      />,
    );

    expect(screen.getByText("Trace history unavailable")).toBeTruthy();

    rerender(
      <TraceGridBentoCard
        state={{ status: "loading", workID: "work-active" }}
      />,
    );
    expect(screen.getByText("Loading trace")).toBeTruthy();
    expect(
      screen.getByText("Reconstructing dispatch history for work-active."),
    ).toBeTruthy();
    expect(container.querySelectorAll(".animate-pulse")).toHaveLength(3);

    rerender(
      <TraceGridBentoCard
        state={{ status: "error", message: "network failed" }}
      />,
    );
    expect(screen.getByText("Trace lookup failed")).toBeTruthy();
    expect(screen.getByText("network failed")).toBeTruthy();
  });
});

describe("TraceGridBentoCard localization", () => {
  useBrowserShimsForTraceGridTests();

  it("renders zh-CN trace shell labels and graph regions", async () => {
    render(
      <TraceGridBentoCard
        locale="zh-CN"
        state={{ status: "ready", trace: populatedTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "追踪下钻" });
    expect(within(card).getByText("追踪分派表")).toBeTruthy();
    expect(within(card).getByText("分派流")).toBeTruthy();
    expect(
      await within(card).findByRole("region", { name: "分派关系图" }),
    ).toBeTruthy();
  });
});
