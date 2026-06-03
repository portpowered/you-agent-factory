import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../../api/factory-definition/api";
import type { EditableWorkerConfigurationState } from "../lib/detail-card-types";
import { getWorkerDetailMessages } from "../messages/worker-detail";
import { WorkerEditableConfigurationSection } from "./worker-editable-configuration-section";

const messages = getWorkerDetailMessages();

function buildReadyWorkerEditableConfigurationState(
  workstationNames: string[],
): Extract<EditableWorkerConfigurationState, { status: "ready" }> {
  return {
    canSave: true,
    draft: {
      argsText: "",
      body: "",
      command: "",
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CURSOR",
      name: "reviewer",
      provider: null,
      type: "MODEL_WORKER",
    },
    hasValidationErrors: false,
    initialValues: {
      args: [],
      body: null,
      command: null,
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CURSOR",
      name: "reviewer",
      provider: null,
      type: "MODEL_WORKER",
      workerName: "reviewer",
      workstationNames,
    },
    isDirty: true,
    onArgsTextChange: vi.fn(),
    onBodyChange: vi.fn(),
    onCommandChange: vi.fn(),
    onExecutorProviderChange: vi.fn(),
    onModelChange: vi.fn(),
    onModelLocalityChange: vi.fn(),
    onModelProviderChange: vi.fn(),
    onNameChange: vi.fn(),
    onProviderChange: vi.fn(),
    markChangesSaved: vi.fn(),
    onResetToLatest: vi.fn(),
    onTypeChange: vi.fn(),
    overwriteFieldNames: [],
    pendingFactoryDefinition: {} as CanonicalFactoryDefinition,
    status: "ready",
    validationErrors: {},
  };
}

describe("WorkerEditableConfigurationSection shared-impact warnings", () => {
  it("shows worker save-impact warning when multiple workstations reference the worker", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildReadyWorkerEditableConfigurationState(["Review", "Plan"])}
        workerName="reviewer"
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain(
      "Saving reviewer updates workstations",
    );
    expect(screen.getByRole("alert").textContent).toMatch(/Review.*Plan/);
    expect(
      screen.queryByText(
        messages.editableConfigurationSharedImpactWarningDetail,
      ),
    ).toBeNull();
  });

  it("does not show worker save-impact warning for a single-workstation worker", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildReadyWorkerEditableConfigurationState(["Review"])}
        workerName="reviewer"
      />,
    );

    expect(
      screen.queryByText(/updates workstations/i),
    ).toBeNull();
  });
});
