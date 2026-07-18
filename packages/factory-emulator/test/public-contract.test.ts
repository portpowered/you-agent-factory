import type {
  FactoryEventBatch,
  FactoryEventSink,
  FactoryEventSinkCloseReceipt,
  FactoryEventSinkWriteReceipt,
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
