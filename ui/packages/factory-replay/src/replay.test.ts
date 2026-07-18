import type { FactoryEvent, FactoryEventType } from "@you-agent-factory/client";
import { describe, expect, it } from "vitest";

import {
  advanceFactoryReplayCheckpoint,
  canonicalizeFactoryEvents,
  type FactoryReplayReducer,
  initializeFactoryReplay,
  projectFactoryStateAtTick,
} from "./index.js";

interface EvidenceState {
  appliedIds: string[];
  selectedTick: number;
  workIds: string[];
}

const reducer: FactoryReplayReducer<EvidenceState> = {
  cloneState: (state, selectedTick) => ({
    ...structuredClone(state),
    selectedTick,
  }),
  createState: (selectedTick) => ({
    appliedIds: [],
    selectedTick,
    workIds: [],
  }),
  applyEvent: (state, event) => {
    const payload = event.payload as Record<string, unknown>;
    const works = Array.isArray(payload.works) ? payload.works : [];
    const workIds = works.flatMap((work) => {
      if (work === null || typeof work !== "object") return [];
      const workId = (work as Record<string, unknown>).workId;
      return typeof workId === "string" ? [workId] : [];
    });
    return {
      ...state,
      appliedIds: [...state.appliedIds, event.id],
      workIds: [...state.workIds, ...workIds],
    };
  },
};

function event(
  id: string,
  tick: number,
  sequence: number,
  options: {
    eventTime?: string;
    eventType?: FactoryEventType;
    sessionSequence?: number;
    works?: unknown[];
  } = {},
): FactoryEvent {
  return {
    schemaVersion: "agent-factory.event.v1",
    id,
    type: options.eventType ?? "WORK_REQUEST",
    context: {
      sequence,
      tick,
      eventTime:
        options.eventTime ??
        `2026-07-18T00:00:${String(sequence).padStart(2, "0")}Z`,
      ...(options.sessionSequence === undefined
        ? {}
        : { sessionSequence: options.sessionSequence }),
    },
    payload: {
      type: "FACTORY_REQUEST_BATCH",
      ...(options.works === undefined ? {} : { works: options.works }),
    },
  } as FactoryEvent;
}

describe("canonicalizeFactoryEvents", () => {
  it("orders out-of-order input by tick and effective same-tick sequence", () => {
    const input = [
      event("later-tick", 2, 1, { sessionSequence: 1 }),
      event("later-in-tick", 1, 2, { sessionSequence: 8 }),
      event("earlier-in-tick", 1, 9, { sessionSequence: 3 }),
    ];
    const original = structuredClone(input);

    expect(canonicalizeFactoryEvents(input).map(({ id }) => id)).toEqual([
      "earlier-in-tick",
      "later-in-tick",
      "later-tick",
    ]);
    expect(input).toEqual(original);
  });

  it("uses canonical ties and accepts a repeated ID once in either arrival order", () => {
    const beta = event("beta", 1, 4, {
      eventTime: "2026-07-18T00:00:04Z",
    });
    const alpha = event("alpha", 1, 4, {
      eventTime: "2026-07-18T00:00:04Z",
    });
    const duplicateLater = event("duplicate", 3, 9);
    const duplicateEarlier = event("duplicate", 2, 8);

    const forward = canonicalizeFactoryEvents([
      beta,
      duplicateLater,
      alpha,
      duplicateEarlier,
    ]);
    const reverse = canonicalizeFactoryEvents([
      duplicateEarlier,
      alpha,
      duplicateLater,
      beta,
    ]);

    expect(forward.map(({ id }) => id)).toEqual(["alpha", "beta", "duplicate"]);
    expect(reverse).toEqual(forward);
    expect(forward[2]?.context.tick).toBe(2);

    const forwardReplay = initializeFactoryReplay({
      events: [beta, duplicateLater, alpha, duplicateEarlier],
      reducer,
      selection: { mode: "current" },
    });
    const reverseReplay = initializeFactoryReplay({
      events: [duplicateEarlier, alpha, duplicateLater, beta],
      reducer,
      selection: { mode: "current" },
    });
    expect(reverseReplay.state).toEqual(forwardReplay.state);
  });
});

describe("Factory replay selection", () => {
  it("initializes empty history at baseline tick zero", () => {
    expect(
      initializeFactoryReplay({
        events: [],
        reducer,
        selection: { mode: "current" },
      }),
    ).toEqual({
      appliedEvents: [],
      acceptedEventIds: new Set(),
      events: [],
      latestTick: 0,
      selectedTick: 0,
      selection: { mode: "current" },
      state: { appliedIds: [], selectedTick: 0, workIds: [] },
    });
  });

  it("resolves current selection to the latest accepted tick", () => {
    const result = initializeFactoryReplay({
      events: [event("latest", 7, 2), event("earliest", 2, 1)],
      reducer,
      selection: { mode: "current" },
    });

    expect(result.selectedTick).toBe(7);
    expect(result.latestTick).toBe(7);
    expect(result.state.appliedIds).toEqual(["earliest", "latest"]);
  });

  it("reconstructs a fixed historical tick without applying future events", () => {
    const result = projectFactoryStateAtTick({
      events: [event("future", 6, 3), event("included", 3, 2)],
      reducer,
      tick: 3,
    });

    expect(result.events.map(({ id }) => id)).toEqual(["included", "future"]);
    expect(result.appliedEvents.map(({ id }) => id)).toEqual(["included"]);
    expect(result.state).toEqual({
      appliedIds: ["included"],
      selectedTick: 3,
      workIds: [],
    });
  });

  it("passes incomplete optional payload evidence without crashing or inventing work", () => {
    const incomplete = event("incomplete", 1, 1);
    const partial = event("partial", 2, 2, {
      works: [null, {}, { workId: "known-work" }],
    });

    const result = projectFactoryStateAtTick({
      events: [partial, incomplete],
      reducer,
      tick: 2,
    });

    expect(result.state.workIds).toEqual(["known-work"]);
    expect(result.state.appliedIds).toEqual(["incomplete", "partial"]);
  });
});

describe("Factory replay checkpoint advancement", () => {
  function checkpoint(events: readonly FactoryEvent[], tick: number) {
    return projectFactoryStateAtTick({ events, reducer, tick });
  }

  it("returns an independent result for an empty tail", () => {
    const original = checkpoint([event("accepted", 1, 1)], 1);
    const advanced = advanceFactoryReplayCheckpoint({
      checkpoint: original,
      reducer,
      tail: [],
      tick: 1,
    });

    expect(advanced.state).toEqual(original.state);
    expect(advanced.state).not.toBe(original.state);
    expect(advanced.acceptedEventIds).toEqual(new Set(["accepted"]));
  });

  it("ignores checkpoint IDs, duplicate tail IDs, and future events", () => {
    const original = checkpoint([event("accepted", 1, 1)], 1);
    const advanced = advanceFactoryReplayCheckpoint({
      checkpoint: original,
      reducer,
      tail: [
        event("future", 4, 4),
        event("tail", 2, 3),
        event("accepted", 2, 2),
        event("tail", 3, 5),
      ],
      tick: 2,
    });

    expect(advanced.state.appliedIds).toEqual(["accepted", "tail"]);
    expect(advanced.events.map(({ id }) => id)).toEqual(["accepted", "tail"]);
    expect(advanced.acceptedEventIds).toEqual(new Set(["accepted", "tail"]));
  });

  it("accepts a later same-tick sequence not represented by the checkpoint", () => {
    const original = checkpoint([event("first", 3, 4)], 3);
    const advanced = advanceFactoryReplayCheckpoint({
      checkpoint: original,
      reducer,
      tail: [event("second", 3, 8)],
      tick: 3,
    });

    expect(advanced.state.appliedIds).toEqual(["first", "second"]);
    expect(advanced.selectedTick).toBe(3);
  });

  it("keeps checkpoint IDs, events, and reachable state isolated from the result", () => {
    const original = checkpoint([event("accepted", 1, 1)], 1);
    const originalEvent = structuredClone(original.appliedEvents[0]);
    const advanced = advanceFactoryReplayCheckpoint({
      checkpoint: original,
      reducer,
      tail: [event("tail", 2, 2)],
      tick: 2,
    });

    advanced.acceptedEventIds.add("caller-mutation");
    advanced.state.appliedIds.push("caller-mutation");
    const advancedPayload = advanced.appliedEvents[0]?.payload as Record<
      string,
      unknown
    >;
    advancedPayload.callerMutation = true;

    expect(original.acceptedEventIds).toEqual(new Set(["accepted"]));
    expect(original.state.appliedIds).toEqual(["accepted"]);
    expect(original.appliedEvents[0]).toEqual(originalEvent);
  });

  it("matches full replay for representative Factory lifecycle, Work, and Dispatch tails", () => {
    const factoryStarted = event("factory-started", 1, 1, {
      eventType: "SESSION_STARTED",
    });
    const workAccepted = event("work-accepted", 2, 2, {
      eventType: "WORK_REQUEST",
      works: [{ workId: "work-1" }],
    });
    const dispatchStarted = event("dispatch-started", 3, 3, {
      eventType: "DISPATCH_REQUEST",
    });
    const dispatchCompleted = event("dispatch-completed", 4, 4, {
      eventType: "DISPATCH_RESPONSE",
    });
    const history = [
      factoryStarted,
      workAccepted,
      dispatchStarted,
      dispatchCompleted,
    ];
    const original = checkpoint(history.slice(0, 2), 2);
    const advanced = advanceFactoryReplayCheckpoint({
      checkpoint: original,
      reducer,
      tail: [dispatchCompleted, dispatchStarted],
      tick: 4,
    });
    const full = projectFactoryStateAtTick({
      events: history,
      reducer,
      tick: 4,
    });

    expect(advanced.state).toEqual(full.state);
    expect(advanced.events).toEqual(full.appliedEvents);
    expect(advanced.acceptedEventIds).toEqual(full.acceptedEventIds);
  });
});
