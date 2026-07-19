import type { FactoryTopologyNode } from "@you-agent-factory/factory-replay";
import {
  FactoryTimelineScrubber,
  FactoryTopologyReplay,
} from "@you-agent-factory/factory-visualizers";

import { useHostedTopologyReplayAdapter } from "../../hooks/topology-replay/use-hosted-topology-replay-adapter";
import { getHostedTopologyReplayMessages } from "../../messages/hosted-topology-replay";

export interface HostedTopologyReplayProps {
  locale?: string;
  onSelectResource?: (resourceID: string) => void;
  onSelectStateNode?: (stateID: string) => void;
  onSelectWorker?: (workerID: string) => void;
  onSelectWorkType?: (workTypeID: string) => void;
  onSelectWorkstation?: (workstationID: string) => void;
  selectedNodeID?: string;
}

export function HostedTopologyReplay({
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

  return (
    <div className="grid min-h-0 min-w-0 gap-2 overflow-hidden">
      <FactoryTopologyReplay
        messages={messages.topology}
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
