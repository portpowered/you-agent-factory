import type { FactoryEvent } from "@you-agent-factory/client";
import type {
  FactoryEmulatorRuntimeReferenceProvenance,
  FactoryEmulatorRuntimeReferenceTick,
} from "./runtime-reference.js";

type FrozenEvidence = Readonly<{
  provenance: FactoryEmulatorRuntimeReferenceProvenance;
  ticks: readonly FactoryEmulatorRuntimeReferenceTick[];
  orderedEventKinds: readonly FactoryEvent["type"][];
}>;

const source = (anchor: string): FactoryEmulatorRuntimeReferenceProvenance => ({
  kind: "public-documentation",
  source: `docs/reference/${anchor} (pre-existing public runtime behavior documentation)`,
});

const semantics = (
  overrides: Partial<FactoryEmulatorRuntimeReferenceTick["semantics"]> = {},
) => ({
  dispatchChoices: [],
  consumedWork: [],
  outcomes: [],
  routes: [],
  terminalStates: [],
  replayProjection: [],
  ...overrides,
});
const tick = (
  logicalTick: number,
  eventKinds: readonly FactoryEvent["type"][],
  values = {},
): FactoryEmulatorRuntimeReferenceTick => ({
  logicalTick,
  eventKinds,
  semantics: semantics(values),
});
const ordered = (ticks: readonly FactoryEmulatorRuntimeReferenceTick[]) =>
  ticks.flatMap(({ eventKinds }) => eventKinds);
const bootstrap = () =>
  tick(0, ["RUN_REQUEST", "INITIAL_STRUCTURE_REQUEST", "SESSION_STARTED"]);
const submitted = (
  projection: readonly string[],
  kinds: readonly FactoryEvent["type"][] = ["WORK_REQUEST"],
) => tick(1, kinds, { replayProjection: projection });
const dispatch = (
  logicalTick: number,
  choices: readonly string[],
  projection: readonly string[],
  terminalStates: readonly string[] = [],
) =>
  tick(
    logicalTick,
    choices.map(() => "DISPATCH_REQUEST"),
    { dispatchChoices: choices, terminalStates, replayProjection: projection },
  );
const response = (
  logicalTick: number,
  consumedWork: readonly string[],
  outcomes: readonly string[],
  routes: readonly string[],
  terminalStates: readonly string[],
  projection: readonly string[],
  kinds: readonly FactoryEvent["type"][] = [],
) =>
  tick(
    logicalTick,
    kinds.length === 0
      ? outcomes.map(() => "DISPATCH_RESPONSE" as const)
      : kinds,
    {
      consumedWork,
      outcomes,
      routes,
      terminalStates,
      replayProjection: projection,
    },
  );

const input = ["input:task:ready"];
const parallel = ["first:task:ready", "second:task:ready"];
const contention = [
  "first:task:ready",
  "second:task:ready",
  "third:task:ready",
];
const dependencies = ["blocked:task:ready", "prerequisite:task:ready"];

// This is intentionally independent, literal expected evidence. Do not derive it from Factory or scenario inputs.
export const frozenRuntimeReferenceEvidence: Readonly<
  Record<string, FrozenEvidence>
> = {
  "basic-execution": (() => {
    const ticks = [
      bootstrap(),
      submitted(input),
      dispatch(2, ["execute:input"], input),
      response(
        3,
        ["input"],
        ["ACCEPTED"],
        ['task:done:"frozen input"'],
        ["task:done"],
        input,
      ),
    ];
    return {
      provenance: source("workstations.md#agent_run-execution"),
      ticks,
      orderedEventKinds: ordered(ticks),
    };
  })(),
  repeaters: (() => {
    const ticks = [
      bootstrap(),
      submitted(input),
      dispatch(2, ["execute:input"], input),
      response(
        3,
        ["input"],
        ["CONTINUE"],
        ['task:ready:"frozen input"'],
        [],
        input,
      ),
      dispatch(4, ["execute:input/execute/0"], input),
      response(
        5,
        ["input"],
        ["ACCEPTED"],
        ['task:done:"frozen input"'],
        ["task:done"],
        input,
      ),
    ];
    return {
      provenance: source("workstations.md#repeater-workstations"),
      ticks,
      orderedEventKinds: ordered(ticks),
    };
  })(),
  routing: (() => {
    const ticks = [
      bootstrap(),
      submitted(input),
      dispatch(2, ["execute:input"], input),
      response(
        3,
        ["input"],
        ["REJECTED"],
        ['task:failed:"frozen input"'],
        ["task:failed"],
        input,
      ),
    ];
    return {
      provenance: source("workstations.md#topology-fields"),
      ticks,
      orderedEventKinds: ordered(ticks),
    };
  })(),
  "logical-moves": (() => {
    const ticks = [
      bootstrap(),
      submitted(input),
      dispatch(2, ["execute:input"], input),
      response(
        3,
        ["input"],
        ["ACCEPTED"],
        ['task:ready:"worker output"'],
        [],
        input,
      ),
      response(
        4,
        ["input"],
        ["ACCEPTED"],
        ['task:done:"worker output"'],
        ["task:done"],
        input,
      ),
    ];
    return {
      provenance: source("workstations.md#behavior-versus-type"),
      ticks,
      orderedEventKinds: ordered(ticks),
    };
  })(),
  "multi-input-output": (() => {
    const projection = ["input-two:task:ready", "input:task:ready"];
    const ticks = [
      bootstrap(),
      submitted(projection),
      dispatch(2, ["execute:input,input-two"], projection),
      response(
        3,
        ["input", "input-two"],
        ["ACCEPTED"],
        ['task:done:"frozen input"', 'task:done:"frozen input"'],
        ["task:done", "task:done"],
        projection,
      ),
    ];
    return {
      provenance: source("workstations.md#topology-fields"),
      ticks,
      orderedEventKinds: ordered(ticks),
    };
  })(),
  propagation: (() => {
    const ticks = [
      bootstrap(),
      submitted(input),
      dispatch(2, ["execute:input"], input),
      response(
        3,
        ["input"],
        ["ACCEPTED"],
        ['task:done:"frozen input"'],
        ["task:done"],
        input,
      ),
    ];
    return {
      provenance: source("workstations.md#work-payload-propagation"),
      ticks,
      orderedEventKinds: ordered(ticks),
    };
  })(),
  "parallel-dispatch": (() => {
    const ticks = [
      bootstrap(),
      submitted(parallel),
      dispatch(2, ["execute:first", "execute:second"], parallel),
      response(
        3,
        ["first", "second"],
        ["ACCEPTED", "ACCEPTED"],
        ['task:done:"first frozen input"', 'task:done:"second frozen input"'],
        ["task:done", "task:done"],
        parallel,
      ),
    ];
    return {
      provenance: source("workstations.md#agent_run-execution"),
      ticks,
      orderedEventKinds: ordered(ticks),
    };
  })(),
  "simultaneous-completion": (() => {
    const ticks = [
      bootstrap(),
      submitted(parallel),
      dispatch(2, ["execute:first", "execute:second"], parallel),
      response(
        3,
        ["first", "second"],
        ["ACCEPTED", "ACCEPTED"],
        ['task:done:"first frozen input"', 'task:done:"second frozen input"'],
        ["task:done", "task:done"],
        parallel,
      ),
    ];
    return {
      provenance: source("workstations.md#agent_run-execution"),
      ticks,
      orderedEventKinds: ordered(ticks),
    };
  })(),
  "resource-contention": (() => {
    const ticks = [
      bootstrap(),
      submitted(contention),
      dispatch(2, ["execute:first", "execute:third"], contention),
      response(
        3,
        ["first", "third"],
        ["ACCEPTED", "ACCEPTED"],
        ['task:done:"first frozen input"', 'task:done:"third frozen input"'],
        ["task:done", "task:done"],
        contention,
      ),
      dispatch(4, ["execute:second"], contention, ["task:done", "task:done"]),
      response(
        5,
        ["second"],
        ["ACCEPTED"],
        ['task:done:"second frozen input"'],
        ["task:done", "task:done", "task:done"],
        contention,
      ),
    ];
    return {
      provenance: source("workstations.md#current-contract"),
      ticks,
      orderedEventKinds: ordered(ticks),
    };
  })(),
  "depends-on-release": (() => {
    const ticks = [
      bootstrap(),
      submitted(dependencies, ["WORK_REQUEST", "RELATIONSHIP_CHANGE_REQUEST"]),
      dispatch(2, ["execute:prerequisite"], dependencies),
      response(
        3,
        ["prerequisite"],
        ["ACCEPTED"],
        ['task:done:"prerequisite"'],
        ["task:done"],
        dependencies,
      ),
      dispatch(4, ["execute:blocked"], dependencies, ["task:done"]),
      response(
        5,
        ["blocked"],
        ["ACCEPTED"],
        ['task:done:"blocked"'],
        ["task:done", "task:done"],
        dependencies,
      ),
    ];
    return {
      provenance: source("relationships.md#depends_on"),
      ticks,
      orderedEventKinds: ordered(ticks),
    };
  })(),
  "depends-on-terminal-failure": (() => {
    const ticks = [
      bootstrap(),
      submitted(dependencies, ["WORK_REQUEST", "RELATIONSHIP_CHANGE_REQUEST"]),
      dispatch(2, ["execute:prerequisite"], dependencies),
      response(
        3,
        ["prerequisite"],
        ["FAILED"],
        ['task:failed:"prerequisite"'],
        ["task:failed", "task:failed"],
        dependencies,
        ["DISPATCH_RESPONSE", "WORK_STATE_CHANGE"],
      ),
    ];
    return {
      provenance: source("relationships.md#depends_on"),
      ticks,
      orderedEventKinds: ordered(ticks),
    };
  })(),
};
