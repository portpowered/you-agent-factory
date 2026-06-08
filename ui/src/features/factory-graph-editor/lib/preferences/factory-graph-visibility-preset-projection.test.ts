import { describe, expect, it } from "vitest";

import type { FactoryGraphTopology } from "../draft/factory-graph-draft-types";
import { projectFactoryGraphByVisibilityPreset } from "./factory-graph-visibility-preset-projection";

const SAMPLE_TOPOLOGY: FactoryGraphTopology = {
  nodes: [
    {
      id: "work-type:task",
      key: { kind: "work-type", name: "task" },
      kind: "work-type",
      label: "task",
    },
    {
      id: "work-state:task:init",
      key: { kind: "work-state", name: "init", workTypeName: "task" },
      kind: "work-state",
      label: "init",
    },
    {
      id: "workstation:process",
      key: { kind: "workstation", name: "process" },
      kind: "workstation",
      label: "process",
    },
    {
      id: "worker:processor",
      key: { kind: "worker", name: "processor" },
      kind: "worker",
      label: "processor",
    },
    {
      id: "resource:slot",
      key: { kind: "resource", name: "slot" },
      kind: "resource",
      label: "slot",
    },
  ],
  edges: [
    {
      id: "work-type-state:task:init",
      kind: "work-type-state",
      sourceId: "work-type:task",
      targetId: "work-state:task:init",
    },
    {
      id: "workstation-input:process",
      kind: "workstation-input",
      sourceId: "work-state:task:init",
      targetId: "workstation:process",
    },
    {
      id: "worker-assignment:processor",
      kind: "worker-assignment",
      sourceId: "worker:processor",
      targetId: "workstation:process",
    },
    {
      id: "worker-resource:slot",
      kind: "worker-resource",
      sourceId: "worker:processor",
      targetId: "resource:slot",
    },
  ],
};

describe("projectFactoryGraphByVisibilityPreset", () => {
  it("keeps the full topology for the all preset", () => {
    expect(
      projectFactoryGraphByVisibilityPreset(SAMPLE_TOPOLOGY, "all"),
    ).toEqual(SAMPLE_TOPOLOGY);
  });

  it("filters workflow nodes and edges", () => {
    const projected = projectFactoryGraphByVisibilityPreset(
      SAMPLE_TOPOLOGY,
      "workflow",
    );

    expect(projected.nodes.map((node) => node.id)).toEqual([
      "work-type:task",
      "work-state:task:init",
      "workstation:process",
    ]);
    expect(projected.edges.map((edge) => edge.kind)).toEqual([
      "work-type-state",
      "workstation-input",
    ]);
  });

  it("filters infrastructure nodes and edges", () => {
    const projected = projectFactoryGraphByVisibilityPreset(
      SAMPLE_TOPOLOGY,
      "infrastructure",
    );

    expect(projected.nodes.map((node) => node.kind)).toEqual([
      "workstation",
      "worker",
      "resource",
    ]);
    expect(projected.edges.map((edge) => edge.kind)).toEqual([
      "worker-assignment",
      "worker-resource",
    ]);
  });

  it("filters execution nodes and edges", () => {
    const topology: FactoryGraphTopology = {
      ...SAMPLE_TOPOLOGY,
      edges: [
        ...SAMPLE_TOPOLOGY.edges,
        {
          id: "workstation-on-continue:process",
          kind: "workstation-on-continue",
          sourceId: "workstation:process",
          targetId: "work-state:task:init",
        },
      ],
    };
    const projected = projectFactoryGraphByVisibilityPreset(
      topology,
      "execution",
    );

    expect(projected.nodes.map((node) => node.kind)).toEqual([
      "work-state",
      "workstation",
    ]);
    expect(projected.edges.map((edge) => edge.kind)).toEqual([
      "workstation-input",
      "workstation-on-continue",
    ]);
  });
});
