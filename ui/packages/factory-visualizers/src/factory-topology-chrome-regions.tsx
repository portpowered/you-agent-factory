import { Background, Controls, ReactFlow } from "@xyflow/react";
import { useState } from "react";

import type { ResolvedFactoryTopologyChrome } from "./factory-topology-chrome";
import type {
  FactoryTopologyFlowProjection,
  FactoryTopologyReplayMessages,
} from "./factory-topology-replay";
import { nodeTypes } from "./factory-topology-replay-nodes";

interface FactoryTopologyChromeRegionsProps {
  chrome: ResolvedFactoryTopologyChrome;
  flow: FactoryTopologyFlowProjection;
  messages: FactoryTopologyReplayMessages;
}

/** Renders optional chrome without changing the caller-owned replay projection. */
export function FactoryTopologyChromeRegions({
  chrome,
  flow,
  messages,
}: FactoryTopologyChromeRegionsProps) {
  const [showNodeKinds, setShowNodeKinds] = useState(true);
  const [annotationsVisible, setAnnotationsVisible] = useState(true);
  const renderedFlow = {
    ...flow,
    nodes: flow.nodes
      .filter(
        (node) =>
          annotationsVisible || node.type !== "factoryTopologyAnnotation",
      )
      .map((node) =>
        node.type === "factoryTopologyNode"
          ? { ...node, data: { ...node.data, showNodeKinds } }
          : node,
      ),
  };
  const hasAnnotations = flow.nodes.some(
    (node) => node.type === "factoryTopologyAnnotation",
  );

  return (
    <>
      {chrome.legend ? <TopologyLegend label={messages.legendLabel} /> : null}
      {chrome.visibilityControls ? (
        <>
          <button
            aria-pressed={showNodeKinds}
            className="factory-topology-replay__visibility-control"
            onClick={() => setShowNodeKinds((visible) => !visible)}
            type="button"
          >
            {showNodeKinds ? messages.hideNodeKinds : messages.showNodeKinds}
          </button>
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
        </>
      ) : null}
      <ReactFlow
        edges={renderedFlow.edges}
        edgesFocusable={false}
        elementsSelectable={false}
        fitView
        fitViewOptions={{ includeHiddenNodes: false }}
        key={`${annotationsVisible}:${showNodeKinds}`}
        nodes={renderedFlow.nodes}
        nodesConnectable={false}
        nodesDraggable={false}
        nodeTypes={nodeTypes}
        // XYFlow otherwise places the draggable pane above non-interactive node
        // wrappers. The nested GraphNodeButton remains the selection owner.
        onNodeClick={preserveNestedNodePointerEvents}
        panOnDrag
        proOptions={{ hideAttribution: true }}
      >
        {chrome.background ? (
          <div aria-hidden="true">
            <Background />
          </div>
        ) : null}
        {chrome.viewportControls ? <Controls showInteractive={false} /> : null}
      </ReactFlow>
    </>
  );
}

function preserveNestedNodePointerEvents(): void {}

function TopologyLegend({ label }: { label: string }) {
  return (
    <div
      aria-label={label}
      className="factory-topology-replay__legend"
      role="note"
    >
      <span aria-hidden="true">● / ○</span>
    </div>
  );
}
