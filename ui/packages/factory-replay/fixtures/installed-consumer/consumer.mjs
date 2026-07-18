import {
  projectFactoryStateAtTick,
  projectFactoryWorkProgress,
} from "@you-agent-factory/factory-replay";

const events = [
  {
    schemaVersion: "agent-factory.event.v1",
    id: "later",
    type: "WORK_REQUEST",
    context: { eventTime: "2026-07-18T00:00:02Z", sequence: 0, tick: 2 },
    payload: { type: "FACTORY_REQUEST_BATCH", works: [] },
  },
  {
    schemaVersion: "agent-factory.event.v1",
    id: "selected",
    type: "WORK_REQUEST",
    context: { eventTime: "2026-07-18T00:00:01Z", sequence: 0, tick: 1 },
    payload: { type: "FACTORY_REQUEST_BATCH", works: [] },
  },
];
const replay = projectFactoryStateAtTick({
  events,
  tick: 1,
  reducer: {
    createState: (selectedTick) => ({ ids: [], selectedTick }),
    applyEvent: (state, event) => ({ ...state, ids: [...state.ids, event.id] }),
  },
});
const progress = projectFactoryWorkProgress({
  activeWorkIds: [],
  selectedTick: replay.selectedTick,
  works: [
    { id: "failed", state: { category: "FAILED" } },
    { id: "queued", state: { category: "INITIAL" } },
  ],
});
if (replay.appliedEvents.map(({ id }) => id).join(",") !== "selected") {
  throw new Error("packed replay did not reconstruct the selected tick");
}
if (
  progress.total !== 2 ||
  progress.counts.failed !== 1 ||
  progress.counts.queued !== 1
) {
  throw new Error("packed replay did not classify Work exclusively");
}
process.stdout.write("reconstructed tick 1 and classified 2 Work items\n");
