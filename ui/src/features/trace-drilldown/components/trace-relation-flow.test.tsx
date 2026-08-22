// biome-ignore lint/style/noExcessiveLinesPerFile: relation graph coverage keeps one mocked React Flow harness alongside the textual fallback and responsive surface assertions.
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
import { resetTraceRelationFactoryGraphLayoutCacheForTests } from "../hooks/use-trace-relation-factory-graph-layout";
import { buildTraceFactoryGraphLayoutPositions } from "../lib/trace-factory-graph-layout";
import { buildTraceRelationFactoryGraphFlow } from "../lib/trace-relation-factory-graph-flow";
import { TraceRelationFlow } from "./trace-relation-flow";

const fitViewSpy = vi.fn();

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
      onInit,
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
        height?: number;
        id: string;
        initialHeight?: number;
        initialWidth?: number;
        measured?: { height: number; width: number };
        position?: { x: number; y: number };
        type: string;
        width?: number;
      }>;
      onInit?: (instance: { fitView: typeof fitViewSpy }) => void;
    }) => (
      <div
        data-edge-payload={JSON.stringify(edges)}
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
        data-testid="trace-relation-react-flow"
        ref={() => {
          onInit?.({ fitView: fitViewSpy });
        }}
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

type RenderedTraceFlowNodeBounds = {
  height?: number;
  initialHeight?: number;
  initialWidth?: number;
  measured?: { height: number; width: number };
  width?: number;
};

function renderedNodePositionsById(): Map<string, { x: number; y: number }> {
  const payload = screen
    .getByTestId("trace-relation-react-flow")
    .getAttribute("data-node-positions");
  if (!payload) {
    throw new Error("Expected rendered node positions.");
  }

  return new Map(
    (
      JSON.parse(payload) as Array<{
        id: string;
        position?: { x: number; y: number };
      }>
    ).map((node) => [node.id, node.position ?? { x: 0, y: 0 }]),
  );
}

function renderedNodeBoundsById(): Map<string, RenderedTraceFlowNodeBounds> {
  const payload = screen
    .getByTestId("trace-relation-react-flow")
    .getAttribute("data-node-bounds");
  if (!payload) {
    throw new Error("Expected rendered node bounds.");
  }

  return new Map(
    (
      JSON.parse(payload) as Array<{ id: string } & RenderedTraceFlowNodeBounds>
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

describe("TraceRelationFlow", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    fitViewSpy.mockReset();
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
      const graphFrame = screen.getByRole("region", {
        name: "Batch relation graph",
      });
      expect(graphFrame.getAttribute("data-dashboard-graph-frame")).toBe(
        "true",
      );
      expect(graphFrame.className).toContain("shadow-none");
      expect(graphFrame.className).toContain("bg-transparent");
      expect(graphFrame.className).not.toContain("bg-surface-container-low");
      expect(graphFrame.className).not.toContain("shadow-af-card");
      expect(graphFrame.className).not.toContain("shadow-af-panel");
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
    ).toBe("var(--color-outline)");
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
    ).toContain('"backgroundColor":"var(--color-surface)"');
    expect(
      screen
        .getByTestId("trace-relation-flow-controls")
        .getAttribute("data-controls-style"),
    ).toContain('"borderRadius":8');
    expect(fitViewSpy).toHaveBeenCalledWith({ maxZoom: 1.5, padding: 0.08 });

    const implementButton = screen.getByRole("button", {
      name: "Implement story",
    });
    const implementNode = implementButton.querySelector("article");
    if (!implementNode) {
      throw new Error("Expected selectable relation node shell to render.");
    }
    expect(implementNode.className).toContain("border-info-border");
    expect(implementNode.className).toContain("bg-info-container");
    expect(within(implementButton).getByText("Implement story")).toBeTruthy();
    const textualPath = screen.getByRole("region", {
      name: "Textual relation path",
    });
    expect(within(textualPath).getByText("Parent-child")).toBeTruthy();
    expect(within(textualPath).getByText(/Required state:.*Done/)).toBeTruthy();
    expect(screen.queryByText("Work state")).toBeNull();
    expect(screen.queryByText("Failed")).toBeNull();

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
    expect(edges[0]?.style?.stroke).toBe("var(--color-outline-variant)");
    expect(edges[0]?.style?.strokeDasharray).toBeUndefined();
    expect(edges[1]?.style?.stroke).toBe("var(--color-outline-variant)");
  });
});

describe("TraceRelationFlow selected work", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("highlights the selected work node without making it selectable", async () => {
    const onSelectWorkID = vi.fn();

    render(
      <TraceRelationFlow
        onSelectWorkID={onSelectWorkID}
        relations={RELATIONS}
        selectedWorkID="work-plan"
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("trace-relation-react-flow")).toBeTruthy();
    });

    expect(screen.queryByRole("button", { name: "Plan story" })).toBeNull();

    const selectedShell = within(
      screen.getByTestId("trace-relation-react-flow"),
    )
      .getByText("Plan story")
      .closest("article");
    if (!(selectedShell instanceof HTMLElement)) {
      throw new Error("Expected selected relation node shell to render.");
    }
    expect(selectedShell.className).toContain("border-primary");
    expect(selectedShell.className).toContain("bg-primary-container");

    fireEvent.click(selectedShell);
    expect(onSelectWorkID).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Implement story" }));
    expect(onSelectWorkID).toHaveBeenCalledWith("work-implement");
  });
});

describe("TraceRelationFlow textual fallback", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("keeps the semantic relation path usable when graph rendering is disabled", () => {
    const onSelectWorkID = vi.fn();

    render(
      <TraceRelationFlow
        onSelectWorkID={onSelectWorkID}
        relations={RELATIONS}
        renderGraph={false}
      />,
    );

    const textualPath = screen.getByRole("region", {
      name: "Textual relation path",
    });
    expect(within(textualPath).getAllByRole("listitem")).toHaveLength(2);
    expect(
      screen.queryByRole("region", { name: "Batch relation graph" }),
    ).toBeNull();

    const firstRelation = within(textualPath).getAllByRole("listitem")[0];
    if (!firstRelation) {
      throw new Error("Expected the first textual relation to render.");
    }
    const target = within(firstRelation).getByRole("button", {
      name: "Select work work-implement.",
    });
    target.focus();
    fireEvent.keyDown(target, { key: "Enter" });
    fireEvent.click(target);
    expect(onSelectWorkID).toHaveBeenCalledWith("work-implement");
    expect(target.className).toContain("focus-visible:ring-af-focus-ring");
  });

  it("keeps the zero-relation state named and accessible", () => {
    render(<TraceRelationFlow relations={[]} renderGraph={false} />);

    const textualPath = screen.getByRole("region", {
      name: "Textual relation path",
    });
    expect(within(textualPath).getByRole("status").textContent).toBe(
      "No recorded relations.",
    );
    expect(screen.getByText("None")).toBeTruthy();
  });
});

describe("TraceRelationFlow layout", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    resetTraceRelationFactoryGraphLayoutCacheForTests();
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("applies async factory layout positions after layout resolves", async () => {
    const graph = buildTraceRelationFactoryGraphFlow(RELATIONS);
    const expectedPositions = await buildTraceFactoryGraphLayoutPositions(
      graph.topology,
      graph.endpointKeyByNodeId,
    );

    render(<TraceRelationFlow relations={RELATIONS} />);

    await waitFor(() => {
      const renderedPositions = renderedNodePositionsById();
      const renderedBounds = renderedNodeBoundsById();
      for (const [nodeId, layout] of expectedPositions) {
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

    const textualPath = screen.getByRole("region", {
      name: "文本关系路径",
    });
    expect(within(textualPath).getByText("Review story")).toBeTruthy();
    expect(within(textualPath).getByText("Fix story")).toBeTruthy();
    expect(within(textualPath).getByText("派生自")).toBeTruthy();
    expect(
      within(textualPath).getByText(/未知状态：escalated_review/),
    ).toBeTruthy();
    expect(renderedEdges()[0]?.ariaLabel).toBe(
      "派生自关系：从 Review story 到 Fix story，要求 未知状态：escalated_review",
    );
    expect(renderedEdges()[0]?.style).toMatchObject({
      stroke: "var(--color-outline-variant)",
      strokeWidth: 1.7,
    });
  });

  it("renders fallback node labels when relation names are missing", async () => {
    render(
      <TraceRelationFlow
        locale="zh-CN"
        relations={[
          {
            request_id: "request-missing-source-name",
            source_work_id: "",
            source_work_name: "",
            target_work_id: "work-target",
            target_work_name: "",
            type: "RELATED_TO",
          },
        ]}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("region", { name: "批次关系图" })).toBeTruthy();
    });

    const textualPath = screen.getByRole("region", {
      name: "文本关系路径",
    });
    expect(within(textualPath).getByText("未知来源")).toBeTruthy();
    expect(within(textualPath).getAllByText("work-target")).toHaveLength(2);
  });
});
