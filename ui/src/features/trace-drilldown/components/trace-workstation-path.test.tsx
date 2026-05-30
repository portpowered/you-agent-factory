import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type {
  DashboardTraceDispatch,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { TraceWorkstationPath } from "./trace-workstation-path";

vi.mock("../lib/trace-factory-graph-layout", () => ({
  buildTraceFactoryGraphLayoutPositions: async () => new Map(),
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
      onError,
    }: {
      children?: ReactNode;
      edges: Array<{ id: string; source: string; target: string }>;
      nodeTypes: Record<
        string,
        (props: { data: Record<string, unknown> }) => ReactNode
      >;
      nodes: Array<{ data: Record<string, unknown>; id: string; type: string }>;
      onError?: (id: string, message: string) => void;
    }) => (
      <div
        data-edges={JSON.stringify(edges)}
        data-has-on-error={String(Boolean(onError))}
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

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
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
    ).toContain('"backgroundColor":"var(--color-af-graph-controls-surface)"');
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

    expect(screen.queryByText("Dispatch")).toBeNull();
    expect(screen.getAllByText("Workstation").length).toBeGreaterThan(0);

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
