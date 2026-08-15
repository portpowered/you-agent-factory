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

export interface FactoryGraphLargeEditorParityFixture {
  authoredSizeByNodeId: ReadonlyMap<string, { height: number; width: number }>;
  fixture: FactoryGraphLargeEditorFixture;
  groups: NonNullable<FactoryLayout["groups"]>;
  layout: FactoryLayout;
  workStateNodeIds: readonly string[];
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

/**
 * Build one representative large graph for the visual and browser parity
 * matrix. The layout metadata is authored state; runtime counts and emphasis
 * are supplied by the host projection that consumes this fixture.
 */
export function buildLargeFactoryEditorParityFixture(
  fixture: FactoryGraphLargeEditorFixture,
): FactoryGraphLargeEditorParityFixture {
  const authoredSizeByNodeId = new Map<
    string,
    { height: number; width: number }
  >();
  const layoutNodes = (fixture.layout.nodes ?? []).map((node, index) => {
    const size = {
      height: 96 + (index % 3) * 24,
      width: 196 + (index % 3) * 48,
    };
    authoredSizeByNodeId.set(node.id, size);
    return { ...node, size };
  });
  const topologyNodeIds = fixture.topology.nodes.map((node) => node.id);
  const workStateNodeIds = fixture.topology.nodes
    .filter((node) => node.kind === "work-state")
    .map((node) => node.id);
  const groups = [
    {
      bounds: { height: 760, width: 1_320, x: -64, y: -64 },
      color: "info",
      id: "large-parity-workflow",
      // hardcoded-ui-copy-exception: non-product-diagnostic
      label: "Workflow context",
      nodeIds: topologyNodeIds.slice(0, 120),
    },
    {
      bounds: { height: 760, width: 1_320, x: 840, y: 240 },
      color: "warning",
      id: "large-parity-review",
      // hardcoded-ui-copy-exception: non-product-diagnostic
      label: "Review lane",
      nodeIds: topologyNodeIds.slice(120, 240),
    },
    {
      bounds: { height: 760, width: 1_320, x: 1_744, y: 544 },
      color: "success",
      id: "large-parity-infrastructure",
      // hardcoded-ui-copy-exception: non-product-diagnostic
      label: "Infrastructure context",
      nodeIds: topologyNodeIds.slice(240, 360),
    },
  ] satisfies NonNullable<FactoryLayout["groups"]>;

  return {
    authoredSizeByNodeId,
    fixture,
    groups,
    layout: {
      ...fixture.layout,
      groups,
      nodes: layoutNodes,
    },
    workStateNodeIds,
  };
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
