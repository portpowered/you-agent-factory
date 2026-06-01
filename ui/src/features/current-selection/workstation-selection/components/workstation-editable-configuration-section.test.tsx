import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { FactoryDefinition } from "../../../../api/factory-definition/api";
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
      guards: Array<{
        type: "VISIT_COUNT";
        workstation: string;
        maxVisits: number;
      }>;
    };
    isDirty: boolean;
    hasValidationErrors: boolean;
    pendingFactoryDefinition: FactoryDefinition | null;
    workerOptionsState:
      | { status: "ready"; options: string[] }
      | { message: string; status: "empty" | "error" };
    overwriteFieldNames: Array<"worker" | "prompt" | "behavior" | "runner">;
    workstationType: "MODEL_WORKSTATION" | "LOGICAL_MOVE";
  }>,
) {
  return {
    draft: {
      behavior: "STANDARD" as const,
      guards: overrides?.draft?.guards ?? [],
      inputs: [],
      prompt: "Review prompt",
      runnerName: "gemini",
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
      sharedWorkerWorkstationNamesByWorkerName: {},
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
    promptDiagnostics: [],
    promptHelpState: { status: "empty" as const, message: "No prompt help." },
    promptValidationState: {
      diagnostics: [],
      result: { diagnostics: [], valid: true },
      status: "ready" as const,
    },
    status: "ready" as const,
    validationErrors: {},
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

describe("EditableConfigurationSection", () => {
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
      screen.getByText(
        "Max visits must be a positive whole number.",
      ),
    ).toBeInTheDocument();
  });
});
