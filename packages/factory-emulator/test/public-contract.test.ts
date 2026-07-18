import type {
  FactoryEmulator,
  FactoryEmulatorAdvanceReceipt,
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
  createFactoryRecordingSink,
  createMemoryFactoryEventSink,
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
