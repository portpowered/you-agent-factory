// biome-ignore lint/nursery/noExcessiveLinesPerFile: projection contract scenarios stay together around one adapter.
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
import { createFactoryGraphWorkstationResolver } from "./factory-graph-editor-connections";
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

  it("projects renamed work-state node ids and labels from an updated factory definition", () => {
    const renamedFactoryDefinition = {
      ...baseFactoryDefinition,
      workTypes: [
        {
          name: "story",
          states: [
            { name: "ready", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          ...baseFactoryDefinition.workstations?.[0],
          inputs: [{ state: "ready", workType: "story" }],
          name: "draft",
          outputs: [{ state: "done", workType: "story" }],
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const topology = buildFactoryGraphTopologyFromDefinition(
      renamedFactoryDefinition,
    );

    const projection = projectFactoryGraphToReactFlow({
      factoryDefinition: renamedFactoryDefinition,
      topology,
    });

    expect(
      projection.nodes.find((node) => node.id === "work-state:story:ready"),
    ).toMatchObject({
      data: {
        kind: "work-state",
        label: "story:ready",
      },
      id: "work-state:story:ready",
    });
    expect(
      projection.nodes.some((node) => node.id === "work-state:story:queued"),
    ).toBe(false);
    expect(projection.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-input:work-state:story:ready->workstation:draft",
        }),
      ]),
    );
  });

  it("projects workStateType from factory definition for all lifecycle phases", () => {
    const lifecycleFactoryDefinition = {
      ...baseFactoryDefinition,
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "done", type: "TERMINAL" },
            { name: "failed", type: "FAILED" },
          ],
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const topology = buildFactoryGraphTopologyFromDefinition(
      lifecycleFactoryDefinition,
    );

    const projection = projectFactoryGraphToReactFlow({
      factoryDefinition: lifecycleFactoryDefinition,
      topology,
    });

    expect(
      projection.nodes.find((node) => node.id === "work-state:story:queued")
        ?.data.workStateType,
    ).toBe("INITIAL");
    expect(
      projection.nodes.find((node) => node.id === "work-state:story:review")
        ?.data.workStateType,
    ).toBe("PROCESSING");
    expect(
      projection.nodes.find((node) => node.id === "work-state:story:done")?.data
        .workStateType,
    ).toBe("TERMINAL");
    expect(
      projection.nodes.find((node) => node.id === "work-state:story:failed")
        ?.data.workStateType,
    ).toBe("FAILED");
    expect(
      projection.nodes.find((node) => node.id === "work-type:story")?.data,
    ).not.toHaveProperty("workStateType");
    expect(
      projection.nodes.find((node) => node.id === "worker:writer")?.data,
    ).not.toHaveProperty("workStateType");
  });

  it("marks default work-type nodes from the canonical factory definition", () => {
    const factoryDefinition = {
      ...baseFactoryDefinition,
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const topology = buildFactoryGraphTopologyFromDefinition(factoryDefinition);

    const projection = projectFactoryGraphToReactFlow({
      factoryDefinition,
      topology,
    });

    expect(
      projection.nodes.find((node) => node.id === "work-type:story")?.data,
    ).toMatchObject({
      defaultWorkTypeLabel: "Default work type",
      isDefaultWorkType: true,
    });
  });

  it("omits workStateType when factory definition is not provided", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );

    const projection = projectFactoryGraphToReactFlow({
      topology,
    });

    for (const node of projection.nodes.filter(
      (entry) => entry.data.kind === "work-state",
    )) {
      expect(node.data.workStateType).toBeUndefined();
    }
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
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "rejected", type: "FAILED" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          ...baseFactoryDefinition.workstations[0],
          behavior: "STANDARD",
          onContinue: [{ state: "queued", workType: "story" }],
          onFailure: [{ state: "rejected", workType: "story" }],
          onRejection: [{ state: "rejected", workType: "story" }],
          stopWords: undefined,
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const topology = buildFactoryGraphTopologyFromDefinition(
      factoryWithoutStopWords,
    );
    const workstationResolver = {
      resolveWorkstation: (name: string) =>
        factoryWithoutStopWords.workstations.find(
          (workstation) => workstation.name === name,
        ),
    };

    const projection = projectFactoryGraphToReactFlow({
      filterEdgesToRenderedHandles: true,
      topology,
      workstationResolver,
    });
    const anchorIds =
      projection.nodes
        .find((node) => node.id === "workstation:draft")
        ?.data.connectionAnchors.map((anchor) => anchor.id) ?? [];
    const edgeKinds = projection.edges.map((edge) => edge.data?.kind);

    expect(anchorIds).toEqual(
      expect.arrayContaining([
        "workstation-output-source",
        "workstation-on-failure-source",
      ]),
    );
    expect(anchorIds).not.toContain("workstation-on-continue-source");
    expect(anchorIds).not.toContain("workstation-on-rejection-source");
    expect(edgeKinds).not.toContain("workstation-on-continue");
    expect(edgeKinds).not.toContain("workstation-on-rejection");
    expect(edgeKinds).toContain("workstation-on-failure");
    expect(edgeKinds).toContain("workstation-output");
  });

  it("keeps progress-outcome edges for observer projections when handle filtering is disabled", () => {
    const factoryWithoutStopWords = {
      ...baseFactoryDefinition,
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "rejected", type: "FAILED" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          ...baseFactoryDefinition.workstations[0],
          behavior: "STANDARD",
          onContinue: [{ state: "queued", workType: "story" }],
          onFailure: [{ state: "rejected", workType: "story" }],
          onRejection: [{ state: "rejected", workType: "story" }],
          stopWords: undefined,
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const topology = buildFactoryGraphTopologyFromDefinition(
      factoryWithoutStopWords,
    );

    const projection = projectFactoryGraphToReactFlow({
      mode: "observer",
      topology,
      workstationResolver: {
        resolveWorkstation: (name) =>
          factoryWithoutStopWords.workstations.find(
            (workstation) => workstation.name === name,
          ),
      },
    });
    const edgeKinds = projection.edges.map((edge) => edge.data?.kind);

    expect(edgeKinds).toContain("workstation-on-continue");
    expect(edgeKinds).toContain("workstation-on-rejection");
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
    expect(logicalMoveAnchorIds).not.toContain(
      "workstation-on-continue-source",
    );
    expect(logicalMoveAnchorIds).not.toContain("workstation-on-failure-source");
    expect(logicalMoveAnchorIds).not.toContain(
      "workstation-on-rejection-source",
    );
    expect(modelWorkstationAnchorIds).toContain(
      "workstation-on-failure-source",
    );
  });

  it("exposes continue and reject handles when stopWords is configured", () => {
    const factoryWithStopWords = {
      ...baseFactoryDefinition,
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "rejected", type: "FAILED" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          ...baseFactoryDefinition.workstations[0],
          behavior: "STANDARD",
          onContinue: [{ state: "queued", workType: "story" }],
          onFailure: [{ state: "rejected", workType: "story" }],
          onRejection: [{ state: "rejected", workType: "story" }],
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
    const edgeKinds = projection.edges.map((edge) => edge.data?.kind);

    expect(anchorIds).toContain("workstation-on-continue-source");
    expect(anchorIds).toContain("workstation-on-rejection-source");
    expect(edgeKinds).toContain("workstation-on-continue");
    expect(edgeKinds).toContain("workstation-on-rejection");
  });

  it("exposes continue and reject handles when the assigned worker has a stop token", () => {
    const factoryWithWorkerStopToken = {
      ...baseFactoryDefinition,
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "rejected", type: "FAILED" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workers: [
        {
          name: "processor",
          stopToken: "<COMPLETE>",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          ...baseFactoryDefinition.workstations[0],
          behavior: "STANDARD",
          onContinue: [{ state: "queued", workType: "story" }],
          onFailure: [{ state: "rejected", workType: "story" }],
          onRejection: [{ state: "rejected", workType: "story" }],
          stopWords: undefined,
          worker: "processor",
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const topology = buildFactoryGraphTopologyFromDefinition(
      factoryWithWorkerStopToken,
    );
    const workstationResolver = createFactoryGraphWorkstationResolver(
      factoryWithWorkerStopToken.workstations,
      factoryWithWorkerStopToken.workers,
    );

    const projection = projectFactoryGraphToReactFlow({
      filterEdgesToRenderedHandles: true,
      topology,
      workstationResolver,
    });
    const anchorIds =
      projection.nodes
        .find((node) => node.id === "workstation:draft")
        ?.data.connectionAnchors.map((anchor) => anchor.id) ?? [];
    const progressOutcomeEdges = projection.edges.filter((edge) =>
      ["workstation-on-continue", "workstation-on-rejection"].includes(
        edge.data?.kind ?? "",
      ),
    );

    expect(anchorIds).toContain("workstation-on-continue-source");
    expect(anchorIds).toContain("workstation-on-rejection-source");
    expect(progressOutcomeEdges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-on-continue:workstation:draft->work-state:story:queued",
          sourceHandle: "workstation-on-continue-source",
          targetHandle: "work-state-input-target",
        }),
        expect.objectContaining({
          id: "workstation-on-rejection:workstation:draft->work-state:story:rejected",
          sourceHandle: "workstation-on-rejection-source",
          targetHandle: "work-state-input-target",
        }),
      ]),
    );
  });

  it("highlights compatible continue targets in connect mode when worker stop token enables progress routes", () => {
    const factoryWithWorkerStopToken = {
      ...baseFactoryDefinition,
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workers: [
        {
          name: "processor",
          stopToken: "<COMPLETE>",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          ...baseFactoryDefinition.workstations[0],
          behavior: "STANDARD",
          stopWords: undefined,
          worker: "processor",
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const topology = buildFactoryGraphTopologyFromDefinition(
      factoryWithWorkerStopToken,
    );

    const projection = projectFactoryGraphToReactFlow({
      editor: {
        canEditConnections: true,
        pendingAdditionEdgeIds: new Set<string>(),
        pendingAdditionNodeIds: new Set<string>(),
        pendingConnectionSource: {
          anchorId: "workstation-on-continue-source",
          nodeId: "workstation:draft",
        },
        pendingRemovalEdgeIds: new Set<string>(),
        pendingRemovalNodeIds: new Set<string>(),
      },
      topology,
      workstationResolver: createFactoryGraphWorkstationResolver(
        factoryWithWorkerStopToken.workstations,
        factoryWithWorkerStopToken.workers,
      ),
    });
    const queuedNode = projection.nodes.find(
      (node) => node.id === "work-state:story:queued",
    );

    expect(
      projection.nodes
        .find((node) => node.id === "workstation:draft")
        ?.data.connectionAnchors.some(
          (anchor) => anchor.id === "workstation-on-continue-source",
        ),
    ).toBe(true);
    expect(
      queuedNode?.data.connectionAnchors.find(
        (anchor) => anchor.id === "work-state-input-target",
      ),
    ).toMatchObject({
      variant: "valid-target",
    });
  });
});
