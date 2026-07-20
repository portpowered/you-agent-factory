import type { FactoryVisualizationLayoutV1 } from "@you-agent-factory/client";
import type { FactoryTopologyReplayProjection } from "../factory-topology-replay";

export function allNodesEmptyLayout(
  projection: FactoryTopologyReplayProjection,
): FactoryVisualizationLayoutV1 {
  return {
    nodeEmptyStates: projection.topology.nodes.map((node) => ({
      content: {
        kind: "text",
        text: `Configured empty content for ${node.label}`,
      },
      nodeId: node.id,
    })),
    schemaVersion: "factory-visualization-layout/v1",
  };
}

export function responsiveLayout(): FactoryVisualizationLayoutV1 {
  return {
    annotations: [
      {
        body: "Long review guidance wraps inside the topology.",
        id: "responsive-review-note",
        kind: "note",
        position: { x: 80, y: 320 },
        title: "Review guidance",
        tone: "info",
      },
      {
        altText: "Review flow overview",
        id: "responsive-review-image",
        kind: "image",
        position: { x: 900, y: 320 },
        size: { height: 96, width: 144 },
        source: embeddedPixel(),
      },
    ],
    nodeEmptyStates: [
      {
        content: { kind: "text", text: "No review work is waiting." },
        nodeId: "workstation:review",
      },
      {
        content: {
          altText: "Worker availability illustration",
          kind: "image",
          source: embeddedPixel(),
        },
        nodeId: "worker:alice",
      },
    ],
    schemaVersion: "factory-visualization-layout/v1",
  };
}

export function embeddedPixel() {
  return {
    base64:
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M/wHwAF/gL+X9g5WQAAAABJRU5ErkJggg==",
    kind: "embedded" as const,
    mediaType: "image/png" as const,
  };
}
