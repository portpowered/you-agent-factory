import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import type { DashboardWorkRelation } from "../../api/dashboard/types";
import { TraceRelationFlow } from "./trace-relation-flow";

vi.mock("../lib/trace-elk-layout", () => ({
  getCachedTraceGraphLayout: () => null,
  async layoutTraceGraphWithElk<TNode>(nodes: TNode[]): Promise<TNode[]> {
    return nodes.map((node, index) => ({
      ...node,
      position: { x: index * 260, y: index * 140 },
    }));
  },
  traceGraphLayoutKey: () => "trace-relation-layout-test",
}));

vi.mock("@xyflow/react", async () => {
  return {
    Background: ({
      color,
      gap,
      size,
    }: {
      color?: string;
      gap?: number;
      size?: number;
    }) => (
      <div
        data-background-color={color}
        data-background-gap={String(gap ?? "")}
        data-background-size={String(size ?? "")}
        data-testid="trace-relation-flow-background"
      />
    ),
    Controls: ({
      fitViewOptions,
      showInteractive,
      style,
    }: {
      fitViewOptions?: Record<string, number>;
      showInteractive?: boolean;
      style?: Record<string, string | number>;
    }) => (
      <div
        data-controls-style={JSON.stringify(style ?? null)}
        data-fit-view-options={JSON.stringify(fitViewOptions ?? null)}
        data-show-interactive={String(showInteractive ?? true)}
        data-testid="trace-relation-flow-controls"
      />
    ),
    Handle: () => null,
    MarkerType: { ArrowClosed: "arrowclosed" },
    Position: { Left: "left", Right: "right" },
    ReactFlow: ({
      children,
      edges,
      nodeTypes,
      nodes,
    }: {
      children?: ReactNode;
      edges: Array<{
        ariaLabel?: string;
        id: string;
        source: string;
        style?: Record<string, string | number | undefined>;
        target: string;
      }>;
      nodeTypes: Record<
        string,
        (props: { data: Record<string, unknown> }) => ReactNode
      >;
      nodes: Array<{
        data: Record<string, unknown>;
        id: string;
        position?: { x: number; y: number };
        type: string;
      }>;
    }) => (
      <div
        data-edge-payload={JSON.stringify(edges)}
        data-node-ids={JSON.stringify(nodes.map((node) => node.id))}
        data-node-positions={JSON.stringify(nodes.map((node) => node.position))}
        data-testid="trace-relation-react-flow"
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
  };
});

const RELATIONS: DashboardWorkRelation[] = [
  {
    request_id: "request-parent-child",
    required_state: "DONE",
    source_work_id: "work-plan",
    source_work_name: "Plan story",
    target_work_id: "work-implement",
    target_work_name: "Implement story",
    type: "PARENT_CHILD",
  },
  {
    request_id: "request-retry",
    required_state: "FAILED",
    source_work_id: "work-implement",
    source_work_name: "Implement story",
    target_work_id: "work-repair",
    target_work_name: "Repair story",
    type: "RETRY",
  },
];

function renderedEdges() {
  const payload = screen
    .getByTestId("trace-relation-react-flow")
    .getAttribute("data-edge-payload");
  if (!payload) {
    throw new Error("Expected rendered edge payload.");
  }

  return JSON.parse(payload) as Array<{
    ariaLabel?: string;
    style?: Record<string, string | number | undefined>;
  }>;
}

function renderedNodePositions() {
  const payload = screen
    .getByTestId("trace-relation-react-flow")
    .getAttribute("data-node-positions");
  if (!payload) {
    throw new Error("Expected rendered node positions.");
  }

  return JSON.parse(payload) as Array<{ x: number; y: number }>;
}

describe("TraceRelationFlow", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("uses the shared dashboard graph chrome and preserves work-item selection", async () => {
    const onSelectWorkID = vi.fn();

    render(
      <TraceRelationFlow
        onSelectWorkID={onSelectWorkID}
        relations={RELATIONS}
      />,
    );

    await waitFor(() => {
      expect(
        screen
          .getByRole("region", { name: "Batch relation graph" })
          .getAttribute("data-dashboard-graph-frame"),
      ).toBe("true");
    });

    expect(
      screen
        .getByTestId("trace-relation-flow-controls")
        .getAttribute("data-fit-view-options"),
    ).toBe(JSON.stringify({ maxZoom: 1.5, padding: 0.08 }));
    expect(
      screen
        .getByTestId("trace-relation-flow-controls")
        .getAttribute("data-show-interactive"),
    ).toBe("false");
    expect(
      screen
        .getByTestId("trace-relation-flow-background")
        .getAttribute("data-background-color"),
    ).toBe("var(--color-af-edge-muted-soft)");
    expect(
      screen
        .getByTestId("trace-relation-flow-background")
        .getAttribute("data-background-gap"),
    ).toBe("24");
    expect(
      screen
        .getByTestId("trace-relation-flow-background")
        .getAttribute("data-background-size"),
    ).toBe("1");
    expect(
      screen
        .getByTestId("trace-relation-flow-controls")
        .getAttribute("data-controls-style"),
    ).toContain('"backgroundColor":"var(--color-af-graph-controls-surface)"');
    expect(
      screen
        .getByTestId("trace-relation-flow-controls")
        .getAttribute("data-controls-style"),
    ).toContain('"borderRadius":8');

    const implementButton = screen.getByRole("button", {
      name: "Implement story",
    });
    expect(implementButton.className).toContain("border-af-success-border");
    expect(implementButton.className).toContain("bg-af-success-surface");
    expect(within(implementButton).getByText("Parent-child")).toBeTruthy();
    expect(within(implementButton).getByText("Done")).toBeTruthy();
    expect(screen.getByText("Failed")).toBeTruthy();

    fireEvent.click(implementButton);
    expect(onSelectWorkID).toHaveBeenCalledWith("work-implement");
  });

  it("renders semantic edge labels and tones for comparable relation states", async () => {
    render(<TraceRelationFlow relations={RELATIONS} />);

    await waitFor(() => {
      expect(renderedEdges()).toHaveLength(2);
    });

    const edges = renderedEdges();
    expect(edges[0]?.ariaLabel).toBe(
      "Parent-child relation from Plan story to Implement story, requiring Done",
    );
    expect(edges[0]?.style?.stroke).toBe("var(--color-af-success)");
    expect(edges[0]?.style?.strokeDasharray).toBe("7 5");
    expect(edges[1]?.style?.stroke).toBe("var(--color-af-danger-text)");
  });
});

describe("TraceRelationFlow layout", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("applies ELK positions after layout so relation nodes do not overlay", async () => {
    render(<TraceRelationFlow relations={RELATIONS} />);

    await waitFor(() => {
      expect(renderedNodePositions()).toEqual([
        { x: 0, y: 0 },
        { x: 260, y: 140 },
        { x: 520, y: 280 },
      ]);
    });
  });
});

describe("TraceRelationFlow localization", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("localizes relation enums in zh-CN and preserves raw values in unknown fallbacks", async () => {
    render(
      <TraceRelationFlow
        locale="zh-CN"
        relations={[
          {
            request_id: "request-unknown-state",
            required_state: "escalated_review",
            source_work_id: "work-review",
            source_work_name: "Review story",
            target_work_id: "work-fix",
            target_work_name: "Fix story",
            type: "SPAWNED_BY",
          },
        ]}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("region", { name: "批次关系图" })).toBeTruthy();
    });

    expect(screen.getAllByText("派生自").length).toBeGreaterThan(0);
    expect(
      screen.getAllByText("未知状态：escalated_review").length,
    ).toBeGreaterThan(0);
    expect(renderedEdges()[0]?.ariaLabel).toBe(
      "派生自关系：从 Review story 到 Fix story，要求 未知状态：escalated_review",
    );
  });
});
