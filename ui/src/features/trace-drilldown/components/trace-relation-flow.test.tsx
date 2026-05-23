import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { within } from "@testing-library/react";

import type { DashboardWorkRelation } from "../../api/dashboard/types";
import { TraceRelationFlow } from "./trace-relation-flow";

vi.mock("../lib/trace-elk-layout", () => ({
  getCachedTraceGraphLayout: () => null,
  async layoutTraceGraphWithElk<TNode>(nodes: TNode[]): Promise<TNode[]> {
    return nodes;
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
      nodeTypes: Record<string, (props: { data: Record<string, unknown> }) => ReactNode>;
      nodes: Array<{
        data: Record<string, unknown>;
        id: string;
        type: string;
      }>;
    }) => (
      <div
        data-edge-payload={JSON.stringify(edges)}
        data-node-ids={JSON.stringify(nodes.map((node) => node.id))}
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

describe("TraceRelationFlow", () => {
  afterEach(() => {
    cleanup();
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
    ).toContain("\"backgroundColor\":\"rgb(from var(--color-af-surface) r g b / 0.88)\"");
    expect(
      screen
        .getByTestId("trace-relation-flow-controls")
        .getAttribute("data-controls-style"),
    ).toContain("\"borderRadius\":8");

    const implementButton = screen.getByRole("button", {
      name: "Implement story",
    });
    expect(implementButton.className).toContain("border-af-success/20");
    expect(within(implementButton).getByText("PARENT CHILD")).toBeTruthy();
    expect(within(implementButton).getByText("DONE")).toBeTruthy();
    expect(screen.getByText("FAILED")).toBeTruthy();

    fireEvent.click(implementButton);
    expect(onSelectWorkID).toHaveBeenCalledWith("work-implement");
  });

  it("renders semantic edge labels and tones for comparable relation states", () => {
    render(<TraceRelationFlow relations={RELATIONS} />);

    const edges = renderedEdges();
    expect(edges[0]?.ariaLabel).toBe(
      "PARENT CHILD relation from Plan story to Implement story, requiring DONE",
    );
    expect(edges[0]?.style?.stroke).toBe("var(--color-af-success)");
    expect(edges[0]?.style?.strokeDasharray).toBe("7 5");
    expect(edges[1]?.style?.stroke).toBe("var(--color-af-danger-ink)");
  });
});
