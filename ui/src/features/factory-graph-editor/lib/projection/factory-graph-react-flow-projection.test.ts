// biome-ignore lint/style/noExcessiveLinesPerFile: projection contract scenarios stay together around one adapter.
import { describe, expect, it, vi } from "vitest";
import { WorkerType, WorkstationType } from "../../../../api/generated/openapi";
import { baseFactoryDefinition } from "../draft/factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphTopology,
} from "../draft/factory-graph-draft-types";
import { createEmptyFactoryGraphDraft } from "../draft/factory-graph-draft-types";
import { createFactoryGraphWorkstationResolver } from "../editor/factory-graph-editor-connections";
import {
  addFactoryLayoutEdgeWaypoint,
  moveFactoryLayoutEdgeWaypoint,
  removeFactoryLayoutEdgeWaypoint,
  setFactoryLayoutEdgeWaypoints,
} from "../layout/factory-graph-layout-edge-waypoints";
import { createDefaultFactoryLayout } from "../layout/factory-graph-layout-operations";
import {
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
  SYSTEM_TIME_WORK_TYPE_ID,
  systemTimeGraphNodeId,
} from "../operations/factory-graph-customer-display";
import { applyFactoryGraphPendingEdits } from "../operations/factory-graph-operations";
import { projectFactoryGraphToReactFlow } from "../projection/factory-graph-react-flow-projection";
import {
  decorateProjectedEdgesWithWaypoints,
  factoryGraphReactFlowEdgeIdentity,
} from "./factory-graph-react-flow-edge-waypoint-projection";

const FACTORY_GRAPH_EDITOR_EDGE_HOVER_CLASS =
  "agent-factory-editor-edge--hoverable";

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
    expect(
      projection.edges.some((edge) =>
        edge.className?.includes(FACTORY_GRAPH_EDITOR_EDGE_HOVER_CLASS),
      ),
    ).toBe(true);
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

  it("projects bundled source files as factory graph nodes", () => {
    const factoryDefinition = {
      ...baseFactoryDefinition,
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Guide" },
            targetPath: "factory/docs/guide.md",
            type: "DOC",
          },
          {
            content: { encoding: "utf-8", inline: "print('setup')" },
            targetPath: "factory/scripts/setup-workspace.py",
            type: "SCRIPT",
          },
        ],
      },
    } satisfies CanonicalFactoryDefinition;
    const topology = buildFactoryGraphTopologyFromDefinition(factoryDefinition);

    const projection = projectFactoryGraphToReactFlow({
      factoryDefinition,
      topology,
    });

    expect(projection.nodes).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          data: expect.objectContaining({
            connectionAnchors: [],
            kind: "doc",
            kindLabel: "Doc",
            label: "factory/docs/guide.md",
          }),
          id: "doc:factory/docs/guide.md",
          type: "factoryEntity",
        }),
        expect.objectContaining({
          data: expect.objectContaining({
            connectionAnchors: [],
            kind: "doc",
            label: "factory/scripts/setup-workspace.py",
          }),
          id: "doc:factory/scripts/setup-workspace.py",
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
        systemTimeGraphNodeId("workstation", "route-story", "Route story"),
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

  it("projects poller workstation behavior into editor semantic icon metadata", () => {
    const factoryWithPoller = {
      ...baseFactoryDefinition,
      workers: [
        {
          name: "linear-poller",
          provider: "LINEAR",
          type: "HOSTED_WORKER",
        },
      ],
      workstations: [
        {
          behavior: "POLLER",
          id: "linear-poller",
          name: "linear-poller",
          outputs: [{ state: "scheduled", workType: "story" }],
          worker: "linear-poller",
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const topology = buildFactoryGraphTopologyFromDefinition(factoryWithPoller);

    const projection = projectFactoryGraphToReactFlow({
      topology,
      workstationResolver: createFactoryGraphWorkstationResolver(
        factoryWithPoller.workstations,
        factoryWithPoller.workers,
      ),
    });
    const pollerNode = projection.nodes.find(
      (node) => node.id === "workstation:linear-poller",
    );

    expect(pollerNode?.data).toMatchObject({
      workstationSemanticBorderClassName: "border-dotted",
      workstationSemanticIconKind: "poller",
      workstationSemanticLabel: "Poller workstation",
    });
  });
});

const WAYPOINT_EDGE_ID =
  "workstation-output:workstation:draft->work-state:story:done";

function projectedEdgeIdentities(
  edges: ReturnType<typeof projectFactoryGraphToReactFlow>["edges"],
) {
  return new Map(
    edges.map((edge) => [edge.id, factoryGraphReactFlowEdgeIdentity(edge)]),
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: waypoint projection semantics stay grouped around one adapter.
describe("factory graph React Flow projection waypoint semantics", () => {
  it("keeps canonical edge identity when decorating projected edges with authored waypoints", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const baseline = projectFactoryGraphToReactFlow({ topology });
    const layout = setFactoryLayoutEdgeWaypoints(
      createDefaultFactoryLayout(),
      WAYPOINT_EDGE_ID,
      [
        { x: 120, y: 80 },
        { x: 180, y: 140 },
      ],
    );

    const decorated = decorateProjectedEdgesWithWaypoints({
      edges: baseline.edges,
      layout,
      selectedWaypointEdgeId: WAYPOINT_EDGE_ID,
    });

    const baselineIdentities = projectedEdgeIdentities(baseline.edges);
    for (const edge of decorated) {
      expect(factoryGraphReactFlowEdgeIdentity(edge)).toEqual(
        baselineIdentities.get(edge.id),
      );
    }

    const decoratedTarget = decorated.find(
      (edge) => edge.id === WAYPOINT_EDGE_ID,
    );
    expect(decoratedTarget?.data?.waypoints).toEqual([
      { x: 120, y: 80 },
      { x: 180, y: 140 },
    ]);
    expect(
      decorated
        .filter((edge) => edge.id !== WAYPOINT_EDGE_ID)
        .every((edge) => edge.data?.waypoints === undefined),
    ).toBe(true);
  });

  it("keeps generated-route and authored-route edges on the same canonical identity", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const generated = projectFactoryGraphToReactFlow({ topology });
    const authored = decorateProjectedEdgesWithWaypoints({
      edges: generated.edges,
      layout: setFactoryLayoutEdgeWaypoints(
        createDefaultFactoryLayout(),
        WAYPOINT_EDGE_ID,
        [{ x: 90, y: 120 }],
      ),
    });

    expect(projectedEdgeIdentities(authored)).toEqual(
      projectedEdgeIdentities(generated.edges),
    );
  });

  it("keeps projected handles compatible with rendered connection anchors after waypoint layout", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const projection = projectFactoryGraphToReactFlow({
      filterEdgesToRenderedHandles: true,
      topology,
    });
    const nodesById = new Map(projection.nodes.map((node) => [node.id, node]));
    const decorated = decorateProjectedEdgesWithWaypoints({
      edges: projection.edges,
      layout: setFactoryLayoutEdgeWaypoints(
        createDefaultFactoryLayout(),
        WAYPOINT_EDGE_ID,
        [{ x: 90, y: 120 }],
      ),
    });

    for (const edge of decorated) {
      if (!edge.sourceHandle || !edge.targetHandle) {
        continue;
      }

      const sourceAnchorIds = new Set(
        nodesById
          .get(edge.source)
          ?.data.connectionAnchors.map((anchor) => anchor.id) ?? [],
      );
      const targetAnchorIds = new Set(
        nodesById
          .get(edge.target)
          ?.data.connectionAnchors.map((anchor) => anchor.id) ?? [],
      );

      expect(sourceAnchorIds.has(edge.sourceHandle)).toBe(true);
      expect(targetAnchorIds.has(edge.targetHandle)).toBe(true);
    }
  });

  it("returns edges unchanged when waypoint decoration receives edges without data", () => {
    const edgeWithoutData = {
      id: WAYPOINT_EDGE_ID,
      source: "workstation:draft",
      target: "work-state:story:done",
    };

    const decorated = decorateProjectedEdgesWithWaypoints({
      edges: [edgeWithoutData as typeof edgeWithoutData & { data?: undefined }],
      layout: createDefaultFactoryLayout(),
    });

    expect(decorated).toEqual([edgeWithoutData]);
  });

  it("decorates rendered edges through their canonical factory graph edge id", () => {
    const renderedEdge = {
      id: "workstation-resource:resource:story->workstation:draft",
      source: "resource:story",
      target: "workstation:draft",
      data: {
        factoryGraphEdgeId:
          "workstation-input:work-state:story:queued->workstation:draft",
      },
    };
    const layout = setFactoryLayoutEdgeWaypoints(
      createDefaultFactoryLayout(),
      "workstation-input:work-state:story:queued->workstation:draft",
      [{ x: 200, y: 120 }],
    );

    const decorated = decorateProjectedEdgesWithWaypoints({
      edges: [renderedEdge as never],
      layout,
    });

    expect(decorated[0]?.type).toBe("factoryEditorEdge");
    expect(decorated[0]?.data?.waypoints).toEqual([{ x: 200, y: 120 }]);
  });

  it("leaves graph topology unchanged through add, move, and remove waypoint layout operations", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const originalTopology = structuredClone(topology);
    const baseline = projectFactoryGraphToReactFlow({ topology });

    let layout = addFactoryLayoutEdgeWaypoint(
      createDefaultFactoryLayout(),
      WAYPOINT_EDGE_ID,
      { x: 10, y: 20 },
    );
    layout = addFactoryLayoutEdgeWaypoint(layout, WAYPOINT_EDGE_ID, {
      x: 30,
      y: 40,
    });
    layout = moveFactoryLayoutEdgeWaypoint(layout, WAYPOINT_EDGE_ID, 1, {
      x: 50,
      y: 60,
    });
    layout = removeFactoryLayoutEdgeWaypoint(layout, WAYPOINT_EDGE_ID, 0);

    decorateProjectedEdgesWithWaypoints({
      edges: baseline.edges,
      layout,
    });

    expect(topology).toEqual(originalTopology);
    expect(topology.edges.map((edge) => edge.id)).toEqual(
      originalTopology.edges.map((edge) => edge.id),
    );
    for (const edge of topology.edges) {
      const original = originalTopology.edges.find(
        (candidate) => candidate.id === edge.id,
      );
      expect(edge).toEqual(original);
    }
  });

  it("leaves saved topology output unchanged after layout-only waypoint edits", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const pendingLayout = setFactoryLayoutEdgeWaypoints(
      createDefaultFactoryLayout(),
      WAYPOINT_EDGE_ID,
      [{ x: 200, y: 300 }],
    );
    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      pendingLayout,
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    const savedTopology = buildFactoryGraphTopologyFromDefinition(
      saveInput.value,
    );

    expect(savedTopology.edges.map((edge) => edge.id)).toEqual(
      topology.edges.map((edge) => edge.id),
    );
    for (const edge of savedTopology.edges) {
      const original = topology.edges.find(
        (candidate) => candidate.id === edge.id,
      );
      expect(edge).toMatchObject({
        id: original?.id,
        kind: original?.kind,
        sourceId: original?.sourceId,
        targetId: original?.targetId,
      });
      expect(edge).not.toHaveProperty("waypoints");
    }
    expect(saveInput.value.layout?.edges).toEqual([
      {
        id: WAYPOINT_EDGE_ID,
        waypoints: [{ x: 200, y: 300 }],
      },
    ]);
    for (const [index, workstation] of (
      baseFactoryDefinition.workstations ?? []
    ).entries()) {
      expect(saveInput.value.workstations?.[index]).toMatchObject(workstation);
    }
  });

  it("preserves graph node identity for legacy and new taxonomy factories", () => {
    const legacyFactory: CanonicalFactoryDefinition = {
      name: "Legacy Factory",
      workers: [
        {
          model: "gpt-5",
          name: "writer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          body: "Draft the story.",
          inputs: [{ state: "queued", workType: "story" }],
          name: "draft",
          outputs: [{ state: "done", workType: "story" }],
          type: "MODEL_WORKSTATION",
          worker: "writer",
        },
      ],
    };
    const legacyWorkstation = legacyFactory.workstations?.[0];
    if (!legacyWorkstation) {
      throw new Error("expected legacy workstation fixture");
    }

    const taxonomyFactory: CanonicalFactoryDefinition = {
      ...legacyFactory,
      workers: [
        {
          model: "gpt-5",
          name: "writer",
          type: WorkerType.INFERENCE_WORKER,
        },
      ],
      workstations: [
        {
          ...legacyWorkstation,
          type: WorkstationType.AGENT_RUN,
        },
      ],
    };

    const projectNodeIds = (factory: CanonicalFactoryDefinition) => {
      const topology = buildFactoryGraphTopologyFromDefinition(factory);
      return projectFactoryGraphToReactFlow({ topology }).nodes.map(
        (node) => node.id,
      );
    };

    expect(projectNodeIds(legacyFactory)).toEqual(
      projectNodeIds(taxonomyFactory),
    );
    expect(projectNodeIds(legacyFactory)).toEqual([
      "worker:writer",
      "workstation:draft",
      "work-type:story",
      "work-state:story:done",
      "work-state:story:queued",
    ]);
  });
});
