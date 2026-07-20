import type { FactoryEvent } from "@you-agent-factory/client";
import {
  createFactoryEmulatorSession,
  type FactoryEmulatorSession,
  type FactoryEmulatorSessionState,
  inspectFactoryEmulatorCompatibility,
  safeParseFactoryEmulatorScenario,
} from "@you-agent-factory/factory-emulator";
import { describe, expect, it } from "vitest";

import {
  type CustomerFactoryEmulatorDemoFixture,
  customerFactoryEmulatorDemoFixtures,
} from "./customer-demo-fixtures";

interface CompletedDemoRun {
  readonly events: readonly FactoryEvent[];
  readonly state: Extract<
    FactoryEmulatorSessionState,
    { readonly lifecycle: "started" }
  >;
}

function createDemoSession(
  fixture: CustomerFactoryEmulatorDemoFixture,
  events: FactoryEvent[],
): FactoryEmulatorSession {
  return createFactoryEmulatorSession({
    factory: fixture.factory,
    scenario: fixture.scenario,
    sink: {
      write: async (batch) => {
        events.push(...structuredClone(batch));
      },
    },
  });
}

async function finishDemo(session: FactoryEmulatorSession) {
  for (let command = 0; command < 20; command += 1) {
    if (session.status().phase === "idle") return;
    await session.advanceToNext();
  }
  throw new Error("Customer demo did not become idle within 20 advances.");
}

async function runDemo(
  fixture: CustomerFactoryEmulatorDemoFixture,
): Promise<CompletedDemoRun> {
  const events: FactoryEvent[] = [];
  const session = createDemoSession(fixture, events);
  await session.start();
  await finishDemo(session);
  const state = session.state();
  if (state.lifecycle !== "started") {
    throw new Error("Completed customer demo must remain inspectable.");
  }
  return { events, state };
}

function dispatchEvidence(events: readonly FactoryEvent[]) {
  return events
    .filter(({ type }) => type === "DISPATCH_RESPONSE")
    .map((event) => {
      const request = events.find(
        ({ context, type }) =>
          type === "DISPATCH_REQUEST" &&
          context.dispatchId === event.context.dispatchId,
      );
      const payload = event.payload as {
        outcome?: string;
        outputWork?: readonly { state: { name: string } }[];
        transitionId?: string;
      };
      return {
        durationMs:
          Date.parse(event.context.eventTime) -
          Date.parse(request?.context.eventTime ?? ""),
        elapsedMs: Date.parse(event.context.eventTime),
        outcome: payload.outcome,
        outputStates: payload.outputWork?.map(({ state }) => state.name) ?? [],
        tick: event.context.tick,
        workstation: payload.transitionId,
      };
    });
}

describe("customer Factory emulator demo fixtures", () => {
  it.each(Object.values(customerFactoryEmulatorDemoFixtures))(
    "validates the $id Factory/scenario pair against the supported subset",
    (fixture) => {
      expect(inspectFactoryEmulatorCompatibility(fixture.factory)).toEqual({
        diagnostics: [],
        supported: true,
      });
      expect(
        safeParseFactoryEmulatorScenario(fixture.scenario, fixture.factory),
      ).toMatchObject({ success: true });
      expect(() => createDemoSession(fixture, [])).not.toThrow();
    },
  );

  it("finishes the success demo with one 1,500 ms accepted Execute outcome", async () => {
    const fixture = customerFactoryEmulatorDemoFixtures.success;
    const run = await runDemo(fixture);
    const startAt = Date.parse(fixture.scenario.startAt);

    expect(dispatchEvidence(run.events)).toEqual([
      {
        durationMs: 1_500,
        elapsedMs: startAt + 1_500,
        outcome: "ACCEPTED",
        outputStates: ["done"],
        tick: 2,
        workstation: "Execute",
      },
    ]);
    expect(run.state.virtualElapsedMs).toBe(1_500);
    expect(run.state.works.at(-1)).toMatchObject({
      phase: "completed",
      state: "done",
    });
  });

  it("follows the exact repeat, review, rework, and terminal-failure sequence", async () => {
    const fixture = customerFactoryEmulatorDemoFixtures.repeatReviewFailure;
    const run = await runDemo(fixture);
    const startAt = Date.parse(fixture.scenario.startAt);

    expect(dispatchEvidence(run.events)).toEqual([
      {
        durationMs: 1_500,
        elapsedMs: startAt + 1_500,
        outcome: "CONTINUE",
        outputStates: ["ready"],
        tick: 2,
        workstation: "Execute",
      },
      {
        durationMs: 1_500,
        elapsedMs: startAt + 3_000,
        outcome: "ACCEPTED",
        outputStates: ["review"],
        tick: 4,
        workstation: "Execute",
      },
      {
        durationMs: 1_000,
        elapsedMs: startAt + 4_000,
        outcome: "REJECTED",
        outputStates: ["ready"],
        tick: 6,
        workstation: "Review",
      },
      {
        durationMs: 1_500,
        elapsedMs: startAt + 5_500,
        outcome: "ACCEPTED",
        outputStates: ["review"],
        tick: 8,
        workstation: "Execute",
      },
      {
        durationMs: 1_000,
        elapsedMs: startAt + 6_500,
        outcome: "FAILED",
        outputStates: ["failed"],
        tick: 10,
        workstation: "Review",
      },
    ]);
    expect(run.state.virtualElapsedMs).toBe(6_500);
    expect(Object.values(run.state.ruleCursors).sort()).toEqual([2, 3]);
    expect(
      new Set(run.state.works.map(({ rootWorkId }) => rootWorkId)),
    ).toHaveProperty("size", 1);
    expect(run.state.works.at(-1)).toMatchObject({
      phase: "completed",
      state: "failed",
    });
  });

  it.each(Object.values(customerFactoryEmulatorDemoFixtures))(
    "reproduces $id canonical history and projection after reset",
    async (fixture) => {
      const events: FactoryEvent[] = [];
      const session = createDemoSession(fixture, events);
      await session.start();
      await finishDemo(session);
      const firstHistory = structuredClone(events);
      const firstState = structuredClone(session.state());

      session.reset();
      events.length = 0;
      await session.start();
      await finishDemo(session);

      expect(JSON.stringify(events)).toBe(JSON.stringify(firstHistory));
      expect(session.state()).toEqual(firstState);
    },
  );
});
