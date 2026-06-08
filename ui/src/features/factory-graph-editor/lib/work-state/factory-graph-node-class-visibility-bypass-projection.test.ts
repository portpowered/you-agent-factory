import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import type { CanonicalFactoryDefinition } from "../draft/factory-graph-draft-types";
import { projectFactoryGraphToReactFlow } from "../projection/factory-graph-react-flow-projection";
import { projectFactoryGraphByHiddenNodeClasses } from "../work-state/factory-graph-node-class-visibility";

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
