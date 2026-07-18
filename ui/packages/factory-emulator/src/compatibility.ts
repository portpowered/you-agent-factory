import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import type { FactoryEventSink } from "./event-sink.js";

export type FactoryEmulatorCompatibilityIssueCode =
  | "invalid_logical_move"
  | "invalid_route_reference"
  | "invalid_worker_reference"
  | "provider_global_resource"
  | "unresolved_resource_requirement"
  | "unsupported_classifier"
  | "unsupported_execution"
  | "unsupported_guard"
  | "unsupported_orchestrator"
  | "unsupported_relationship_behavior"
  | "unsupported_workstation_behavior";

export interface FactoryEmulatorCompatibilityIssue {
  readonly code: FactoryEmulatorCompatibilityIssueCode;
  readonly path: readonly (string | number)[];
  readonly message: string;
}

export type FactoryEmulatorCompatibilityResult =
  | { readonly supported: true; readonly diagnostics: readonly [] }
  | {
      readonly supported: false;
      readonly diagnostics: readonly FactoryEmulatorCompatibilityIssue[];
    };

type IssuePath = readonly (string | number)[];

function addIssue(
  diagnostics: FactoryEmulatorCompatibilityIssue[],
  code: FactoryEmulatorCompatibilityIssueCode,
  path: IssuePath,
  message: string,
): void {
  diagnostics.push({ code, path, message });
}

function inspectRoutes(
  factory: FactoryDefinition,
  workstationIndex: number,
  diagnostics: FactoryEmulatorCompatibilityIssue[],
): void {
  const workstation = factory.workstations?.[workstationIndex];
  if (workstation === undefined) return;
  const workStates = new Map(
    (factory.workTypes ?? []).map((workType) => [
      workType.name,
      new Set(workType.states.map((state) => state.name)),
    ]),
  );
  const routeGroups = [
    ["inputs", workstation.inputs],
    ["outputs", workstation.outputs ?? []],
    ["onContinue", workstation.onContinue ?? []],
    ["onRejection", workstation.onRejection ?? []],
    ["onFailure", workstation.onFailure ?? []],
  ] as const;

  for (const [routeName, routes] of routeGroups) {
    for (const [routeIndex, route] of routes.entries()) {
      const path = [
        "workstations",
        workstationIndex,
        routeName,
        routeIndex,
      ] as const;
      const states = workStates.get(route.workType);
      if (states === undefined) {
        addIssue(
          diagnostics,
          "invalid_route_reference",
          [...path, "workType"],
          `Work type ${route.workType} is not declared by the Factory.`,
        );
      } else if (!states.has(route.state)) {
        addIssue(
          diagnostics,
          "invalid_route_reference",
          [...path, "state"],
          `State ${route.state} is not declared by Work type ${route.workType}.`,
        );
      }
      for (const [guardIndex, guard] of (route.guards ?? []).entries()) {
        const guardPath = [...path, "guards", guardIndex] as const;
        const relationshipAware =
          guard.parentInput !== undefined ||
          guard.matchInput !== undefined ||
          guard.spawnedBy !== undefined ||
          guard.type !== "VISIT_COUNT";
        addIssue(
          diagnostics,
          relationshipAware
            ? "unsupported_relationship_behavior"
            : "unsupported_guard",
          guardPath,
          relationshipAware
            ? "Parent-aware, peer-aware, and runtime-spawn-aware input behavior is not supported by emulator v1."
            : "Input guards are not supported by emulator v1.",
        );
      }
    }
  }
}

function inspectWorkstation(
  factory: FactoryDefinition,
  workstationIndex: number,
  diagnostics: FactoryEmulatorCompatibilityIssue[],
): void {
  const workstation = factory.workstations?.[workstationIndex];
  if (workstation === undefined) return;
  const path = ["workstations", workstationIndex] as const;
  const behavior = workstation.behavior ?? "STANDARD";
  const type = workstation.type;

  if (behavior !== "STANDARD" && behavior !== "REPEATER") {
    addIssue(
      diagnostics,
      "unsupported_workstation_behavior",
      [...path, "behavior"],
      `Workstation behavior ${behavior} is not supported by emulator v1.`,
    );
  }

  if (
    type === "CLASSIFIER_WORKSTATION" ||
    workstation.classificationRoutes !== undefined ||
    workstation.outcomeFormat !== undefined
  ) {
    addIssue(
      diagnostics,
      "unsupported_classifier",
      [
        ...path,
        type === "CLASSIFIER_WORKSTATION"
          ? "type"
          : workstation.classificationRoutes !== undefined
            ? "classificationRoutes"
            : "outcomeFormat",
      ],
      "Classifier routing and classifier output contracts are not supported by emulator v1.",
    );
  } else if (type !== "AGENT_RUN" && type !== "LOGICAL_MOVE") {
    addIssue(
      diagnostics,
      "unsupported_execution",
      [...path, "type"],
      `Workstation execution ${type ?? "omitted"} is not supported by emulator v1.`,
    );
  }

  const workerNames = new Set(
    (factory.workers ?? []).map((worker) => worker.name),
  );
  if (type === "LOGICAL_MOVE") {
    if (workstation.worker !== "") {
      addIssue(
        diagnostics,
        "invalid_logical_move",
        [...path, "worker"],
        "A LOGICAL_MOVE must be workerless.",
      );
    }
    const guards = workstation.guards ?? [];
    if (guards.length === 0) {
      addIssue(
        diagnostics,
        "invalid_logical_move",
        [...path, "guards"],
        "A LOGICAL_MOVE requires at least one VISIT_COUNT guard.",
      );
    }
    for (const [guardIndex, guard] of guards.entries()) {
      const guardPath = [...path, "guards", guardIndex] as const;
      if (
        guard.type !== "VISIT_COUNT" ||
        guard.parentInput !== undefined ||
        guard.matchInput !== undefined ||
        guard.spawnedBy !== undefined
      ) {
        addIssue(
          diagnostics,
          "unsupported_guard",
          guardPath,
          "LOGICAL_MOVE supports only VISIT_COUNT guards in emulator v1.",
        );
      } else if (
        guard.workstation === undefined ||
        !new Set(factory.workstations?.map((item) => item.name)).has(
          guard.workstation,
        ) ||
        guard.maxVisits === undefined ||
        !Number.isInteger(guard.maxVisits) ||
        guard.maxVisits < 1
      ) {
        addIssue(
          diagnostics,
          "invalid_logical_move",
          guardPath,
          "VISIT_COUNT requires a declared workstation and a positive integer maxVisits.",
        );
      }
    }
  } else if (type === "AGENT_RUN") {
    if (workstation.worker === "" || !workerNames.has(workstation.worker)) {
      addIssue(
        diagnostics,
        "invalid_worker_reference",
        [...path, "worker"],
        `Worker ${workstation.worker || "<empty>"} is not declared by the Factory.`,
      );
    } else {
      const worker = factory.workers?.find(
        (candidate) => candidate.name === workstation.worker,
      );
      if (worker?.type !== undefined && worker.type !== "AGENT_WORKER") {
        addIssue(
          diagnostics,
          "unsupported_execution",
          ["workers", factory.workers?.indexOf(worker) ?? -1, "type"],
          `Worker execution ${worker.type} is not supported for AGENT_RUN emulator workstations.`,
        );
      }
    }
    for (const [guardIndex] of (workstation.guards ?? []).entries()) {
      addIssue(
        diagnostics,
        "unsupported_guard",
        [...path, "guards", guardIndex],
        "AGENT_RUN workstation guards are not supported by emulator v1.",
      );
    }
  } else {
    for (const [guardIndex] of (workstation.guards ?? []).entries()) {
      addIssue(
        diagnostics,
        "unsupported_guard",
        [...path, "guards", guardIndex],
        "Workstation guards on unsupported execution types are not supported by emulator v1.",
      );
    }
  }

  const declaredResources = new Set(
    (factory.resources ?? []).map((resource) => resource.name),
  );
  for (const [resourceIndex, requirement] of (
    workstation.resources ?? []
  ).entries()) {
    if (!declaredResources.has(requirement.name)) {
      addIssue(
        diagnostics,
        "unresolved_resource_requirement",
        [...path, "resources", resourceIndex, "name"],
        `Resource ${requirement.name} is not declared at Factory scope.`,
      );
    }
  }

  inspectRoutes(factory, workstationIndex, diagnostics);
}

/** Inspect the executable subset without mutating or retaining the Factory. */
export function inspectFactoryEmulatorCompatibility(
  factory: FactoryDefinition,
): FactoryEmulatorCompatibilityResult {
  const diagnostics: FactoryEmulatorCompatibilityIssue[] = [];
  if (
    factory.orchestrator !== undefined &&
    factory.orchestrator.kind !== "PETRI"
  ) {
    addIssue(
      diagnostics,
      "unsupported_orchestrator",
      ["orchestrator", "kind"],
      `Orchestrator ${factory.orchestrator.kind} is not supported by emulator v1.`,
    );
  }
  for (const [guardIndex] of (factory.guards ?? []).entries()) {
    addIssue(
      diagnostics,
      "unsupported_guard",
      ["guards", guardIndex],
      "Factory-level guards are not supported by emulator v1.",
    );
  }
  for (const [resourceIndex, resource] of (factory.resources ?? []).entries()) {
    if (resource.type === "PROVIDER_QUOTA" || resource.provider !== undefined) {
      addIssue(
        diagnostics,
        "provider_global_resource",
        ["resources", resourceIndex],
        `Resource ${resource.name} depends on provider-global capacity.`,
      );
    }
  }
  for (const workstationIndex of (factory.workstations ?? []).keys()) {
    inspectWorkstation(factory, workstationIndex, diagnostics);
  }
  return diagnostics.length === 0
    ? { supported: true, diagnostics: [] }
    : { supported: false, diagnostics };
}

/** Write only after the complete compatibility preflight succeeds. */
export async function writeFactoryEventsIfCompatible(
  factory: FactoryDefinition,
  events: readonly FactoryEvent[],
  sink: FactoryEventSink,
): Promise<FactoryEmulatorCompatibilityResult> {
  const result = inspectFactoryEmulatorCompatibility(factory);
  if (!result.supported) return result;
  await sink.write(events);
  return result;
}
