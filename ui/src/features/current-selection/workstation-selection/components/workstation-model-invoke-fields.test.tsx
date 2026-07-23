// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: grouped model-invoke field coverage stays in one harness.
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import { selectLabeledComboboxOption } from "../../../../testing/select-test-helpers";
import { EditableConfigurationSection } from "./editable/workstation-editable-configuration-section";
import {
  buildEditableConfigurationSectionReadyState,
  editableConfigurationSectionMessages,
  expandEditableConfigurationSection,
} from "./editable/workstation-editable-configuration-section.test-helpers";
import { EditableConfigurationModelInvokeFields } from "./workstation-model-invoke-fields";

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

describe("EditableConfigurationModelInvokeFields", () => {
  it("renders model invoke workstation fields and hides prompt-oriented controls", () => {
    render(
      <EditableConfigurationSection
        messages={editableConfigurationSectionMessages}
        state={buildEditableConfigurationSectionReadyState({
          workstationType: "MODEL_INVOKE",
          draft: {
            operation: "TTS",
            workerName: "tts-worker",
          },
          workerOptionsState: {
            options: ["tts-worker"],
            status: "ready",
          },
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(
      screen.getByRole("combobox", { name: "Workstation type" }),
    ).toHaveTextContent("Model invoke (legacy)");
    expect(screen.getByLabelText("Worker")).toBeInTheDocument();
    expect(screen.getByLabelText("Operation")).toBeInTheDocument();
    expect(screen.getByText("Operation bindings")).toBeInTheDocument();
    expect(screen.getByText("text (required)")).toBeInTheDocument();
    expect(screen.getByLabelText("Config content")).toBeInTheDocument();
    expect(screen.getByLabelText("Default content")).toBeInTheDocument();
    expect(screen.queryByLabelText("Prompt")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Kind")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Runner")).not.toBeInTheDocument();
  });

  it("surfaces worker option empty and error states", () => {
    const { rerender } = render(
      <EditableConfigurationModelInvokeFields
        messages={editableConfigurationSectionMessages}
        state={buildEditableConfigurationSectionReadyState({
          workstationType: "MODEL_INVOKE",
          workerOptionsState: {
            message: "Add a model worker to the factory first.",
            status: "empty",
          },
        })}
        validationErrors={{}}
      />,
    );

    expect(
      screen.getByText("Add a model worker to the factory first."),
    ).toBeInTheDocument();

    rerender(
      <EditableConfigurationModelInvokeFields
        messages={editableConfigurationSectionMessages}
        state={buildEditableConfigurationSectionReadyState({
          workstationType: "MODEL_INVOKE",
          workerOptionsState: {
            message: "Worker catalog unavailable.",
            status: "error",
          },
        })}
        validationErrors={{}}
      />,
    );

    expect(screen.getByText(/Worker catalog unavailable/i)).toBeInTheDocument();
  });

  it("updates worker, operation, and binding fields", async () => {
    const user = userEvent.setup();
    const onWorkerChange = vi.fn();
    const onOperationChange = vi.fn();
    const onOperationBindingsChange = vi.fn();

    render(
      <EditableConfigurationModelInvokeFields
        messages={editableConfigurationSectionMessages}
        state={{
          ...buildEditableConfigurationSectionReadyState({
            workstationType: "MODEL_INVOKE",
            draft: {
              operation: "TTS",
              operationBindings: [],
              workerName: "tts-worker",
            },
            operationOptionsState: {
              operations: [
                {
                  name: "TTS",
                  inputs: [
                    { name: "text", contentTypes: ["TEXT"], required: true },
                  ],
                  outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
                },
              ],
              options: ["TTS"],
              status: "ready",
            },
            workerOptionsState: {
              options: ["tts-worker", "reviewer"],
              status: "ready",
            },
          }),
          onWorkerChange,
          onOperationChange,
          onOperationBindingsChange,
        }}
        validationErrors={{
          "operationBindings[text]": "Binding text is required.",
        }}
      />,
    );

    fireEvent.change(screen.getByLabelText("Selector label"), {
      target: { value: "utterance" },
    });
    expect(onOperationBindingsChange).toHaveBeenCalledWith([
      expect.objectContaining({
        slot: "text",
        selector: expect.objectContaining({ label: "utterance" }),
      }),
    ]);

    fireEvent.change(screen.getByLabelText("Selector slot"), {
      target: { value: "input.text" },
    });
    fireEvent.change(screen.getByLabelText("Selector role"), {
      target: { value: "runtime" },
    });
    await selectLabeledComboboxOption(user, "Selector type", "TEXT");

    fireEvent.change(screen.getByLabelText("Config content"), {
      target: { value: "speak clearly" },
    });
    expect(onOperationBindingsChange).toHaveBeenCalledWith([
      expect.objectContaining({
        slot: "text",
        configText: "speak clearly",
      }),
    ]);

    fireEvent.change(screen.getByLabelText("Default content"), {
      target: { value: "fallback text" },
    });
    expect(onOperationBindingsChange).toHaveBeenLastCalledWith([
      expect.objectContaining({
        slot: "text",
        defaultContentText: "fallback text",
      }),
    ]);

    expect(screen.getByText("Binding text is required.")).toBeInTheDocument();

    await selectLabeledComboboxOption(user, "Worker", "reviewer");
    expect(onWorkerChange).toHaveBeenCalledWith("reviewer");
  });

  it("shows empty bindings guidance when the selected operation has no input slots", () => {
    const readyState = buildEditableConfigurationSectionReadyState({
      workstationType: "MODEL_INVOKE",
      draft: {
        operation: "NO_INPUTS",
        operationBindings: [],
        workerName: "tts-worker",
      },
      operationOptionsState: {
        operations: [
          {
            name: "NO_INPUTS",
            inputs: [],
            outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
          },
        ],
        options: ["NO_INPUTS"],
        status: "ready",
      },
    });

    render(
      <EditableConfigurationModelInvokeFields
        messages={editableConfigurationSectionMessages}
        state={{
          ...readyState,
          initialValues: {
            ...readyState.initialValues,
            modelOperationsByWorkerName: {
              "tts-worker": [
                {
                  name: "NO_INPUTS",
                  inputs: [],
                  outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
                },
              ],
            },
          },
        }}
        validationErrors={{}}
      />,
    );

    expect(
      screen.getByText(
        "Select a worker operation to edit input slot bindings.",
      ),
    ).toBeInTheDocument();
  });

  it("surfaces operation validation and field-level errors", () => {
    render(
      <EditableConfigurationModelInvokeFields
        messages={editableConfigurationSectionMessages}
        state={buildEditableConfigurationSectionReadyState({
          workstationType: "MODEL_INVOKE",
          draft: {
            operation: "TTS",
            workerName: "tts-worker",
          },
          operationOptionsState: {
            operations: [
              {
                name: "TTS",
                inputs: [
                  { name: "text", contentTypes: ["TEXT"], required: true },
                ],
                outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
              },
            ],
            options: ["TTS"],
            status: "ready",
          },
          workerOptionsState: {
            options: ["tts-worker"],
            status: "ready",
          },
        })}
        validationErrors={{
          operation: "Operation is required.",
          workerName: "Worker is required.",
        }}
      />,
    );

    expect(screen.getByText("Operation is required.")).toBeInTheDocument();
    expect(screen.getByText("Worker is required.")).toBeInTheDocument();
  });

  it("surfaces operation option empty and error states", () => {
    const { rerender } = render(
      <EditableConfigurationModelInvokeFields
        messages={editableConfigurationSectionMessages}
        state={buildEditableConfigurationSectionReadyState({
          workstationType: "MODEL_INVOKE",
          draft: {
            operation: "",
            workerName: "",
          },
          operationOptionsState: {
            message: "Select a model worker before choosing an operation.",
            status: "empty",
          },
        })}
        validationErrors={{}}
      />,
    );

    expect(
      screen.getByText("Select a model worker before choosing an operation."),
    ).toBeInTheDocument();

    rerender(
      <EditableConfigurationModelInvokeFields
        messages={editableConfigurationSectionMessages}
        state={buildEditableConfigurationSectionReadyState({
          workstationType: "MODEL_INVOKE",
          draft: {
            operation: "MISSING",
            workerName: "tts-worker",
          },
          operationOptionsState: {
            message: "Selected operation is no longer declared on the worker.",
            status: "error",
          },
        })}
        validationErrors={{}}
      />,
    );

    expect(
      screen.getByText(
        "Selected operation is no longer declared on the worker.",
      ),
    ).toBeInTheDocument();
  });
});
