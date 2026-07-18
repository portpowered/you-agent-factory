import type {
  FactoryEmulator,
  FactoryEmulatorAdvanceReceipt,
  FactoryEmulatorCloseReceipt,
  FactoryEmulatorStatus,
  FactoryEmulatorTick,
  FactoryEventBatch,
  FactoryEventSink,
  FactoryEventSinkCloseReceipt,
  FactoryEventSinkWriteReceipt,
  FactoryRecordingSink,
  MemoryFactoryEventSink,
} from "@you-agent-factory/factory-emulator";
import {
  createFactoryEmulator,
  createFactoryEmulatorSession,
  createFactoryRecordingSink,
  createMemoryFactoryEventSink,
} from "@you-agent-factory/factory-emulator";
import type {
  FactoryEmulatorSession,
  FactoryEmulatorSessionAdvanceReceipt,
  FactoryEmulatorSessionStatus,
  FactoryEmulatorSessionState,
  FactoryEmulatorStartReceipt,
  FactoryEmulatorSubmitReceipt,
} from "@you-agent-factory/factory-emulator";
import type { FactoryEvent } from "@you-agent-factory/client";

type Equal<Left, Right> =
  (<Value>() => Value extends Left ? 1 : 2) extends <
    Value,
  >() => Value extends Right ? 1 : 2
    ? true
    : false;
type Assert<Value extends true> = Value;

type _BatchUsesCanonicalEvents = Assert<
  Equal<FactoryEventBatch["events"][number], FactoryEvent>
>;

declare const sink: FactoryEventSink;
declare const batch: FactoryEventBatch;

const writeReceipt: Promise<FactoryEventSinkWriteReceipt> = sink.write(batch);
const closeReceipt: Promise<FactoryEventSinkCloseReceipt> = sink.close();
void writeReceipt;
void closeReceipt;

const acceptedReceipt: FactoryEventSinkWriteReceipt = { status: "accepted" };
const closedReceipt: FactoryEventSinkCloseReceipt = { status: "closed" };
void acceptedReceipt;
void closedReceipt;

// A successful batch receipt cannot describe partial acceptance.
// @ts-expect-error The contract exposes no partial-success receipt.
const partialReceipt: FactoryEventSinkWriteReceipt = { status: "partial" };
void partialReceipt;

const memorySink: MemoryFactoryEventSink = createMemoryFactoryEventSink({
  maxEvents: 1,
});
const memoryHistory: readonly FactoryEventBatch[] = memorySink.batches();
void memoryHistory;

const recordingSink: FactoryRecordingSink = createFactoryRecordingSink({
  maxEvents: 1,
  sessionId: "session-1",
});
const recordingEvents: readonly FactoryEvent[] = recordingSink.recording().events;
void recordingEvents;

const emulator: FactoryEmulator<{ readonly count: number }> = createFactoryEmulator({
  initialState: { count: 0 },
  sink,
  calculateTick(state): FactoryEmulatorTick<{ readonly count: number }> {
    return { batch, state: { count: state.count + 1 } };
  },
});
const advanceReceipt: Promise<FactoryEmulatorAdvanceReceipt> = emulator.advance();
void advanceReceipt;

const emulatorStatus: FactoryEmulatorStatus = emulator.status();
const pendingBatch: FactoryEventBatch | undefined = emulator.pending();
const emulatorCloseReceipt: Promise<FactoryEmulatorCloseReceipt> = emulator.close();
void emulatorStatus;
void pendingBatch;
void emulatorCloseReceipt;

const emulatorSession: FactoryEmulatorSession = createFactoryEmulatorSession({
  factory: { name: "public-contract" },
  scenario: {
    version: "you-agent-factory.emulator.scenario.v1",
    id: "public-contract",
    seed: "public-seed",
    startAt: "2026-07-18T07:30:00Z",
    rules: [],
    unmatchedBehavior: { kind: "ignore" },
  },
  sink,
});
const sessionState: FactoryEmulatorSessionState = emulatorSession.state();
const startReceipt: Promise<FactoryEmulatorStartReceipt> = emulatorSession.start();
const submitReceipt: Promise<FactoryEmulatorSubmitReceipt> = emulatorSession.submit({
  id: "public-work",
  workType: "task",
});
const sessionStatus: FactoryEmulatorSessionStatus = emulatorSession.status();
const sessionAdvanceReceipt: Promise<FactoryEmulatorSessionAdvanceReceipt> =
  emulatorSession.advanceBy(5);
const sessionNextReceipt: Promise<FactoryEmulatorSessionAdvanceReceipt> =
  emulatorSession.advanceToNext();
void sessionState;
void startReceipt;
void submitReceipt;
void sessionStatus;
void sessionAdvanceReceipt;
void sessionNextReceipt;

import {
  parseEmulatorScenario,
  emulatorScenarioExamples,
  inspectEmulatorSupport,
  resolveEmulatorScenarioResult,
  selectEmulatorRule,
  type EmulatorScenarioResolution,
  type EmulatorScenarioParseResult,
  type EmulatorScenario,
  type EmulatorScenarioVersion,
  scenarioSchema,
  SUPPORTED_SCENARIO_VERSION,
} from "@you-agent-factory/factory-emulator";

const version: EmulatorScenarioVersion = SUPPORTED_SCENARIO_VERSION;
const scenario: EmulatorScenario = {
  version,
  id: "public-contract",
  seed: "seed-public",
  startAt: "2026-07-18T07:30:00Z",
  rules: [
    {
      id: "match-all",
      match: { kind: "all" },
      outcomes: [{ kind: "complete" }],
      exhaustionBehavior: { kind: "useUnmatchedBehavior" },
    },
  ],
  unmatchedBehavior: { kind: "ignore" },
};

void scenario;
void scenarioSchema;
void emulatorScenarioExamples;
void inspectEmulatorSupport;

const parseResult: EmulatorScenarioParseResult = parseEmulatorScenario(
  scenario,
  { workstations: [{ behavior: "STANDARD" }] },
);
void parseResult;

const rule = selectEmulatorRule(scenario, { id: "public-1", workType: "task" });
const resolution: EmulatorScenarioResolution = resolveEmulatorScenarioResult(
  scenario,
  { id: "public-1", workType: "task" },
  0,
);
void rule;
void resolution;
