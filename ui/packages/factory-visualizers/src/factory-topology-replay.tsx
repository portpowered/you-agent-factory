import {
  FactoryGraphReplaySurface,
  type FactoryGraphSource,
} from "@you-agent-factory/factory-graph";
import type { FactoryTopologyNode } from "@you-agent-factory/factory-replay";

import {
  FactoryTopologyErrorBoundary,
  FactoryTopologyStateRegion,
} from "./factory-topology-state";
import type { FactoryTopologyReplayError } from "./visualizer-error";

/**
 * Controlled state for the canonical Factory graph. A ready graph always has
 * the complete Factory definition and the selected-tick runtime projection.
 */
export type FactoryTopologyReplayState =
  | { status: "empty" }
  | { status: "failed" }
  | { status: "loading" }
  | { source: FactoryGraphSource; status: "ready" };

export interface FactoryTopologyReplayMessages {
  activeDispatches: (count: number) => string;
  activeWorkDuration?: (durationTicks: number) => string;
  activeWorkOverflow?: (count: number) => string;
  activeWorkRegionLabel?: string;
  annotationsHidden: string;
  annotationsVisible: string;
  empty: string;
  failed: string;
  inactiveDispatches: string;
  imageFailed: string;
  imageLoading: string;
  legendActiveRoute?: string;
  legendInactiveRoute?: string;
  legendLabel?: string;
  loading: string;
  nodeLabel: (kind: FactoryTopologyNode["kind"], label: string) => string;
  regionLabel: string;
  resourceOccupancy: (occupied: number, capacity: number) => string;
  resourceOccupancyUnavailable: string;
  retry: string;
  selectedNode: string;
  viewportControlsLabel?: string;
  workStateCount: (count: number) => string;
  workStateCountUnavailable: string;
}

export interface FactoryTopologyReplayProps {
  messages: FactoryTopologyReplayMessages;
  onError?: (error: FactoryTopologyReplayError) => void;
  onRetry?: () => void;
  onSelectNode?: (node: FactoryTopologyNode) => void;
  selectedNodeId?: string;
  state: FactoryTopologyReplayState;
}

/**
 * Public read-only Factory graph. Rendering is delegated to the same semantic
 * renderer registry used by the embedded Factory graph; there is no
 * topology-only renderer or visual fallback.
 */
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
  onRetry,
  onSelectNode,
  selectedNodeId,
  state,
}: FactoryTopologyReplayProps) {
  if (state.status !== "ready") {
    return (
      <FactoryTopologyStateRegion
        messages={messages}
        onRetry={onRetry}
        state={state.status}
      />
    );
  }

  return (
    <section
      aria-label={messages.regionLabel}
      className="factory-topology-replay"
    >
      <FactoryGraphReplaySurface
        onSelectNode={
          onSelectNode
            ? (nodeId) => {
                const node = state.source.runtime.topology.nodes.find(
                  (entry) => entry.id === nodeId,
                );
                if (node) onSelectNode(node);
              }
            : undefined
        }
        selectedNodeId={selectedNodeId}
        source={state.source}
      />
    </section>
  );
}
