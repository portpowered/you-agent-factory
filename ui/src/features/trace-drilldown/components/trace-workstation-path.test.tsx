// biome-ignore lint/style/noExcessiveLinesPerFile: trace workstation path coverage shares one mocked React Flow harness for lineage, layout bounds, and semantics regressions.
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type {
  DashboardTraceDispatch,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { resetTraceDispatchFactoryGraphLayoutCacheForTests } from "../hooks/use-trace-dispatch-factory-graph-layout";
import { TraceWorkstationPath } from "./trace-workstation-path";

const { mockBuildTraceFactoryGraphLayoutPositions } = vi.hoisted(() => ({
  mockBuildTraceFactoryGraphLayoutPositions: vi.fn(async () => new Map()),
}));

vi.mock("../lib/trace-factory-graph-layout", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../lib/trace-factory-graph-layout")>();
  return {
    ...actual,
    buildTraceFactoryGraphLayoutPositions:
      mockBuildTraceFactoryGraphLayoutPositions,
  };
});

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
      onError,
    }: {
      children?: ReactNode;
      edges: Array<{ id: string; source: string; target: string }>;
      nodeTypes: Record<
        string,
        (props: { data: Record<string, unknown> }) => ReactNode
      >;
      nodes: Array<{
        data: Record<string, unknown>;
        height?: number;
        id: string;
        initialHeight?: number;
        initialWidth?: number;
        measured?: { height: number; width: number };
        position?: { x: number; y: number };
        type: string;
        width?: number;
      }>;
      onError?: (id: string, message: string) => void;
    }) => (
      <div
        data-edges={JSON.stringify(edges)}
        data-has-on-error={String(Boolean(onError))}
        data-node-bounds={JSON.stringify(
          nodes.map((node) => ({
            height: node.height,
            id: node.id,
            initialHeight: node.initialHeight,
            initialWidth: node.initialWidth,
            measured: node.measured,
            width: node.width,
          })),
        )}
        data-node-ids={JSON.stringify(nodes.map((node) => node.id))}
        data-node-positions={JSON.stringify(
          nodes.map((node) => ({
            id: node.id,
            position: node.position,
          })),
        )}
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
  const edgePayload = screen
    .getByTestId("trace-react-flow")
    .getAttribute("data-edges");
  if (!edgePayload) {
    throw new Error("Expected mock React Flow edges to be captured.");
  }

  return (JSON.parse(edgePayload) as Array<{ source: string; target: string }>)
    .map((edge) => `${edge.source}->${edge.target}`)
    .sort();
}

function renderedNodeIDs(): string[] {
  const nodePayload = screen
    .getByTestId("trace-react-flow")
    .getAttribute("data-node-ids");
  if (!nodePayload) {
    throw new Error("Expected mock React Flow nodes to be captured.");
  }

  return JSON.parse(nodePayload) as string[];
}

type RenderedTraceFlowNodeBounds = {
  height?: number;
  initialHeight?: number;
  initialWidth?: number;
  measured?: { height: number; width: number };
  width?: number;
};

function renderedNodePositionsById(): Map<string, { x: number; y: number }> {
  const nodePayload = screen
    .getByTestId("trace-react-flow")
    .getAttribute("data-node-positions");
  if (!nodePayload) {
    throw new Error("Expected mock React Flow node positions to be captured.");
  }

  return new Map(
    (
      JSON.parse(nodePayload) as Array<{
        id: string;
        position?: { x: number; y: number };
      }>
    ).map((node) => [node.id, node.position ?? { x: 0, y: 0 }]),
  );
}

function renderedNodeBoundsById(): Map<string, RenderedTraceFlowNodeBounds> {
  const nodePayload = screen
    .getByTestId("trace-react-flow")
    .getAttribute("data-node-bounds");
  if (!nodePayload) {
    throw new Error("Expected mock React Flow node bounds to be captured.");
  }

  return new Map(
    (
      JSON.parse(nodePayload) as Array<
        { id: string } & RenderedTraceFlowNodeBounds
      >
    ).map((node) => [
      node.id,
      {
        height: node.height,
        initialHeight: node.initialHeight,
        initialWidth: node.initialWidth,
        measured: node.measured,
        width: node.width,
      },
    ]),
  );
}

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  resetTraceDispatchFactoryGraphLayoutCacheForTests();
  mockBuildTraceFactoryGraphLayoutPositions.mockReset();
  mockBuildTraceFactoryGraphLayoutPositions.mockResolvedValue(new Map());
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

describe("TraceWorkstationPath explicit lineage", () => {
  it("prefers explicit predecessor chains and preserves fan-in edges", async () => {
    render(
      <TraceWorkstationPath
        dispatches={[
          buildDispatch("dispatch-plan", {
            current_chaining_trace_id: "trace-plan-chain",
            output_items: [
              buildWorkItem("work-reviewed", {
                current_chaining_trace_id: "trace-plan-chain",
              }),
            ],
          }),
          buildDispatch("dispatch-research", {
            current_chaining_trace_id: "trace-research-chain",
            output_items: [
              buildWorkItem("work-context", {
                current_chaining_trace_id: "trace-research-chain",
              }),
            ],
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
        renderedEdgePairs().filter((edge) =>
          edge.endsWith("->dispatch-implement"),
        ),
      ).toEqual([
        "dispatch-plan->dispatch-implement",
        "dispatch-research->dispatch-implement",
      ]);
    });

    const graphFrame = screen.getByRole("region", {
      name: "Dispatch relationship graph",
    });
    expect(graphFrame.getAttribute("data-dashboard-graph-frame")).toBe("true");
    expect(graphFrame.className).toContain("shadow-none");
    expect(graphFrame.className).not.toContain("shadow-af-card");
    expect(graphFrame.className).not.toContain("shadow-af-panel");
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
    ).toBe("var(--color-outline)");
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
    ).toContain('"backgroundColor":"var(--color-surface)"');
    expect(
      screen
        .getByTestId("trace-react-flow-controls")
        .getAttribute("data-controls-style"),
    ).toContain('"borderRadius":8');
    expect(
      screen.getByTestId("trace-react-flow").getAttribute("data-has-on-error"),
    ).toBe("true");
  });
});

describe("TraceWorkstationPath fallback lineage", () => {
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

describe("TraceWorkstationPath localization", () => {
  it("renders zh-CN graph chrome without changing workstation names", async () => {
    render(
      <TraceWorkstationPath
        dispatches={[
          buildDispatch("dispatch-plan", {
            current_chaining_trace_id: "trace-plan-chain",
            output_items: [
              buildWorkItem("work-reviewed", {
                current_chaining_trace_id: "trace-plan-chain",
                display_name: "已审阅故事",
              }),
            ],
            workstation_name: "计划",
          }),
          buildDispatch("dispatch-implement", {
            input_items: [
              buildWorkItem("work-reviewed", {
                display_name: "已审阅故事",
              }),
            ],
            previous_chaining_trace_ids: ["trace-plan-chain"],
            workstation_name: "实现",
          }),
        ]}
        locale="zh-CN"
      />,
    );

    await waitFor(() => {
      expect(
        screen
          .getByRole("region", { name: "分派关系图" })
          .getAttribute("data-dashboard-graph-frame"),
      ).toBe("true");
    });

    expect(screen.getByText("计划")).toBeTruthy();
    expect(screen.getByText("实现")).toBeTruthy();
    expect(
      screen
        .getByTestId("trace-react-flow-controls")
        .getAttribute("data-fit-view-options"),
    ).toBe(JSON.stringify({ maxZoom: 1.15, padding: 0.16 }));
    expect(
      screen
        .getByTestId("trace-react-flow-controls")
        .getAttribute("data-controls-style"),
    ).toContain('"backgroundColor":"var(--color-surface)"');
  });
});

describe("TraceWorkstationPath captured selections", () => {
  it("renders the captured trace-654e selection with seven dispatch nodes", async () => {
    render(
      <TraceWorkstationPath
        dispatches={[
          buildDispatch("8a56a3ce-6277-41d8-9bc8-840aa10a8d74", {
            end_time: "2026-05-27T14:15:24.172854+07:00",
            outcome: "ACCEPTED",
            start_time: "2026-05-27T14:13:35.734332+07:00",
            transition_id: "plan",
            workstation_name: "plan",
          }),
          buildDispatch("145638a2-67c9-4f2a-8a7d-6297ebcd7a19", {
            end_time: "2026-05-27T14:15:25.614203+07:00",
            outcome: "ACCEPTED",
            start_time: "2026-05-27T14:15:24.183895+07:00",
            transition_id: "setup-workspace",
            workstation_name: "setup-workspace",
          }),
          buildDispatch("a5399deb-dffe-4a6d-9b4f-0310aa988bf2", {
            end_time: "2026-05-27T14:24:49.786812+07:00",
            outcome: "CONTINUE",
            start_time: "2026-05-27T14:15:25.624941+07:00",
            transition_id: "process",
            workstation_name: "process",
          }),
          buildDispatch("534f91c4-4e83-4310-b211-dbb3ee3cabd1", {
            end_time: "2026-05-27T14:35:06.431153+07:00",
            outcome: "CONTINUE",
            start_time: "2026-05-27T14:24:49.796584+07:00",
            transition_id: "process",
            workstation_name: "process",
          }),
          buildDispatch("be0ca2a8-c4f7-42c2-8bd3-a54c0bd9de25", {
            end_time: "2026-05-27T14:43:28.570516+07:00",
            outcome: "CONTINUE",
            start_time: "2026-05-27T14:35:06.441828+07:00",
            transition_id: "process",
            workstation_name: "process",
          }),
          buildDispatch("74d8f3b3-d91b-4bcc-927d-b2643e71bc8a", {
            end_time: "2026-05-27T14:54:13.015495+07:00",
            outcome: "CONTINUE",
            start_time: "2026-05-27T14:43:28.579605+07:00",
            transition_id: "process",
            workstation_name: "process",
          }),
          buildDispatch("82d4be6a-68c3-4c94-ad3b-53fd53326015", {
            end_time: "2026-05-27T15:19:09.330262+07:00",
            outcome: "ACCEPTED",
            start_time: "2026-05-27T14:54:13.02456+07:00",
            transition_id: "process",
            workstation_name: "process",
          }),
        ]}
      />,
    );

    await waitFor(() => {
      expect(renderedNodeIDs().sort()).toEqual(
        [
          "8a56a3ce-6277-41d8-9bc8-840aa10a8d74",
          "145638a2-67c9-4f2a-8a7d-6297ebcd7a19",
          "a5399deb-dffe-4a6d-9b4f-0310aa988bf2",
          "534f91c4-4e83-4310-b211-dbb3ee3cabd1",
          "be0ca2a8-c4f7-42c2-8bd3-a54c0bd9de25",
          "74d8f3b3-d91b-4bcc-927d-b2643e71bc8a",
          "82d4be6a-68c3-4c94-ad3b-53fd53326015",
        ].sort(),
      );
    });
  });
});

describe("TraceWorkstationPath layout", () => {
  it("applies async factory layout positions after layout resolves", async () => {
    const expectedLayout = new Map([
      ["dispatch-plan", { x: 420, y: 80, width: 156, height: 196 }],
      ["dispatch-implement", { x: 860, y: 160, width: 156, height: 196 }],
    ]);
    mockBuildTraceFactoryGraphLayoutPositions.mockResolvedValue(expectedLayout);

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
      const renderedPositions = renderedNodePositionsById();
      const renderedBounds = renderedNodeBoundsById();
      for (const [nodeId, layout] of expectedLayout) {
        expect(renderedPositions.get(nodeId)).toEqual({
          x: layout.x,
          y: layout.y,
        });
        expect(renderedBounds.get(nodeId)).toEqual({
          height: layout.height,
          initialHeight: layout.height,
          initialWidth: layout.width,
          measured: { height: layout.height, width: layout.width },
          width: layout.width,
        });
      }
    });
  });
});

describe("TraceWorkstationPath semantics", () => {
  it("renders factory-style workstation identity without dispatch metadata", async () => {
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

    expect(screen.queryByText("Dispatch")).toBeNull();
    expect(screen.queryByText("Workstation")).toBeNull();
    expect(screen.queryByText("Accepted")).toBeNull();
    expect(screen.queryByText("Failed")).toBeNull();
    expect(screen.queryByText(/^In:/)).toBeNull();
    expect(screen.queryByText(/^Out:/)).toBeNull();

    const acceptedNode = screen.getByText("dispatch-plan").closest("article");
    if (!acceptedNode) {
      throw new Error("Expected accepted workstation node to render.");
    }
    expect(acceptedNode.className).toContain(
      "border-outline-variant bg-surface-container-highest",
    );
    expect(acceptedNode.className).toContain("border-info-border");

    const failedNode = screen.getByText("dispatch-repair").closest("article");
    if (!failedNode) {
      throw new Error("Expected failed workstation node to render.");
    }
    expect(failedNode.className).toContain(
      "border-outline-variant bg-surface-container-highest",
    );
    expect(failedNode.className).toContain("border-info-border");
  });
});
