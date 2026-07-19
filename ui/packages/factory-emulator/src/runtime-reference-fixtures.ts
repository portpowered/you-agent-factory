import type { FactoryDefinition } from "@you-agent-factory/client";
import type { FactoryEmulatorRuntimeReference } from "./runtime-reference.js";
import { frozenRuntimeReferenceEvidence } from "./runtime-reference-evidence.js";
import type { FactoryEmulatorScenario } from "./scenario.js";

type ExecutableReferenceInput = Pick<
  FactoryEmulatorRuntimeReference,
  "id" | "title" | "factory" | "scenario"
>;

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

function scenario(
  id: string,
  factoryName: string,
  workstation: string,
): FactoryEmulatorScenario {
  return {
    schemaVersion: "factory-emulator-scenario/v1",
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
        cursor: { scope: "lineage", input: "rootWorkId" },
        outcomes: [{ result: "accepted", durationMs: 1 }],
        exhaustion: "repeat-last",
      },
    ],
    unmatched: { behavior: "error" },
  };
}

function standardFactory(
  name: string,
  workstation = "execute",
): FactoryDefinition {
  return {
    name,
    orchestrator: { kind: "PETRI" },
    workTypes: [
      {
        name: "task",
        states: [
          { name: "ready", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
          { name: "failed", type: "FAILED" },
        ],
      },
    ],
    workers: [{ name: "worker", type: "AGENT_WORKER" }],
    workstations: [
      {
        name: workstation,
        type: "AGENT_RUN",
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
  const base = scenario(id, factoryName, workstation);
  const [rule] = base.rules;
  if (rule === undefined) throw new Error("Frozen reference needs a rule.");
  return { ...base, rules: [{ ...rule, outcomes: [outcome] }] };
}

function concurrentScenario(
  id: string,
  factoryName: string,
  submissions: readonly string[],
): FactoryEmulatorScenario {
  return {
    ...scenario(id, factoryName, "execute"),
    initialSubmissions: submissions.map((name) => ({
      name,
      workType: "task",
      state: "ready",
      input: `${name} frozen input`,
    })),
  };
}

function dependencyInput(
  id: string,
  title: string,
  prerequisiteOutcome: "accepted" | "failed",
): ExecutableReferenceInput {
  const factory = standardFactory(`runtime-reference-${id}`);
  return {
    id,
    title,
    factory,
    scenario: {
      ...scenario(id, factory.name, "execute"),
      initialSubmissions: {
        works: [
          {
            name: "blocked",
            workType: "task",
            state: "ready",
            input: "blocked",
          },
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
              : {
                  result: "failed",
                  durationMs: 1,
                  error: "prerequisite failed",
                },
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
    },
  };
}

const basic = standardFactory("runtime-reference-basic");
const repeater = cloneFactory(standardFactory("runtime-reference-repeater"));
firstWorkstation(repeater).behavior = "REPEATER";
firstWorkstation(repeater).onContinue = [{ workType: "task", state: "ready" }];
const repeaterScenario = scenario("repeaters", repeater.name, "execute");
const [repeaterRule] = repeaterScenario.rules;
if (repeaterRule === undefined) {
  throw new Error("Frozen repeater reference needs a rule.");
}
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
multiInputOutputWorkstation.inputs.push({ workType: "task", state: "ready" });
if (multiInputOutputWorkstation.outputs === undefined) {
  throw new Error("Frozen multi-output reference requires an output route.");
}
multiInputOutputWorkstation.outputs.push({ workType: "task", state: "done" });
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
const parallel = cloneFactory(standardFactory("runtime-reference-parallel"));
const contention = cloneFactory(
  standardFactory("runtime-reference-resource-contention"),
);
contention.resources = [{ name: "agent-slot", capacity: 2 }];
firstWorkstation(contention).resources = [{ name: "agent-slot", capacity: 1 }];

const runtimeReferenceInputs: readonly ExecutableReferenceInput[] = [
  {
    id: "basic-execution",
    title: "Basic execution",
    factory: basic,
    scenario: scenario("basic-execution", basic.name, "execute"),
  },
  {
    id: "repeaters",
    title: "Repeater continuation",
    factory: repeater,
    scenario: {
      ...repeaterScenario,
      rules: [
        {
          ...repeaterRule,
          outcomes: [
            { result: "continued", durationMs: 1 },
            { result: "accepted", durationMs: 1 },
          ],
        },
      ],
    },
  },
  {
    id: "routing",
    title: "Outcome routing",
    factory: routing,
    scenario: scenarioWithOutcome("routing", routing.name, "execute", {
      result: "rejected",
      durationMs: 1,
    }),
  },
  {
    id: "logical-moves",
    title: "Logical move",
    factory: logicalMove,
    scenario: scenarioWithOutcome(
      "logical-moves",
      logicalMove.name,
      "execute",
      {
        result: "accepted",
        durationMs: 1,
        output: "worker output",
      },
    ),
  },
  {
    id: "multi-input-output",
    title: "Multi-input and multi-output",
    factory: multiInputOutput,
    scenario: multiInputOutputScenario,
  },
  {
    id: "propagation",
    title: "Work propagation",
    factory: propagation,
    scenario: scenarioWithOutcome("propagation", propagation.name, "execute", {
      result: "accepted",
      durationMs: 1,
      output: "worker output",
    }),
  },
  {
    id: "parallel-dispatch",
    title: "Parallel dispatch",
    factory: parallel,
    scenario: concurrentScenario("parallel-dispatch", parallel.name, [
      "first",
      "second",
    ]),
  },
  {
    id: "simultaneous-completion",
    title: "Simultaneous completion",
    factory: cloneFactory(parallel),
    scenario: concurrentScenario("simultaneous-completion", parallel.name, [
      "first",
      "second",
    ]),
  },
  {
    id: "resource-contention",
    title: "Resource contention",
    factory: contention,
    scenario: concurrentScenario("resource-contention", contention.name, [
      "first",
      "second",
      "third",
    ]),
  },
  dependencyInput(
    "depends-on-release",
    "DEPENDS_ON prerequisite release",
    "accepted",
  ),
  dependencyInput(
    "depends-on-terminal-failure",
    "DEPENDS_ON terminal failure cascade",
    "failed",
  ),
];

/**
 * Factories and scenarios are executable inputs; expected evidence lives in a
 * separate immutable declaration so changing either input cannot recompute a
 * passing expectation.
 */
export const runtimeReferenceFixtures: readonly FactoryEmulatorRuntimeReference[] =
  runtimeReferenceInputs.map(({ id, title, factory, scenario }) => {
    const evidence = frozenRuntimeReferenceEvidence[id];
    if (evidence === undefined) {
      throw new Error(`Missing frozen runtime-reference evidence for ${id}.`);
    }
    return {
      schemaVersion: "factory-emulator-runtime-reference/v1",
      id,
      title,
      provenance: evidence.provenance,
      factory,
      scenario,
      ticks: evidence.ticks,
      orderedEventKinds: evidence.orderedEventKinds,
    };
  });
