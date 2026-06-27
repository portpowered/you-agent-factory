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
      selectedDispatchID: null,
    });

    expectActions(availability.actions, ["pause", "cancel", "terminate"]);
    expect(availability.showDispatchSelectionHint).toBe(false);
    expect(availability.showEmptyState).toBe(false);
  });

  it("shows resume, cancel, and terminate for a paused durable session", () => {
    const availability = resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus: "PAUSED",
      selectedDispatchID: null,
    });

    expectActions(availability.actions, ["resume", "cancel", "terminate"]);
  });

  it("shows approve, cancel, and terminate for an awaiting-approval durable session", () => {
    const availability = resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus: "AWAITING_APPROVAL",
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
      selectedDispatchID: null,
    });

    expectActions(availability.actions, []);
    expect(availability.showDispatchSelectionHint).toBe(true);
    expect(availability.showEmptyState).toBe(true);
  });

  it("shows no lifecycle actions for a terminal durable session without a failed dispatch selection", () => {
    const availability = resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus: "SUCCEEDED",
      selectedDispatchID: null,
    });

    expectActions(availability.actions, []);
    expect(availability.showEmptyState).toBe(true);
  });
});
