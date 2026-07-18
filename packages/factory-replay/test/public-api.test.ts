import type { FactoryEvent } from "@you-agent-factory/client";
import {
  initializeFactoryReplay,
  type FactoryReplayReducer,
} from "../src/index.js";

interface ReplayState {
  ids: string[];
}

const reducer: FactoryReplayReducer<ReplayState, readonly string[]> = {
  createState: () => ({ ids: [] }),
  applyEvent: (state, event) => ({ ids: [...state.ids, event.id] }),
  projectWorld: (state) => state.ids,
};

const event = {
  context: {
    eventTime: "2026-07-18T05:00:00Z",
    sequence: 1,
    tick: 1,
  },
  id: "event-1",
  payload: {},
  type: "SESSION_STARTED",
} as FactoryEvent;

const result = initializeFactoryReplay({
  events: [event],
  reducer,
  selection: { mode: "current" },
});

void result.world;
