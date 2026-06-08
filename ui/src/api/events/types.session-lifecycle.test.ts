import type { FactoryEvent } from "./types";
import { FACTORY_EVENT_TYPES } from "./types";

const eventTime = "2026-04-18T12:30:00Z";

describe("factory session lifecycle event types", () => {
  it("exposes session lifecycle event variants with shared context identity fields", () => {
    const started: FactoryEvent = {
      context: {
        checkpointId: "ckpt-2",
        dispatchId: "dispatch-child-1",
        eventTime,
        orchestratorDialect: "workflow-v1",
        orchestratorKind: "JAVASCRIPT",
        phaseId: "phase-plan",
        phaseName: "plan",
        sequence: 15,
        sessionId: "session-alpha",
        sessionSequence: 0,
        source: "api",
        tick: 4,
      },
      id: "event-session-started",
      payload: {
        argsDigest: "sha256:args",
        factoryId: "factory-alpha",
        policyHash: "sha256:policy",
        sourceHash: "sha256:source",
        sourceRef: "workflow/main.js",
        startedAt: eventTime,
      },
      schemaVersion: "agent-factory.event.v1",
      type: FACTORY_EVENT_TYPES.sessionStarted,
    };

    expect(started.type).toBe("SESSION_STARTED");
    expect(FACTORY_EVENT_TYPES.artifactCreated).toBe("ARTIFACT_CREATED");
    expect(started.context.sessionId).toBe("session-alpha");
    expect(started.context.sessionSequence).toBe(0);
    expect(started.context.orchestratorKind).toBe("JAVASCRIPT");
    expect(started.payload).not.toHaveProperty("sessionId");
  });
});
