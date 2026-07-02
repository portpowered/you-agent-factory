import {
  formatFactoryOrchestratorKind,
  formatFactorySessionRuntimeStatus,
  formatFactorySessionScriptStatus,
  resolveFactoryDispatchStatusTone,
} from "./factory-session-runtime-display";

describe("factory session runtime display", () => {
  it("formats live runtime status labels", () => {
    expect(formatFactorySessionRuntimeStatus("ACTIVE")).toBe("Active");
    expect(formatFactorySessionRuntimeStatus("IDLE")).toBe("Idle");
    expect(formatFactorySessionRuntimeStatus("FINISHED")).toBe("Finished");
  });

  it("prefers durable lifecycle status labels for durable sessions", () => {
    expect(
      formatFactorySessionRuntimeStatus("ACTIVE", "AWAITING_APPROVAL"),
    ).toBe("Awaiting approval");
    expect(formatFactorySessionRuntimeStatus("ACTIVE", "RUNNING")).toBe(
      "Running",
    );
  });

  it("formats orchestrator kind and script status with customer-facing labels", () => {
    expect(formatFactoryOrchestratorKind("JAVASCRIPT")).toBe(
      "JavaScript workflow",
    );
    expect(formatFactoryOrchestratorKind("PETRI")).toBe("Petri net");
    expect(formatFactorySessionScriptStatus("RUNNING")).toBe("Running");
    expect(formatFactorySessionScriptStatus("IDLE")).toBe("Idle");
  });

  it("maps dispatch status to semantic status-pill tones", () => {
    expect(resolveFactoryDispatchStatusTone({ status: "FAILED" })).toBe(
      "danger",
    );
    expect(
      resolveFactoryDispatchStatusTone({
        status: "COMPLETED",
        warningCount: 1,
      }),
    ).toBe("warning");
    expect(resolveFactoryDispatchStatusTone({ status: "COMPLETED" })).toBe(
      "success",
    );
    expect(resolveFactoryDispatchStatusTone({ status: "RUNNING" })).toBe(
      "active",
    );
    expect(resolveFactoryDispatchStatusTone({ status: "QUEUED" })).toBe("info");
  });
});
