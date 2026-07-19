import {
  Background,
  Controls,
  type Edge,
  type Node,
  type NodeProps,
  ReactFlow,
} from "@xyflow/react";
import {
  GraphNodeButton,
  type GraphNodeHandle,
  GraphNodeShell,
} from "@you-agent-factory/components/graphs";
import type {
  FactoryVisualizationAnnotation,
  FactoryVisualizationEmbeddedImageSource,
  FactoryVisualizationLayoutV1,
} from "@you-agent-factory/client";
import type {
  FactoryActivityProjection,
  FactoryLoadProjection,
  FactoryTopologyConnection,
  FactoryTopologyNode,
  FactoryTopologyProjection,
} from "@you-agent-factory/factory-replay";
import { useEffect, useMemo, useRef, useState } from "react";

import {
  FactoryTopologyErrorBoundary,
  FactoryTopologyStateRegion,
  useDistinctTopologyErrorReport,
} from "./factory-topology-state";

import {
  type FactoryVisualizerError,
  FactoryVisualizerInternalError,
  normalizeFactoryVisualizerError,
  toFactoryVisualizerError,
} from "./visualizer-error";

export interface FactoryTopologyReplayProjection {
  activity: FactoryActivityProjection;
  load: FactoryLoadProjection;
  topology: FactoryTopologyProjection;
}

export type FactoryTopologyReplayState =
  | { status: "empty" }
  | { status: "failed" }
  | { status: "loading" }
  | { projection: FactoryTopologyReplayProjection; status: "ready" };

export interface FactoryTopologyReplayMessages {
  activeDispatches: (count: number) => string;
  annotationsHidden: string;
  annotationsVisible: string;
  empty: string;
  failed: string;
  inactiveDispatches: string;
  imageFailed: string;
  imageLoading: string;
  loading: string;
  nodeLabel: (kind: FactoryTopologyNode["kind"], label: string) => string;
  regionLabel: string;
  resourceOccupancy: (occupied: number, capacity: number) => string;
  resourceOccupancyUnavailable: string;
  retry: string;
  selectedNode: string;
  workStateCount: (count: number) => string;
  workStateCountUnavailable: string;
}

export interface FactoryTopologyReplayProps {
  /** Validated, presentation-only data owned by the visualizer host. */
  layout?: FactoryVisualizationLayoutV1;
  messages: FactoryTopologyReplayMessages;
  onError?: (error: FactoryVisualizerError) => void;
  onRetry?: () => void;
  onSelectNode?: (node: FactoryTopologyNode) => void;
  selectedNodeId?: string;
  state: FactoryTopologyReplayState;
}

interface TopologyNodeData extends Record<string, unknown> {
  activityCount: number;
  messages: FactoryTopologyReplayMessages;
  node: FactoryTopologyNode;
  occupancy?: {
    capacity?: number;
    evidence: "known" | "unavailable";
    occupied?: number;
  };
  onSelectNode?: (node: FactoryTopologyNode) => void;
  selected: boolean;
  workStateCount?: {
    count?: number;
    evidence: "known" | "unavailable";
  };
}

interface AnnotationNodeData extends Record<string, unknown> {
  annotation: FactoryVisualizationAnnotation;
  messages: FactoryTopologyReplayMessages;
}

export interface FactoryTopologyFlowProjection {
  edges: Edge[];
  nodes: Node<TopologyNodeData | AnnotationNodeData>[];
  validEndpoints: boolean;
}

interface PreparedFlowSuccess {
  flow: FactoryTopologyFlowProjection;
  status: "ready";
}

interface PreparedFlowFailure {
  error: FactoryVisualizerError;
  status: "failed";
}

type PreparedFlow = PreparedFlowFailure | PreparedFlowSuccess;

const nodeTypes = {
  factoryTopologyAnnotation: FactoryTopologyAnnotationView,
  factoryTopologyNode: FactoryTopologyNodeView,
};
const columnByKind: Record<FactoryTopologyNode["kind"], number> = {
  resource: 0,
  worker: 1,
  "work-type": 2,
  "work-state": 3,
  workstation: 4,
};

/** Convert immutable replay projections into disposable React Flow view data. */
export function projectFactoryTopologyFlow(
  projection: FactoryTopologyReplayProjection,
  messages: FactoryTopologyReplayMessages,
  selectedNodeId: string | undefined,
  onSelectNode: FactoryTopologyReplayProps["onSelectNode"],
  prefersReducedMotion = false,
  layout?: FactoryVisualizationLayoutV1,
): FactoryTopologyFlowProjection {
  let topologyNodes: FactoryTopologyNode[];
  let connections: FactoryTopologyConnection[];
  try {
    topologyNodes = projection.topology.nodes;
    connections = projection.topology.connections;
  } catch (error) {
    throw new FactoryVisualizerInternalError("projection", error);
  }

  try {
    const nodeById = new Map(topologyNodes.map((node) => [node.id, node]));
    const endpointsValid = connections.every((connection) =>
      connectionHasRenderedEndpoints(connection, nodeById),
    );
    const activityCountByNode = activityCounts(projection.activity);
    const occupancyByNode = new Map(
      projection.load.resourceOccupancy.map((occupancy) => [
        occupancy.resourceNodeId,
        occupancy,
      ]),
    );
    const workStateCountByNode = new Map(
      projection.load.workStateCounts.map((count) => [
        count.workStateNodeId,
        count,
      ]),
    );
    const rowByKind = new Map<FactoryTopologyNode["kind"], number>();

    const nodes = topologyNodes.map<Node<TopologyNodeData>>((node) => {
      const row = rowByKind.get(node.kind) ?? 0;
      rowByKind.set(node.kind, row + 1);
      const occupancy = occupancyByNode.get(node.id);
      const workStateCount = workStateCountByNode.get(node.id);
      return {
        data: {
          activityCount: activityCountByNode.get(node.id) ?? 0,
          messages,
          node,
          ...(occupancy
            ? {
                occupancy: {
                  capacity: occupancy.capacity,
                  evidence: occupancy.evidence,
                  occupied: occupancy.occupiedQuantity,
                },
              }
            : {}),
          onSelectNode,
          selected: selectedNodeId === node.id,
          ...(workStateCount
            ? {
                workStateCount: {
                  count: workStateCount.count,
                  evidence: workStateCount.evidence,
                },
              }
            : {}),
        },
        draggable: false,
        id: node.id,
        position: layoutNode(node.kind, row),
        selectable: false,
        type: "factoryTopologyNode",
      };
    });

    const annotationNodes = (layout?.annotations ?? []).map<
      Node<AnnotationNodeData>
    >((annotation) => ({
      data: { annotation, messages },
      draggable: false,
      id: `annotation:${annotation.id}`,
      position: annotation.position,
      selectable: false,
      type: "factoryTopologyAnnotation",
      ...(annotation.size
        ? {
            style: {
              height: annotation.size.height,
              width: annotation.size.width,
            },
          }
        : {}),
    }));

    return {
      edges: endpointsValid
        ? connections.map((connection) => ({
            animated:
              !prefersReducedMotion &&
              projection.activity.activeDispatchOverlays.some((overlay) =>
                overlay.connectionIds.includes(connection.id),
              ),
            data: { relationship: connection.kind },
            id: connection.id,
            source: connection.source.nodeId,
            sourceHandle: connection.source.handleId,
            target: connection.target.nodeId,
            targetHandle: connection.target.handleId,
          }))
        : [],
      nodes: [...nodes, ...annotationNodes],
      validEndpoints: endpointsValid,
    };
  } catch (error) {
    if (error instanceof FactoryVisualizerInternalError) throw error;
    throw new FactoryVisualizerInternalError("projection", error);
  }
}

export function FactoryTopologyReplay(props: FactoryTopologyReplayProps) {
  return (
    <FactoryTopologyErrorBoundary
      errorKind="render"
      messages={props.messages}
      onError={props.onError}
      onRetry={props.onRetry}
      resetKeys={[props.state, props.messages]}
    >
      <FactoryTopologyReplayContent {...props} />
    </FactoryTopologyErrorBoundary>
  );
}

function FactoryTopologyReplayContent({
  messages,
  onError,
  layout,
  onRetry,
  onSelectNode,
  selectedNodeId,
  state,
}: FactoryTopologyReplayProps) {
  if (state.status !== "ready") {
    return (
      <FactoryTopologyStateRegion
        messages={messages}
        state={state.status}
        onRetry={onRetry}
      />
    );
  }
  return (
    <PreparedTopology
      messages={messages}
      onError={onError}
      onRetry={onRetry}
      onSelectNode={onSelectNode}
      layout={layout}
      projection={state.projection}
      selectedNodeId={selectedNodeId}
    />
  );
}

function PreparedTopology({
  messages,
  onError,
  layout,
  onRetry,
  onSelectNode,
  projection,
  selectedNodeId,
}: Omit<FactoryTopologyReplayProps, "state"> & {
  projection: FactoryTopologyReplayProjection;
}) {
  const prefersReducedMotion = usePrefersReducedMotion();
  const prepared = useMemo<PreparedFlow>(() => {
    try {
      const flow = projectFactoryTopologyFlow(
        projection,
        messages,
        selectedNodeId,
        onSelectNode,
        prefersReducedMotion,
        layout,
      );
      return flow.validEndpoints
        ? { flow, status: "ready" }
        : { error: toFactoryVisualizerError("endpoint"), status: "failed" };
    } catch (error) {
      return {
        error: normalizeFactoryVisualizerError(error, "projection"),
        status: "failed",
      };
    }
  }, [
    messages,
    onSelectNode,
    prefersReducedMotion,
    projection,
    layout,
    selectedNodeId,
  ]);
  useDistinctTopologyErrorReport(
    prepared.status === "failed" ? prepared.error : undefined,
    onError,
  );

  if (prepared.status === "failed") {
    return (
      <FactoryTopologyStateRegion
        messages={messages}
        state="failed"
        onRetry={onRetry}
      />
    );
  }

  return (
    <section
      aria-label={messages.regionLabel}
      className="factory-topology-replay"
      data-endpoints-valid="true"
      data-reduced-motion={prefersReducedMotion ? "true" : "false"}
    >
      <FactoryTopologyErrorBoundary
        errorKind="react-flow"
        messages={messages}
        onError={onError}
        onRetry={onRetry}
        resetKeys={[projection, messages, selectedNodeId, onSelectNode, layout]}
        withinRegion
      >
        <ReactFlowCanvas flow={prepared.flow} messages={messages} />
      </FactoryTopologyErrorBoundary>
    </section>
  );
}

function ReactFlowCanvas({
  flow,
  messages,
}: {
  flow: FactoryTopologyFlowProjection;
  messages: FactoryTopologyReplayMessages;
}) {
  const [annotationsVisible, setAnnotationsVisible] = useState(true);
  const visibleNodes = annotationsVisible
    ? flow.nodes
    : flow.nodes.filter((node) => node.type !== "factoryTopologyAnnotation");
  const hasAnnotations = flow.nodes.some(
    (node) => node.type === "factoryTopologyAnnotation",
  );

  return (
    <>
      {hasAnnotations ? (
        <button
          aria-pressed={annotationsVisible}
          className="factory-topology-replay__annotation-toggle"
          onClick={() => setAnnotationsVisible((visible) => !visible)}
          type="button"
        >
          {annotationsVisible
            ? messages.annotationsVisible
            : messages.annotationsHidden}
        </button>
      ) : null}
      <ReactFlow
        edges={flow.edges}
        edgesFocusable={false}
        elementsSelectable={false}
        fitView
        fitViewOptions={{ includeHiddenNodes: false }}
        key={annotationsVisible ? "annotations-visible" : "annotations-hidden"}
        nodes={visibleNodes}
        nodesConnectable={false}
        nodesDraggable={false}
        nodeTypes={nodeTypes}
        // XYFlow disables pointer events on otherwise non-interactive node
        // wrappers. The nested GraphNodeButton remains the selection owner.
        onNodeClick={preserveNestedNodePointerEvents}
        panOnDrag
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </>
  );
}

function FactoryTopologyAnnotationView({
  data,
}: NodeProps<Node<AnnotationNodeData>>) {
  const { annotation, messages } = data;
  if (annotation.kind === "image") {
    return (
      <FactoryTopologyAnnotationImage annotation={annotation} messages={messages} />
    );
  }

  return (
    <aside
      className="factory-topology-replay__annotation"
      data-tone={annotation.tone ?? "neutral"}
    >
      {annotation.title ? (
        <strong className="factory-topology-replay__annotation-title">
          {annotation.title}
        </strong>
      ) : null}
      <span className="factory-topology-replay__annotation-body">
        {annotation.body}
      </span>
    </aside>
  );
}

function FactoryTopologyAnnotationImage({
  annotation,
  messages,
}: {
  annotation: Extract<FactoryVisualizationAnnotation, { kind: "image" }>;
  messages: FactoryTopologyReplayMessages;
}) {
  const image = useEmbeddedImageUrl(annotation.source);
  return (
    <figure className="factory-topology-replay__annotation factory-topology-replay__annotation--image">
      {image.status === "ready" ? (
        <img
          alt={annotation.altText}
          className="factory-topology-replay__annotation-image"
          onError={image.fail}
          src={image.url}
        />
      ) : (
        <div
          aria-label={annotation.altText}
          className="factory-topology-replay__annotation-image-state"
          role={image.status === "failed" ? "alert" : "status"}
        >
          {image.status === "failed"
            ? messages.imageFailed
            : messages.imageLoading}
        </div>
      )}
    </figure>
  );
}

function useEmbeddedImageUrl(source: FactoryVisualizationEmbeddedImageSource): {
  fail: () => void;
  status: "failed" | "loading" | "ready";
  url?: string;
} {
  const urlRef = useRef<string | undefined>(undefined);
  const [state, setState] = useState<
    { status: "failed" | "loading" } | { status: "ready"; url: string }
  >({ status: "loading" });
  useEffect(() => {
    try {
      const bytes = decodeEmbeddedImage(source.base64);
      const url = URL.createObjectURL(new Blob([bytes], { type: source.mediaType }));
      urlRef.current = url;
      setState({ status: "ready", url });
      return () => {
        if (urlRef.current === url) {
          URL.revokeObjectURL(url);
          urlRef.current = undefined;
        }
      };
    } catch {
      setState({ status: "failed" });
      return undefined;
    }
  }, [source]);

  return {
    fail: () => {
      if (urlRef.current) {
        URL.revokeObjectURL(urlRef.current);
        urlRef.current = undefined;
      }
      setState({ status: "failed" });
    },
    ...state,
  };
}

function decodeEmbeddedImage(base64: string): ArrayBuffer {
  const decoded = atob(base64);
  const bytes = new Uint8Array(decoded.length);
  for (let index = 0; index < decoded.length; index += 1) {
    bytes[index] = decoded.charCodeAt(index);
  }
  return bytes.buffer;
}

function preserveNestedNodePointerEvents(): void {}

function layoutNode(
  kind: FactoryTopologyNode["kind"],
  row: number,
): { x: number; y: number } {
  const column = columnByKind[kind];
  if (column === undefined || !Number.isSafeInteger(row) || row < 0) {
    throw new FactoryVisualizerInternalError("layout");
  }
  return { x: column * 260, y: row * 170 };
}

function FactoryTopologyNodeView({ data }: NodeProps<Node<TopologyNodeData>>) {
  const {
    activityCount,
    messages,
    node,
    occupancy,
    onSelectNode,
    selected,
    workStateCount,
  } = data;
  const state = selected ? "selected" : "default";
  const handles: GraphNodeHandle[] = node.handles.map((handle) => ({
    connectable: false,
    id: handle.id,
    label: handle.id,
    side: handle.role === "target" ? "left" : "right",
    type: handle.role,
  }));
  const content = (
    <GraphNodeShell
      className={
        activityCount > 0 ? "factory-topology-replay__node--active" : ""
      }
      data-dispatch-activity={activityCount > 0 ? "active" : "inactive"}
      handles={handles}
      nodeKind={node.kind}
      showStateIndicator={false}
      state={state}
    >
      <strong className="factory-topology-replay__node-title">
        {node.label}
      </strong>
      <span className="factory-topology-replay__node-kind">{node.kind}</span>
      <span className="factory-topology-replay__node-cue">
        {activityCount > 0 ? "●" : "○"}{" "}
        {activityCount > 0
          ? messages.activeDispatches(activityCount)
          : messages.inactiveDispatches}
      </span>
      {node.kind === "resource" ? (
        <span className="factory-topology-replay__node-cue">
          ◫{" "}
          {occupancy?.evidence === "known" &&
          occupancy.occupied !== undefined &&
          occupancy.capacity !== undefined
            ? messages.resourceOccupancy(occupancy.occupied, occupancy.capacity)
            : messages.resourceOccupancyUnavailable}
        </span>
      ) : null}
      {node.kind === "work-state" ? (
        <span className="factory-topology-replay__node-cue">
          ∑{" "}
          {workStateCount?.evidence === "known" &&
          workStateCount.count !== undefined
            ? messages.workStateCount(workStateCount.count)
            : messages.workStateCountUnavailable}
        </span>
      ) : null}
      {selected ? (
        <span className="factory-topology-replay__node-cue">
          ✓ {messages.selectedNode}
        </span>
      ) : null}
    </GraphNodeShell>
  );

  return onSelectNode ? (
    <GraphNodeButton
      aria-label={messages.nodeLabel(node.kind, node.label)}
      className="factory-topology-replay__node-button"
      graphState={state}
      onClick={() => onSelectNode(node)}
    >
      {content}
    </GraphNodeButton>
  ) : (
    <figure
      aria-label={messages.nodeLabel(node.kind, node.label)}
      className="factory-topology-replay__node-static"
    >
      {content}
    </figure>
  );
}

function connectionHasRenderedEndpoints(
  connection: FactoryTopologyConnection,
  nodeById: ReadonlyMap<string, FactoryTopologyNode>,
): boolean {
  const source = nodeById.get(connection.source.nodeId);
  const target = nodeById.get(connection.target.nodeId);
  return Boolean(
    source?.handles.some(
      (handle) =>
        handle.id === connection.source.handleId && handle.role === "source",
    ) &&
      target?.handles.some(
        (handle) =>
          handle.id === connection.target.handleId && handle.role === "target",
      ),
  );
}

function activityCounts(
  activity: FactoryActivityProjection,
): Map<string, number> {
  const counts = new Map<string, number>();
  for (const overlay of activity.activeDispatchOverlays) {
    const nodeIds = new Set([
      overlay.workerNodeId,
      overlay.workstationNodeId,
      ...(overlay.resourceNodeIds ?? []),
    ]);
    for (const nodeId of nodeIds) {
      if (nodeId) counts.set(nodeId, (counts.get(nodeId) ?? 0) + 1);
    }
  }
  return counts;
}

const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

function usePrefersReducedMotion(): boolean {
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(
    () => reducedMotionMediaQuery()?.matches ?? false,
  );

  useEffect(() => {
    const mediaQuery = reducedMotionMediaQuery();
    if (!mediaQuery) return;

    const updatePreference = () => setPrefersReducedMotion(mediaQuery.matches);
    updatePreference();
    mediaQuery.addEventListener("change", updatePreference);
    return () => mediaQuery.removeEventListener("change", updatePreference);
  }, []);

  return prefersReducedMotion;
}

function reducedMotionMediaQuery(): MediaQueryList | undefined {
  return typeof window === "undefined" ||
    typeof window.matchMedia !== "function"
    ? undefined
    : window.matchMedia(REDUCED_MOTION_QUERY);
}
