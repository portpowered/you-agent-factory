import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { EditableConfigurationModelInvokeFields } from "./workstation-model-invoke-fields";
import {
  buildEditableConfigurationSectionReadyState,
  editableConfigurationSectionMessages,
  expandEditableConfigurationSection,
} from "./workstation-editable-configuration-section.test-helpers";
import { EditableConfigurationSection } from "./workstation-editable-configuration-section";

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

    expect(screen.getByText("Model invoke")).toBeInTheDocument();
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

  it("surfaces operation option empty state", () => {
    render(
      <EditableConfigurationModelInvokeFields
        messages={editableConfigurationSectionMessages}
        state={buildEditableConfigurationSectionReadyState({
          workstationType: "MODEL_INVOKE",
          draft: {
            operation: "",
            workerName: "",
          },
          operationOptionsState: {
            message:
              "Select a model worker before choosing an operation.",
            status: "empty",
          },
        })}
        validationErrors={{}}
      />,
    );

    expect(
      screen.getByText(
        "Select a model worker before choosing an operation.",
      ),
    ).toBeInTheDocument();
  });
});
