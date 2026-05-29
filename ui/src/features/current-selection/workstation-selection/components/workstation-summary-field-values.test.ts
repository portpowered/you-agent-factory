import { describe, expect, it } from "vitest";

import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import {
  resolveWorkstationSummaryRunnerValue,
  resolveWorkstationSummaryTypeValue,
} from "./workstation-summary-field-values";

describe("resolveWorkstationSummaryTypeValue", () => {
  const messages = getWorkstationDetailMessages("en");

  it("localizes the authoritative workstation type when editable configuration is ready", () => {
    expect(
      resolveWorkstationSummaryTypeValue(
        {
          draft: {
            behavior: "STANDARD",
            prompt: "Review",
            runnerName: null,
            workerName: "reviewer",
          },
          hasValidationErrors: false,
          initialValues: {
            behavior: "STANDARD",
            behaviorOptions: ["STANDARD"],
            effectiveRunnerName: "codex",
            factoryRunnerName: null,
            prompt: "Review",
            runnerName: null,
            runnerOptions: ["codex"],
            sharedWorkerWorkstationNamesByWorkerName: {},
            sharedWorkerWorkstationNames: [],
            workerName: "reviewer",
            workerOptions: ["reviewer"],
            workerTypeByName: {},
            workstationName: "Review",
            workstationType: "MODEL_WORKSTATION",
          },
          isDirty: false,
          markChangesSaved: () => {},
          baseVersion: { logical: "1", physical: "2026-01-01T00:00:00Z" },
          onBehaviorChange: () => {},
          onPromptChange: () => {},
          onResetToLatest: () => {},
          onRunnerChange: () => {},
          onWorkerChange: () => {},
          overwriteFieldNames: [],
          pendingFactoryDefinition: null,
          promptDiagnostics: [],
          promptHelpState: { status: "loading" },
          promptValidationState: { status: "idle" },
          status: "ready",
          validationErrors: {},
          workerOptionsState: { options: ["reviewer"], status: "ready" },
        },
        messages,
      ),
    ).toBe("Model workstation");
  });

  it("returns loading and unavailable copy for non-ready editable configuration states", () => {
    expect(
      resolveWorkstationSummaryTypeValue({ status: "loading" }, messages),
    ).toBe("Loading workstation type...");
    expect(
      resolveWorkstationSummaryTypeValue(
        { errorMessage: "Factory unavailable.", status: "error" },
        messages,
      ),
    ).toBe("Workstation type unavailable");
    expect(resolveWorkstationSummaryTypeValue(undefined, messages)).toBe(
      "Loading workstation type...",
    );
  });
});

describe("resolveWorkstationSummaryRunnerValue", () => {
  it("re-exports the runner summary resolver", () => {
    expect(resolveWorkstationSummaryRunnerValue).toBeTypeOf("function");
  });
});
