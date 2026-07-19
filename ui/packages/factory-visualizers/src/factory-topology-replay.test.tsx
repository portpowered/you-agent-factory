import "@testing-library/jest-dom/vitest";
import "./testing/vitest.setup";

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { FactoryVisualizationLayoutV1 } from "@you-agent-factory/client";
import type { ComponentType } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
  type FactoryTopologyReplayProjection,
  projectFactoryTopologyFlow,
} from "./factory-topology-replay";
import { createFactoryTopologyProjection } from "./testing/factory-topology-projection";

const mockFlow = vi.hoisted(() => ({
  error: undefined as Error | undefined,
  nodes: [] as Array<{
    draggable?: boolean;
    id: string;
    position: { x: number; y: number };
    selectable?: boolean;
    type: string;
  }>,
}));

vi.mock("@xyflow/react", () => ({
  Background: () => <div data-testid="flow-background" />,
  Controls: () => <div data-testid="flow-controls" />,
  Handle: ({ id, type }: { id: string; type: string }) => (
    <span data-handle-id={id} data-handle-role={type} />
  ),
  Position: { Left: "left", Right: "right" },
  ReactFlow: ({
    children,
    edges,
    nodes,
    nodeTypes,
  }: {
    children: React.ReactNode;
    edges: Array<{
      animated?: boolean;
      id: string;
      sourceHandle?: string;
      targetHandle?: string;
    }>;
    nodes: Array<{ data: Record<string, unknown>; id: string; type: string }>;
    nodeTypes: Record<string, ComponentType<{ data: Record<string, unknown> }>>;
  }) => {
    if (mockFlow.error) throw mockFlow.error;
    mockFlow.nodes = nodes;
    return (
      <div data-testid="react-flow">
        {nodes.map((node) => {
          const NodeView = nodeTypes[node.type];
          return NodeView ? <NodeView data={node.data} key={node.id} /> : null;
        })}
        {edges.map((edge) => (
          <span
            data-animated={edge.animated ? "true" : "false"}
            data-edge-id={edge.id}
            data-source-handle={edge.sourceHandle}
            data-target-handle={edge.targetHandle}
            key={edge.id}
          />
        ))}
        {children}
      </div>
    );
  },
}));

const messages: FactoryTopologyReplayMessages = {
  activeDispatches: (count) => `${count} active Dispatches`,
  annotationsHidden: "Show annotations",
  annotationsVisible: "Hide annotations",
  empty: "No Factory topology is available.",
  failed: "The Factory topology could not be shown.",
  inactiveDispatches: "No active Dispatch",
  imageFailed: "The annotation image could not be shown.",
  imageLoading: "Loading annotation image.",
  loading: "Loading Factory topology.",
  nodeLabel: (kind, label) => `${kind}: ${label}`,
  regionLabel: "Factory topology replay",
  resourceOccupancy: (occupied, capacity) =>
    `${occupied} of ${capacity} resources occupied`,
  resourceOccupancyUnavailable: "Resource occupancy unavailable",
  retry: "Try again",
  selectedNode: "Selected",
  workStateCount: (count) => `${count} Work in this state`,
  workStateCountUnavailable: "Work count unavailable",
};

describe("FactoryTopologyReplay", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFlow.error = undefined;
    mockFlow.nodes = [];
  });

  it("renders semantic endpoints and selected-tick activity and load evidence", () => {
    const projection = createFactoryTopologyProjection();
    render(
      <FactoryTopologyReplay
        messages={messages}
        state={{ projection, status: "ready" }}
      />,
    );

    expect(
      screen.getByRole("region", { name: messages.regionLabel }),
    ).toHaveAttribute("data-endpoints-valid", "true");
    expect(screen.getAllByText(/1 active Dispatches/).length).toBeGreaterThan(
      0,
    );
    expect(screen.getByText(/2 of 4 resources occupied/)).toBeVisible();
    expect(screen.getByText(/3 Work in this state/)).toBeVisible();
    expect(screen.getAllByText(/No active Dispatch/).length).toBeGreaterThan(0);
    expect(
      document.querySelector('[data-edge-id="worker-assignment"]'),
    ).toHaveAttribute("data-source-handle", "worker-assignment-source");
    expect(
      document.querySelector('[data-edge-id="worker-assignment"]'),
    ).toHaveAttribute("data-target-handle", "worker-assignment-target");
    expect(
      document.querySelector('[data-edge-id="worker-assignment"]'),
    ).toHaveAttribute("data-animated", "true");
    expect(
      document.querySelector(
        '[data-handle-id="worker-assignment-source"][data-handle-role="source"]',
      ),
    ).not.toBeNull();
    expect(
      document.querySelector(
        '[data-handle-id="worker-assignment-target"][data-handle-role="target"]',
      ),
    ).not.toBeNull();
  });

  it("fails visibly without sending invalid endpoints to React Flow", async () => {
    const projection = createFactoryTopologyProjection();
    projection.topology.connections.push({
      ...projection.topology.connections[0],
      id: "invalid-edge",
      target: { handleId: "missing-handle", nodeId: "workstation:review" },
    });

    const flow = projectFactoryTopologyFlow(
      projection,
      messages,
      undefined,
      undefined,
    );
    expect(flow.validEndpoints).toBe(false);
    expect(flow.edges).toEqual([]);

    const onError = vi.fn();
    render(
      <FactoryTopologyReplay
        messages={messages}
        onError={onError}
        state={{ projection, status: "ready" }}
      />,
    );
    expect(document.querySelector("[data-edge-id]")).toBeNull();
    expect(screen.getByRole("alert")).toHaveTextContent(messages.failed);
    await waitFor(() =>
      expect(onError).toHaveBeenCalledWith(
        expect.objectContaining({ kind: "endpoint", recoverable: true }),
      ),
    );
  });

  it("projects read-only annotations without changing topology or routing", () => {
    const projection = createFactoryTopologyProjection();
    const layout = annotationLayout();
    const flow = projectFactoryTopologyFlow(
      projection,
      messages,
      undefined,
      undefined,
      false,
      layout,
    );

    expect(flow.edges).toHaveLength(projection.topology.connections.length);
    expect(
      flow.nodes.filter((node) => node.type === "factoryTopologyNode"),
    ).toHaveLength(projection.topology.nodes.length);
    expect(flow.nodes).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          draggable: false,
          id: "annotation:review-note",
          position: { x: 90, y: 45 },
          selectable: false,
          type: "factoryTopologyAnnotation",
        }),
        expect.objectContaining({
          id: "annotation:diagram",
          position: { x: 940, y: 20 },
          type: "factoryTopologyAnnotation",
        }),
      ]),
    );
  });

  it("toggles annotations as an accessible group without rendering hidden content", () => {
    render(
      <FactoryTopologyReplay
        layout={annotationLayout()}
        messages={messages}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );

    expect(annotationBody()).toBeVisible();
    expect(screen.getByRole("img", { name: "Review diagram" })).toBeVisible();
    expect(mockFlow.nodes).toHaveLength(6);
    const toggle = screen.getByRole("button", {
      name: messages.annotationsVisible,
    });
    expect(toggle).toHaveAttribute("aria-pressed", "true");

    fireEvent.click(toggle);

    expect(screen.queryByText(annotationBodyMatcher)).not.toBeInTheDocument();
    expect(
      screen.queryByRole("img", { name: "Review diagram" }),
    ).not.toBeInTheDocument();
    expect(mockFlow.nodes).toHaveLength(4);
    expect(
      screen.getByRole("button", { name: messages.annotationsHidden }),
    ).toHaveAttribute("aria-pressed", "false");
  });

  it("shows an empty state only until Work, Dispatch, or route evidence is selected", () => {
    const active = createFactoryTopologyProjection();
    const inactive = createFactoryTopologyProjection();
    inactive.activity = {
      ...inactive.activity,
      activeDispatchOverlays: [],
      activeWorkstationNodeIds: [],
    };
    inactive.load = {
      ...inactive.load,
      workStateCounts: inactive.load.workStateCounts.map((count) => ({
        ...count,
        count: 0,
      })),
    };
    const layout = nodeEmptyStateLayout();
    const { rerender } = render(
      <FactoryTopologyReplay
        layout={layout}
        messages={messages}
        state={{ projection: inactive, status: "ready" }}
      />,
    );

    expect(screen.getByText("No reviewers are waiting.")).toBeVisible();
    expect(screen.getByText("No requests are queued.")).toBeVisible();
    expect(screen.getByText(/2 of 4 resources occupied/)).toBeVisible();
    expect(screen.getByText(/0 Work in this state/)).toBeVisible();

    rerender(
      <FactoryTopologyReplay
        layout={layout}
        messages={messages}
        state={{ projection: active, status: "ready" }}
      />,
    );

    expect(
      screen.queryByText("No reviewers are waiting."),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("No requests are queued."),
    ).not.toBeInTheDocument();
    expect(screen.getAllByText(/1 active Dispatches/).length).toBeGreaterThan(
      0,
    );
    expect(screen.getByText(/3 Work in this state/)).toBeVisible();
    expect(screen.getByText(/2 of 4 resources occupied/)).toBeVisible();
  });

  it("suppresses the matching empty state for Work, Dispatch, and route evidence independently", () => {
    const layout = nodeEmptyStateLayout();
    const base = createFactoryTopologyProjection();
    base.activity = {
      ...base.activity,
      activeDispatchOverlays: [],
      activeWorkstationNodeIds: [],
    };
    base.load = {
      ...base.load,
      workStateCounts: base.load.workStateCounts.map((count) => ({
        ...count,
        count: 0,
      })),
    };
    const work = structuredClone(base);
    work.load.workStateCounts[0].count = 1;
    const dispatch = structuredClone(base);
    dispatch.activity.activeDispatchOverlays = [
      {
        ...createFactoryTopologyProjection().activity.activeDispatchOverlays[0],
        connectionIds: [],
      },
    ];
    const route = structuredClone(base);
    route.activity.activeDispatchOverlays = [
      {
        ...createFactoryTopologyProjection().activity.activeDispatchOverlays[0],
        resourceNodeIds: [],
        workerNodeId: undefined,
        workstationNodeId: undefined,
      },
    ];

    expect(emptyStateFor(base, layout, "workstation:review")).toBeDefined();
    expect(
      emptyStateFor(work, layout, "work-state:task:queued"),
    ).toBeUndefined();
    expect(
      emptyStateFor(dispatch, layout, "workstation:review"),
    ).toBeUndefined();
    expect(emptyStateFor(route, layout, "workstation:review")).toBeUndefined();
  });

  it("renders an inactive image empty state through a Blob URL and revokes it on activity", async () => {
    const createObjectURL = vi.fn(() => "blob:empty-state");
    const revokeObjectURL = vi.fn();
    const BrowserUrl = class extends URL {
      static createObjectURL = createObjectURL;
      static revokeObjectURL = revokeObjectURL;
    };
    vi.stubGlobal("URL", BrowserUrl);
    vi.stubGlobal("atob", () => "\u0089PNG\r\n\u001a\n");
    const inactive = createFactoryTopologyProjection();
    inactive.activity = {
      ...inactive.activity,
      activeDispatchOverlays: [],
      activeWorkstationNodeIds: [],
    };
    const layout = nodeEmptyStateLayout({
      kind: "image",
      altText: "Idle review illustration",
      source: {
        base64: "iVBORw0KGgo=",
        kind: "embedded",
        mediaType: "image/png",
      },
    });
    const { rerender } = render(
      <FactoryTopologyReplay
        layout={layout}
        messages={messages}
        state={{ projection: inactive, status: "ready" }}
      />,
    );

    expect(
      await screen.findByRole("img", { name: "Idle review illustration" }),
    ).toHaveAttribute("src", "blob:empty-state");
    rerender(
      <FactoryTopologyReplay
        layout={layout}
        messages={messages}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );
    expect(
      screen.queryByRole("img", { name: "Idle review illustration" }),
    ).not.toBeInTheDocument();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:empty-state");
    vi.unstubAllGlobals();
  });

  it("creates and revokes Blob URLs as annotation images change, disappear, and unmount", async () => {
    const createObjectURL = vi
      .fn()
      .mockReturnValueOnce("blob:diagram-one")
      .mockReturnValueOnce("blob:diagram-two")
      .mockReturnValueOnce("blob:diagram-three");
    const revokeObjectURL = vi.fn();
    const BrowserUrl = class extends URL {
      static createObjectURL = createObjectURL;
      static revokeObjectURL = revokeObjectURL;
    };
    vi.stubGlobal("URL", BrowserUrl);
    vi.stubGlobal("atob", () => "\u0089PNG\r\n\u001a\n");
    const { rerender, unmount } = render(
      <FactoryTopologyReplay
        layout={imageOnlyLayout("iVBORw0KGgo=")}
        messages={messages}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );

    expect(
      await screen.findByRole("img", { name: "Review diagram" }),
    ).toHaveAttribute("src", "blob:diagram-one");
    rerender(
      <FactoryTopologyReplay
        layout={imageOnlyLayout("iVBORw0KGgoAAA==")}
        messages={messages}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );
    await waitFor(() =>
      expect(
        screen.getByRole("img", { name: "Review diagram" }),
      ).toHaveAttribute("src", "blob:diagram-two"),
    );
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:diagram-one");

    fireEvent.click(
      screen.getByRole("button", { name: messages.annotationsVisible }),
    );
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:diagram-two");

    rerender(
      <FactoryTopologyReplay
        layout={imageOnlyLayout("iVBORw0KGgo=")}
        messages={messages}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: messages.annotationsHidden }),
    );
    expect(
      await screen.findByRole("img", { name: "Review diagram" }),
    ).toHaveAttribute("src", "blob:diagram-three");
    unmount();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:diagram-three");
    vi.unstubAllGlobals();
  });

  it("contains image preparation and loading failures in the annotation region", async () => {
    const createObjectURL = vi.fn(() => {
      throw new Error("image preparation failed");
    });
    const BrowserUrl = class extends URL {
      static createObjectURL = createObjectURL;
      static revokeObjectURL = vi.fn();
    };
    vi.stubGlobal("URL", BrowserUrl);
    vi.stubGlobal("atob", () => "\u0089PNG\r\n\u001a\n");
    const { rerender } = render(
      <FactoryTopologyReplay
        layout={imageOnlyLayout("iVBORw0KGgo=")}
        messages={messages}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Review diagram",
    );
    expect(screen.getByRole("alert")).toHaveTextContent(messages.imageFailed);
    expect(screen.getAllByText(/No active Dispatch/)).not.toHaveLength(0);

    createObjectURL.mockReturnValue("blob:diagram");
    rerender(
      <FactoryTopologyReplay
        layout={imageOnlyLayout("iVBORw0KGgo=")}
        messages={messages}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );
    expect(
      await screen.findByRole("img", { name: "Review diagram" }),
    ).toBeVisible();
    fireEvent.error(screen.getByRole("img", { name: "Review diagram" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Review diagram",
    );
    expect(screen.getByRole("alert")).toHaveTextContent(messages.imageFailed);
    vi.unstubAllGlobals();
  });
});

function annotationLayout(): FactoryVisualizationLayoutV1 {
  return {
    annotations: [
      {
        body: "First line\nSecond line",
        id: "review-note",
        kind: "note",
        position: { x: 90, y: 45 },
        title: "Review notes",
        tone: "info",
      },
      {
        altText: "Review diagram",
        id: "diagram",
        kind: "image",
        position: { x: 940, y: 20 },
        size: { height: 80, width: 120 },
        source: {
          base64: "iVBORw0KGgo=",
          kind: "embedded",
          mediaType: "image/png",
        },
      },
    ],
    schemaVersion: "factory-visualization-layout/v1",
  };
}

function imageOnlyLayout(base64: string): FactoryVisualizationLayoutV1 {
  return {
    annotations: [
      {
        altText: "Review diagram",
        id: "diagram",
        kind: "image",
        position: { x: 940, y: 20 },
        size: { height: 80, width: 120 },
        source: { base64, kind: "embedded", mediaType: "image/png" },
      },
    ],
    schemaVersion: "factory-visualization-layout/v1",
  };
}

function nodeEmptyStateLayout(
  reviewContent: FactoryVisualizationLayoutV1["nodeEmptyStates"][number]["content"] = {
    kind: "text",
    text: "No reviewers are waiting.",
  },
): FactoryVisualizationLayoutV1 {
  return {
    nodeEmptyStates: [
      { content: reviewContent, nodeId: "workstation:review" },
      {
        content: { kind: "text", text: "No requests are queued." },
        nodeId: "work-state:task:queued",
      },
    ],
    schemaVersion: "factory-visualization-layout/v1",
  };
}

function emptyStateFor(
  projection: FactoryTopologyReplayProjection,
  layout: FactoryVisualizationLayoutV1,
  nodeId: string,
): unknown {
  return projectFactoryTopologyFlow(
    projection,
    messages,
    undefined,
    undefined,
    false,
    layout,
  ).nodes.find((node) => node.id === nodeId)?.data.emptyState;
}

const annotationBodyMatcher = (_: string, element: Element | null) =>
  element?.textContent === "First line\nSecond line";

function annotationBody(): HTMLElement {
  return screen.getByText(annotationBodyMatcher);
}

describe("FactoryTopologyReplay controlled states and failures", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFlow.error = undefined;
  });

  it.each([
    ["loading", messages.loading, "status"],
    ["empty", messages.empty, "status"],
    ["failed", messages.failed, "alert"],
  ] as const)("renders the explicit %s state", (status, message, role) => {
    render(<FactoryTopologyReplay messages={messages} state={{ status }} />);

    expect(
      screen.getByRole("region", { name: messages.regionLabel }),
    ).toBeVisible();
    expect(screen.getByRole(role)).toHaveTextContent(message);
    expect(screen.queryByTestId("react-flow")).not.toBeInTheDocument();
    if (status === "loading") {
      expect(screen.getByRole("region")).toHaveAttribute("aria-busy", "true");
    }
  });

  it("shows retry only when supplied and emits host intent", () => {
    const onRetry = vi.fn();
    const { rerender } = render(
      <FactoryTopologyReplay
        messages={messages}
        state={{ status: "failed" }}
      />,
    );
    expect(
      screen.queryByRole("button", { name: messages.retry }),
    ).not.toBeInTheDocument();

    rerender(
      <FactoryTopologyReplay
        messages={messages}
        onRetry={onRetry}
        state={{ status: "failed" }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: messages.retry }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("sanitizes projection failures and reports each distinct failure once", async () => {
    const projection = createFactoryTopologyProjection();
    Object.defineProperty(projection, "topology", {
      get() {
        throw Object.assign(new Error("secret projection payload"), {
          code: "INVALID_PROJECTION",
        });
      },
    });
    const onError = vi.fn();
    const { rerender } = render(
      <FactoryTopologyReplay
        messages={messages}
        onError={onError}
        state={{ projection, status: "ready" }}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(messages.failed);
    await waitFor(() => expect(onError).toHaveBeenCalledTimes(1));
    expect(onError).toHaveBeenCalledWith({
      cause: { code: "INVALID_PROJECTION", name: "Error" },
      kind: "projection",
      message: "The prepared topology projection could not be read.",
      recoverable: true,
    });
    expect(JSON.stringify(onError.mock.calls)).not.toContain(
      "secret projection payload",
    );

    rerender(
      <FactoryTopologyReplay
        messages={messages}
        onError={onError}
        selectedNodeId="worker:alice"
        state={{ projection, status: "ready" }}
      />,
    );
    await waitFor(() => expect(onError).toHaveBeenCalledTimes(1));
  });

  it("classifies invalid layout input and recovers from a replacement projection", async () => {
    const invalid = createFactoryTopologyProjection();
    invalid.topology.nodes[0] = {
      ...invalid.topology.nodes[0],
      kind: "invalid-kind" as FactoryTopologyReplayProjection["topology"]["nodes"][number]["kind"],
    };
    const onError = vi.fn();
    const { rerender } = render(
      <FactoryTopologyReplay
        messages={messages}
        onError={onError}
        state={{ projection: invalid, status: "ready" }}
      />,
    );
    await waitFor(() =>
      expect(onError).toHaveBeenCalledWith(
        expect.objectContaining({ kind: "layout" }),
      ),
    );
    expect(screen.getByRole("alert")).toBeVisible();

    rerender(
      <FactoryTopologyReplay
        messages={messages}
        onError={onError}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );
    expect(await screen.findByTestId("react-flow")).toBeVisible();
  });

  it("contains malformed caller layout with field-level diagnostics", async () => {
    const onError = vi.fn();
    render(
      <>
        <FactoryTopologyReplay
          layout={{
            annotations: [
              {
                body: "Valid text, invalid geometry",
                id: "bad-note",
                kind: "note",
                position: { x: Number.NaN, y: 10 },
              },
            ],
            nodeEmptyStates: [
              { content: { kind: "text", text: "Nope" }, nodeId: "unknown" },
            ],
            schemaVersion: "factory-visualization-layout/v1",
          }}
          messages={messages}
          onError={onError}
          state={{
            projection: createFactoryTopologyProjection(),
            status: "ready",
          }}
        />
        <p>Canonical host content survives</p>
      </>,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(messages.failed);
    expect(screen.getByText("Canonical host content survives")).toBeVisible();
    await waitFor(() =>
      expect(onError).toHaveBeenCalledWith(
        expect.objectContaining({
          issues: expect.arrayContaining([
            expect.objectContaining({
              code: "invalid_coordinate",
              path: ["annotations", 0, "position", "x"],
            }),
            expect.objectContaining({
              code: "unknown_canonical_node_id",
              path: ["nodeEmptyStates", 0, "nodeId"],
            }),
          ]),
          kind: "layout-validation",
        }),
      ),
    );
  });
});

describe("FactoryTopologyReplay controlled updates", () => {
  it("emits selection intent while selection styling remains host-controlled", () => {
    const projection = createFactoryTopologyProjection();
    const original = structuredClone(projection);
    const onSelectNode = vi.fn();
    const { rerender } = render(
      <FactoryTopologyReplay
        messages={messages}
        onSelectNode={onSelectNode}
        state={{ projection, status: "ready" }}
      />,
    );
    const workstation = screen.getByRole("button", {
      name: "workstation: Review",
    });

    fireEvent.click(workstation);

    expect(onSelectNode).toHaveBeenCalledWith(projection.topology.nodes[3]);
    expect(screen.queryByText("Selected")).not.toBeInTheDocument();
    expect(workstation).not.toHaveAttribute("aria-pressed", "true");
    expect(projection).toEqual(original);

    rerender(
      <FactoryTopologyReplay
        messages={messages}
        onSelectNode={onSelectNode}
        selectedNodeId="workstation:review"
        state={{ projection, status: "ready" }}
      />,
    );
    expect(screen.getByText(/Selected/)).toBeVisible();
    expect(
      screen.getByRole("button", { name: "workstation: Review" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("replaces activity overlays without changing stable topology identities", () => {
    const first = createFactoryTopologyProjection();
    const second = createFactoryTopologyProjection();
    second.activity = {
      ...second.activity,
      activeDispatchOverlays: [],
      selectedTick: 9,
    };
    second.topology = { ...second.topology, selectedTick: 9 };
    second.load = { ...second.load, selectedTick: 9 };
    const firstFlow = projectFactoryTopologyFlow(
      first,
      messages,
      undefined,
      undefined,
    );
    const secondFlow = projectFactoryTopologyFlow(
      second,
      messages,
      undefined,
      undefined,
    );
    const { rerender } = render(
      <FactoryTopologyReplay
        messages={messages}
        state={{ projection: first, status: "ready" }}
      />,
    );

    expect(screen.getAllByText(/1 active Dispatches/).length).toBeGreaterThan(
      0,
    );
    rerender(
      <FactoryTopologyReplay
        messages={messages}
        state={{ projection: second, status: "ready" }}
      />,
    );

    expect(screen.queryByText(/1 active Dispatches/)).not.toBeInTheDocument();
    expect(screen.getAllByText(/No active Dispatch/)).toHaveLength(4);
    expect(secondFlow.nodes.map(({ id }) => id)).toEqual(
      firstFlow.nodes.map(({ id }) => id),
    );
    expect(secondFlow.edges.map(({ id }) => id)).toEqual(
      firstFlow.edges.map(({ id }) => id),
    );
  });
});
