import type { FactoryEvent } from "@you-agent-factory/client";
import { MemoryFactoryEventSink } from "./event-sink.js";
import type {
  FactoryEmulatorRuntimeReference,
  FactoryEmulatorRuntimeReferenceIssue,
  FactoryEmulatorRuntimeReferenceSemantics,
  FactoryEmulatorRuntimeReferenceTick,
} from "./runtime-reference.js";
import { safeParseFactoryEmulatorRuntimeReference } from "./runtime-reference.js";
import { createFactoryEmulatorSession } from "./session.js";
import { replayFactoryEmulatorSubmissions } from "./submission-replay.js";

export type FactoryEmulatorRuntimeReferenceComparisonSurface =
  | keyof FactoryEmulatorRuntimeReferenceSemantics
  | "eventKinds";

export interface FactoryEmulatorRuntimeReferenceDivergence {
  readonly fixture: string;
  readonly logicalTick: number;
  readonly surface: FactoryEmulatorRuntimeReferenceComparisonSurface;
  readonly expected: unknown;
  readonly actual: unknown;
}

export type FactoryEmulatorRuntimeReferenceConformanceResult =
  | { readonly matches: true }
  | {
      readonly matches: false;
      readonly validationIssues: readonly FactoryEmulatorRuntimeReferenceIssue[];
    }
  | {
      readonly matches: false;
      readonly divergence: FactoryEmulatorRuntimeReferenceDivergence;
    };

function normalizedSemantics(
  reference: FactoryEmulatorRuntimeReference,
  events: readonly FactoryEvent[],
  batch: readonly FactoryEvent[],
  state: ReturnType<ReturnType<typeof createFactoryEmulatorSession>["state"]>,
): FactoryEmulatorRuntimeReferenceSemantics {
  const terminalStates = new Set(
    (reference.factory.workTypes ?? []).flatMap(({ name, states }) =>
      (states ?? [])
        .filter(({ type }) => type === "TERMINAL" || type === "FAILED")
        .map(({ name: state }) => `${name}:${state}`),
    ),
  );
  const works = "works" in state ? state.works : [];
  const workByToken = new Map(works.map((work) => [work.tokenId, work]));
  const dispatched = works.flatMap(({ phase, dispatch }) =>
    phase === "active" && dispatch !== undefined
      ? [
          `${dispatch.workstation}:${dispatch.inputTokenIds.map((id) => workByToken.get(id)?.submissionId ?? id).join(",")}`,
        ]
      : [],
  );
  const responses = batch.filter(({ type }) => type === "DISPATCH_RESPONSE");
  const routed = responses.flatMap(({ payload }) => {
    const outputWork = (payload as { outputWork?: unknown }).outputWork;
    return Array.isArray(outputWork)
      ? outputWork.flatMap((work) => {
          const value = work as {
            workTypeName?: unknown;
            state?: { name?: unknown };
            payload?: unknown;
          };
          return typeof value.workTypeName === "string" &&
            typeof value.state?.name === "string"
            ? [
                `${value.workTypeName}:${value.state.name}:${JSON.stringify(value.payload)}`,
              ]
            : [];
        })
      : [];
  });
  return {
    dispatchChoices: dispatched.sort(),
    consumedWork: responses
      .flatMap(({ context }) => context.workIds ?? [])
      .map(
        (workId) =>
          works.find((work) => work.workId === workId)?.submissionId ?? workId,
      )
      .sort(),
    outcomes: responses
      .map(({ payload }) => String((payload as { outcome?: unknown }).outcome))
      .sort(),
    routes: routed.sort(),
    terminalStates: works
      .filter((work) => terminalStates.has(`${work.workType}:${work.state}`))
      .map((work) => `${work.workType}:${work.state}`)
      .sort(),
    replayProjection: replayFactoryEmulatorSubmissions(events)
      .map(
        (work) =>
          `${work.submissionId}:${work.workType ?? ""}:${work.state ?? ""}`,
      )
      .sort(),
  };
}

function firstDifference(
  reference: FactoryEmulatorRuntimeReference,
  tick: FactoryEmulatorRuntimeReferenceTick,
  actual: FactoryEmulatorRuntimeReferenceTick,
): FactoryEmulatorRuntimeReferenceDivergence | undefined {
  for (const surface of [
    "eventKinds",
    "dispatchChoices",
    "consumedWork",
    "outcomes",
    "routes",
    "terminalStates",
    "replayProjection",
  ] as const) {
    const expectedValue =
      surface === "eventKinds" ? tick.eventKinds : tick.semantics[surface];
    const actualValue =
      surface === "eventKinds" ? actual.eventKinds : actual.semantics[surface];
    if (JSON.stringify(expectedValue) !== JSON.stringify(actualValue)) {
      return {
        fixture: reference.id,
        logicalTick: tick.logicalTick,
        surface,
        expected: expectedValue,
        actual: actualValue,
      };
    }
  }
  return undefined;
}

/** Runs the supported emulator subset and reports its first frozen-reference divergence. */
export async function compareFactoryEmulatorRuntimeReference(
  input: unknown,
): Promise<FactoryEmulatorRuntimeReferenceConformanceResult> {
  const parsed = safeParseFactoryEmulatorRuntimeReference(input);
  if (!parsed.success) {
    return { matches: false, validationIssues: parsed.issues };
  }
  const reference = parsed.data;
  const sink = new MemoryFactoryEventSink({ maxEvents: 10_000 });
  const emulator = createFactoryEmulatorSession({
    factory: reference.factory,
    scenario: reference.scenario,
    sink,
  });
  const actual: FactoryEmulatorRuntimeReferenceTick[] = [];
  let events: FactoryEvent[] = [];
  const append = (
    batch: readonly FactoryEvent[],
    state: ReturnType<typeof emulator.state>,
  ) => {
    events = [...events, ...batch];
    actual.push({
      logicalTick: actual.length,
      eventKinds: batch.map(({ type }) => type),
      semantics: normalizedSemantics(reference, events, batch, state),
    });
  };
  const started = await emulator.start();
  for (const batch of started.batches) append(batch, started.state);
  while (true) {
    const advanced = await emulator.advanceToNext();
    for (const batch of advanced.batches) append(batch, advanced.state);
    if (advanced.status === "idle") break;
  }
  for (const [index, tick] of reference.ticks.entries()) {
    const observed = actual[index];
    if (observed === undefined) {
      return {
        matches: false,
        divergence: {
          fixture: reference.id,
          logicalTick: tick.logicalTick,
          surface: "eventKinds",
          expected: tick.eventKinds,
          actual: undefined,
        },
      };
    }
    const divergence = firstDifference(reference, tick, observed);
    if (divergence !== undefined) return { matches: false, divergence };
  }
  if (actual.length !== reference.ticks.length) {
    return {
      matches: false,
      divergence: {
        fixture: reference.id,
        logicalTick: reference.ticks.length,
        surface: "eventKinds",
        expected: undefined,
        actual: actual[reference.ticks.length]?.eventKinds,
      },
    };
  }
  return { matches: true };
}
