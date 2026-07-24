import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import { useCurrentSelectionDispatchHistoryMessages } from "../../../base/components/presentation/current-selection-locale";
import { buildSelectedWorkRelationshipGraph } from "../../lib/selected-work-relationship-graph";
import {
  localAgentCliRuntimeBatchSnapshot,
  localAgentCliRuntimeLoopbackWorkItem,
  selectedWorkItem,
  snapshotFixture,
} from "../../lib/selected-work-relationship-graph.fixture";
import { WorkRelationshipsSection } from "./work-item-relationship-graph";

const fitViewSpy = vi.fn();

vi.mock("@xyflow/react", async () => {
  return {
    Background: () => <div data-testid="trace-relation-flow-background" />,
    Controls: () => <div data-testid="trace-relation-flow-controls" />,
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
      edges: Array<{ id: string }>;
      nodeTypes: Record<
        string,
        (props: { data: Record<string, unknown> }) => ReactNode
      >;
      nodes: Array<{
        data: Record<string, unknown>;
        id: string;
        type: string;
      }>;
      onInit?: (instance: { fitView: typeof fitViewSpy }) => void;
    }) => (
      <div
        data-edge-payload={JSON.stringify(edges)}
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

function repeatedDependsOnSnapshot() {
  const snapshot = snapshotFixture();
  snapshot.relationsByWorkID["work-active-story"] = [
    {
      source_work_id: "work-active-story",
      sourceWorkName: "Active Story",
      targetWorkId: "work-dependency-story",
      targetWorkName: "Dependency Story",
      type: "DEPENDS_ON",
      requiredState: "ready",
    },
    {
      source_work_id: "work-active-story",
      sourceWorkName: "Active Story",
      targetWorkId: "work-second-dependency-story",
      targetWorkName: "Second Dependency Story",
      type: "DEPENDS_ON",
      requiredState: "ready",
    },
  ];
  snapshot.runtime.active_executions_by_dispatch_id[
    "dispatch-active-story"
  ].work_items?.push({
    display_name: "Second Dependency Story",
    state: "ready",
    trace_id: "trace-second-dependency-story",
    work_id: "work-second-dependency-story",
    work_type_id: "story",
  });
  return snapshot;
}

function getTraceGraphNodeButton(
  container: HTMLElement,
  label: string,
): HTMLButtonElement {
  const button = container.querySelector(`button[aria-label="${label}"]`);

  if (!(button instanceof HTMLButtonElement)) {
    throw new Error(`expected trace graph node button for ${label}`);
  }

  return button;
}

function LocalAgentCliRuntimeBatchHarness() {
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const relationshipGraph = buildSelectedWorkRelationshipGraph({
    selectedWorkItem: localAgentCliRuntimeLoopbackWorkItem(),
    snapshot: localAgentCliRuntimeBatchSnapshot(),
  });

  return (
    <WorkRelationshipsSection
      messages={messages}
      relationshipGraph={relationshipGraph}
    />
  );
}

function WorkRelationshipsSectionHarness({
  onSelectWorkID,
}: {
  onSelectWorkID?: (workID: string) => void;
}) {
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const relationshipGraph = buildSelectedWorkRelationshipGraph({
    selectedWorkItem,
    snapshot: repeatedDependsOnSnapshot(),
  });

  return (
    <WorkRelationshipsSection
      messages={messages}
      onSelectWorkID={onSelectWorkID}
      relationshipGraph={relationshipGraph}
    />
  );
}

function readyRepeatedDependsOnGraph() {
  const graph = buildSelectedWorkRelationshipGraph({
    selectedWorkItem,
    snapshot: repeatedDependsOnSnapshot(),
  });
  if (graph.status !== "ready") {
    throw new Error(`expected ready graph, got ${graph.status}`);
  }
  return graph;
}

function renderedEdgeCount(container: HTMLElement): number {
  const payload = container
    .querySelector("[data-testid='trace-relation-react-flow']")
    ?.getAttribute("data-edge-payload");
  if (!payload) {
    throw new Error("Expected rendered edge payload.");
  }

  return (JSON.parse(payload) as unknown[]).length;
}

describe("WorkRelationshipsSection repeated DEPENDS_ON rendering", () => {
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

  it("renders every dependency node and edge from the ready relationship graph", async () => {
    const onSelectWorkID = vi.fn();
    const graph = readyRepeatedDependsOnGraph();

    render(<WorkRelationshipsSectionHarness onSelectWorkID={onSelectWorkID} />);

    const relationshipSection = screen.getByRole("region", {
      name: "Work relationships",
    });
    const traceGraph = await within(relationshipSection).findByRole("region", {
      name: "Batch relation graph",
    });

    expect(within(traceGraph).getByText("Dependency Story")).toBeTruthy();
    expect(
      within(traceGraph).getByText("Second Dependency Story"),
    ).toBeTruthy();
    expect(within(traceGraph).getByText("Active Story")).toBeTruthy();

    await waitFor(() => {
      expect(renderedEdgeCount(traceGraph)).toBe(graph.relations.length);
    });
    expect(
      graph.relations.filter(
        (relation) =>
          relation.type === "DEPENDS_ON" &&
          relation.source_work_id === "work-active-story",
      ),
    ).toHaveLength(2);
  });

  it("keeps related dependency nodes selectable by click and keyboard", async () => {
    const user = userEvent.setup();
    const onSelectWorkID = vi.fn();

    render(<WorkRelationshipsSectionHarness onSelectWorkID={onSelectWorkID} />);

    const relationshipSection = screen.getByRole("region", {
      name: "Work relationships",
    });
    const traceGraph = await within(relationshipSection).findByRole("region", {
      name: "Batch relation graph",
    });

    await user.click(getTraceGraphNodeButton(traceGraph, "Dependency Story"));
    getTraceGraphNodeButton(traceGraph, "Second Dependency Story").focus();
    await user.keyboard("{Enter}");

    expect(onSelectWorkID).toHaveBeenNthCalledWith(1, "work-dependency-story");
    expect(onSelectWorkID).toHaveBeenNthCalledWith(
      2,
      "work-second-dependency-story",
    );
  });

  it("renders every loopback dependency from the smoke test fixture", async () => {
    const loopbackWorkItem = localAgentCliRuntimeLoopbackWorkItem();
    const relationshipGraph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem: loopbackWorkItem,
      snapshot: localAgentCliRuntimeBatchSnapshot(),
    });
    if (relationshipGraph.status !== "ready") {
      throw new Error(`expected ready graph, got ${relationshipGraph.status}`);
    }

    render(<LocalAgentCliRuntimeBatchHarness />);

    const relationshipSection = screen.getByRole("region", {
      name: "Work relationships",
    });
    const traceGraph = await within(relationshipSection).findByRole("region", {
      name: "Batch relation graph",
    });

    await waitFor(() => {
      expect(renderedEdgeCount(traceGraph)).toBe(
        relationshipGraph.relations.length,
      );
    });

    const edgePayload = traceGraph
      .querySelector("[data-testid='trace-relation-react-flow']")
      ?.getAttribute("data-edge-payload");
    if (!edgePayload) {
      throw new Error("Expected rendered edge payload.");
    }
    const loopbackEdges = (
      JSON.parse(edgePayload) as Array<{ id: string }>
    ).filter((edge) =>
      edge.id.includes("work-local-agent-cli-runtime-loopback"),
    );
    expect(loopbackEdges).toHaveLength(5);
    expect(
      relationshipGraph.relations.filter(
        (relation) =>
          relation.type === "DEPENDS_ON" &&
          relation.source_work_id === loopbackWorkItem.work_id,
      ),
    ).toHaveLength(5);
  });
});
