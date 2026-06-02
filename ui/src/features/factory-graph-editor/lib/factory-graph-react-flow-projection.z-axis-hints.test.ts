import { describe, expect, it } from "vitest";
import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";
import type { CanonicalFactoryDefinition } from "./factory-graph-draft-types";
import { projectFactoryGraphToReactFlow } from "./factory-graph-react-flow-projection";

describe("factory graph React Flow projection z-axis incomplete hints", () => {
  const factoryWithoutStopWords = {
    ...baseFactoryDefinition,
    workstations: [
      {
        ...baseFactoryDefinition.workstations[0],
        behavior: "STANDARD",
        stopWords: undefined,
      },
    ],
  } satisfies CanonicalFactoryDefinition;

  const workstationResolver = {
    resolveWorkstation: (name: string) =>
      factoryWithoutStopWords.workstations.find(
        (workstation) => workstation.name === name,
      ),
  };

  it("omits z-axis incomplete hints when connection editing is disabled", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      factoryWithoutStopWords,
    );

    const projection = projectFactoryGraphToReactFlow({
      editor: {
        canEditConnections: false,
        pendingAdditionEdgeIds: new Set(),
        pendingAdditionNodeIds: new Set(),
        pendingConnectionSource: null,
        pendingRemovalEdgeIds: new Set(),
        pendingRemovalNodeIds: new Set(),
      },
      topology,
      workstationResolver,
    });

    expect(
      projection.nodes.find((node) => node.id === "workstation:draft")?.data
        .zAxisIncompleteHints,
    ).toBeNull();
  });

  it("omits z-axis incomplete hints when continue and reject handles are not rendered", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      factoryWithoutStopWords,
    );

    const projection = projectFactoryGraphToReactFlow({
      editor: {
        canEditConnections: true,
        pendingAdditionEdgeIds: new Set(),
        pendingAdditionNodeIds: new Set(),
        pendingConnectionSource: null,
        pendingRemovalEdgeIds: new Set(),
        pendingRemovalNodeIds: new Set(),
      },
      topology,
      workstationResolver,
    });
    const workstationNode = projection.nodes.find(
      (node) => node.id === "workstation:draft",
    );

    expect(workstationNode?.data.zAxisIncompleteHints).toBeNull();
    expect(
      projection.nodes.find((node) => node.id === "work-state:story:queued")
        ?.data.zAxisIncompleteHints,
    ).toBeNull();
  });

  it("omits z-axis incomplete hints when projection has no editor overlay", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      factoryWithoutStopWords,
    );

    const projection = projectFactoryGraphToReactFlow({
      topology,
      workstationResolver,
    });

    expect(
      projection.nodes.find((node) => node.id === "workstation:draft")?.data
        .zAxisIncompleteHints,
    ).toBeNull();
  });

  it("omits z-axis incomplete hints when stopWords are configured", () => {
    const factoryWithStopWords = {
      ...baseFactoryDefinition,
      workstations: [
        {
          ...baseFactoryDefinition.workstations[0],
          behavior: "STANDARD",
          stopWords: ["DONE"],
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const topology =
      buildFactoryGraphTopologyFromDefinition(factoryWithStopWords);

    const projection = projectFactoryGraphToReactFlow({
      topology,
      workstationResolver: {
        resolveWorkstation: (name) =>
          factoryWithStopWords.workstations.find(
            (workstation) => workstation.name === name,
          ),
      },
    });

    expect(
      projection.nodes.find((node) => node.id === "workstation:draft")?.data
        .zAxisIncompleteHints,
    ).toBeNull();
  });
});
