import type { FactoryVisualizationLayoutV1 } from "@you-agent-factory/client";
import type { FactoryTopologyNode } from "@you-agent-factory/factory-replay";
import {
  FactoryTimelineScrubber,
  FactoryTopologyReplay,
} from "@you-agent-factory/factory-visualizers";
import { useState } from "react";

import { useHostedTopologyReplayAdapter } from "../../hooks/topology-replay/use-hosted-topology-replay-adapter";
import {
  getHostedTopologyReplayMessages,
  type HostedTopologyReplayMessages,
} from "../../messages/hosted-topology-replay";

export interface HostedTopologyReplayProps {
  /** Validated, caller-owned presentation data; never part of replay state. */
  layout?: FactoryVisualizationLayoutV1;
  locale?: string;
  onSelectResource?: (resourceID: string) => void;
  onSelectStateNode?: (stateID: string) => void;
  onSelectWorker?: (workerID: string) => void;
  onSelectWorkType?: (workTypeID: string) => void;
  onSelectWorkstation?: (workstationID: string) => void;
  selectedNodeID?: string;
}

export function HostedTopologyReplay({
  layout,
  locale,
  onSelectResource,
  onSelectStateNode,
  onSelectWorker,
  onSelectWorkType,
  onSelectWorkstation,
  selectedNodeID,
}: HostedTopologyReplayProps) {
  const adapter = useHostedTopologyReplayAdapter();
  const messages = getHostedTopologyReplayMessages(locale);
  const [visualizerRetryKey, setVisualizerRetryKey] = useState(0);
  const handleSelectNode = (node: FactoryTopologyNode) => {
    switch (node.kind) {
      case "resource":
        onSelectResource?.(node.entityId);
        break;
      case "worker":
        onSelectWorker?.(node.entityId);
        break;
      case "work-state":
        onSelectStateNode?.(node.entityId);
        break;
      case "work-type":
        onSelectWorkType?.(node.entityId);
        break;
      case "workstation":
        onSelectWorkstation?.(node.entityId);
        break;
    }
  };
  const streamNotice = streamNoticeForStatus(
    adapter.state.streamState.status,
    messages.stream,
  );

  return (
    <div className="grid min-h-0 min-w-0 gap-2 overflow-hidden">
      {streamNotice ? (
        <p
          className="m-0 rounded-lg border border-af-warning-border bg-warning-container px-3 py-2 text-sm text-on-warning-container"
          role={
            adapter.state.streamState.status === "reconnecting"
              ? "status"
              : "alert"
          }
        >
          {streamNotice}
        </p>
      ) : null}
      <FactoryTopologyReplay
        key={visualizerRetryKey}
        layout={layout}
        messages={messages.topology}
        onRetry={
          adapter.state.topologyState.status === "ready"
            ? () => {
                setVisualizerRetryKey((current) => current + 1);
              }
            : undefined
        }
        onSelectNode={handleSelectNode}
        selectedNodeId={selectedNodeID}
        state={adapter.state.topologyState}
      />
      <FactoryTimelineScrubber
        formatTick={messages.formatTick}
        messages={messages.timeline}
        onFollowLatest={adapter.followLatest}
        onSelectTick={adapter.selectTick}
        state={adapter.state.timelineState}
      />
    </div>
  );
}

function streamNoticeForStatus(
  status:
    | "connecting"
    | "live"
    | "offline"
    | "reconnecting"
    | "recovery_failed",
  messages: HostedTopologyReplayMessages["stream"],
): string | null {
  switch (status) {
    case "offline":
      return messages.offline;
    case "reconnecting":
      return messages.reconnecting;
    case "recovery_failed":
      return messages.recoveryFailed;
    default:
      return null;
  }
}
