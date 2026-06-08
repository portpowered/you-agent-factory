import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { cleanup, render, screen } from "@testing-library/react";
import type { Edge, Node } from "@xyflow/react";
import type { ReactNode } from "react";

import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { buildCurrentActivityGraphLayoutFromFactory } from "../lib/current-activity-factory-graph-layout";
import { buildGraphEdges } from "../lib/react-flow-current-activity-card-edges";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
  buildVisibleGraphEdges,
  EMPTY_NODE_POSITIONS,
} from "../lib/react-flow-current-activity-card-graph";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

let reactFlowErrorToReport: { errorId: string; message: string } | null = null;

vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual("@xyflow/react");

  return {
    ...actual,
    Background: () => <div data-testid="graph-background" />,
    Controls: () => <div data-testid="graph-controls" />,
    ReactFlow: ({
      children,
      edges,
      nodes,
      onError,
    }: {
      children: ReactNode;
      edges?: Edge[];
      nodes?: Node[];
      onError?: (errorId: string, message: string) => void;
    }) => {
      if (reactFlowErrorToReport) {
        onError?.(
          reactFlowErrorToReport.errorId,
          reactFlowErrorToReport.message,
        );
      }
      reportMissingEdgeHandles({
        edges: edges ?? [],
        nodes: nodes ?? [],
        onError,
      });

      return (
        <div data-testid="mock-react-flow">
          <ul aria-label="Rendered graph edges">
            {(edges ?? []).map((edge) => (
              <li key={edge.id}>{edge.id}</li>
            ))}
          </ul>
          {children}
        </div>
      );
    },
  };
});

function reportMissingEdgeHandles({
  edges,
  nodes,
  onError,
}: {
  edges: Edge[];
  nodes: Node[];
  onError?: (errorId: string, message: string) => void;
}) {
  for (const edge of edges) {
    const sourceNode = nodes.find((node) => node.id === edge.source);
    const targetNode = nodes.find((node) => node.id === edge.target);
    if (!sourceNode || !targetNode) {
      onError?.(
        "006",
        `Couldn't create edge for missing node, edge id: ${edge.id}.`,
      );
      continue;
    }

    if (!nodeHasHandle(sourceNode, edge.sourceHandle)) {
      onError?.(
        "008",
        `Couldn't create edge for source handle id: "${edge.sourceHandle}", edge id: ${edge.id}.`,
      );
      continue;
    }

    if (!nodeHasHandle(targetNode, edge.targetHandle)) {
      onError?.(
        "008",
        `Couldn't create edge for target handle id: "${edge.targetHandle}", edge id: ${edge.id}.`,
      );
    }
  }
}

function nodeHasHandle(node: Node, handleId: string | null | undefined) {
  const handles = (node.data as { handles?: Array<{ id: string }> }).handles;
  return Boolean(handleId && handles?.some((handle) => handle.id === handleId));
}

const importController: CurrentActivityImportController = {
  activateImport: vi.fn().mockResolvedValue(undefined),
  activationState: { status: "idle" },
  clearActivationError: vi.fn(),
  clearError: vi.fn(),
  closeImportPreview: vi.fn(),
  dropState: { status: "idle" },
  importPreviewState: { status: "idle" },
  onDragEnter: vi.fn(),
  onDragLeave: vi.fn(),
  onDragOver: vi.fn(),
  onDrop: vi.fn(),
};

const DEFAULT_GRAPH_RECT = {
  bottom: 720,
  height: 720,
  left: 0,
  right: 1280,
  top: 0,
  width: 1280,
  x: 0,
  y: 0,
  toJSON: () => ({}),
} as DOMRect;

afterEach(() => {
  cleanup();
  reactFlowErrorToReport = null;
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: React Flow error coverage keeps the viewport bootstrap setup and endpoint assertions together.
describe("CurrentActivityGraphViewport React Flow errors", () => {
  const originalBoundingClientRect =
    HTMLElement.prototype.getBoundingClientRect;
  const originalResizeObserver = globalThis.ResizeObserver;

  beforeEach(() => {
    HTMLElement.prototype.getBoundingClientRect = () => DEFAULT_GRAPH_RECT;
    globalThis.ResizeObserver = class {
      public constructor(private readonly callback: ResizeObserverCallback) {}

      public disconnect(): void {}

      public observe(target: Element): void {
        this.callback(
          [
            {
              contentRect: DEFAULT_GRAPH_RECT,
              target,
            } as ResizeObserverEntry,
          ],
          this as unknown as ResizeObserver,
        );
      }

      public unobserve(): void {}
    } as unknown as typeof ResizeObserver;
  });

  afterEach(() => {
    HTMLElement.prototype.getBoundingClientRect = originalBoundingClientRect;
    globalThis.ResizeObserver = originalResizeObserver;
  });

  it("throws when React Flow reports an edge endpoint handle mismatch", () => {
    reactFlowErrorToReport = {
      errorId: "008",
      message:
        'Couldn\'t create edge for source handle id: "missing-review-source", edge id: hidden-edge.',
    };

    expect(() => renderViewport()).toThrow(
      /React Flow factory graph endpoint error 008/,
    );
  });

  it("renders semantic endpoint edges when source and target handles exist", async () => {
    renderViewport({
      edges: [
        {
          id: "workstation-output:workstation:review->place:story:done",
          source: "workstation:review",
          sourceHandle: "workstation-output-source",
          target: "place:story:done",
          targetHandle: "work-state-input-target",
        },
      ],
      nodes: [
        semanticNode("workstation:review", "workstation-output-source"),
        semanticNode("place:story:done", "work-state-input-target"),
      ],
    });

    expect(
      (await screen.findByRole("list", { name: "Rendered graph edges" }))
        .textContent,
    ).toContain("workstation-output:workstation:review->place:story:done");
  });

  it("throws when missing observe-mode endpoints target semantic graph nodes", () => {
    expect(() =>
      renderViewport({
        edges: [
          {
            id: "workstation-output:workstation:review->place:story:done",
            source: "workstation:review",
            sourceHandle: "missing-review-source",
            target: "place:story:done",
            targetHandle: "missing-done-target",
          },
        ],
        nodes: [
          semanticNode("workstation:review", "workstation-output-source"),
          semanticNode("place:story:done", "work-state-input-target"),
        ],
      }),
    ).toThrow(/React Flow factory graph endpoint error 008/);
  });

  it("renders the sample factory graph with relationship-specific endpoint handles", async () => {
    const sampleFactory = JSON.parse(
      readFileSync(
        resolve(
          process.cwd(),
          "src/features/workflow-activity/lib/current-activity-sample-factory.fixture.json",
        ),
        "utf-8",
      ),
    ) as CanonicalFactoryDefinition;
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(sampleFactory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const handleAssignments = buildHandleAssignments(
      visibleGraphEdges,
      graphLayout.nodes,
    );
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      graphLayout,
      now: Date.parse("2026-05-28T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: {
        factory: sampleFactory,
        runtime: { place_token_counts: {} },
        topology: { workstation_nodes_by_id: {} },
      } as never,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });
    const edges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      new Set(),
      visibleGraphEdges,
      nodes,
    );

    expect(edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          sourceHandle: expect.stringMatching(/-source$/),
          targetHandle: expect.stringMatching(/-target$/),
        }),
      ]),
    );
    expect(
      edges.some(
        (edge) =>
          edge.sourceHandle === "workstation-source" ||
          edge.targetHandle === "workstation-target",
      ),
    ).toBe(false);
    expect(() => renderViewport({ edges, nodes })).not.toThrow();
  });
});

function semanticNode(id: string, handleId: string): Node {
  return {
    data: {
      handles: [
        {
          id: handleId,
          label: handleId,
          side: handleId.endsWith("-source") ? "right" : "left",
          type: handleId.endsWith("-source") ? "source" : "target",
        },
      ],
    },
    id,
    position: { x: 0, y: 0 },
  };
}

function renderViewport({
  edges = [],
  nodes = [],
}: {
  edges?: Edge[];
  nodes?: Node[];
} = {}) {
  const flowContainerRef = { current: null as HTMLElement | null };

  return render(
    <CurrentActivityGraphViewport
      activeTool={null}
      canInteractWithEditor={false}
      canSaveDraft={false}
      editorMode={false}
      edges={edges}
      flowContainerRef={flowContainerRef}
      graphKey="test-graph"
      handleDiscardPendingChanges={vi.fn()}
      handleNodesChange={vi.fn()}
      handleSaveDraft={vi.fn()}
      hasPendingChanges={false}
      hiddenNodeClasses={new Set()}
      hideShowMenuOpen={false}
      onClearPreferences={vi.fn()}
      onSelectVisibilityPreset={vi.fn()}
      preferencesDirty={false}
      visibilityPreset="all"
      headingID="test-heading"
      imports={importController}
      initialFitViewKey="full-graph"
      initialFitViewOptions={{ padding: 0.18 }}
      nodeTypes={{}}
      nodes={nodes}
      onHideShowMenuOpenChange={vi.fn()}
      onToggleHiddenNodeClass={vi.fn()}
      onSelectTool={vi.fn()}
      setStoredNodePosition={vi.fn()}
    />,
  );
}
