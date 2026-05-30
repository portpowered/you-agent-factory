import {
  filterFactoryGraphTopologyForCustomerDisplay,
  isHiddenSystemTimeGraphNode,
  isSystemTimeGraphWorkStateNode,
  isSystemTimeGraphWorkstationNode,
  isSystemTimeGraphWorkTypeNode,
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
  SYSTEM_TIME_WORK_TYPE_ID,
  systemTimeGraphNodeId,
} from "./factory-graph-customer-display";
import type { FactoryGraphNode } from "./factory-graph-draft-types";
import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";
import type { CanonicalFactoryDefinition } from "./factory-graph-draft-types";

const pureSystemTimeFactory = {
  name: "system-time-only",
  workTypes: [
    {
      name: SYSTEM_TIME_WORK_TYPE_ID,
      states: [{ name: "pending", type: "PROCESSING" as const }],
    },
  ],
  workstations: [
    {
      id: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
      inputs: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
      name: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
      outputs: [],
      worker: "",
    },
  ],
} satisfies CanonicalFactoryDefinition;

const mixedSystemTimeFactory = {
  name: "mixed-public-system-time",
  workTypes: [
    {
      name: "story",
      states: [
        { name: "new", type: "INITIAL" as const },
        { name: "reviewing", type: "PROCESSING" as const },
        { name: "done", type: "TERMINAL" as const },
      ],
    },
    {
      name: SYSTEM_TIME_WORK_TYPE_ID,
      states: [{ name: "pending", type: "PROCESSING" as const }],
    },
  ],
  workstations: [
    {
      behavior: "CLASSIFIER_WORKSTATION",
      classificationRoutes: [
        {
          label: "ready",
          outputs: [{ state: "reviewing", workType: "story" }],
        },
        {
          label: "tick",
          outputs: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
        },
      ],
      id: "route-story",
      inputs: [
        { state: "new", workType: "story" },
        { state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID },
      ],
      name: "Route story",
      onContinue: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
      onFailure: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
      onRejection: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
      outputs: [
        { state: "done", workType: "story" },
        { state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID },
      ],
      worker: "router",
    },
    {
      id: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
      inputs: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
      name: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
      outputs: [],
      worker: "",
    },
  ],
} satisfies CanonicalFactoryDefinition;

const customerOnlyFactory = {
  name: "customer-only",
  workTypes: [
    {
      name: "story",
      states: [
        { name: "new", type: "INITIAL" as const },
        { name: "done", type: "TERMINAL" as const },
      ],
    },
  ],
  workstations: [
    {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      outputs: [{ state: "done", workType: "story" }],
      worker: "reviewer",
    },
  ],
} satisfies CanonicalFactoryDefinition;

function nodeIds(
  topology: ReturnType<typeof filterFactoryGraphTopologyForCustomerDisplay>,
) {
  return topology.nodes.map((node) => node.id);
}

function edgeIds(
  topology: ReturnType<typeof filterFactoryGraphTopologyForCustomerDisplay>,
) {
  return topology.edges.map((edge) => edge.id);
}

describe("system time graph predicates", () => {
  const publicWorkTypeNode = {
    id: systemTimeGraphNodeId("work-type", "story"),
    key: { kind: "work-type" as const, name: "story" },
  } satisfies FactoryGraphNode;

  it("identifies system-time work type, state, and workstation nodes", () => {
    expect(
      isSystemTimeGraphWorkTypeNode({
        id: systemTimeGraphNodeId("work-type", SYSTEM_TIME_WORK_TYPE_ID),
        key: { kind: "work-type", name: SYSTEM_TIME_WORK_TYPE_ID },
      }),
    ).toBe(true);
    expect(
      isSystemTimeGraphWorkStateNode({
        id: systemTimeGraphNodeId(
          "work-state",
          SYSTEM_TIME_WORK_TYPE_ID,
          "pending",
        ),
        key: {
          kind: "work-state",
          stateName: "pending",
          workTypeName: SYSTEM_TIME_WORK_TYPE_ID,
        },
      }),
    ).toBe(true);
    expect(
      isSystemTimeGraphWorkstationNode({
        id: systemTimeGraphNodeId("workstation", SYSTEM_TIME_EXPIRY_TRANSITION_ID),
        key: {
          kind: "workstation",
          name: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
        },
      }),
    ).toBe(true);
    expect(isHiddenSystemTimeGraphNode(publicWorkTypeNode)).toBe(false);
  });

  it("builds graph node ids for each supported node kind", () => {
    expect(systemTimeGraphNodeId("work-type", "story")).toBe("work-type:story");
    expect(systemTimeGraphNodeId("work-state", "story", "new")).toBe(
      "work-state:story:new",
    );
    expect(systemTimeGraphNodeId("workstation", "Review")).toBe(
      "workstation:Review",
    );
  });
});

describe("filterFactoryGraphTopologyForCustomerDisplay", () => {
  it("removes pure system-time graph nodes and incident edges", () => {
    const rawTopology = buildFactoryGraphTopologyFromDefinition(
      pureSystemTimeFactory,
    );
    const filtered = filterFactoryGraphTopologyForCustomerDisplay(rawTopology);

    expect(nodeIds(rawTopology)).toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", SYSTEM_TIME_WORK_TYPE_ID),
        systemTimeGraphNodeId(
          "work-state",
          SYSTEM_TIME_WORK_TYPE_ID,
          "pending",
        ),
        systemTimeGraphNodeId("workstation", SYSTEM_TIME_EXPIRY_TRANSITION_ID),
      ]),
    );
    expect(
      nodeIds(filtered).every(
        (nodeId) => !nodeId.includes(SYSTEM_TIME_WORK_TYPE_ID),
      ),
    ).toBe(true);
    expect(
      edgeIds(filtered).every(
        (edgeId) =>
          !edgeId.includes(SYSTEM_TIME_WORK_TYPE_ID) &&
          !edgeId.includes(SYSTEM_TIME_EXPIRY_TRANSITION_ID),
      ),
    ).toBe(true);
  });

  it("removes system-time nodes from mixed factories while keeping public routes", () => {
    const rawTopology = buildFactoryGraphTopologyFromDefinition(
      mixedSystemTimeFactory,
    );
    const filtered = filterFactoryGraphTopologyForCustomerDisplay(rawTopology);

    expect(nodeIds(rawTopology)).toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", SYSTEM_TIME_WORK_TYPE_ID),
        systemTimeGraphNodeId(
          "work-state",
          SYSTEM_TIME_WORK_TYPE_ID,
          "pending",
        ),
        systemTimeGraphNodeId("workstation", SYSTEM_TIME_EXPIRY_TRANSITION_ID),
        systemTimeGraphNodeId("work-type", "story"),
        systemTimeGraphNodeId("work-state", "story", "new"),
        systemTimeGraphNodeId("work-state", "story", "reviewing"),
        systemTimeGraphNodeId("work-state", "story", "done"),
        systemTimeGraphNodeId("workstation", "Route story"),
      ]),
    );
    expect(nodeIds(filtered)).not.toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", SYSTEM_TIME_WORK_TYPE_ID),
        systemTimeGraphNodeId(
          "work-state",
          SYSTEM_TIME_WORK_TYPE_ID,
          "pending",
        ),
        systemTimeGraphNodeId("workstation", SYSTEM_TIME_EXPIRY_TRANSITION_ID),
      ]),
    );
    expect(nodeIds(filtered)).toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", "story"),
        systemTimeGraphNodeId("work-state", "story", "new"),
        systemTimeGraphNodeId("work-state", "story", "reviewing"),
        systemTimeGraphNodeId("work-state", "story", "done"),
        systemTimeGraphNodeId("workstation", "Route story"),
      ]),
    );
    expect(
      edgeIds(filtered).some((edgeId) =>
        edgeId.includes(SYSTEM_TIME_WORK_TYPE_ID),
      ),
    ).toBe(false);
    expect(
      edgeIds(filtered).some((edgeId) =>
        edgeId.includes(systemTimeGraphNodeId("workstation", "Route story")),
      ),
    ).toBe(true);
  });

  it("keeps customer-only factories unchanged", () => {
    const rawTopology =
      buildFactoryGraphTopologyFromDefinition(customerOnlyFactory);
    const filtered = filterFactoryGraphTopologyForCustomerDisplay(rawTopology);

    expect(filtered).toEqual(rawTopology);
    expect(nodeIds(filtered)).toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", "story"),
        systemTimeGraphNodeId("work-state", "story", "new"),
        systemTimeGraphNodeId("work-state", "story", "done"),
        systemTimeGraphNodeId("workstation", "Review"),
      ]),
    );
    expect(
      nodeIds(filtered).some((nodeId) =>
        nodeId.includes(SYSTEM_TIME_WORK_TYPE_ID),
      ),
    ).toBe(false);
  });
});
