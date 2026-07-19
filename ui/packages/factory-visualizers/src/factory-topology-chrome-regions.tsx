import { Background, Controls, ReactFlow } from "@xyflow/react";
import { useState } from "react";

import type { ResolvedFactoryTopologyChrome } from "./factory-topology-chrome";
import type {
  FactoryTopologyFlowProjection,
  FactoryTopologyReplayMessages,
} from "./factory-topology-replay";
import { factoryTopologyNodeTypes } from "./factory-topology-replay";

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
  const renderedFlow = showNodeKinds
    ? flow
    : {
        ...flow,
        nodes: flow.nodes.map((node) => ({
          ...node,
          data: { ...node.data, showNodeKinds: false },
        })),
      };

  return (
    <>
      {chrome.legend ? <TopologyLegend label={messages.legendLabel} /> : null}
      {chrome.visibilityControls ? (
        <button
          aria-pressed={showNodeKinds}
          className="factory-topology-replay__visibility-control"
          onClick={() => setShowNodeKinds((visible) => !visible)}
          type="button"
        >
          {showNodeKinds ? messages.hideNodeKinds : messages.showNodeKinds}
        </button>
      ) : null}
      <ReactFlow
        edges={renderedFlow.edges}
        edgesFocusable={false}
        elementsSelectable={false}
        fitView
        nodes={renderedFlow.nodes}
        nodesConnectable={false}
        nodesDraggable={false}
        nodeTypes={factoryTopologyNodeTypes}
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
