import { describe, expect, it, vi } from "vitest";
import {
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
  SYSTEM_TIME_WORK_TYPE_ID,
  systemTimeGraphNodeId,
} from "./factory-graph-customer-display";
import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphTopology,
} from "./factory-graph-draft-types";
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
          targetHandle: "work-state-input-target",
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
        (anchor) => anchor.id === "work-state-input-target",
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

  it("omits system-time topology from React Flow projection while raw topology still contains it", () => {
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
              outputs: [
                { state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID },
              ],
            },
          ],
          id: "route-story",
          inputs: [
            { state: "new", workType: "story" },
            { state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID },
          ],
          name: "Route story",
          onContinue: [
            { state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID },
          ],
          onFailure: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
          onRejection: [
            { state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID },
          ],
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
    const rawTopology = buildFactoryGraphTopologyFromDefinition(
      mixedSystemTimeFactory,
    );
    const originalTopology = structuredClone(rawTopology);

    expect(rawTopology.nodes.map((node) => node.id)).toEqual(
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

    const projection = projectFactoryGraphToReactFlow({
      topology: rawTopology,
    });
    const projectedNodeIds = projection.nodes.map((node) => node.id);

    expect(rawTopology).toEqual(originalTopology);
    expect(projectedNodeIds).not.toEqual(
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
    expect(projectedNodeIds).toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", "story"),
        systemTimeGraphNodeId("workstation", "Route story"),
      ]),
    );
    expect(
      projection.edges
        .flatMap((edge) => [edge.source, edge.target])
        .some((nodeId) => nodeId?.includes(SYSTEM_TIME_WORK_TYPE_ID)),
    ).toBe(false);
  });

  it("omits continue and reject handles for a standard processor without stopWords", () => {
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
    const topology = buildFactoryGraphTopologyFromDefinition(
      factoryWithoutStopWords,
    );

    const projection = projectFactoryGraphToReactFlow({
      topology,
      workstationResolver: {
        resolveWorkstation: (name) =>
          factoryWithoutStopWords.workstations.find(
            (workstation) => workstation.name === name,
          ),
      },
    });
    const anchorIds =
      projection.nodes
        .find((node) => node.id === "workstation:draft")
        ?.data.connectionAnchors.map((anchor) => anchor.id) ?? [];

    expect(anchorIds).toEqual(
      expect.arrayContaining([
        "workstation-output-source",
        "workstation-on-failure-source",
      ]),
    );
    expect(anchorIds).not.toContain("workstation-on-continue-source");
    expect(anchorIds).not.toContain("workstation-on-rejection-source");
  });

  it("omits worker-assignment handles on LOGICAL_MOVE workstations", () => {
    const factoryWithLogicalMove = {
      ...baseFactoryDefinition,
      workstations: [
        ...(baseFactoryDefinition.workstations ?? []),
        {
          body: "Move work downstream.",
          inputs: [
            {
              state: "queued",
              workType: "story",
            },
          ],
          name: "router",
          outputs: [
            {
              state: "done",
              workType: "story",
            },
          ],
          type: "LOGICAL_MOVE",
          worker: "",
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const topology = buildFactoryGraphTopologyFromDefinition(
      factoryWithLogicalMove,
    );

    const projection = projectFactoryGraphToReactFlow({
      topology,
      workstationResolver: {
        resolveWorkstation: (name) =>
          factoryWithLogicalMove.workstations?.find(
            (workstation) => workstation.name === name,
          ),
      },
    });
    const logicalMoveAnchorIds =
      projection.nodes
        .find((node) => node.id === "workstation:router")
        ?.data.connectionAnchors.map((anchor) => anchor.id) ?? [];
    const modelWorkstationAnchorIds =
      projection.nodes
        .find((node) => node.id === "workstation:draft")
        ?.data.connectionAnchors.map((anchor) => anchor.id) ?? [];

    expect(logicalMoveAnchorIds).not.toContain("worker-assignment-target");
    expect(modelWorkstationAnchorIds).toContain("worker-assignment-target");
  });

  it("exposes continue and reject handles when stopWords is configured", () => {
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
    const anchorIds =
      projection.nodes
        .find((node) => node.id === "workstation:draft")
        ?.data.connectionAnchors.map((anchor) => anchor.id) ?? [];

    expect(anchorIds).toContain("workstation-on-continue-source");
    expect(anchorIds).toContain("workstation-on-rejection-source");
  });
});
