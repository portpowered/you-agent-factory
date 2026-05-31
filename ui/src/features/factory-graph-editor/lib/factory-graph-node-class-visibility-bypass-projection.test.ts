import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";
import type { CanonicalFactoryDefinition } from "./factory-graph-draft-types";
import { projectFactoryGraphByHiddenNodeClasses } from "./factory-graph-node-class-visibility";
import { projectFactoryGraphToReactFlow } from "./factory-graph-react-flow-projection";

const workstationChainFactory = {
  name: "work-state-bypass-projection-fixture",
  resources: [],
  workers: [{ name: "reviewer" }, { name: "drafter" }],
  workTypes: [
    {
      name: "story",
      states: [{ name: "done", type: "TERMINAL" as const }],
    },
  ],
  workstations: [
    {
      inputs: [],
      name: "review",
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

describe("projectFactoryGraphToReactFlow with hidden work states", () => {
  it("projects bypass edges onto workstation output and input handles", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      workstationChainFactory,
    );
    const visibleTopology = projectFactoryGraphByHiddenNodeClasses(
      topology,
      new Set(["work-state"]),
    );
    const projection = projectFactoryGraphToReactFlow({
      filterEdgesToRenderedHandles: true,
      topology: visibleTopology,
    });

    const bypassEdge = projection.edges.find(
      (edge) => edge.data?.kind === "work-state-visibility-bypass",
    );

    expect(bypassEdge).toMatchObject({
      source: "workstation:review",
      sourceHandle: "workstation-output-source",
      target: "workstation:draft",
      targetHandle: "workstation-input-target",
    });
    expect(bypassEdge?.data?.label).toBe("Success route");
  });
});
