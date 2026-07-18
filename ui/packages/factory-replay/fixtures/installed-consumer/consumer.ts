import {
  type FactoryReplayReducer,
  projectFactoryStateAtTick,
  projectFactoryWorkProgress,
} from "@you-agent-factory/factory-replay";

type ReplayEvent = Parameters<
  typeof projectFactoryStateAtTick
>[0]["events"][number];
interface State {
  ids: string[];
  selectedTick: number;
}
const reducer: FactoryReplayReducer<State> = {
  createState: (selectedTick) => ({ ids: [], selectedTick }),
  applyEvent: (state, event) => ({ ...state, ids: [...state.ids, event.id] }),
};
declare const events: readonly ReplayEvent[];
const replay = projectFactoryStateAtTick({ events, reducer, tick: 1 });
const progress = projectFactoryWorkProgress({
  activeWorkIds: [],
  selectedTick: replay.selectedTick,
  works: [{ id: "queued", state: { category: "INITIAL" } }],
});
void progress;
