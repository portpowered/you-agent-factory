import { describe, expect, it } from "vitest";

import {
  isDurableTerminalLifecycleStatus,
  resolveDurableInspectionPresentation,
  resolveDurableResultStatus,
} from "./durable-inspection-state";
import type { FactorySessionDetailData } from "../hooks/use-factory-session-detail";

describe("durable inspection state", () => {
  it("treats terminal lifecycle and final result statuses as terminal presentation", () => {
    const terminalData: Extract<FactorySessionDetailData, { kind: "durable" }> =
      {
        durableResult: {
          resultStatus: "FINAL",
          sessionId: "dur-sess-petri-success-001",
        },
        kind: "durable",
        session: {
          orchestratorKind: "PETRI",
          resolvedSource: { kind: "FACTORY_ID" },
          sessionId: "dur-sess-petri-success-001",
          status: "SUCCEEDED",
        },
      };

    expect(resolveDurableInspectionPresentation(terminalData)).toBe("terminal");
    expect(isDurableTerminalLifecycleStatus("SUCCEEDED")).toBe(true);
    expect(resolveDurableResultStatus(terminalData)).toBe("FINAL");
  });

  it("treats running sessions without a final result as partial presentation", () => {
    const partialData: Extract<FactorySessionDetailData, { kind: "durable" }> =
      {
        durablePartialResult: {
          mode: "partial",
          resultStatus: "PARTIAL",
          sessionId: "dur-sess-js-run-n-001",
        },
        kind: "durable",
        session: {
          orchestratorKind: "JAVASCRIPT",
          phase: "review",
          resolvedSource: { kind: "INLINE_WORKFLOW" },
          resultSummary: {
            resultStatus: "PARTIAL",
            summary: "Review checkpoint saved.",
          },
          sessionId: "dur-sess-js-run-n-001",
          status: "RUNNING",
        },
      };

    expect(resolveDurableInspectionPresentation(partialData)).toBe("partial");
    expect(isDurableTerminalLifecycleStatus("RUNNING")).toBe(false);
    expect(resolveDurableResultStatus(partialData)).toBe("PARTIAL");
  });
});
