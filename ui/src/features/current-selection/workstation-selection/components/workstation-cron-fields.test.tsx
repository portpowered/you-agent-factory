import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { EditableConfigurationCronFields } from "./workstation-cron-fields";

const messages = getWorkstationDetailMessages("en");

function buildCronReadyState(
  overrides: Partial<{
    cron: {
      expiryWindow: string;
      jitter: string;
      schedule: string;
      triggerAtStart: boolean;
    } | null;
    onCronExpiryWindowChange: (value: string) => void;
    onCronJitterChange: (value: string) => void;
    onCronScheduleChange: (value: string) => void;
    onCronTriggerAtStartChange: (value: boolean) => void;
    overwriteFieldNames: Array<
      "cronExpiryWindow" | "cronJitter" | "cronSchedule" | "cronTriggerAtStart"
    >;
    validationErrors: {
      cronExpiryWindow?: string;
      cronJitter?: string;
      cronSchedule?: string;
      cronTriggerAtStart?: string;
    };
  }> = {},
) {
  return {
    draft: {
      behavior: "CRON" as const,
      cron: overrides.cron ?? {
        expiryWindow: "45s",
        jitter: "5s",
        schedule: "0 9 * * 1-5",
        triggerAtStart: true,
      },
      prompt: "Review the story.",
      runnerName: "gemini",
      workerName: "reviewer",
    },
    hasValidationErrors: false,
    initialValues: {
      behavior: "CRON" as const,
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER", "CRON"],
      cron: {
        expiryWindow: "30s",
        jitter: "1s",
        schedule: "*/5 * * * *",
        triggerAtStart: false,
      },
      effectiveRunnerName: "gemini",
      factoryRunnerName: "codex",
      prompt: "Review the story.",
      resolvedRunnerSelection: {
        runnerId: "gemini",
        source: "workstation" as const,
      },
      runnerName: "gemini",
      runnerOptions: ["codex", "gemini"],
      runnerSelectionSource: "workstation" as const,
      sharedWorkerWorkstationNames: [],
      sharedWorkerWorkstationNamesByWorkerName: {},
      workerModelProvider: null,
      workerName: "reviewer",
      workerOptions: ["reviewer"],
      workerTypeByName: { reviewer: "MODEL_WORKER" },
      workstationName: "Review",
      workstationType: "MODEL_WORKSTATION",
    },
    isDirty: false,
    markChangesSaved: vi.fn(),
    baseVersion: { logical: "1", physical: "2026-01-01T00:00:00Z" },
    onBehaviorChange: vi.fn(),
    onCronExpiryWindowChange:
      overrides.onCronExpiryWindowChange ?? vi.fn(),
    onCronJitterChange: overrides.onCronJitterChange ?? vi.fn(),
    onCronScheduleChange: overrides.onCronScheduleChange ?? vi.fn(),
    onCronTriggerAtStartChange:
      overrides.onCronTriggerAtStartChange ?? vi.fn(),
    onPromptChange: vi.fn(),
    onResetToLatest: vi.fn(),
    onRunnerChange: vi.fn(),
    onWorkerChange: vi.fn(),
    overwriteFieldNames: overrides.overwriteFieldNames ?? [],
    pendingFactoryDefinition: null,
    promptDiagnostics: [],
    promptHelpState: { status: "idle" as const },
    status: "ready" as const,
    validationErrors: overrides.validationErrors ?? {},
    workerOptionsState: { status: "ready" as const, options: ["reviewer"] },
  };
}

describe("EditableConfigurationCronFields", () => {
  it("returns null for non-CRON behavior", () => {
    render(
      <EditableConfigurationCronFields
        messages={messages}
        state={{
          ...buildCronReadyState(),
          draft: {
            ...buildCronReadyState().draft,
            behavior: "STANDARD",
            cron: null,
          },
        }}
      />,
    );

    expect(screen.queryByLabelText("Cron schedule")).toBeNull();
  });

  it("wires jitter and expiry window changes and shows field validation errors", () => {
    const onCronJitterChange = vi.fn();
    const onCronExpiryWindowChange = vi.fn();

    render(
      <EditableConfigurationCronFields
        messages={messages}
        state={buildCronReadyState({
          onCronExpiryWindowChange,
          onCronJitterChange,
          overwriteFieldNames: ["cronExpiryWindow"],
          validationErrors: {
            cronExpiryWindow: 'expiry_window must be a positive duration, got "0s"',
            cronJitter: 'jitter must be a non-negative duration, got "bad"',
            cronSchedule: "cron workstation requires non-empty 'schedule'",
            cronTriggerAtStart: "trigger_at_start must be a boolean",
          },
        })}
      />,
    );

    fireEvent.change(screen.getByLabelText("Cron jitter"), {
      target: { value: "10s" },
    });
    fireEvent.change(screen.getByLabelText("Cron expiry window"), {
      target: { value: "2m" },
    });

    expect(onCronJitterChange).toHaveBeenCalledWith("10s");
    expect(onCronExpiryWindowChange).toHaveBeenCalledWith("2m");

    expect(
      screen.getByText("cron workstation requires non-empty 'schedule'"),
    ).toBeTruthy();
    expect(
      screen.getByText('jitter must be a non-negative duration, got "bad"'),
    ).toBeTruthy();
    expect(
      screen.getByText('expiry_window must be a positive duration, got "0s"'),
    ).toBeTruthy();
    expect(
      screen.getByText("trigger_at_start must be a boolean"),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "The running factory changed this field while you were editing. Reset to latest to discard the local draft value.",
      ),
    ).toBeTruthy();

    const scheduleInput = screen.getByLabelText("Cron schedule");
    expect(scheduleInput.getAttribute("aria-invalid")).toBe("true");
    expect(scheduleInput.getAttribute("aria-describedby")).toContain("error");
  });
});
