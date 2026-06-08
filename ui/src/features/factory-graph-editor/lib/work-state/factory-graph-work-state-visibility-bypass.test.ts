import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import type { CanonicalFactoryDefinition } from "../draft/factory-graph-draft-types";
import { projectFactoryGraphByHiddenNodeClasses } from "../work-state/factory-graph-node-class-visibility";
import { synthesizeWorkStateVisibilityBypassEdges } from "../work-state/factory-graph-work-state-visibility-bypass";

const workstationChainFactory = {
  name: "work-state-bypass-fixture",
  resources: [],
  workers: [{ name: "reviewer" }, { name: "drafter" }],
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" as const },
        { name: "done", type: "TERMINAL" as const },
      ],
    },
  ],
  workstations: [
    {
      inputs: [],
      name: "review",
      onFailure: [{ state: "queued", workType: "story" }],
      outputs: [{ state: "done", workType: "story" }],
      worker: "reviewer",
    },
    {
      inputs: [{ state: "done", workType: "story" }],
      name: "draft",
      outputs: [],
      worker: "drafter",
    },
  ],
} satisfies CanonicalFactoryDefinition;

const topology = buildFactoryGraphTopologyFromDefinition(
  workstationChainFactory,
);

describe("synthesizeWorkStateVisibilityBypassEdges", () => {
  it("connects producing workstations to consuming workstations through a hidden state", () => {
    const hiddenWorkStateIds = new Set(["work-state:story:done"]);
    const visibleNodeIds = new Set(
      topology.nodes
        .filter((node) => node.kind !== "work-state")
        .map((node) => node.id),
    );

    const bypassEdges = synthesizeWorkStateVisibilityBypassEdges(
      topology,
      hiddenWorkStateIds,
      visibleNodeIds,
    );

    expect(bypassEdges).toEqual([
      expect.objectContaining({
        id: "work-state-visibility-bypass:workstation-output:review->draft:via-work-state:story:done",
        kind: "work-state-visibility-bypass",
        outcomeRouteKind: "workstation-output",
        sourceId: "workstation:review",
        targetId: "workstation:draft",
      }),
    ]);
  });

  it("fans out bypass edges for multiple producers and consumers of the same hidden state", () => {
    const fanOutFactory = {
      ...workstationChainFactory,
      workstations: [
        {
          inputs: [],
          name: "review-a",
          outputs: [{ state: "queued", workType: "story" }],
          worker: "reviewer",
        },
        {
          inputs: [],
          name: "review-b",
          outputs: [{ state: "queued", workType: "story" }],
          worker: "reviewer",
        },
        {
          inputs: [{ state: "queued", workType: "story" }],
          name: "draft-a",
          outputs: [],
          worker: "drafter",
        },
        {
          inputs: [{ state: "queued", workType: "story" }],
          name: "draft-b",
          outputs: [],
          worker: "drafter",
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const fanOutTopology =
      buildFactoryGraphTopologyFromDefinition(fanOutFactory);
    const hiddenWorkStateIds = new Set(["work-state:story:queued"]);
    const visibleNodeIds = new Set(
      fanOutTopology.nodes
        .filter((node) => node.kind !== "work-state")
        .map((node) => node.id),
    );

    const bypassEdges = synthesizeWorkStateVisibilityBypassEdges(
      fanOutTopology,
      hiddenWorkStateIds,
      visibleNodeIds,
    );

    expect(bypassEdges).toHaveLength(4);
    expect(bypassEdges.map((edge) => edge.id).sort()).toEqual(
      [
        "work-state-visibility-bypass:workstation-output:review-a->draft-a:via-work-state:story:queued",
        "work-state-visibility-bypass:workstation-output:review-a->draft-b:via-work-state:story:queued",
        "work-state-visibility-bypass:workstation-output:review-b->draft-a:via-work-state:story:queued",
        "work-state-visibility-bypass:workstation-output:review-b->draft-b:via-work-state:story:queued",
      ].sort(),
    );
  });

  it("preserves outcome route metadata for non-success producer routes", () => {
    const failureTopology = buildFactoryGraphTopologyFromDefinition({
      ...workstationChainFactory,
      workstations: [
        {
          inputs: [],
          name: "review",
          onFailure: [{ state: "queued", workType: "story" }],
          outputs: [],
          worker: "reviewer",
        },
        {
          inputs: [{ state: "queued", workType: "story" }],
          name: "draft",
          outputs: [],
          worker: "drafter",
        },
      ],
    });
    const hiddenWorkStateIds = new Set(["work-state:story:queued"]);
    const visibleNodeIds = new Set(
      failureTopology.nodes
        .filter((node) => node.kind !== "work-state")
        .map((node) => node.id),
    );

    const bypassEdges = synthesizeWorkStateVisibilityBypassEdges(
      failureTopology,
      hiddenWorkStateIds,
      visibleNodeIds,
    );

    expect(bypassEdges).toEqual([
      expect.objectContaining({
        outcomeRouteKind: "workstation-on-failure",
        sourceId: "workstation:review",
        targetId: "workstation:draft",
      }),
    ]);
  });
});

describe("projectFactoryGraphByHiddenNodeClasses work-state bypass", () => {
  it("adds bypass edges when work states are hidden and removes them when shown", () => {
    const hidden = projectFactoryGraphByHiddenNodeClasses(
      topology,
      new Set(["work-state"]),
    );
    const visible = projectFactoryGraphByHiddenNodeClasses(topology, new Set());

    expect(hidden.nodes.some((node) => node.kind === "work-state")).toBe(false);
    expect(
      hidden.edges.some((edge) => edge.kind === "work-state-visibility-bypass"),
    ).toBe(true);
    expect(
      hidden.edges.some(
        (edge) =>
          edge.kind !== "work-state-visibility-bypass" &&
          (edge.sourceId === "work-state:story:done" ||
            edge.targetId === "work-state:story:done"),
      ),
    ).toBe(false);
    expect(
      visible.edges.some(
        (edge) => edge.kind === "work-state-visibility-bypass",
      ),
    ).toBe(false);
    expect(
      visible.edges.some(
        (edge) =>
          edge.kind === "workstation-output" &&
          edge.targetId === "work-state:story:done",
      ),
    ).toBe(true);
  });
});
