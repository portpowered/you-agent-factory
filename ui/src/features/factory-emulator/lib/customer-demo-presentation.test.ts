import type { FactoryEvent } from "@you-agent-factory/client";
import { describe, expect, it } from "vitest";

import { customerFactoryEmulatorDemoFixtures } from "./customer-demo-fixtures";
import { selectCustomerFactoryEmulatorActivity } from "./customer-demo-presentation";

function dispatchEvent(
  type: "DISPATCH_REQUEST" | "DISPATCH_RESPONSE",
  tick: number,
): FactoryEvent {
  return {
    schemaVersion: "agent-factory.event.v1",
    id: `${type}-${tick}`,
    type,
    context: {
      dispatchId: "dispatch-1",
      eventTime: `2026-07-19T16:00:0${tick}.000Z`,
      sequence: tick,
      tick,
    },
    payload:
      type === "DISPATCH_REQUEST"
        ? { inputs: [], transitionId: "Execute" }
        : {
            completionId: "completion-1",
            durationMillis: 1_500,
            outcome: "ACCEPTED",
            transitionId: "Execute",
          },
  } as FactoryEvent;
}

describe("customer Factory emulator demo presentation", () => {
  it("uses transient scenario activity only at the live head", () => {
    const fixture = customerFactoryEmulatorDemoFixtures.success;
    const events = [dispatchEvent("DISPATCH_REQUEST", 1)];

    expect(
      selectCustomerFactoryEmulatorActivity(fixture, events, 1, true),
    ).toEqual({
      activityLabel: "Preparing the launch summary",
      durationMs: 1_500,
      workstation: "Execute",
    });
    expect(
      selectCustomerFactoryEmulatorActivity(fixture, events, 1, false),
    ).toEqual({ durationMs: 1_500, workstation: "Execute" });
  });

  it("does not retain completed activity at a later replay tick", () => {
    const fixture = customerFactoryEmulatorDemoFixtures.success;
    const events = [
      dispatchEvent("DISPATCH_REQUEST", 1),
      dispatchEvent("DISPATCH_RESPONSE", 2),
    ];

    expect(
      selectCustomerFactoryEmulatorActivity(fixture, events, 2, true),
    ).toBeUndefined();
  });
});
