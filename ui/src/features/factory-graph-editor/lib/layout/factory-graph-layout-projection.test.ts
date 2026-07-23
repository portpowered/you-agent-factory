import { describe, expect, it } from "vitest";

import { baseFactoryDefinition } from "../draft/factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import {
  createDefaultFactoryLayout,
  moveFactoryLayoutNode,
} from "./factory-graph-layout-operations";
import { projectFactoryGraphWithCanonicalLayout } from "./factory-graph-layout-projection";

describe("factory graph canonical layout projection", () => {
  it("maps canonical topology plus layout to expected React Flow node positions", async () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const canonicalLayout = moveFactoryLayoutNode(
      createDefaultFactoryLayout(),
      "workstation:draft",
      { x: 480, y: 220 },
    );

    const { layoutPositionsByNodeId, projection } =
      await projectFactoryGraphWithCanonicalLayout({
        canonicalLayout,
        topology,
      });

    const draftNode = projection.nodes.find(
      (node) => node.id === "workstation:draft",
    );
    const queuedNode = projection.nodes.find(
      (node) => node.id === "work-state:story:queued",
    );

    expect(draftNode?.position).toEqual({ x: 480, y: 220 });
    expect(queuedNode?.position).toEqual(
      layoutPositionsByNodeId.get("work-state:story:queued"),
    );
    expect(queuedNode?.position).not.toEqual({ x: 480, y: 220 });
  });
});
