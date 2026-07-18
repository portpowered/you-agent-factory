/** Stable category path for `@you-agent-factory/components/visualizers`. */
export const COMPONENTS_CATEGORY = "visualizers" as const;

export {
  FactoryTopologyReplay,
  type FactoryTopologyReplayProps,
} from "./factory-topology-replay";
export type { FactoryTopologyFlowProjection } from "./factory-topology-replay-projection";
export { projectFactoryTopologyToFlow } from "./factory-topology-replay-projection";
export type {
  FactoryTopologyConnectionKind,
  FactoryTopologyNodeKind,
  FactoryTopologyReplayActivity,
  FactoryTopologyReplayConnection,
  FactoryTopologyReplayDispatch,
  FactoryTopologyReplayHandle,
  FactoryTopologyReplayMessages,
  FactoryTopologyReplayNode,
  FactoryTopologyReplayOccupancy,
  FactoryTopologyReplayProjection,
  FactoryTopologyReplayTopology,
  FactoryTopologyReplayWorkStateCount,
  FactoryVisualizerError,
  FactoryVisualizerErrorKind,
} from "./factory-topology-replay-types";
