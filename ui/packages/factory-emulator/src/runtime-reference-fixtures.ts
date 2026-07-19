import type { FactoryDefinition } from "@you-agent-factory/client";
import type { FactoryEmulatorRuntimeReference } from "./runtime-reference.js";
import type { FactoryEmulatorScenario } from "./scenario.js";

const source =
  "ui/packages/factory-emulator/README.md (documented deterministic session behavior)";
const eventKinds = [
  "RUN_REQUEST",
  "INITIAL_STRUCTURE_REQUEST",
  "SESSION_STARTED",
  "WORK_REQUEST",
  "DISPATCH_REQUEST",
  "DISPATCH_RESPONSE",
] as const;

const emptySemantics = {
  dispatchChoices: [],
  consumedWork: [],
  outcomes: [],
  routes: [],
  terminalStates: [],
} as const;

function cloneFactory(factory: FactoryDefinition): FactoryDefinition {
  return JSON.parse(JSON.stringify(factory)) as FactoryDefinition;
}

function firstWorkstation(factory: FactoryDefinition) {
  const workstation = factory.workstations?.[0];
  if (workstation === undefined) {
    throw new Error("Frozen runtime-reference Factory requires a workstation.");
  }
  return workstation;
}

function fixture(
  id: string,
  title: string,
  factory: FactoryEmulatorRuntimeReference["factory"],
  scenario: FactoryEmulatorRuntimeReference["scenario"],
): FactoryEmulatorRuntimeReference {
  const initialSubmissions = Array.isArray(scenario.initialSubmissions)
    ? scenario.initialSubmissions
    : scenario.initialSubmissions !== undefined &&
        "works" in scenario.initialSubmissions
      ? scenario.initialSubmissions.works
      : [];
  const replayProjection = initialSubmissions
    .map(({ name, workType, state }) => `${name}:${workType}:${state}`)
    .sort();
  const terminal = `${initialSubmissions[0]?.workType}:done`;
  const outputCount = firstWorkstation(factory).outputs?.length ?? 0;
  const holdsAtVisitGuard = firstWorkstation(factory).type === "LOGICAL_MOVE";
  const ticks = holdsAtVisitGuard
    ? [
        {
          logicalTick: 0,
          eventKinds: eventKinds.slice(0, 3),
          semantics: { ...emptySemantics, replayProjection: [] },
        },
        {
          logicalTick: 1,
          eventKinds: eventKinds.slice(3, 4),
          semantics: { ...emptySemantics, replayProjection },
        },
      ]
    : [
        {
          logicalTick: 0,
          eventKinds: eventKinds.slice(0, 3),
          semantics: { ...emptySemantics, replayProjection: [] },
        },
        {
          logicalTick: 1,
          eventKinds: eventKinds.slice(3, 4),
          semantics: { ...emptySemantics, replayProjection },
        },
        {
          logicalTick: 2,
          eventKinds: eventKinds.slice(4, 5),
          semantics: {
            ...emptySemantics,
            dispatchChoices: [
              `${firstWorkstation(factory).name}:${initialSubmissions.map(({ name }) => name).join(",")}`,
            ],
            replayProjection,
          },
        },
        {
          logicalTick: 3,
          eventKinds: eventKinds.slice(5),
          semantics: {
            ...emptySemantics,
            consumedWork: initialSubmissions.map(({ name }) => name).sort(),
            outcomes: ["ACCEPTED"],
            routes: Array.from({ length: outputCount }, () => terminal),
            terminalStates: Array.from({ length: outputCount }, () => terminal),
            replayProjection,
          },
        },
      ];
  return {
    schemaVersion: "factory-emulator-runtime-reference/v1",
    id,
    title,
    provenance: { kind: "public-documentation", source },
    factory,
    scenario,
    ticks,
    orderedEventKinds: ticks.flatMap(({ eventKinds }) => eventKinds),
  };
}

function scenario(
  id: string,
  factoryName: string,
  workstation: string,
): FactoryEmulatorScenario {
  return {
    schemaVersion: "factory-emulator-scenario/v1" as const,
    id,
    factory: { name: factoryName },
    seed: `${id}-seed`,
    startAt: "2026-07-18T16:00:00.000Z",
    initialSubmissions: [
      {
        name: "input",
        workType: "task",
        state: "ready",
        input: "frozen input",
      },
    ],
    rules: [
      {
        id: "accepted",
        selector: { workstation },
        cursor: { scope: "lineage" as const, input: "rootWorkId" as const },
        outcomes: [{ result: "accepted" as const, durationMs: 1 }],
        exhaustion: "repeat-last" as const,
      },
    ],
    unmatched: { behavior: "error" as const },
  };
}

function standardFactory(
  name: string,
  workstation = "execute",
): FactoryDefinition {
  return {
    name,
    orchestrator: { kind: "PETRI" as const },
    workTypes: [
      {
        name: "task",
        states: [
          { name: "ready", type: "INITIAL" as const },
          { name: "done", type: "TERMINAL" as const },
          { name: "failed", type: "FAILED" as const },
        ],
      },
    ],
    workers: [{ name: "worker", type: "AGENT_WORKER" as const }],
    workstations: [
      {
        name: workstation,
        type: "AGENT_RUN" as const,
        worker: "worker",
        inputs: [{ workType: "task", state: "ready" }],
        outputs: [{ workType: "task", state: "done" }],
        onFailure: [{ workType: "task", state: "failed" }],
      },
    ],
  };
}

const basic = standardFactory("runtime-reference-basic");
const repeater = cloneFactory(standardFactory("runtime-reference-repeater"));
firstWorkstation(repeater).behavior = "REPEATER";
firstWorkstation(repeater).onContinue = [{ workType: "task", state: "ready" }];
const routing = cloneFactory(standardFactory("runtime-reference-routing"));
firstWorkstation(routing).onRejection = [{ workType: "task", state: "failed" }];
const propagation = cloneFactory(
  standardFactory("runtime-reference-propagation"),
);
firstWorkstation(propagation).workPropagation = { mode: "PRESERVE_INPUT" };
const multiInputOutput = cloneFactory(
  standardFactory("runtime-reference-multi-input-output"),
);
const multiInputOutputWorkstation = firstWorkstation(multiInputOutput);
multiInputOutputWorkstation.inputs.push({
  workType: "task",
  state: "ready",
});
if (multiInputOutputWorkstation.outputs === undefined) {
  throw new Error("Frozen multi-output reference requires an output route.");
}
multiInputOutputWorkstation.outputs.push({
  workType: "task",
  state: "done",
});
const multiInputOutputScenario = scenario(
  "multi-input-output",
  multiInputOutput.name,
  "execute",
);
if (!Array.isArray(multiInputOutputScenario.initialSubmissions)) {
  throw new Error("Frozen multi-input reference requires direct submissions.");
}
multiInputOutputScenario.initialSubmissions.push({
  name: "input-two",
  workType: "task",
  state: "ready",
  input: "second frozen input",
});
const logicalMove = cloneFactory(
  standardFactory("runtime-reference-logical-move", "move"),
);
const logicalMoveWorkstations = logicalMove.workstations;
if (logicalMoveWorkstations === undefined) {
  throw new Error("Frozen logical-move reference requires a workstation.");
}
logicalMoveWorkstations[0] = {
  name: "move",
  type: "LOGICAL_MOVE",
  worker: "",
  inputs: [{ workType: "task", state: "ready" }],
  outputs: [{ workType: "task", state: "done" }],
  guards: [{ type: "VISIT_COUNT", workstation: "move", maxVisits: 1 }],
};

export const runtimeReferenceFixtures = [
  fixture(
    "basic-execution",
    "Basic execution",
    basic,
    scenario("basic-execution", basic.name, "execute"),
  ),
  fixture(
    "repeaters",
    "Repeater continuation",
    repeater,
    scenario("repeaters", repeater.name, "execute"),
  ),
  fixture(
    "routing",
    "Outcome routing",
    routing,
    scenario("routing", routing.name, "execute"),
  ),
  fixture(
    "logical-moves",
    "Logical move",
    logicalMove,
    scenario("logical-moves", logicalMove.name, "move"),
  ),
  fixture(
    "multi-input-output",
    "Multi-input and multi-output",
    multiInputOutput,
    multiInputOutputScenario,
  ),
  fixture(
    "propagation",
    "Work propagation",
    propagation,
    scenario("propagation", propagation.name, "execute"),
  ),
] satisfies readonly FactoryEmulatorRuntimeReference[];
