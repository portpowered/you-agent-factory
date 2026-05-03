import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DashboardTraceDispatch, DashboardWorkItemRef } from "../../api/dashboard/types";
import { TraceWorkstationPath } from "./trace-workstation-path";

vi.mock("./trace-elk-layout", () => ({
  getCachedTraceGraphLayout: () => null,
  async layoutTraceGraphWithElk<TNode>(nodes: TNode[]): Promise<TNode[]> {
    return nodes;
  },
  traceGraphLayoutKey: () => "trace-layout-test",
}));

vi.mock("@xyflow/react", async () => {
  return {
    Background: () => null,
    Controls: () => null,
    Handle: () => null,
    MarkerType: { ArrowClosed: "arrowclosed" },
    Position: { Left: "left", Right: "right" },
    ReactFlow: ({
      edges,
      nodes,
    }: {
      edges: Array<{ id: string; source: string; target: string }>;
      nodes: Array<{ id: string }>;
    }) => (
      <div
        data-edges={JSON.stringify(edges)}
        data-node-ids={JSON.stringify(nodes.map((node) => node.id))}
        data-testid="trace-react-flow"
      />
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

describe("TraceWorkstationPath", () => {
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

