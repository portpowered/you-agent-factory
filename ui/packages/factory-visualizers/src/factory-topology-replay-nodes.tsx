import type { Node, NodeProps } from "@xyflow/react";
import {
  GraphNodeButton,
  type GraphNodeHandle,
  GraphNodeShell,
} from "@you-agent-factory/components/graphs";
import type {
  FactoryVisualizationAnnotation,
  FactoryVisualizationEmbeddedImageSource,
  FactoryVisualizationImageContent,
  FactoryVisualizationNodeEmptyState,
} from "@you-agent-factory/client";
import type { FactoryTopologyNode } from "@you-agent-factory/factory-replay";
import { useEffect, useRef, useState } from "react";

import type { FactoryTopologyReplayMessages } from "./factory-topology-replay";

export interface TopologyNodeData extends Record<string, unknown> {
  activityCount: number;
  emptyState?: FactoryVisualizationNodeEmptyState["content"];
  messages: FactoryTopologyReplayMessages;
  node: FactoryTopologyNode;
  occupancy?: {
    capacity?: number;
    evidence: "known" | "unavailable";
    occupied?: number;
  };
  onSelectNode?: (node: FactoryTopologyNode) => void;
  selected: boolean;
  workStateCount?: { count?: number; evidence: "known" | "unavailable" };
}

export interface AnnotationNodeData extends Record<string, unknown> {
  annotation: FactoryVisualizationAnnotation;
  messages: FactoryTopologyReplayMessages;
}

export const nodeTypes = {
  factoryTopologyAnnotation: FactoryTopologyAnnotationView,
  factoryTopologyNode: FactoryTopologyNodeView,
};

function FactoryTopologyAnnotationView({
  data,
}: NodeProps<Node<AnnotationNodeData>>) {
  const { annotation, messages } = data;
  if (annotation.kind === "image") {
    return (
      <FactoryTopologyAnnotationImage
        annotation={annotation}
        messages={messages}
      />
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
          className="factory-topology-replay__annotation-image-state"
          role={image.status === "failed" ? "alert" : "status"}
        >
          <span className="sr-only">{annotation.altText}</span>
          {image.status === "failed"
            ? messages.imageFailed
            : messages.imageLoading}
        </div>
      )}
    </figure>
  );
}

function FactoryTopologyNodeView({ data }: NodeProps<Node<TopologyNodeData>>) {
  const {
    activityCount,
    emptyState,
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
      <div className="factory-topology-replay__node-activity-detail">
        {emptyState ? (
          <FactoryTopologyNodeEmptyStateView
            content={emptyState}
            messages={messages}
          />
        ) : (
          <span className="factory-topology-replay__node-cue">
            {activityCount > 0 ? "●" : "○"}{" "}
            {activityCount > 0
              ? messages.activeDispatches(activityCount)
              : messages.inactiveDispatches}
          </span>
        )}
      </div>
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

function FactoryTopologyNodeEmptyStateView({
  content,
  messages,
}: {
  content: FactoryVisualizationNodeEmptyState["content"];
  messages: FactoryTopologyReplayMessages;
}) {
  return content.kind === "image" ? (
    <FactoryTopologyEmptyStateImage content={content} messages={messages} />
  ) : (
    <span className="factory-topology-replay__node-empty-state">
      {content.text}
    </span>
  );
}

function FactoryTopologyEmptyStateImage({
  content,
  messages,
}: {
  content: FactoryVisualizationImageContent;
  messages: FactoryTopologyReplayMessages;
}) {
  const image = useEmbeddedImageUrl(content.source);
  return image.status === "ready" ? (
    <img
      alt={content.altText}
      className="factory-topology-replay__node-empty-state-image"
      onError={image.fail}
      src={image.url}
    />
  ) : (
    <span
      className="factory-topology-replay__node-empty-state"
      role={image.status === "failed" ? "alert" : "status"}
    >
      <span className="sr-only">{content.altText}</span>
      {image.status === "failed" ? messages.imageFailed : messages.imageLoading}
    </span>
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
      const url = URL.createObjectURL(
        new Blob([decodeEmbeddedImage(source.base64)], {
          type: source.mediaType,
        }),
      );
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
  for (let index = 0; index < decoded.length; index += 1)
    bytes[index] = decoded.charCodeAt(index);
  return bytes.buffer;
}
