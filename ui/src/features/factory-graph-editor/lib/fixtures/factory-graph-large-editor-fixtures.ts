import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphTopology,
} from "../draft/factory-graph-draft-types";
import type { FactoryLayout } from "../layout/factory-graph-layout-operations";

export const FACTORY_GRAPH_LARGE_EDITOR_FIXTURE_TARGETS = {
  hundred: 100,
  fiveHundred: 500,
  stressThousand: 1000,
} as const;

export type FactoryGraphLargeEditorFixtureKey =
  keyof typeof FACTORY_GRAPH_LARGE_EDITOR_FIXTURE_TARGETS;

export interface FactoryGraphLargeEditorFixture {
  factoryDefinition: CanonicalFactoryDefinition;
  fixtureKey: FactoryGraphLargeEditorFixtureKey;
  graphNodeCount: number;
  layout: FactoryLayout;
  targetGraphNodeCount: number;
  topology: FactoryGraphTopology;
}

function workstationCountForTargetGraphNodeCount(targetGraphNodeCount: number) {
  return Math.max(1, Math.ceil((targetGraphNodeCount - 2) / 4));
}

function buildRepresentativeLayoutMetadata(
  workstationCount: number,
): NonNullable<FactoryLayout["nodes"]> {
  const layoutNodes: NonNullable<FactoryLayout["nodes"]> = [];
  for (let index = 0; index < workstationCount; index += 1) {
    if (index % 5 !== 0) {
      continue;
    }

    layoutNodes.push({
      id: `workstation:ws-${index}`,
      position: {
        x: (index % 20) * 180,
        y: Math.floor(index / 20) * 120,
      },
    });
  }
  return layoutNodes;
}

export function buildLargeFactoryEditorFixture(
  targetGraphNodeCount: number,
  fixtureKey: FactoryGraphLargeEditorFixtureKey,
): FactoryGraphLargeEditorFixture {
  const workstationCount =
    workstationCountForTargetGraphNodeCount(targetGraphNodeCount);
  const workTypes: NonNullable<CanonicalFactoryDefinition["workTypes"]> = [];
  const workstations: NonNullable<CanonicalFactoryDefinition["workstations"]> =
    [];

  for (let index = 0; index < workstationCount; index += 1) {
    const workTypeName = `task-${index}`;
    workTypes.push({
      name: workTypeName,
      states: [
        { name: "init", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    });
    workstations.push({
      inputs: [{ state: "init", workType: workTypeName }],
      name: `ws-${index}`,
      outputs: [{ state: "done", workType: workTypeName }],
      type: "MODEL_WORKSTATION",
      worker: "processor",
    });
  }

  const layout: FactoryLayout = {
    nodes: buildRepresentativeLayoutMetadata(workstationCount),
    schemaVersion: 1,
    viewport: {
      x: 0,
      y: 0,
      zoom: 0.75,
    },
  };

  const factoryDefinition: CanonicalFactoryDefinition = {
    layout,
    name: `large-factory-${fixtureKey}`,
    resources: [{ capacity: 10, name: "slot" }],
    workers: [{ name: "processor", type: "MODEL_WORKER" }],
    workTypes,
    workstations,
  };

  const topology = buildFactoryGraphTopologyFromDefinition(factoryDefinition);

  return {
    factoryDefinition,
    fixtureKey,
    graphNodeCount: topology.nodes.length,
    layout,
    targetGraphNodeCount,
    topology,
  };
}

const GRID_AUTO_LAYOUT_COLUMNS = 25;
const GRID_AUTO_LAYOUT_X_STEP = 180;
const GRID_AUTO_LAYOUT_Y_STEP = 120;

export function buildGridAutoLayoutPositionsByNodeId(
  nodeIds: readonly string[],
): Map<string, NonNullable<FactoryLayout["nodes"]>[number]["position"]> {
  const positions = new Map<
    string,
    NonNullable<FactoryLayout["nodes"]>[number]["position"]
  >();

  for (let index = 0; index < nodeIds.length; index += 1) {
    const nodeId = nodeIds[index];
    if (!nodeId) {
      continue;
    }

    positions.set(nodeId, {
      x: (index % GRID_AUTO_LAYOUT_COLUMNS) * GRID_AUTO_LAYOUT_X_STEP,
      y: Math.floor(index / GRID_AUTO_LAYOUT_COLUMNS) * GRID_AUTO_LAYOUT_Y_STEP,
    });
  }

  return positions;
}

export const factoryGraphLargeEditorFixtures = {
  hundred: buildLargeFactoryEditorFixture(
    FACTORY_GRAPH_LARGE_EDITOR_FIXTURE_TARGETS.hundred,
    "hundred",
  ),
  fiveHundred: buildLargeFactoryEditorFixture(
    FACTORY_GRAPH_LARGE_EDITOR_FIXTURE_TARGETS.fiveHundred,
    "fiveHundred",
  ),
  stressThousand: buildLargeFactoryEditorFixture(
    FACTORY_GRAPH_LARGE_EDITOR_FIXTURE_TARGETS.stressThousand,
    "stressThousand",
  ),
} satisfies Record<
  FactoryGraphLargeEditorFixtureKey,
  FactoryGraphLargeEditorFixture
>;
