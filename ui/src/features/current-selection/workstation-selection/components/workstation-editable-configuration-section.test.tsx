import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { FactoryDefinition } from "../../../../api/factory-definition/api";
import { expectNoInlineSaveOutcomesIn } from "../../base/components/current-selection-save-toast-test-helpers";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { EditableConfigurationSection } from "./workstation-editable-configuration-section";

const messages = getWorkstationDetailMessages();

function expandConfiguration() {
  fireEvent.click(
    screen.getByRole("button", { name: "Expand editable configuration" }),
  );
}

function buildReadyState(
  overrides?: Partial<{
    draft: {
      behavior?: "STANDARD" | "REPEATER" | "POLLER";
      guards: Array<{
        type: "VISIT_COUNT";
        workstation: string;
        maxVisits: number;
      }>;
      prompt?: string;
      runnerName?: string;
    };
    hasValidationErrors: boolean;
    initialValues: Partial<{
      sharedWorkerWorkstationNamesByWorkerName: Record<string, string[]>;
    }>;
    isDirty: boolean;
    pendingFactoryDefinition: FactoryDefinition | null;
    promptDiagnostics: Array<{ message: string; severity: "error" }>;
    validationErrors: Record<string, string | undefined>;
    workerOptionsState:
      | { status: "ready"; options: string[] }
      | { message: string; status: "empty" | "error" };
    overwriteFieldNames: Array<"worker" | "prompt" | "behavior" | "runner">;
    workstationType: "MODEL_WORKSTATION" | "LOGICAL_MOVE";
  }>,
) {
  return {
    draft: {
      behavior: overrides?.draft?.behavior ?? ("STANDARD" as const),
      guards: overrides?.draft?.guards ?? [],
      inputs: [],
      prompt: overrides?.draft?.prompt ?? "Review prompt",
      runnerName: overrides?.draft?.runnerName ?? "gemini",
      workerName: "reviewer",
    },
    hasValidationErrors: overrides?.hasValidationErrors ?? false,
    initialValues: {
      behavior: "STANDARD" as const,
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER"] as const,
      effectiveRunnerName: "gemini",
      factoryRunnerName: "codex",
      guards: [],
      inputs: [],
      prompt: "Review prompt",
      resolvedRunnerSelection: {
        runnerId: "gemini",
        source: "workstation" as const,
      },
      runnerName: "gemini",
      runnerOptions: ["codex", "gemini"],
      runnerSelectionSource: "workstation" as const,
      sharedWorkerWorkstationNames: [],
      sharedWorkerWorkstationNamesByWorkerName:
        overrides?.initialValues?.sharedWorkerWorkstationNamesByWorkerName ??
        {},
      workerModelProvider: null,
      workerName: "reviewer",
      workerOptions: ["reviewer", "planner"],
      workerTypeByName: {
        planner: "MODEL_WORKER" as const,
        reviewer: "MODEL_WORKER" as const,
      },
      workstationName: "Review",
      workstationOptions: ["Plan", "Review"],
      workstationType: overrides?.workstationType ?? "MODEL_WORKSTATION",
    },
    isDirty: overrides?.isDirty ?? false,
    markChangesSaved: vi.fn(),
    baseVersion: { logical: "1", physical: "2026-06-01T00:00:00Z" },
    onBehaviorChange: vi.fn(),
    onGuardsChange: vi.fn(),
    onInputsChange: vi.fn(),
    onPromptChange: vi.fn(),
    onResetToLatest: vi.fn(),
    onRunnerChange: vi.fn(),
    onWorkerChange: vi.fn(),
    overwriteFieldNames: overrides?.overwriteFieldNames ?? [],
    pendingFactoryDefinition:
      overrides?.pendingFactoryDefinition === undefined
        ? ({ workstations: [] } as unknown as FactoryDefinition)
        : overrides.pendingFactoryDefinition,
    promptDiagnostics: overrides?.promptDiagnostics ?? [],
    promptHelpState: { status: "empty" as const, message: "No prompt help." },
    promptValidationState:
      overrides?.promptDiagnostics && overrides.promptDiagnostics.length > 0
        ? {
            diagnostics: overrides.promptDiagnostics,
            result: {
              diagnostics: overrides.promptDiagnostics,
              valid: false,
            },
            status: "ready" as const,
          }
        : {
            diagnostics: [],
            result: { diagnostics: [], valid: true },
            status: "ready" as const,
          },
    status: "ready" as const,
    validationErrors: overrides?.validationErrors ?? {},
    workerOptionsState: overrides?.workerOptionsState ?? {
      options: ["reviewer", "planner"],
      status: "ready" as const,
    },
    workstationOptionsState: {
      options: ["Plan", "Review"],
      status: "ready" as const,
    },
  };
}

describe("EditableConfigurationSection async states", () => {
  it("shows loading, error, and empty copy when expanded", () => {
    const { rerender } = render(
      <EditableConfigurationSection
        messages={messages}
        state={{ status: "loading" }}
      />,
    );

    expandConfiguration();
    expect(
      screen.getByText(
        "Loading the current factory definition for this workstation.",
      ),
    ).toBeInTheDocument();

    rerender(
      <EditableConfigurationSection
        messages={messages}
        state={{ status: "error", errorMessage: "Factory unavailable." }}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Editable configuration unavailable. Factory unavailable.",
    );

    rerender(
      <EditableConfigurationSection
        messages={messages}
        state={{ status: "empty", message: "No editable values." }}
      />,
    );
    expect(screen.getByText("No editable values.")).toBeInTheDocument();
  });
});

describe("EditableConfigurationSection footer save controls", () => {
  it("renders footer save and reset controls when onSaveConfiguration is provided", async () => {
    const user = userEvent.setup();
    const onSaveConfiguration = vi.fn();
    const onResetToLatest = vi.fn();

    render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={onSaveConfiguration}
        state={{
          ...buildReadyState({ isDirty: true }),
          onResetToLatest,
        }}
      />,
    );

    expandConfiguration();

    const saveButton = screen.getByRole("button", { name: "Save changes" });
    expect(saveButton).toBeEnabled();

    await user.click(saveButton);
    expect(onSaveConfiguration).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "Reset to latest" }));
    expect(onResetToLatest).toHaveBeenCalledTimes(1);
  });

  it("disables footer save when validation errors exist or the draft is clean", () => {
    const { rerender } = render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        state={buildReadyState({
          hasValidationErrors: true,
          isDirty: true,
        })}
      />,
    );

    expandConfiguration();
    expect(screen.getByRole("button", { name: "Save changes" })).toBeDisabled();

    rerender(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        state={buildReadyState({ isDirty: false })}
      />,
    );
    expect(screen.getByRole("button", { name: "Save changes" })).toBeDisabled();
  });
});

describe("EditableConfigurationSection worker options", () => {
  it("surfaces worker option empty state inside the ready form", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildReadyState({
          workerOptionsState: {
            message: "No workers are configured in this factory.",
            status: "empty",
          },
        })}
      />,
    );

    expandConfiguration();
    expect(
      screen.getByText("No workers are configured in this factory."),
    ).toBeInTheDocument();
  });

  it("surfaces worker option error state inside the ready form", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildReadyState({
          workerOptionsState: {
            message: "Worker list failed to load.",
            status: "error",
          },
        })}
      />,
    );

    expandConfiguration();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Worker selection unavailable. Worker list failed to load.",
    );
  });

  it("renders worker select when options are ready", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildReadyState()}
      />,
    );

    expandConfiguration();
    expect(screen.getByLabelText("Worker")).toHaveValue("reviewer");
  });
});

describe("EditableConfigurationSection logical move workstations", () => {
  it("shows guard fields without worker assignment controls", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildReadyState({
          workstationType: "LOGICAL_MOVE",
        })}
      />,
    );

    expandConfiguration();

    expect(screen.queryByLabelText("Worker")).not.toBeInTheDocument();
    expect(screen.getByText("Workstation guards")).toBeInTheDocument();
    expect(screen.getByText("Input guards")).toBeInTheDocument();
  });
});

describe("EditableConfigurationSection model workstation fields", () => {
  it("renders kind, runner, and prompt fields and updates behavior", async () => {
    const user = userEvent.setup();
    const onBehaviorChange = vi.fn();

    render(
      <EditableConfigurationSection
        messages={messages}
        state={{
          ...buildReadyState(),
          onBehaviorChange,
        }}
      />,
    );

    expandConfiguration();

    expect(screen.getByLabelText("Kind")).toHaveValue("STANDARD");
    expect(screen.getByLabelText("Runner")).toBeInTheDocument();
    expect(screen.getByLabelText("Prompt")).toHaveValue("Review prompt");

    await user.selectOptions(screen.getByLabelText("Kind"), "REPEATER");
    expect(onBehaviorChange).toHaveBeenCalledWith("REPEATER");
  });

  it("shows shared worker scope hint when the draft worker is shared", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildReadyState({
          initialValues: {
            sharedWorkerWorkstationNamesByWorkerName: {
              reviewer: ["Plan"],
            },
          },
        })}
      />,
    );

    expandConfiguration();

    expect(
      screen.getByText(/Worker reviewer is also used by Plan/i),
    ).toBeInTheDocument();
  });

  it("shows server-changed hints for overwritten worker, kind, runner, and prompt fields", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildReadyState({
          overwriteFieldNames: ["worker", "behavior", "runner", "prompt"],
        })}
      />,
    );

    expandConfiguration();

    expect(
      screen.getAllByText(messages.editableConfigurationServerFieldChangedHint),
    ).toHaveLength(4);
  });
});

describe("EditableConfigurationSection model workstation save feedback", () => {
  it("shows validation alert and keeps save outcomes out of the inline form", () => {
    const { container, rerender } = render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        state={buildReadyState({
          hasValidationErrors: true,
          validationErrors: { workerName: "Select a worker." },
        })}
      />,
    );

    expandConfiguration();

    expect(screen.getByRole("alert")).toHaveTextContent(
      messages.editableConfigurationValidationStatus,
    );

    rerender(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        saveState={{
          status: "success",
          message: messages.editableConfigurationSaveSuccess,
        }}
        state={buildReadyState({ isDirty: false })}
      />,
    );

    expectNoInlineSaveOutcomesIn(container);
  });

  it("treats prompt-only validation as diagnostics instead of a form alert", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildReadyState({
          hasValidationErrors: true,
          isDirty: true,
          promptDiagnostics: [
            { message: "Unknown variable.", severity: "error" },
          ],
          validationErrors: { prompt: "Resolve prompt diagnostics." },
        })}
      />,
    );

    expandConfiguration();

    expect(
      screen.getByText(messages.editableConfigurationPromptDiagnosticsHeading),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        `${messages.editableConfigurationPromptVariableDiagnosticLabel}: Unknown variable.`,
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(messages.editableConfigurationValidationStatus),
    ).not.toBeInTheDocument();
  });

  it("disables footer save and reset while submitting", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        saveState={{ status: "submitting" }}
        state={buildReadyState({ isDirty: true })}
      />,
    );

    expandConfiguration();

    expect(screen.getByRole("button", { name: "Saving..." })).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Reset to latest" }),
    ).toBeDisabled();
  });
});

describe("EditableConfigurationSection overwrite and field errors", () => {
  it("shows overwrite warning and merges save field errors into guard fields", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        saveState={{
          fieldErrors: {
            "guards[0].maxVisits":
              "Max visits must be a positive whole number.",
          },
          status: "error",
          message: "Saving failed.",
        }}
        state={buildReadyState({
          draft: {
            guards: [
              {
                type: "VISIT_COUNT",
                workstation: "Plan",
                maxVisits: 0,
              },
            ],
          },
          isDirty: true,
          overwriteFieldNames: ["worker", "prompt"],
        })}
      />,
    );

    expandConfiguration();

    expect(
      screen.getByText(/Saving now will overwrite newer server values/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Max visits must be a positive whole number."),
    ).toBeInTheDocument();
  });
});
