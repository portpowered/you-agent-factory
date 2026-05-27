import { describe, expect, it, vi } from "vitest";
import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";
import type { FactoryGraphTopology } from "./factory-graph-draft-types";
import { projectFactoryGraphToReactFlow } from "./factory-graph-react-flow-projection";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: projection contract scenarios stay together around one adapter.
describe("factory graph React Flow projection", () => {
  it("projects canonical graph topology into deterministic React Flow nodes and semantic handles", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );

    const projection = projectFactoryGraphToReactFlow({
      topology,
    });

    expect(projection.nodes.map((node) => node.id)).toEqual([
      "resource:gpu",
      "worker:writer",
      "workstation:draft",
      "work-type:story",
      "work-state:story:done",
      "work-state:story:queued",
    ]);
    expect(projection.nodes[0]).toMatchObject({
      data: {
        canEditConnections: false,
        draftStatus: "none",
        kind: "resource",
        label: "gpu",
      },
      position: { x: 0, y: 0 },
      type: "factoryEntity",
    });
    expect(projection.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-input:work-state:story:queued->workstation:draft",
          sourceHandle: "workstation-input-source",
          targetHandle: "workstation-input-target",
        }),
        expect.objectContaining({
          id: "workstation-output:workstation:draft->work-state:story:done",
          sourceHandle: "workstation-output-source",
          targetHandle: "workstation-output-target",
        }),
      ]),
    );
  });

  it("applies active runtime overlays without mutating graph topology", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const originalTopology = structuredClone(topology);

    const projection = projectFactoryGraphToReactFlow({
      runtime: {
        activeEdgeIds: new Set([
          "workstation-output:workstation:draft->work-state:story:done",
        ]),
        activeNodeIds: new Set(["workstation:draft"]),
        focusedNodeIds: new Set(["work-state:story:done"]),
        mutedNodeIds: new Set(["work-state:story:queued"]),
        placeTokenCountsByNodeId: new Map([["work-state:story:queued", 2]]),
        selectedWorkId: "story-123",
        workerStatusByName: new Map([["writer", "active"]]),
      },
      topology,
    });

    expect(topology).toEqual(originalTopology);
    expect(
      projection.nodes.find((node) => node.id === "workstation:draft"),
    ).toMatchObject({
      className: "agent-factory-editor-node--active",
      data: {
        active: true,
        activeFlow: true,
        selectedWorkId: "story-123",
      },
    });
    expect(
      projection.nodes.find((node) => node.id === "worker:writer")?.data,
    ).toMatchObject({
      workerStatus: "active",
      workerStatusLabel: "Active",
    });
    expect(
      projection.nodes.find((node) => node.id === "work-state:story:queued"),
    ).toMatchObject({
      className: "agent-factory-editor-node--muted",
      data: {
        muted: true,
        tokenCount: 2,
      },
    });
    expect(
      projection.edges.find(
        (edge) =>
          edge.id ===
          "workstation-output:workstation:draft->work-state:story:done",
      ),
    ).toMatchObject({
      animated: true,
      className: "agent-factory-editor-edge--active",
      data: {
        active: true,
      },
    });
  });

  it("applies pending editor overlays, validation state, and connection metadata", () => {
    const topology: FactoryGraphTopology =
      buildFactoryGraphTopologyFromDefinition(baseFactoryDefinition);
    const onConnectionAnchorClick = vi.fn();

    const projection = projectFactoryGraphToReactFlow({
      editor: {
        activeTool: "connect",
        canEditConnections: true,
        onConnectionAnchorClick,
        pendingAdditionEdgeIds: new Set([
          "workstation-output:workstation:draft->work-state:story:done",
        ]),
        pendingAdditionNodeIds: new Set(["workstation:draft"]),
        pendingConnectionSource: {
          anchorId: "workstation-output-source",
          nodeId: "workstation:draft",
        },
        pendingRemovalEdgeIds: new Set([
          "workstation-input:work-state:story:queued->workstation:draft",
        ]),
        pendingRemovalNodeIds: new Set(["work-state:story:queued"]),
        validationErrors: [
          {
            code: "MISSING_REQUIRED_FIELD",
            message: "Workstation body is required.",
            target: {
              id: "workstation:draft",
              kind: "node",
            },
          },
        ],
      },
      topology,
    });

    const draftNode = projection.nodes.find(
      (node) => node.id === "workstation:draft",
    );
    const queuedNode = projection.nodes.find(
      (node) => node.id === "work-state:story:queued",
    );
    const successEdge = projection.edges.find(
      (edge) =>
        edge.id ===
        "workstation-output:workstation:draft->work-state:story:done",
    );
    const inputEdge = projection.edges.find(
      (edge) =>
        edge.id ===
        "workstation-input:work-state:story:queued->workstation:draft",
    );

    expect(draftNode).toMatchObject({
      className: "agent-factory-editor-node--pending-addition",
      data: {
        activeTool: "connect",
        canEditConnections: true,
        draftStatus: "addition",
        validationMessage: "Workstation body is required.",
      },
    });
    expect(
      draftNode?.data.connectionAnchors.find(
        (anchor) => anchor.id === "workstation-output-source",
      ),
    ).toMatchObject({
      buttonPressed: true,
      connectable: true,
      variant: "selected",
    });
    expect(
      queuedNode?.data.connectionAnchors.find(
        (anchor) => anchor.id === "workstation-output-target",
      ),
    ).toMatchObject({
      buttonDisabled: true,
      connectable: false,
      variant: "valid-target",
    });
    expect(successEdge?.data).toMatchObject({
      alwaysShowLabel: true,
      pendingStatus: "addition",
    });
    expect(inputEdge?.data).toMatchObject({
      pendingStatus: "removal",
    });
  });
});
