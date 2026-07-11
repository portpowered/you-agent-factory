import { describe, expect, it } from "vitest";

import {
  type FactorySessionLifecycleActionID,
  resolveFactorySessionLifecycleActionAvailability,
} from "./factory-session-lifecycle-controls";

function expectActions(
  actions: FactorySessionLifecycleActionID[],
  expected: FactorySessionLifecycleActionID[],
) {
  expect(actions).toEqual(expected);
}

describe("factory session lifecycle action availability", () => {
  it("shows pause, cancel, and terminate for a running durable session", () => {
    const availability = resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus: "RUNNING",
      isDurableSession: true,
      selectedDispatchID: null,
    });

    expectActions(availability.actions, ["pause", "cancel", "terminate"]);
    expect(availability.showDispatchSelectionHint).toBe(false);
    expect(availability.showEmptyState).toBe(false);
  });

  it("shows resume, cancel, and terminate for a paused durable session", () => {
    const availability = resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus: "PAUSED",
      isDurableSession: true,
      selectedDispatchID: null,
    });

    expectActions(availability.actions, ["resume", "cancel", "terminate"]);
  });

  it("shows approve, cancel, and terminate for an awaiting-approval durable session", () => {
    const availability = resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus: "AWAITING_APPROVAL",
      isDurableSession: true,
      selectedDispatchID: null,
    });

    expectActions(availability.actions, ["approve", "cancel", "terminate"]);
  });

  it("shows retry dispatch only when a failed dispatch is selected", () => {
    const availability = resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus: "FAILED",
      dispatches: [
        {
          dispatchKind: "JAVASCRIPT_AGENT",
          id: "dispatch-failed",
          orchestratorKind: "JAVASCRIPT",
          sessionId: "dur-sess-js-failed-001",
          status: "FAILED",
        },
      ],
      isDurableSession: true,
      selectedDispatchID: "dispatch-failed",
    });

    expectActions(availability.actions, ["retry-dispatch"]);
    expect(availability.selectedDispatch?.id).toBe("dispatch-failed");
    expect(availability.showDispatchSelectionHint).toBe(false);
  });

  it("keeps retry hidden until a failed dispatch is selected", () => {
    const availability = resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus: "FAILED",
      dispatches: [
        {
          dispatchKind: "JAVASCRIPT_AGENT",
          id: "dispatch-failed",
          orchestratorKind: "JAVASCRIPT",
          sessionId: "dur-sess-js-failed-001",
          status: "FAILED",
        },
      ],
      isDurableSession: true,
      selectedDispatchID: null,
    });

    expectActions(availability.actions, []);
    expect(availability.showDispatchSelectionHint).toBe(true);
    expect(availability.showEmptyState).toBe(true);
  });

  it("shows interrupt dispatch only when a running dispatch is selected", () => {
    const runningDispatch = {
      dispatchKind: "JAVASCRIPT_AGENT" as const,
      id: "dispatch-running",
      orchestratorKind: "JAVASCRIPT" as const,
      sessionId: "dur-sess-js-running-001",
      status: "RUNNING" as const,
    };
    const unselected = resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus: "RUNNING",
      dispatches: [runningDispatch],
      isDurableSession: true,
      selectedDispatchID: null,
    });
    const selected = resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus: "RUNNING",
      dispatches: [runningDispatch],
      isDurableSession: true,
      selectedDispatchID: runningDispatch.id,
    });

    expectActions(unselected.actions, ["pause", "cancel", "terminate"]);
    expect(unselected.showDispatchSelectionHint).toBe(true);
    expectActions(selected.actions, [
      "pause",
      "cancel",
      "terminate",
      "interrupt-dispatch",
    ]);
    expect(selected.showDispatchSelectionHint).toBe(false);
  });

  it("shows no lifecycle actions for a terminal durable session without a failed dispatch selection", () => {
    const availability = resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus: "SUCCEEDED",
      isDurableSession: true,
      selectedDispatchID: null,
    });

    expectActions(availability.actions, []);
    expect(availability.showEmptyState).toBe(true);
  });

  it("shows no lifecycle actions or empty-state copy for non-durable javascript sessions", () => {
    const availability = resolveFactorySessionLifecycleActionAvailability({
      dispatches: [
        {
          dispatchKind: "JAVASCRIPT_AGENT",
          id: "dispatch-failed",
          orchestratorKind: "JAVASCRIPT",
          sessionId: "session-beta",
          status: "FAILED",
        },
      ],
      isDurableSession: false,
      selectedDispatchID: "dispatch-failed",
    });

    expectActions(availability.actions, []);
    expect(availability.showDispatchSelectionHint).toBe(false);
    expect(availability.showEmptyState).toBe(false);
  });
});
