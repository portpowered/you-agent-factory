// biome-ignore lint/style/noExcessiveLinesPerFile: Frozen fixture evidence remains intentionally adjacent for review.
import type { FactoryDefinition } from "@you-agent-factory/client";
import type {
  FactoryEmulatorRuntimeReference,
  FactoryEmulatorRuntimeReferenceTick,
} from "./runtime-reference.js";
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
  const terminalRoute = `${terminal}:${JSON.stringify(initialSubmissions[0]?.input)}`;
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
            routes: Array.from({ length: outputCount }, () => terminalRoute),
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

function singleOutcomeFixture(
  id: string,
  title: string,
  factory: FactoryDefinition,
  scenario: FactoryEmulatorScenario,
  outcome: "ACCEPTED" | "REJECTED",
  route: string,
  terminalState: string,
): FactoryEmulatorRuntimeReference {
  const initial = scenario.initialSubmissions;
  if (!Array.isArray(initial) || initial[0] === undefined) {
    throw new Error("Frozen single-outcome reference requires one submission.");
  }
  const input = initial[0];
  const replayProjection = [`${input.name}:${input.workType}:${input.state}`];
  const ticks: FactoryEmulatorRuntimeReferenceTick[] = [
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
        dispatchChoices: [`${firstWorkstation(factory).name}:${input.name}`],
        replayProjection,
      },
    },
    {
      logicalTick: 3,
      eventKinds: eventKinds.slice(5),
      semantics: {
        ...emptySemantics,
        consumedWork: [input.name],
        outcomes: [outcome],
        routes: [route],
        terminalStates: [terminalState],
        replayProjection,
      },
    },
  ];
  return {
    schemaVersion: "factory-emulator-runtime-reference/v1",
    id,
    title,
    provenance: {
      kind: "public-documentation",
      source:
        "ui/packages/factory-emulator/README.md#long-lived-session-lifecycle (documented outcome routes and propagation)",
    },
    factory,
    scenario,
    ticks,
    orderedEventKinds: ticks.flatMap(({ eventKinds }) => eventKinds),
  };
}

function repeaterFixture(): FactoryEmulatorRuntimeReference {
  const baseScenario = scenario("repeaters", repeater.name, "execute");
  const [rule] = baseScenario.rules;
  if (rule === undefined)
    throw new Error("Frozen repeater reference needs a rule.");
  const repeaterScenario: FactoryEmulatorScenario = {
    ...baseScenario,
    rules: [
      {
        ...rule,
        outcomes: [
          { result: "continued", durationMs: 1 },
          { result: "accepted", durationMs: 1 },
        ],
      },
    ],
  };
  const replayProjection = ["input:task:ready"];
  const ticks: FactoryEmulatorRuntimeReferenceTick[] = [
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
        dispatchChoices: ["execute:input"],
        replayProjection,
      },
    },
    {
      logicalTick: 3,
      eventKinds: eventKinds.slice(5),
      semantics: {
        ...emptySemantics,
        consumedWork: ["input"],
        outcomes: ["CONTINUE"],
        routes: ['task:ready:"frozen input"'],
        replayProjection,
      },
    },
    {
      logicalTick: 4,
      eventKinds: eventKinds.slice(4, 5),
      semantics: {
        ...emptySemantics,
        dispatchChoices: ["execute:input/execute/0"],
        replayProjection,
      },
    },
    {
      logicalTick: 5,
      eventKinds: eventKinds.slice(5),
      semantics: {
        ...emptySemantics,
        consumedWork: ["input"],
        outcomes: ["ACCEPTED"],
        routes: ['task:done:"frozen input"'],
        terminalStates: ["task:done"],
        replayProjection,
      },
    },
  ];
  return {
    schemaVersion: "factory-emulator-runtime-reference/v1",
    id: "repeaters",
    title: "Repeater continuation",
    provenance: {
      kind: "public-documentation",
      source:
        "ui/packages/factory-emulator/README.md#long-lived-session-lifecycle (documented continued outcome route)",
    },
    factory: repeater,
    scenario: repeaterScenario,
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

function scenarioWithOutcome(
  id: string,
  factoryName: string,
  workstation: string,
  outcome: FactoryEmulatorScenario["rules"][number]["outcomes"][number],
): FactoryEmulatorScenario {
  const baseScenario = scenario(id, factoryName, workstation);
  const [rule] = baseScenario.rules;
  if (rule === undefined) throw new Error("Frozen reference needs a rule.");
  return { ...baseScenario, rules: [{ ...rule, outcomes: [outcome] }] };
}

function concurrentFixture(
  id: string,
  title: string,
  factory: FactoryDefinition,
  submissions: readonly string[],
  dispatchGroups: readonly (readonly string[])[],
): FactoryEmulatorRuntimeReference {
  const concurrentScenario: FactoryEmulatorScenario = {
    ...scenario(id, factory.name, "execute"),
    initialSubmissions: submissions.map((name) => ({
      name,
      workType: "task",
      state: "ready",
      input: `${name} frozen input`,
    })),
  };
  const replayProjection = submissions
    .map((name) => `${name}:task:ready`)
    .sort();
  const ticks: FactoryEmulatorRuntimeReferenceTick[] = [
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
  ];
  const completed: string[] = [];
  for (const group of dispatchGroups) {
    ticks.push({
      logicalTick: ticks.length,
      eventKinds: group.map(() => "DISPATCH_REQUEST"),
      semantics: {
        ...emptySemantics,
        dispatchChoices: group.map((name) => `execute:${name}`).sort(),
        terminalStates: completed.map(() => "task:done"),
        replayProjection,
      },
    });
    completed.push(...group);
    ticks.push({
      logicalTick: ticks.length,
      eventKinds: group.map(() => "DISPATCH_RESPONSE"),
      semantics: {
        ...emptySemantics,
        consumedWork: [...group].sort(),
        outcomes: group.map(() => "ACCEPTED"),
        routes: group.map(
          (name) => `task:done:${JSON.stringify(`${name} frozen input`)}`,
        ),
        terminalStates: completed.map(() => "task:done"),
        replayProjection,
      },
    });
  }
  return {
    schemaVersion: "factory-emulator-runtime-reference/v1",
    id,
    title,
    provenance: { kind: "public-documentation", source },
    factory,
    scenario: concurrentScenario,
    ticks,
    orderedEventKinds: ticks.flatMap(({ eventKinds }) => eventKinds),
  };
}

function dependencyFixture(
  id: string,
  title: string,
  prerequisiteOutcome: "accepted" | "failed",
): FactoryEmulatorRuntimeReference {
  const factory = standardFactory(`runtime-reference-${id}`);
  const dependencyScenario: FactoryEmulatorScenario = {
    ...scenario(id, factory.name, "execute"),
    initialSubmissions: {
      works: [
        { name: "blocked", workType: "task", state: "ready", input: "blocked" },
        {
          name: "prerequisite",
          workType: "task",
          state: "ready",
          input: "prerequisite",
        },
      ],
      relations: [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "prerequisite",
          requiredState: "done",
        },
      ],
    },
    rules: [
      {
        id: "prerequisite-outcome",
        selector: { input: { name: "prerequisite" } },
        cursor: { scope: "lineage", input: "rootWorkId" },
        outcomes: [
          prerequisiteOutcome === "accepted"
            ? { result: "accepted", durationMs: 1 }
            : { result: "failed", durationMs: 1, error: "prerequisite failed" },
        ],
        exhaustion: "repeat-last",
      },
      {
        id: "blocked-outcome",
        selector: { input: { name: "blocked" } },
        cursor: { scope: "lineage", input: "rootWorkId" },
        outcomes: [{ result: "accepted", durationMs: 1 }],
        exhaustion: "repeat-last",
      },
    ],
  };
  const failed = prerequisiteOutcome === "failed";
  const ticks: FactoryEmulatorRuntimeReferenceTick[] = [
    {
      logicalTick: 0,
      eventKinds: eventKinds.slice(0, 3),
      semantics: { ...emptySemantics, replayProjection: [] },
    },
    {
      logicalTick: 1,
      eventKinds: ["WORK_REQUEST", "RELATIONSHIP_CHANGE_REQUEST"],
      semantics: {
        ...emptySemantics,
        replayProjection: ["blocked:task:ready", "prerequisite:task:ready"],
      },
    },
    {
      logicalTick: 2,
      eventKinds: ["DISPATCH_REQUEST"],
      semantics: {
        ...emptySemantics,
        dispatchChoices: ["execute:prerequisite"],
        replayProjection: ["blocked:task:ready", "prerequisite:task:ready"],
      },
    },
    {
      logicalTick: 3,
      eventKinds: failed
        ? ["DISPATCH_RESPONSE", "WORK_STATE_CHANGE"]
        : ["DISPATCH_RESPONSE"],
      semantics: {
        ...emptySemantics,
        consumedWork: ["prerequisite"],
        outcomes: [failed ? "FAILED" : "ACCEPTED"],
        routes: [
          `${failed ? "task:failed" : "task:done"}:${JSON.stringify("prerequisite")}`,
        ],
        terminalStates: failed ? ["task:failed", "task:failed"] : ["task:done"],
        replayProjection: ["blocked:task:ready", "prerequisite:task:ready"],
      },
    },
  ];
  if (!failed) {
    ticks.push(
      {
        logicalTick: 4,
        eventKinds: ["DISPATCH_REQUEST"],
        semantics: {
          ...emptySemantics,
          dispatchChoices: ["execute:blocked"],
          terminalStates: ["task:done"],
          replayProjection: ["blocked:task:ready", "prerequisite:task:ready"],
        },
      },
      {
        logicalTick: 5,
        eventKinds: ["DISPATCH_RESPONSE"],
        semantics: {
          ...emptySemantics,
          consumedWork: ["blocked"],
          outcomes: ["ACCEPTED"],
          routes: ['task:done:"blocked"'],
          terminalStates: ["task:done", "task:done"],
          replayProjection: ["blocked:task:ready", "prerequisite:task:ready"],
        },
      },
    );
  }
  return {
    schemaVersion: "factory-emulator-runtime-reference/v1",
    id,
    title,
    provenance: { kind: "public-documentation", source },
    factory,
    scenario: dependencyScenario,
    ticks,
    orderedEventKinds: ticks.flatMap(({ eventKinds }) => eventKinds),
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
  name: "execute",
  type: "AGENT_RUN",
  worker: "worker",
  inputs: [{ workType: "task", state: "ready" }],
  outputs: [{ workType: "task", state: "ready" }],
};
logicalMoveWorkstations.push({
  name: "move",
  type: "LOGICAL_MOVE",
  worker: "",
  inputs: [{ workType: "task", state: "ready" }],
  outputs: [{ workType: "task", state: "done" }],
  guards: [{ type: "VISIT_COUNT", workstation: "execute", maxVisits: 1 }],
  workPropagation: { mode: "PRESERVE_INPUT" },
});

function logicalMoveFixture(): FactoryEmulatorRuntimeReference {
  const baseScenario = scenario("logical-moves", logicalMove.name, "execute");
  const [rule] = baseScenario.rules;
  if (rule === undefined)
    throw new Error("Frozen logical-move reference needs a rule.");
  const logicalScenario: FactoryEmulatorScenario = {
    ...baseScenario,
    rules: [
      {
        ...rule,
        outcomes: [
          { result: "accepted", durationMs: 1, output: "worker output" },
        ],
      },
    ],
  };
  const replayProjection = ["input:task:ready"];
  const ticks: FactoryEmulatorRuntimeReferenceTick[] = [
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
        dispatchChoices: ["execute:input"],
        replayProjection,
      },
    },
    {
      logicalTick: 3,
      eventKinds: eventKinds.slice(5),
      semantics: {
        ...emptySemantics,
        consumedWork: ["input"],
        outcomes: ["ACCEPTED"],
        routes: ['task:ready:"worker output"'],
        replayProjection,
      },
    },
    {
      logicalTick: 4,
      eventKinds: ["DISPATCH_RESPONSE"],
      semantics: {
        ...emptySemantics,
        consumedWork: ["input"],
        outcomes: ["ACCEPTED"],
        routes: ['task:done:"worker output"'],
        terminalStates: ["task:done"],
        replayProjection,
      },
    },
  ];
  return {
    schemaVersion: "factory-emulator-runtime-reference/v1",
    id: "logical-moves",
    title: "Logical move",
    provenance: {
      kind: "public-documentation",
      source:
        "ui/packages/factory-emulator/README.md#long-lived-session-lifecycle (documented eligible LOGICAL_MOVE route)",
    },
    factory: logicalMove,
    scenario: logicalScenario,
    ticks,
    orderedEventKinds: ticks.flatMap(({ eventKinds }) => eventKinds),
  };
}
const parallel = cloneFactory(standardFactory("runtime-reference-parallel"));
const contention = cloneFactory(
  standardFactory("runtime-reference-resource-contention"),
);
contention.resources = [{ name: "agent-slot", capacity: 2 }];
const contentionWorkstation = firstWorkstation(contention);
contentionWorkstation.resources = [{ name: "agent-slot", capacity: 1 }];

export const runtimeReferenceFixtures = [
  fixture(
    "basic-execution",
    "Basic execution",
    basic,
    scenario("basic-execution", basic.name, "execute"),
  ),
  repeaterFixture(),
  singleOutcomeFixture(
    "routing",
    "Outcome routing",
    routing,
    scenarioWithOutcome("routing", routing.name, "execute", {
      result: "rejected",
      durationMs: 1,
    }),
    "REJECTED",
    'task:failed:"frozen input"',
    "task:failed",
  ),
  logicalMoveFixture(),
  fixture(
    "multi-input-output",
    "Multi-input and multi-output",
    multiInputOutput,
    multiInputOutputScenario,
  ),
  singleOutcomeFixture(
    "propagation",
    "Work propagation",
    propagation,
    scenarioWithOutcome("propagation", propagation.name, "execute", {
      result: "accepted",
      durationMs: 1,
      output: "worker output",
    }),
    "ACCEPTED",
    'task:done:"frozen input"',
    "task:done",
  ),
  concurrentFixture(
    "parallel-dispatch",
    "Parallel dispatch",
    parallel,
    ["first", "second"],
    [["first", "second"]],
  ),
  concurrentFixture(
    "simultaneous-completion",
    "Simultaneous completion",
    cloneFactory(parallel),
    ["first", "second"],
    [["first", "second"]],
  ),
  concurrentFixture(
    "resource-contention",
    "Resource contention",
    contention,
    ["first", "second", "third"],
    [["first", "third"], ["second"]],
  ),
  dependencyFixture(
    "depends-on-release",
    "DEPENDS_ON prerequisite release",
    "accepted",
  ),
  dependencyFixture(
    "depends-on-terminal-failure",
    "DEPENDS_ON terminal failure cascade",
    "failed",
  ),
] satisfies readonly FactoryEmulatorRuntimeReference[];
