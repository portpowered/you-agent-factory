import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DashboardTraceDispatch, DashboardWorkItemRef } from "../../../api/dashboard/types";
import { TraceWorkstationPath } from "./trace-workstation-path";

vi.mock("../lib/trace-elk-layout", () => ({
  getCachedTraceGraphLayout: () => null,
  async layoutTraceGraphWithElk<TNode>(nodes: TNode[]): Promise<TNode[]> {
    return nodes;
  },
  traceGraphLayoutKey: () => "trace-layout-test",
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
        data-testid="trace-react-flow-background"
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
        data-testid="trace-react-flow-controls"
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
      edges: Array<{ id: string; source: string; target: string }>;
      nodeTypes: Record<string, (props: { data: Record<string, unknown> }) => ReactNode>;
      nodes: Array<{ data: Record<string, unknown>; id: string; type: string }>;
    }) => (
      <div
        data-edges={JSON.stringify(edges)}
        data-node-ids={JSON.stringify(nodes.map((node) => node.id))}
        data-testid="trace-react-flow"
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

function buildWorkItem(
  workID: string,
  overrides: Partial<DashboardWorkItemRef> = {},
): DashboardWorkItemRef {
  return {
    display_name: workID,
    work_id: workID,
    work_type_id: "story",
    ...overrides,
  };
}

function buildDispatch(
  dispatchID: string,
  overrides: Partial<DashboardTraceDispatch> = {},
): DashboardTraceDispatch {
  return {
    dispatch_id: dispatchID,
    duration_millis: 1000,
    end_time: "2026-04-22T18:00:01Z",
    outcome: "ACCEPTED",
    start_time: "2026-04-22T18:00:00Z",
    transition_id: dispatchID,
    workstation_name: dispatchID,
    ...overrides,
  };
}

function renderedEdgePairs(): string[] {
  const edgePayload = screen.getByTestId("trace-react-flow").getAttribute("data-edges");
  if (!edgePayload) {
    throw new Error("Expected mock React Flow edges to be captured.");
  }

  return (JSON.parse(edgePayload) as Array<{ source: string; target: string }>)
    .map((edge) => `${edge.source}->${edge.target}`)
    .sort();
}

describe("TraceWorkstationPath lineage", () => {
  afterEach(() => {
    cleanup();
  });

  it("prefers explicit predecessor chains and preserves fan-in edges", async () => {
    render(
      <TraceWorkstationPath
        dispatches={[
          buildDispatch("dispatch-plan", {
            current_chaining_trace_id: "trace-plan-chain",
            output_items: [buildWorkItem("work-reviewed", {
              current_chaining_trace_id: "trace-plan-chain",
            })],
          }),
          buildDispatch("dispatch-research", {
            current_chaining_trace_id: "trace-research-chain",
            output_items: [buildWorkItem("work-context", {
              current_chaining_trace_id: "trace-research-chain",
            })],
          }),
          buildDispatch("dispatch-implement", {
            input_items: [buildWorkItem("work-reviewed")],
            previous_chaining_trace_ids: [
              "trace-plan-chain",
              "trace-research-chain",
            ],
          }),
        ]}
      />,
    );

    await waitFor(() => {
      expect(
        renderedEdgePairs().filter((edge) => edge.endsWith("->dispatch-implement")),
      ).toEqual([
        "dispatch-plan->dispatch-implement",
        "dispatch-research->dispatch-implement",
      ]);
    });

    expect(
      screen
        .getByRole("region", { name: "Dispatch relationship graph" })
        .getAttribute("data-dashboard-graph-frame"),
    ).toBe("true");
    expect(
      screen
        .getByTestId("trace-react-flow-controls")
        .getAttribute("data-fit-view-options"),
    ).toBe(JSON.stringify({ maxZoom: 1.15, padding: 0.16 }));
    expect(
      screen
        .getByTestId("trace-react-flow-controls")
        .getAttribute("data-show-interactive"),
    ).toBe("false");
    expect(
      screen
        .getByTestId("trace-react-flow-background")
        .getAttribute("data-background-color"),
    ).toBe("var(--color-af-edge-muted-soft)");
    expect(
      screen
        .getByTestId("trace-react-flow-background")
        .getAttribute("data-background-gap"),
    ).toBe("24");
    expect(
      screen
        .getByTestId("trace-react-flow-background")
        .getAttribute("data-background-size"),
    ).toBe("1");
    expect(
      screen
        .getByTestId("trace-react-flow-controls")
        .getAttribute("data-controls-style"),
    ).toContain("\"backgroundColor\":\"var(--color-af-graph-controls-surface)\"");
    expect(
      screen
        .getByTestId("trace-react-flow-controls")
        .getAttribute("data-controls-style"),
    ).toContain("\"borderRadius\":8");
  });

  it("falls back to output-to-input work lineage when chaining metadata is absent", async () => {
    render(
      <TraceWorkstationPath
        dispatches={[
          buildDispatch("dispatch-plan", {
            output_items: [buildWorkItem("work-reviewed")],
          }),
          buildDispatch("dispatch-implement", {
            input_items: [buildWorkItem("work-reviewed")],
          }),
        ]}
      />,
    );

    await waitFor(() => {
      expect(renderedEdgePairs()).toEqual([
        "dispatch-plan->dispatch-implement",
      ]);
    });
  });

  it("falls back to sequential ordering when no explicit or work-item lineage is available", async () => {
    render(
      <TraceWorkstationPath
        dispatches={[
          buildDispatch("dispatch-plan"),
          buildDispatch("dispatch-review"),
          buildDispatch("dispatch-implement"),
        ]}
      />,
    );

    await waitFor(() => {
      expect(renderedEdgePairs()).toEqual([
        "dispatch-plan->dispatch-review",
        "dispatch-review->dispatch-implement",
      ]);
    });
  });
});

describe("TraceWorkstationPath semantics", () => {
  it("renders semantic workstation path tones and muted supporting copy", async () => {
    render(
      <TraceWorkstationPath
        dispatches={[
          buildDispatch("dispatch-plan", {
            input_items: [buildWorkItem("work-input")],
            output_items: [buildWorkItem("work-output")],
          }),
          buildDispatch("dispatch-repair", {
            input_items: [buildWorkItem("work-output")],
            outcome: "FAILED",
            output_items: [buildWorkItem("work-fixed")],
          }),
        ]}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("dispatch-plan")).toBeTruthy();
    });

    const acceptedNode = screen.getByText("dispatch-plan").closest("article");
    if (!acceptedNode) {
      throw new Error("Expected accepted workstation node to render.");
    }
    expect(acceptedNode.className).toContain("border-af-success-border");
    expect(acceptedNode.className).toContain("bg-af-success-surface");

    const failedNode = screen.getByText("dispatch-repair").closest("article");
    if (!failedNode) {
      throw new Error("Expected failed workstation node to render.");
    }
    expect(failedNode.className).toContain("border-af-danger-border");
    expect(failedNode.className).toContain("bg-af-danger-surface");

    const inputSummary = screen.getAllByText(/^In:/)[0];
    expect(inputSummary.className).toContain("text-af-text-muted");
  });
});
