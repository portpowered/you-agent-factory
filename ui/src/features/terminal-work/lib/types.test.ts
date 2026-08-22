import { terminalWorkIdentity, terminalWorkStatusFromOutcome } from "./types";

describe("terminal work identity and status normalization", () => {
  it("qualifies canonical Work identity with dispatch identity", () => {
    expect(
      terminalWorkIdentity({
        dispatchID: "dispatch-two",
        traceWorkID: "work-two",
      }),
    ).toBe("dispatch-two:work-two");
  });

  it.each([
    ["ACCEPTED", "completed"],
    ["FAILED", "failed"],
    ["CANCELLED", "canceled"],
    ["CANCELED", "canceled"],
    ["TERMINATED", "terminated"],
    ["future-terminal-value", "unknown"],
  ] as const)("maps %s to the truthful terminal status", (outcome, status) => {
    expect(terminalWorkStatusFromOutcome(outcome)).toBe(status);
  });

  it("does not invent a status when an outcome is absent", () => {
    expect(terminalWorkStatusFromOutcome(undefined)).toBeUndefined();
  });
});
