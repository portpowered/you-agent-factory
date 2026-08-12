import {
  type FactoryDefinition,
  WorkstationKind,
  WorkstationType,
} from "@you-agent-factory/client";
import { describe, expect, test } from "vitest";
import { factoryGraphWorkstationPresentation } from "./semantic-workstation-presentation";
import type { FactoryGraphSource } from "./source";
import {
  factoryGraphWorkstationRuntimeRole,
  projectFactoryGraphWorkstationSemantics,
  resolveFactoryGraphWorkstationRuntimeType,
  resolveFactoryGraphWorkstationSchedulingBehavior,
  resolveFactoryGraphWorkstationSemantics,
} from "./workstation-semantics";

type FactoryWorkstation = NonNullable<
  FactoryDefinition["workstations"]
>[number];

const RUNTIME_TYPES = Object.values(WorkstationType);
const SCHEDULING_BEHAVIORS = Object.values(WorkstationKind);

function workstation(
  overrides: Partial<FactoryWorkstation> = {},
): FactoryWorkstation {
  return {
    inputs: [],
    name: "process",
    worker: "worker",
    ...overrides,
  };
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: semantic axis matrix and guard cases stay grouped around one projection fixture.
describe("Factory workstation axis resolution", () => {
  test.each(
    RUNTIME_TYPES.flatMap((runtimeType) =>
      SCHEDULING_BEHAVIORS.map((behavior) => [runtimeType, behavior] as const),
    ),
  )(
    "keeps runtime type %s independent from scheduling behavior %s",
    (runtimeType, behavior) => {
      const semantics = resolveFactoryGraphWorkstationSemantics(
        workstation({ behavior, type: runtimeType }),
      );

      expect(semantics.runtimeType).toBe(runtimeType);
      expect(semantics.schedulingBehavior).toBe(behavior);
    },
  );

  test.each([
    [WorkstationType.INFERENCE_RUN, "INFERENCE"],
    [WorkstationType.AGENT_RUN, "AGENT"],
    [WorkstationType.SCRIPT_RUN, "SCRIPT"],
    [WorkstationType.POLLER_RUN, "POLLER"],
    [WorkstationType.MODEL_WORKSTATION, "AGENT"],
    [WorkstationType.MODEL_INVOKE, "INFERENCE"],
    [WorkstationType.LOGICAL_MOVE, "LOGICAL_MOVE"],
    [WorkstationType.CLASSIFIER_WORKSTATION, "CLASSIFIER"],
    [WorkstationType.HUMAN_APPROVAL, "HUMAN_APPROVAL"],
  ] as const)("maps canonical runtime type %s to role %s", (type, role) => {
    expect(factoryGraphWorkstationRuntimeRole(type)).toBe(role);
  });

  test("normalizes lowercase legacy scheduling values without changing runtime type", () => {
    const legacyBehavior = workstation({
      behavior: "poller" as FactoryWorkstation["behavior"],
      type: "model_workstation" as FactoryWorkstation["type"],
    });

    expect(resolveFactoryGraphWorkstationRuntimeType(legacyBehavior)).toBe(
      WorkstationType.MODEL_WORKSTATION,
    );
    expect(
      resolveFactoryGraphWorkstationSchedulingBehavior(legacyBehavior),
    ).toBe(WorkstationKind.POLLER);
    expect(
      resolveFactoryGraphWorkstationSemantics(legacyBehavior),
    ).toMatchObject({
      runtimeRole: "AGENT",
      runtimeType: WorkstationType.MODEL_WORKSTATION,
      schedulingBehavior: WorkstationKind.POLLER,
    });
  });

  test("keeps logical routers and classifiers distinct from loop breakers", () => {
    const logicalRouter = resolveFactoryGraphWorkstationSemantics(
      workstation({ type: WorkstationType.LOGICAL_MOVE }),
    );
    const loopBreaker = resolveFactoryGraphWorkstationSemantics(
      workstation({
        guards: [
          {
            maxVisits: 3,
            type: "visit_count" as "VISIT_COUNT",
            workstation: "review",
          },
        ],
        type: WorkstationType.LOGICAL_MOVE,
      }),
    );
    const classifier = resolveFactoryGraphWorkstationSemantics(
      workstation({ type: WorkstationType.CLASSIFIER_WORKSTATION }),
    );

    expect(logicalRouter.controlRole).toBe("LOGICAL_ROUTER");
    expect(loopBreaker.controlRole).toBe("LOOP_BREAKER");
    expect(loopBreaker.guardedControl).toEqual({
      guardType: "VISIT_COUNT",
      limit: { fixed: 3 },
      targetWorkstation: "review",
    });
    expect(classifier.controlRole).toBe("CLASSIFIER");
    expect(
      factoryGraphWorkstationRuntimeRole(WorkstationType.LOGICAL_MOVE),
    ).toBe("LOGICAL_MOVE");
  });

  test("does not treat an incomplete VISIT_COUNT guard as a loop breaker", () => {
    expect(
      resolveFactoryGraphWorkstationSemantics(
        workstation({
          guards: [
            {
              type: "VISIT_COUNT",
              workstation: "review",
            },
          ],
          type: WorkstationType.LOGICAL_MOVE,
        }),
      ).controlRole,
    ).toBe("LOGICAL_ROUTER");
  });

  test("projects human approval as a workerless control role", () => {
    const semantics = resolveFactoryGraphWorkstationSemantics(
      workstation({ type: WorkstationType.HUMAN_APPROVAL, worker: undefined }),
    );

    expect(semantics).toMatchObject({
      controlRole: "HUMAN_APPROVAL",
      runtimeRole: "HUMAN_APPROVAL",
      runtimeType: WorkstationType.HUMAN_APPROVAL,
    });
    expect(factoryGraphWorkstationPresentation(semantics, "en")).toMatchObject({
      controlRole: "HUMAN_APPROVAL",
      iconKind: "constraint",
      label: "Human approval workstation",
    });
  });

  test("exposes an authored argument-backed visit limit", () => {
    expect(
      resolveFactoryGraphWorkstationSemantics(
        workstation({
          guards: [
            {
              maxVisitsArgument: "max_review_visits",
              type: "VISIT_COUNT",
              workstation: "review",
            },
          ],
          type: WorkstationType.LOGICAL_MOVE,
        }),
      ),
    ).toMatchObject({
      controlRole: "LOOP_BREAKER",
      guardedControl: {
        guardType: "VISIT_COUNT",
        limit: { argument: "max_review_visits" },
        targetWorkstation: "review",
      },
    });
  });

  test("uses a neutral unknown result for missing or future semantic metadata", () => {
    expect(resolveFactoryGraphWorkstationSemantics(undefined)).toEqual({
      controlRole: "UNKNOWN",
      runtimeRole: "UNKNOWN",
      runtimeType: "UNKNOWN",
      schedulingBehavior: "UNKNOWN",
    });

    expect(
      resolveFactoryGraphWorkstationSemantics(
        workstation({
          behavior: "future-schedule" as FactoryWorkstation["behavior"],
          type: "future-runtime" as FactoryWorkstation["type"],
        }),
      ),
    ).toEqual({
      controlRole: "UNKNOWN",
      runtimeRole: "UNKNOWN",
      runtimeType: "UNKNOWN",
      schedulingBehavior: "UNKNOWN",
    });
  });
});

describe("Factory workstation presentation", () => {
  test("keeps runtime presentation independent from scheduling presentation", () => {
    const presentation = factoryGraphWorkstationPresentation(
      resolveFactoryGraphWorkstationSemantics(
        workstation({
          behavior: WorkstationKind.REPEATER,
          type: WorkstationType.INFERENCE_RUN,
        }),
      ),
    );

    expect(presentation).toMatchObject({
      borderClassName: "border-double",
      iconKind: "processing",
      runtimeType: WorkstationType.INFERENCE_RUN,
      schedulingLabel: "Repeater schedule",
      schedulingBehavior: WorkstationKind.REPEATER,
    });

    expect(factoryGraphWorkstationPresentation()).toMatchObject({
      iconKind: "workstation",
      label: "Unknown workstation semantics",
      runtimeType: "UNKNOWN",
      schedulingBehavior: "UNKNOWN",
    });
    expect(factoryGraphWorkstationPresentation()).not.toHaveProperty(
      "semanticKind",
    );
  });
});

describe("Factory workstation identity projection", () => {
  test("joins activity by durable id and uses names only for id-less legacy workstations", () => {
    const source = graphSource({
      factoryWorkstations: [
        workstation({
          behavior: WorkstationKind.REPEATER,
          id: "stable-workstation",
          name: "Renamed process",
          type: WorkstationType.AGENT_RUN,
        }),
        workstation({
          behavior: "cron" as FactoryWorkstation["behavior"],
          name: "legacy-process",
          type: WorkstationType.SCRIPT_RUN,
        }),
      ],
      topologyNodes: [
        workstationNode("stable-workstation", "Renamed process"),
        workstationNode("legacy-process", "legacy-process"),
      ],
    });
    source.runtime.activity.activeDispatchOverlays = [
      {
        connectionIds: [],
        dispatchId: "dispatch-1",
        id: "dispatch:dispatch-1",
        evidence: {
          resources: "unavailable",
          route: "unavailable",
          work: "unavailable",
          worker: "unavailable",
          workstation: "known",
        },
        startedTick: 4,
        workstationId: "stable-workstation",
        workstationNodeId: "workstation:stable-workstation",
      },
    ];

    expect(projectFactoryGraphWorkstationSemantics(source)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          activity: { active: true, activeDispatchIds: ["dispatch-1"] },
          authoredWorkstationId: "stable-workstation",
          controlRole: "NONE",
          nodeId: "workstation:stable-workstation",
          runtimeType: WorkstationType.AGENT_RUN,
          schedulingBehavior: WorkstationKind.REPEATER,
          workstationId: "stable-workstation",
        }),
        expect.objectContaining({
          authoredWorkstationId: "legacy-process",
          nodeId: "workstation:legacy-process",
          runtimeRole: "SCRIPT",
          runtimeType: WorkstationType.SCRIPT_RUN,
          schedulingBehavior: WorkstationKind.CRON,
        }),
      ]),
    );
  });

  test("keeps omitted scheduling behavior neutral instead of defaulting to standard", () => {
    expect(
      resolveFactoryGraphWorkstationSemantics(
        workstation({ type: WorkstationType.AGENT_RUN }),
      ),
    ).toEqual({
      controlRole: "NONE",
      runtimeRole: "AGENT",
      runtimeType: WorkstationType.AGENT_RUN,
      schedulingBehavior: "UNKNOWN",
    });
  });

  test("keeps known axes when only the other axis is future metadata", () => {
    expect(
      resolveFactoryGraphWorkstationSemantics(
        workstation({
          behavior: WorkstationKind.REPEATER,
          type: "future-runtime" as FactoryWorkstation["type"],
        }),
      ),
    ).toEqual({
      controlRole: "UNKNOWN",
      runtimeRole: "UNKNOWN",
      runtimeType: "UNKNOWN",
      schedulingBehavior: WorkstationKind.REPEATER,
    });

    expect(
      resolveFactoryGraphWorkstationSemantics(
        workstation({
          behavior: "future-schedule" as FactoryWorkstation["behavior"],
          type: WorkstationType.SCRIPT_RUN,
        }),
      ),
    ).toEqual({
      controlRole: "NONE",
      runtimeRole: "SCRIPT",
      runtimeType: WorkstationType.SCRIPT_RUN,
      schedulingBehavior: "UNKNOWN",
    });
  });

  test("does not fall back to an authored name when an id-backed node cannot be joined", () => {
    const source = graphSource({
      factoryWorkstations: [
        workstation({
          id: "stable-workstation",
          name: "Renamed process",
          type: WorkstationType.AGENT_RUN,
        }),
      ],
      topologyNodes: [workstationNode("old-name", "Renamed process")],
    });

    expect(projectFactoryGraphWorkstationSemantics(source)[0]).toMatchObject({
      controlRole: "UNKNOWN",
      runtimeRole: "UNKNOWN",
      runtimeType: "UNKNOWN",
      schedulingBehavior: "UNKNOWN",
    });
    expect(
      projectFactoryGraphWorkstationSemantics(source)[0],
    ).not.toHaveProperty("authoredWorkstationId");
  });
});

function workstationNode(entityId: string, label: string) {
  return {
    entityId,
    handles: [],
    id: `workstation:${entityId}`,
    kind: "workstation" as const,
    label,
  };
}

function graphSource(input: {
  factoryWorkstations: FactoryWorkstation[];
  topologyNodes: ReturnType<typeof workstationNode>[];
}): FactoryGraphSource {
  return {
    factory: {
      name: "semantic-test",
      workstations: input.factoryWorkstations,
    },
    runtime: {
      activity: {
        activeDispatchOverlays: [],
        activeWorkstationNodeIds: [],
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
      },
      load: {
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
        workStateCounts: [],
      },
      topology: {
        connections: [],
        issues: [],
        nodes: input.topologyNodes,
        ok: true,
        selectedTick: 4,
      },
    },
    selectedTick: 4,
  };
}
